package shellsidecar

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"opscopilot/pkg/achievements"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/filetxn"
	"os"
	"strings"
	"sync"
	"time"
)

func (s *QuickCmdService) Dispatch(svc *TerminalService, commandID string, terminals []string, execute bool) (achievements.DispatchResult, error) {
	commands, err := s.ListChecked()
	if err != nil {
		return achievements.DispatchResult{}, err
	}
	var content string
	for _, cmd := range commands {
		if cmd.ID == commandID {
			content = cmd.Content
			break
		}
	}
	content = strings.TrimRight(content, "\r\n")
	if strings.TrimSpace(content) == "" {
		return achievements.DispatchResult{}, fmt.Errorf("快捷指令不存在或内容为空")
	}
	if execute {
		content += "\r"
	}
	return achievements.Dispatch(s.achievements, commandID, terminals, func(id string) error { return svc.WriteInput(id, []byte(content)) })
}

func timeNowUnixMilli() int64 { return time.Now().UnixMilli() }

// QuickCommand 与终端应用的 quick_commands.json 完全同格式
// （pkg/config.QuickCommand：id/name/content/group），旧配置文件可原样拷入。
type QuickCommand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Group   string `json:"group,omitempty"`
}

// jsonListService 是 {id,name,content,group} 数组文件的通用存取。
// 快捷命令与脚本共用；文件名区分。
type jsonListService struct {
	mu    sync.Mutex
	path  string
	items []QuickCommand
}

func newJSONListService(path string) (*jsonListService, error) {
	s := &jsonListService{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return s, nil
}

func (s *jsonListService) list() []QuickCommand {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]QuickCommand, len(s.items))
	copy(out, s.items)
	return out
}

func (s *jsonListService) upsert(item QuickCommand) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.begin()
	if err != nil {
		return "", err
	}
	defer release()
	if item.Name == "" || item.Content == "" {
		return "", fmt.Errorf("name 和 content 不能为空")
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
		// 同毫秒连建时追加序号
		for _, existing := range s.items {
			if existing.ID == item.ID {
				item.ID = item.ID + "-a"
				break
			}
		}
	}
	replaced := false
	for i := range s.items {
		if s.items[i].ID == item.ID {
			s.items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		s.items = append(s.items, item)
	}
	if err := s.persist(); err != nil {
		return "", err
	}
	return item.ID, nil
}

func (s *jsonListService) remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.begin()
	if err != nil {
		return err
	}
	defer release()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return s.persist()
		}
	}
	return nil // 幂等
}

func (s *jsonListService) persist() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return filetxn.Write(s.path, data)
}

// QuickCmdService 快捷命令（与终端应用同格式，可直接拷贝旧 quick_commands.json）。
type QuickCmdService struct {
	inner        *jsonListService
	achievements *achievements.Store
}

func NewQuickCmdService(dataDir string) (*QuickCmdService, error) {
	inner, err := newJSONListService(dataDir + "/quick-commands.json")
	if err != nil {
		return nil, err
	}
	return &QuickCmdService{inner: inner}, nil
}

func (s *QuickCmdService) List() []QuickCommand { return s.inner.list() }
func (s *QuickCmdService) Save(cmd QuickCommand) (string, error) {
	if cmd.ID == "" {
		cmd.ID = uuid.NewString()
	}
	write := func() error { _, err := s.inner.upsert(cmd); return err }
	if s.achievements == nil {
		return s.inner.upsert(cmd)
	}
	items, err := s.ListChecked()
	if err != nil {
		return "", err
	}
	exists := false
	for _, item := range items {
		if item.ID == cmd.ID {
			exists = true
			break
		}
	}
	if exists {
		err = s.achievements.TrackExisting(social.MedalQuickCommandCreated, cmd.ID, strings.TrimSpace(cmd.Content) != "", write)
	} else {
		err = s.achievements.TrackNew(social.MedalQuickCommandCreated, cmd.ID, strings.TrimSpace(cmd.Content) != "", write)
	}
	return cmd.ID, err
}
func (s *QuickCmdService) Delete(id string) error {
	if s.achievements != nil {
		return s.achievements.TrackExisting(social.MedalQuickCommandCreated, id, false, func() error { return s.inner.remove(id) })
	}
	return s.inner.remove(id)
}

// Reorder only changes the slots belonging to the supplied IDs. Other groups
// and commands added by another control client keep their positions.
func (s *QuickCmdService) Reorder(ids []string) error {
	s.inner.mu.Lock()
	defer s.inner.mu.Unlock()
	release, err := s.inner.begin()
	if err != nil {
		return err
	}
	defer release()
	byID := make(map[string]QuickCommand, len(s.inner.items))
	for _, item := range s.inner.items {
		byID[item.ID] = item
	}
	selected := make(map[string]bool, len(ids))
	ordered := make([]QuickCommand, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok && !selected[id] {
			selected[id] = true
			ordered = append(ordered, item)
		}
	}
	if len(ordered) < 2 {
		return nil
	}
	previous := append([]QuickCommand(nil), s.inner.items...)
	next := 0
	for i, item := range s.inner.items {
		if selected[item.ID] {
			s.inner.items[i] = ordered[next]
			next++
		}
	}
	if err := s.inner.persist(); err != nil {
		s.inner.items = previous
		return err
	}
	return nil
}

// 结构化脚本服务见 scriptsvc.go（阶段 5 起复用 pkg/script，旧文本脚本自动迁移）。

func (s *jsonListService) reload() error {
	data, err := filetxn.Read(s.path)
	if err != nil {
		return err
	}
	items := []QuickCommand{}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &items); err != nil {
			return err
		}
	}
	s.items = items
	return nil
}
func (s *jsonListService) begin() (func(), error) {
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return nil, err
	}
	if err = s.reload(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}
func (s *QuickCmdService) ListChecked() ([]QuickCommand, error) {
	s.inner.mu.Lock()
	defer s.inner.mu.Unlock()
	if err := s.inner.reload(); err != nil {
		return nil, err
	}
	return append([]QuickCommand{}, s.inner.items...), nil
}
