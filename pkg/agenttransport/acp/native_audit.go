package acp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Audit stores identifiers and digests only: arguments and outputs can contain
// credentials. The original operation remains visible in the approval UI.
func nativeAudit(directory, turn string) func(string, string, json.RawMessage) error {
	var mu sync.Mutex
	seen := map[string]bool{}
	return func(event, status string, raw json.RawMessage) error {
		if directory == "" {
			return nil
		} // Standalone transport tests may omit persistence.
		mu.Lock()
		defer mu.Unlock()
		var identity struct {
			Update struct {
				ID string `json:"toolCallId"`
			} `json:"update"`
			ToolCall struct {
				ID string `json:"toolCallId"`
			} `json:"toolCall"`
		}
		_ = json.Unmarshal(raw, &identity)
		id := identity.Update.ID
		if id == "" {
			id = identity.ToolCall.ID
		}
		key := event + ":" + id + ":" + status
		if event == "tool" && id != "" && seen[key] {
			return nil
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			return errors.New("无法创建本机操作审计目录")
		}
		f, err := os.OpenFile(filepath.Join(directory, "native.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return errors.New("无法打开本机操作审计")
		}
		defer f.Close()
		if info, e := f.Stat(); e != nil || info.Size() > 16<<20 {
			return errors.New("本机操作审计已满或不可读取")
		}
		hash := sha256.Sum256(raw)
		call := ""
		if id != "" {
			digest := sha256.Sum256([]byte(id))
			call = hex.EncodeToString(digest[:])
		}
		if err = json.NewEncoder(f).Encode(map[string]string{"time": time.Now().UTC().Format(time.RFC3339Nano), "turn": turn, "event": event, "status": status, "digest": hex.EncodeToString(hash[:]), "call": call}); err != nil {
			return errors.New("保存本机操作审计失败")
		}
		if err = f.Sync(); err != nil {
			return err
		}
		seen[key] = true
		return nil
	}
}
