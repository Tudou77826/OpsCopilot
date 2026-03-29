package ftipc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Server is the HTTP IPC server that runs in the main OpsCopilot process.
type Server struct {
	server   *http.Server
	port     int
	token    string
	tokenDir string

	mu       sync.RWMutex
	handlers map[string]ActionHandler
}

// ActionHandler processes an IPC action and returns a response.
type ActionHandler func(req IPCRequest) IPCResponse

// NewServer creates a new IPC server bound to localhost on a random port.
func NewServer() *Server {
	token := generateToken()
	return &Server{
		token:    token,
		handlers: make(map[string]ActionHandler),
	}
}

// RegisterHandler registers an action handler.
func (s *Server) RegisterHandler(action string, handler ActionHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[action] = handler
}

// Start starts the IPC HTTP server on a random port.
// The server binds to 127.0.0.1 only.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("ipc listen failed: %w", err)
	}

	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/ft/", s.handleFileTransfer)

	s.server = &http.Server{
		Handler: mux,
		// ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("IPC server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the IPC server.
func (s *Server) Stop() error {
	if s.server != nil {
		err := s.server.Shutdown(context.Background())
		s.cleanupTokenFile()
		return err
	}
	return nil
}

// Info returns the IPC connection info for clients.
func (s *Server) Info() IPCInfo {
	return IPCInfo{
		Port:  s.port,
		Token: s.token,
		PID:   os.Getpid(),
	}
}

// WriteTokenFile writes the IPC info to a file for the FTP manager to read.
func (s *Server) WriteTokenFile(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	s.tokenDir = dir

	path := filepath.Join(dir, "ipc.json")
	data, err := json.Marshal(s.Info())
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (s *Server) cleanupTokenFile() {
	if s.tokenDir != "" {
		path := filepath.Join(s.tokenDir, "ipc.json")
		_ = os.Remove(path)
	}
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Delegate to registered handler
	s.mu.RLock()
	handler, ok := s.handlers["sessions"]
	s.mu.RUnlock()

	if !ok {
		writeJSON(w, IPCResponse{OK: false, Error: &IPCError{Code: "NOT_IMPLEMENTED", Message: "sessions handler not registered"}})
		return
	}

	resp := handler(IPCRequest{Action: "sessions"})
	writeJSON(w, resp)
}

func (s *Server) handleFileTransfer(w http.ResponseWriter, r *http.Request) {
	if !s.authenticate(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, IPCResponse{OK: false, Error: &IPCError{Code: "READ_ERROR", Message: err.Error()}})
		return
	}
	defer r.Body.Close()

	var req IPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, IPCResponse{OK: false, Error: &IPCError{Code: "PARSE_ERROR", Message: err.Error()}})
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[req.Action]
	s.mu.RUnlock()

	if !ok {
		writeJSON(w, IPCResponse{OK: false, Error: &IPCError{Code: "UNKNOWN_ACTION", Message: "unknown action: " + req.Action}})
		return
	}

	resp := handler(req)
	writeJSON(w, resp)
}

func (s *Server) authenticate(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return false
	}
	// Expect "Bearer <token>"
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:] == s.token
	}
	return false
}

func writeJSON(w http.ResponseWriter, v IPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "marshal error", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
