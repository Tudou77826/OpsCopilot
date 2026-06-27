package sshclient

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// slowReader 按 chunks 间歇性返回数据，模拟 PTY stdout/stderr 的交错到达。
// 数据发完后保持流打开（返回 0,nil），模拟交互式 PTY 的真实行为。
// Close 后立即返回 EOF（用于测试精确控制结束时机）。
type slowReader struct {
	mu     sync.Mutex
	data   []byte
	chunk  int
	pause  time.Duration
	pos    int
	closed chan struct{}
}

func newSlowReader(data string, chunk int, pause time.Duration) *slowReader {
	return &slowReader{data: []byte(data), chunk: chunk, pause: pause, closed: make(chan struct{})}
}

func (s *slowReader) Read(p []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.EOF
	default:
	}
	if s.pos >= len(s.data) {
		// 数据发完，模拟 PTY：保持流打开，暂无数据。
		// 用 select 监听关闭信号，使 Close 能及时打断等待。
		select {
		case <-s.closed:
			return 0, io.EOF
		case <-time.After(s.pause):
			return 0, nil
		}
	}
	select {
	case <-s.closed:
		return 0, io.EOF
	case <-time.After(s.pause):
	}
	end := s.pos + s.chunk
	if end > len(s.data) {
		end = len(s.data)
	}
	n := copy(p, s.data[s.pos:end])
	s.pos += n
	return n, nil
}

func (s *slowReader) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
}

// TestMuxReader_PreservesOrderWithinStream 验证：两路交错的数据都能完整汇合，
// 不会丢失任何一路的内容（对比 io.MultiReader 会丢 stderr）。
// muxReader 在流保持打开时会阻塞读，故用超时 goroutine 收集已读数据。
func TestMuxReader_PreservesOrderWithinStream(t *testing.T) {
	stdoutData := strings.Repeat("A", 1000)
	stderrData := strings.Repeat("B", 1000)

	r1 := newSlowReader(stdoutData, 50, time.Millisecond)
	r2 := newSlowReader(stderrData, 50, time.Millisecond)

	mr := newMuxReader(r1, r2)

	// 在独立 goroutine 里持续读，超时后停止
	var out bytes.Buffer
	var outMu sync.Mutex
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := mr.Read(buf)
			if err != nil {
				return
			}
			outMu.Lock()
			out.Write(buf[:n])
			outMu.Unlock()
		}
	}()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		// 主动停止：关闭两路 reader，让 muxReader 自然结束
		r1.Close()
		r2.Close()
	}

	outMu.Lock()
	got := out.String()
	outMu.Unlock()

	aCount := strings.Count(got, "A")
	bCount := strings.Count(got, "B")
	if aCount != 1000 {
		t.Errorf("stdout (A) bytes lost: got %d, want 1000", aCount)
	}
	if bCount != 1000 {
		t.Errorf("stderr (B) bytes lost: got %d, want 1000", bCount)
	}
}

// TestMuxReader_NoFalseEOF 验证核心修复点：
// 任一 reader 单独 EOF，不会让 muxReader 返回 EOF，也不会丢失另一路的数据。
// r1 立即 EOF；r2 含正常数据。读 muxReader 必须完整读出 r2 的数据，最后才 EOF。
func TestMuxReader_NoFalseEOF(t *testing.T) {
	r1 := strings.NewReader("") // 立即 EOF
	r2 := strings.NewReader("remaining-data-from-stderr")

	mr := newMuxReader(r1, r2)
	var out bytes.Buffer
	buf := make([]byte, 64)

	// 持续读直到 EOF，期间不应在 r2 数据读完前提前结束
	for {
		n, err := mr.Read(buf)
		out.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if out.String() != "remaining-data-from-stderr" {
		t.Errorf("r2 data lost after r1 EOF: got %q, want %q", out.String(), "remaining-data-from-stderr")
	}
}

// TestMuxReader_EmptyReturnsNoDeadlock 验证单 reader 场景：
// 数据正常读出，结束后 EOF，无死锁。
func TestMuxReader_EmptyReturnsNoDeadlock(t *testing.T) {
	mr := newMuxReader(strings.NewReader("hello"))
	var out bytes.Buffer
	buf := make([]byte, 32)
	for {
		n, err := mr.Read(buf)
		out.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if out.String() != "hello" {
		t.Errorf("got %q, want %q", out.String(), "hello")
	}
}

