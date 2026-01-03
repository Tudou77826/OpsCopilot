package sshclient

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockReader 模拟 Stdout
type MockReader struct {
	data []byte
	pos  int
	mu   sync.Mutex
}

func (r *MockReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// MockWriter 模拟 Stdin
type MockWriter struct {
	Buffer bytes.Buffer
	mu     sync.Mutex
}

func (w *MockWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Buffer.Write(p)
}

func TestAutoSudo(t *testing.T) {
	rootPwd := "secretroot"
	
	tests := []struct {
		name     string
		input    string
		expected string // Stdin 应该收到的内容
	}{
		{
			name:     "English Prompt",
			input:    "Password: ",
			expected: rootPwd + "\n",
		},
		{
			name:     "Chinese Prompt",
			input:    "密码：",
			expected: rootPwd + "\n",
		},
		{
			name:     "Sudo Prompt",
			input:    "[sudo] password for user:",
			expected: rootPwd + "\n",
		},
		{
			name:     "No Prompt",
			input:    "Welcome to Linux",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := &MockWriter{}
			
			// 模拟 SudoHandler 逻辑
			// 在实际代码中，我们需要将这个逻辑封装在 client.StartShell 返回的 Reader 中，或者独立出来
			
			// 这里我们测试核心匹配逻辑
			handler := &SudoHandler{
				RootPassword: rootPwd,
				Stdin:        stdin,
			}
			
			// 模拟收到数据
			handler.Handle([]byte(tt.input))
			
			// 检查 Stdin 是否被写入
			if tt.expected != "" {
				// 给一点时间让 goroutine 执行（如果是异步的）
				time.Sleep(10 * time.Millisecond)
				
				stdin.mu.Lock()
				got := stdin.Buffer.String()
				stdin.mu.Unlock()
				
				if !strings.Contains(got, tt.expected) {
					t.Errorf("Expected stdin to contain %q, got %q", tt.expected, got)
				}
			} else {
				stdin.mu.Lock()
				got := stdin.Buffer.String()
				stdin.mu.Unlock()
				if got != "" {
					t.Errorf("Expected empty stdin, got %q", got)
				}
			}
		})
	}
}
