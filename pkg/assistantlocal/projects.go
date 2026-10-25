package assistantlocal

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"opscopilot/pkg/filetxn"
)

const MaxProjects = 64

type Project struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Directories []string `json:"directories"`
}
type ProjectState struct {
	Version  int       `json:"version"`
	ActiveID string    `json:"activeId"`
	Projects []Project `json:"projects"`
}

func (s ProjectState) ActiveProject() (Project, bool) {
	for _, p := range s.Projects {
		if p.ID == s.ActiveID {
			return p, true
		}
	}
	return Project{}, false
}

// Projects stores directory groups independently of model/chat lifetime. Each
// mutation reloads under an OS lock and atomically replaces the file, so two
// application processes cannot silently overwrite unrelated project edits.
type Projects struct {
	File   string
	mu     sync.Mutex
	memory ProjectState
}

func emptyProjects() ProjectState { return ProjectState{Version: 1, Projects: []Project{}} }
func cloneProjects(s ProjectState) ProjectState {
	out := s
	out.Projects = append([]Project{}, s.Projects...)
	for i := range out.Projects {
		out.Projects[i].Directories = append([]string{}, s.Projects[i].Directories...)
	}
	return out
}
func projectName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
		return "", errors.New("项目名称应为 1 至 64 个字符")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", errors.New("项目名称不能包含控制字符")
		}
	}
	return name, nil
}
func validateProjects(s ProjectState) error {
	if s.Version != 1 || len(s.Projects) > MaxProjects {
		return errors.New("项目配置版本或数量无效")
	}
	ids := map[string]bool{}
	names := []string{}
	for _, p := range s.Projects {
		n, err := projectName(p.Name)
		if err != nil || n != p.Name || p.ID == "" || len(p.ID) > 128 || ids[p.ID] || len(p.Directories) > MaxDirectories {
			return errors.New("本地项目配置格式无效")
		}
		for _, name := range names {
			if strings.EqualFold(name, n) {
				return errors.New("项目名称重复")
			}
		}
		for _, dir := range p.Directories {
			if !filepath.IsAbs(dir) || len(dir) > 2048 || strings.ContainsAny(dir, "\x00\r\n") {
				return errors.New("项目目录配置格式无效")
			}
		}
		ids[p.ID] = true
		names = append(names, n)
	}
	if s.ActiveID != "" && !ids[s.ActiveID] {
		return errors.New("当前项目不存在，请修复项目配置")
	}
	return nil
}
func (p *Projects) transaction(change func(*ProjectState) error) (ProjectState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := cloneProjects(p.memory)
	if state.Version == 0 {
		state = emptyProjects()
	}
	if p.File != "" {
		release, err := filetxn.Lock(p.File)
		if err != nil {
			return ProjectState{}, fmt.Errorf("读取本地项目失败: %w", err)
		}
		defer release()
		data, err := filetxn.Read(p.File)
		if err != nil {
			return ProjectState{}, fmt.Errorf("读取本地项目失败: %w", err)
		}
		state = emptyProjects()
		if len(data) > 3<<20 {
			return ProjectState{}, errors.New("本地项目配置过大")
		}
		if len(data) > 0 {
			state = ProjectState{}
			if json.Unmarshal(data, &state) != nil {
				return ProjectState{}, errors.New("本地项目配置损坏，未覆盖原文件")
			}
		}
	}
	if err := validateProjects(state); err != nil {
		return ProjectState{}, err
	}
	if change != nil {
		if err := change(&state); err != nil {
			return ProjectState{}, err
		}
		if err := validateProjects(state); err != nil {
			return ProjectState{}, err
		}
		if p.File != "" {
			data, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				return ProjectState{}, err
			}
			if err = filetxn.Write(p.File, data); err != nil {
				return ProjectState{}, fmt.Errorf("保存本地项目失败: %w", err)
			}
		}
		p.memory = cloneProjects(state)
	}
	return cloneProjects(state), nil
}
func (p *Projects) Snapshot() (ProjectState, error) { return p.transaction(nil) }
func (p *Projects) Create(name string) (ProjectState, error) {
	name, err := projectName(name)
	if err != nil {
		return ProjectState{}, err
	}
	return p.transaction(func(s *ProjectState) error {
		if len(s.Projects) >= MaxProjects {
			return errors.New("最多保存 64 个本地项目")
		}
		for _, v := range s.Projects {
			if strings.EqualFold(v.Name, name) {
				return errors.New("项目名称已存在")
			}
		}
		id := uuid.NewString()
		s.Projects = append(s.Projects, Project{ID: id, Name: name, Directories: []string{}})
		s.ActiveID = id
		return nil
	})
}
func (p *Projects) Select(id string) (ProjectState, error) {
	return p.transaction(func(s *ProjectState) error {
		if id == "" {
			s.ActiveID = ""
			return nil
		}
		for _, v := range s.Projects {
			if v.ID == id {
				s.ActiveID = id
				return nil
			}
		}
		return errors.New("项目不存在，请刷新后重新选择")
	})
}
func (p *Projects) Rename(id, name string) (ProjectState, error) {
	name, err := projectName(name)
	if err != nil {
		return ProjectState{}, err
	}
	return p.transaction(func(s *ProjectState) error {
		index := -1
		for i, v := range s.Projects {
			if v.ID == id {
				index = i
			} else if strings.EqualFold(v.Name, name) {
				return errors.New("项目名称已存在")
			}
		}
		if index < 0 {
			return errors.New("项目不存在")
		}
		s.Projects[index].Name = name
		return nil
	})
}
func (p *Projects) Delete(id string) (ProjectState, error) {
	return p.transaction(func(s *ProjectState) error {
		for i, v := range s.Projects {
			if v.ID == id {
				s.Projects = append(s.Projects[:i], s.Projects[i+1:]...)
				if s.ActiveID == id {
					s.ActiveID = ""
				}
				return nil
			}
		}
		return errors.New("项目不存在")
	})
}
func (p *Projects) Add(id string, paths []string) (ProjectState, error) {
	// Validate before acquiring the config lock; unavailable network mounts must
	// not block other processes from reading or selecting their projects.
	paths, err := Normalize(paths)
	if err != nil {
		return ProjectState{}, err
	}
	return p.transaction(func(s *ProjectState) error {
		for i, v := range s.Projects {
			if v.ID == id {
				next := append([]string{}, v.Directories...)
				for _, dir := range paths {
					if !contains(next, dir) {
						next = append(next, dir)
					}
				}
				if len(next) > MaxDirectories {
					return errors.New("每个项目最多添加 16 个目录")
				}
				s.Projects[i].Directories = next
				return nil
			}
		}
		return errors.New("目标项目不存在，未追加目录")
	})
}
func (p *Projects) Remove(id, path string) (ProjectState, error) {
	return p.transaction(func(s *ProjectState) error {
		for i, v := range s.Projects {
			if v.ID == id {
				for j, dir := range v.Directories {
					if same(dir, path) {
						s.Projects[i].Directories = append(v.Directories[:j], v.Directories[j+1:]...)
						break
					}
				}
				return nil
			}
		}
		return errors.New("项目不存在")
	})
}
