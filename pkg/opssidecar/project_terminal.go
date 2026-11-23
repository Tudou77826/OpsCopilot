package opssidecar

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/core/ops"
)

const askpassTicketTTL = 60 * time.Second

type askpassEntry struct {
	secret    string
	expiresAt time.Time
}

type projectAskpassManager struct {
	mu      sync.Mutex
	tickets map[string]askpassEntry
}

func newProjectAskpassManager() *projectAskpassManager {
	return &projectAskpassManager{tickets: map[string]askpassEntry{}}
}

func (m *projectAskpassManager) issue(secret string) string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		b = []byte(newRequestID())
	}
	token := "ask-" + hex.EncodeToString(b)
	now := time.Now()
	m.mu.Lock()
	// 顺带清理过期票据
	for k, v := range m.tickets {
		if now.After(v.expiresAt) {
			delete(m.tickets, k)
		}
	}
	m.tickets[token] = askpassEntry{
		secret:    secret,
		expiresAt: now.Add(askpassTicketTTL),
	}
	m.mu.Unlock()
	return token
}

func (m *projectAskpassManager) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	m.mu.Lock()
	entry, ok := m.tickets[token]
	if ok {
		// 一次性票据：取走即焚，防重放
		delete(m.tickets, token)
	}
	m.mu.Unlock()
	if !ok || time.Now().After(entry.expiresAt) {
		http.Error(w, "凭据票据无效或已过期", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, entry.secret)
}

// RunAskpass 作为 OpenSSH SSH_ASKPASS 子进程运行：
// 向本地回环 /project-askpass 兑换一次性凭据并写到 stdout，然后退出。
func RunAskpass(out io.Writer) int {
	url := strings.TrimSpace(os.Getenv("AAW_ASKPASS_URL"))
	token := strings.TrimSpace(os.Getenv("AAW_ASKPASS_TOKEN"))
	if url == "" || token == "" {
		return 1
	}
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return 1
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return 1
	}
	_, _ = out.Write(body)
	return 0
}

func formatKnownHostsLine(host string, port int, authorizedKey string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	key := strings.TrimSpace(authorizedKey)
	if host == "" || key == "" {
		return ""
	}
	if port > 0 && port != 22 {
		return fmt.Sprintf("[%s]:%d %s\n", host, port, key)
	}
	return fmt.Sprintf("%s %s\n", host, key)
}

func (s *Service) handleProjectTerminalPrepare(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		RemoteRoot   string `json:"remoteRoot"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil ||
		strings.TrimSpace(params.ConnectionID) == "" ||
		strings.TrimSpace(params.RemoteRoot) == "" {
		return invalidParams(req.ID, "connectionId 与 remoteRoot 必填"), false
	}

	target, bastion, err := s.resolveProjectDialConfig(strings.TrimSpace(params.ConnectionID), nil)
	if err != nil {
		if errors.Is(err, errServerNotFound) {
			return errorResponse(req.ID, CodeNotFound, err.Error(), ""), false
		}
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	nullKnownHosts := "/dev/null"
	if runtime.GOOS == "windows" {
		nullKnownHosts = "NUL"
	}

	args := []string{
		"-t",
		"-p", fmt.Sprint(target.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=" + nullKnownHosts,
		"-o", "ServerAliveInterval=30",
	}
	if target.AuthType == "key" && strings.TrimSpace(target.PrivateKeyPath) != "" {
		args = append(args, "-i", strings.TrimSpace(target.PrivateKeyPath))
	}
	if bastion != nil && bastion.Host != "" {
		jump := fmt.Sprintf("%s@%s:%d", bastion.User, bastion.Host, bastion.Port)
		args = append(args, "-J", jump)
	}

	remoteRoot := cleanRemotePath(params.RemoteRoot)
	remoteCmd := fmt.Sprintf("cd %s && exec ${SHELL:-/bin/sh} -l", ops.QuoteShell(remoteRoot))
	args = append(args, fmt.Sprintf("%s@%s", target.User, target.Host), remoteCmd)

	env := map[string]string{}
	requiresAskpass := false
	secret := target.Password
	if secret != "" && (target.AuthType == "password" || target.AuthType == "key") {
		addr, _ := s.bridgeInfo()
		exe, exeErr := os.Executable()
		if addr != "" && exeErr == nil {
			ticket := s.projectAskpass.issue(secret)
			requiresAskpass = true
			env["SSH_ASKPASS"] = exe
			env["SSH_ASKPASS_REQUIRE"] = "force"
			env["DISPLAY"] = "aaw:0"
			env["AAW_ASKPASS_URL"] = "http://" + addr + "/project-askpass"
			env["AAW_ASKPASS_TOKEN"] = ticket
		}
	}

	return okResponse(req.ID, map[string]any{
		"sshArgs":         args,
		"env":             env,
		"knownHostsPath":  "",
		"authType":        target.AuthType,
		"requiresAskpass": requiresAskpass,
		"summary":         fmt.Sprintf("%s@%s:%d", target.User, target.Host, target.Port),
	}), false
}
