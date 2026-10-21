package terminalcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ExecutionAudit records evidence, not terminal recording. Raw commands, output,
// directory names, credentials and arbitrary error strings never enter it.
type ExecutionAudit struct {
	Time          time.Time  `json:"time"`
	Actor         string     `json:"actor"`
	Target        Target     `json:"target"`
	Node          string     `json:"node,omitempty"`
	SSHUser       string     `json:"sshUser,omitempty"`
	RunID         string     `json:"runId"`
	OperationID   string     `json:"operationId"`
	ApprovalID    string     `json:"approvalId,omitempty"`
	CommandHash   string     `json:"commandHash"`
	CommandBytes  int        `json:"commandBytes"`
	UID           int        `json:"uid"`
	PID           int        `json:"pid"`
	Sequence      uint64     `json:"sequence"`
	DirectoryHash string     `json:"directoryHash"`
	Phase         string     `json:"phase"`
	Reason        string     `json:"reason,omitempty"`
	Submission    Submission `json:"submission,omitempty"`
	Completion    Completion `json:"completion,omitempty"`
	ExitCode      *int       `json:"exitCode,omitempty"`
}

type ExecutionJournal interface {
	Append(context.Context, ExecutionAudit) error
}

// FileJournal syncs each append before returning. At the size limit it refuses
// new execution; it never silently drops or overwrites audit history.
type FileJournal struct {
	mu     sync.Mutex
	path   string
	limit  int64
	broken bool
}

func NewFileJournal(path string, limit int64) (*FileJournal, error) {
	if !filepath.IsAbs(path) || limit < 1024 || limit > 1<<30 {
		return nil, errors.New("审计文件需要绝对路径，上限为 1 KiB–1 GiB")
	}
	return &FileJournal{path: path, limit: limit}, nil
}

func (j *FileJournal) Append(ctx context.Context, event ExecutionAudit) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if j.broken {
		return errors.New("审计文件发生过不完整写入，需要检查后重新打开")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("编码执行审计失败: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(j.path), 0700); err != nil {
		return fmt.Errorf("创建审计目录失败: %w", err)
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("打开执行审计失败: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("读取执行审计大小失败: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size()+int64(len(data)) > j.limit {
		return errors.New("执行审计文件不可写或已达上限")
	}
	if info.Size() > 0 {
		var tail [1]byte
		if _, err := f.ReadAt(tail[:], info.Size()-1); err != nil || tail[0] != '\n' {
			j.broken = true
			return errors.New("执行审计文件末尾不完整，需要检查后重新打开")
		}
	}
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		j.broken = true
		return fmt.Errorf("持久化执行审计失败: %w", err)
	}
	return nil
}
