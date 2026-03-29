package ftipc

// IPCInfo contains the connection information for the IPC server.
type IPCInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// IPCRequest represents a request from the FTP manager to the main application.
type IPCRequest struct {
	Action    string `json:"action"`
	SessionID string `json:"sessionId"`
	Path      string `json:"path,omitempty"`
	DstPath   string `json:"dstPath,omitempty"`
	LocalPath string `json:"localPath,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
}

// IPCResponse represents a response from the main application to the FTP manager.
type IPCResponse struct {
	OK      bool        `json:"ok"`
	Error   *IPCError   `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// IPCError represents an error in IPC communication.
type IPCError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *IPCError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
