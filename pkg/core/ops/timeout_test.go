package ops

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestIsTimeoutErr_DeadlineExceeded context 超时被识别
func TestIsTimeoutErr_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	err := ctx.Err()
	if !isTimeoutErr(err) {
		t.Errorf("DeadlineExceeded should be a timeout error: %v", err)
	}
}

// TestIsTimeoutErr_Canceled context 取消被识别
func TestIsTimeoutErr_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ctx.Err()
	if !isTimeoutErr(err) {
		t.Errorf("Canceled should be a timeout error: %v", err)
	}
}

// TestIsTimeoutErr_WrappedError 包装的超时错误也被识别（errors.Is 链）
func TestIsTimeoutErr_WrappedError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	// 模拟 RunWithContext 返回的包装错误
	wrapped := errors.Join(ctx.Err(), errors.New("command timed out"))
	if !isTimeoutErr(wrapped) {
		t.Errorf("wrapped timeout error should be detected via errors.Is: %v", wrapped)
	}
}

// TestIsTimeoutErr_PlainError 普通错误不被误判为超时
func TestIsTimeoutErr_PlainError(t *testing.T) {
	plainErr := errors.New("failed to run command: exit status 1")
	if isTimeoutErr(plainErr) {
		t.Errorf("plain error should not be a timeout error: %v", plainErr)
	}
}

// TestIsTimeoutErr_Nil nil 不被判定为超时
func TestIsTimeoutErr_Nil(t *testing.T) {
	if isTimeoutErr(nil) {
		t.Errorf("nil should not be a timeout error")
	}
}

// TestIsConnectionDead_NilConnection nil 连接视为已死
func TestIsConnectionDead_NilConnection(t *testing.T) {
	if !isConnectionDead(nil) {
		t.Errorf("nil connection should be considered dead")
	}
}

// TestIsConnectionDead_EmptyClient Client 为 nil 的连接视为已死
func TestIsConnectionDead_EmptyClient(t *testing.T) {
	conn := &Connection{Name: "test"}
	if !isConnectionDead(conn) {
		t.Errorf("connection with nil Client should be considered dead")
	}
}
