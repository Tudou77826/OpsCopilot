package achievements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/script"
)

// Sources reads business data, never client-provided counts. Build a fresh
// verifier for every repair so its caches do not outlive a ledger transaction.
type Sources struct {
	Scripts, Recordings, Cases, Knowledge string
	QuickCommands                         func() (map[string]bool, error)
	quick                                 map[string]bool
	archives                              map[string]bool
}

func readObject(path, id string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取业务成果失败: %w", err)
	}
	var identity struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &identity); err != nil {
		return false, fmt.Errorf("业务成果损坏，请保留原文件: %w", err)
	}
	if identity.ID != id {
		return false, fmt.Errorf("业务成果标识不匹配")
	}
	if err = json.Unmarshal(data, out); err != nil {
		return false, fmt.Errorf("业务成果损坏: %w", err)
	}
	return true, nil
}

var archiveIdentity = regexp.MustCompile(`(?m)会话ID: ([A-Za-z0-9_-]{1,64})(?:\*|\s|$)`)

func (v *Sources) Active(kind social.MedalKind, id string) (bool, error) {
	if !social.ValidID(id) {
		return false, fmt.Errorf("业务成果标识无效")
	}
	switch kind {
	case social.MedalScriptCreated:
		var sc script.Script
		exists, err := readObject(filepath.Join(v.Scripts, "script_"+id+".json"), id, &sc)
		if !exists && err == nil {
			entries, e := os.ReadDir(v.Scripts)
			if e != nil && !os.IsNotExist(e) {
				return false, e
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				data, e := os.ReadFile(filepath.Join(v.Scripts, entry.Name()))
				if e != nil {
					return false, e
				}
				var candidate script.Script
				if e = json.Unmarshal(data, &candidate); e != nil {
					return false, fmt.Errorf("业务成果损坏: %w", e)
				}
				if candidate.ID == id {
					return EligibleScript(&candidate), nil
				}
			}
		}
		return exists && EligibleScript(&sc), err
	case social.MedalQuickCommandCreated:
		if v.quick == nil {
			if v.QuickCommands == nil {
				return false, fmt.Errorf("快捷指令恢复不可用")
			}
			items, err := v.QuickCommands()
			if err != nil {
				return false, err
			}
			v.quick = items
		}
		return v.quick[id], nil
	case social.MedalCaseCompleted:
		// Desktop persists recorder sessions; standalone sidecar persists cases.
		var cs struct {
			RootCause  string `json:"root_cause"`
			Conclusion string `json:"conclusion"`
		}
		path := filepath.Join(v.Recordings, "troubleshoot", "recording_"+id+".json")
		if v.Cases != "" {
			path = filepath.Join(v.Cases, "case_"+id+".json")
		}
		exists, err := readObject(path, id, &cs)
		return exists && strings.TrimSpace(cs.RootCause) != "" && strings.TrimSpace(cs.Conclusion) != "", err
	case social.MedalKnowledgeArchive:
		if v.archives == nil {
			items := map[string]bool{}
			if v.Knowledge == "" {
				return false, fmt.Errorf("知识目录不可用")
			}
			err := filepath.WalkDir(v.Knowledge, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
					data, e := os.ReadFile(path)
					if e != nil {
						return e
					}
					for _, m := range archiveIdentity.FindAllSubmatch(data, -1) {
						items[string(m[1])] = true
					}
				}
				return nil
			})
			if err != nil {
				return false, fmt.Errorf("核对知识归档失败: %w", err)
			}
			v.archives = items
		}
		return v.archives[id], nil
	default:
		return false, fmt.Errorf("未支持的业务恢复类型")
	}
}
func (s *Store) Repair(v *Sources) error {
	if err := s.Recover(func(i Intent) (bool, error) {
		active, err := v.Active(i.Kind, i.Reference)
		return active == i.Active, err
	}); err != nil {
		return err
	}
	// Refresh caches after recovery; reconciliation runs under a separate lock.
	v.quick = nil
	v.archives = nil
	return s.Reconcile(v.Active)
}
