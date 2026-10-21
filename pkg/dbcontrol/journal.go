package dbcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"opscopilot/pkg/filetxn"
)

// FileJournal contains identities and SQL hashes, never SQL literals/credentials.
type FileJournal struct {
	Path string
	mu   sync.Mutex
}

func (j *FileJournal) Append(ctx context.Context, event Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(j.Path), 0700); err != nil {
		return err
	}
	unlock, err := filetxn.Lock(j.Path)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := os.OpenFile(j.Path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 16<<20 {
		return errors.New("数据库执行审计已达到上限")
	}
	if info.Size() > 0 {
		last := make([]byte, 1)
		if _, err = f.ReadAt(last, info.Size()-1); err != nil || last[0] != '\n' {
			return errors.New("数据库执行审计末尾不完整")
		}
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
