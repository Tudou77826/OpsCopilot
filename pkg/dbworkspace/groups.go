package dbworkspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"opscopilot/pkg/filetxn"
)

func hasGroup(groups []string, name string) bool {
	for _, group := range groups {
		if group == name {
			return true
		}
	}
	return false
}

func groupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || strings.ContainsAny(name, "\x00\r\n") {
		return "", fmt.Errorf("分组名称应为 1–100 个字且不能包含换行")
	}
	return name, nil
}

func (s *Service) Groups() ([]string, error) {
	f, err := s.readProfiles()
	if err != nil {
		return nil, err
	}
	sort.Strings(f.Groups)
	return f.Groups, nil
}

// Group changes are a single file transaction. Renaming or removing a group
// updates all affected profiles without touching their credentials or sessions.
func (s *Service) changeGroups(change func(*profileFile) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("创建连接配置目录失败: %w", err)
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return err
	}
	defer release()
	f, err := s.readProfiles()
	if err != nil {
		return err
	}
	if err = change(&f); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err == nil {
		err = filetxn.Write(s.path, b)
	}
	if err != nil {
		return fmt.Errorf("保存连接分组失败: %w", err)
	}
	return nil
}

func (s *Service) CreateGroup(name string) error {
	name, err := groupName(name)
	if err != nil {
		return err
	}
	return s.changeGroups(func(f *profileFile) error {
		if hasGroup(f.Groups, name) {
			return fmt.Errorf("分组「%s」已存在", name)
		}
		f.Groups = append(f.Groups, name)
		return nil
	})
}

func (s *Service) RenameGroup(oldName, newName string) error {
	newName, err := groupName(newName)
	if err != nil {
		return err
	}
	return s.changeGroups(func(f *profileFile) error {
		if !hasGroup(f.Groups, oldName) {
			return fmt.Errorf("分组已不存在，请刷新")
		}
		if oldName == newName {
			return nil
		}
		if hasGroup(f.Groups, newName) {
			return fmt.Errorf("分组「%s」已存在", newName)
		}
		for i, name := range f.Groups {
			if name == oldName {
				f.Groups[i] = newName
			}
		}
		for i := range f.Profiles {
			if f.Profiles[i].Group == oldName {
				f.Profiles[i].Group = newName
				f.Profiles[i].Revision++
			}
		}
		return nil
	})
}

func (s *Service) DeleteGroup(name string) error {
	return s.changeGroups(func(f *profileFile) error {
		if !hasGroup(f.Groups, name) {
			return fmt.Errorf("分组已不存在，请刷新")
		}
		for i, group := range f.Groups {
			if group == name {
				f.Groups = append(f.Groups[:i], f.Groups[i+1:]...)
				break
			}
		}
		for i := range f.Profiles {
			if f.Profiles[i].Group == name {
				f.Profiles[i].Group = ""
				f.Profiles[i].Revision++
			}
		}
		return nil
	})
}
