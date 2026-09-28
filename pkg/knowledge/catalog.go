package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Catalog 完整目录
type Catalog struct {
	Version  int               `json:"version"`
	BuildAt  time.Time         `json:"buildAt"`
	Services []ServiceEntry    `json:"services"`
	FileHash map[string]string `json:"fileHash"` // path → md5
}

// ServiceEntry 服务层
type ServiceEntry struct {
	Name    string        `json:"name"`
	Modules []ModuleEntry `json:"modules"`
}

// ModuleEntry 模块层
type ModuleEntry struct {
	Name      string          `json:"name"`
	Scenarios []ScenarioEntry `json:"scenarios"`
}

// ScenarioEntry 场景层
type ScenarioEntry struct {
	Title      string   `json:"title"`
	ID         string   `json:"id,omitempty"` // 场景短 ID：由 服务+模块+标题 哈希生成，对外寻址用
	File       string   `json:"file"`
	LineStart  int      `json:"lineStart"`
	LineEnd    int      `json:"lineEnd"`
	Phenomena  string   `json:"phenomena"`
	Keywords   []string `json:"keywords"`
	Components []string `json:"components"`
	Type       string   `json:"type"` // "sop" | "archive"
}

// ScenarioID 生成场景短 ID：sha256(服务+\x00+模块+\x00+标题) 前 10 位十六进制。
// 确定性来源是语义三元组：Catalog 重建后 ID 不变，标题/服务/模块被编辑才会改变。
func ScenarioID(service, module, title string) string {
	sum := sha256.Sum256([]byte(service + "\x00" + module + "\x00" + title))
	return hex.EncodeToString(sum[:])[:10]
}

// TotalScenarios 统计目录中的场景总数
func (c *Catalog) TotalScenarios() int {
	total := 0
	for _, svc := range c.Services {
		for _, mod := range svc.Modules {
			total += len(mod.Scenarios)
		}
	}
	return total
}

// FindEntry 按文件路径和场景标题查找条目
// title 支持模糊匹配：只要 entry.Title 包含 title 或 title 包含 entry.Title 即可
func (c *Catalog) FindEntry(filePath string, title string) *ScenarioEntry {
	titleLower := strings.ToLower(title)
	for _, svc := range c.Services {
		for _, mod := range svc.Modules {
			for i := range mod.Scenarios {
				s := &mod.Scenarios[i]
				if s.File != filePath {
					continue
				}
				entryLower := strings.ToLower(s.Title)
				if strings.Contains(entryLower, titleLower) || strings.Contains(titleLower, entryLower) {
					return s
				}
			}
		}
	}
	return nil
}

// FindEntryLocation 返回条目的层级路径，格式为 "服务 > 模块 > 场景"
func (c *Catalog) FindEntryLocation(target *ScenarioEntry) string {
	for _, svc := range c.Services {
		for _, mod := range svc.Modules {
			for i := range mod.Scenarios {
				if &mod.Scenarios[i] == target {
					return fmt.Sprintf("%s › %s › %s", svc.Name, mod.Name, target.Title)
				}
			}
		}
	}
	return ""
}

const maxCatalogBytes = 25 * 1024 // 25K 上限

// RenderForLLM 将目录渲染为紧凑的文本，供 LLM 作为上下文使用
// 三级降级：完整 → 压缩（去 phenomena） → 模块级（只展示服务+模块层）
func (c *Catalog) RenderForLLM() string {
	if len(c.Services) == 0 {
		return ""
	}

	// 级别 1：完整渲染（标题 | 现象 | 关键词 | 文件）
	full := c.renderFull()
	if len(full) <= maxCatalogBytes {
		return full
	}

	// 级别 2：压缩渲染（标题 | 关键词 | 文件），去掉 phenomena
	compressed := c.renderCompressed()
	if len(compressed) <= maxCatalogBytes {
		return compressed
	}

	// 级别 3：模块级（只展示 服务 > 模块 (N场景)）
	return c.renderModuleLevel()
}

// renderFull 完整渲染：标题 | 现象 | 关键词 | 文件
func (c *Catalog) renderFull() string {
	var sb strings.Builder
	for _, svc := range c.Services {
		totalScenarios := 0
		for _, mod := range svc.Modules {
			totalScenarios += len(mod.Scenarios)
		}
		sb.WriteString(fmt.Sprintf("## %s (%d %s, %d %s)\n",
			svc.Name, len(svc.Modules), pluralUnit("module", len(svc.Modules)),
			totalScenarios, pluralUnit("scenario", totalScenarios)))

		for _, mod := range svc.Modules {
			sb.WriteString(fmt.Sprintf("### %s (%d %s)\n",
				mod.Name, len(mod.Scenarios), pluralUnit("scenario", len(mod.Scenarios))))
			for _, s := range mod.Scenarios {
				keywords := strings.Join(s.Keywords, ",")
				sb.WriteString(fmt.Sprintf(
					"- %s | %s | %s | %s#L%d\n",
					s.Title, s.Phenomena, keywords, s.File, s.LineStart,
				))
			}
		}
	}
	return sb.String()
}

// renderCompressed 压缩渲染：标题 | 关键词 | 文件（去掉 phenomena）
func (c *Catalog) renderCompressed() string {
	var sb strings.Builder
	for _, svc := range c.Services {
		totalScenarios := 0
		for _, mod := range svc.Modules {
			totalScenarios += len(mod.Scenarios)
		}
		sb.WriteString(fmt.Sprintf("## %s (%d %s, %d %s)\n",
			svc.Name, len(svc.Modules), pluralUnit("module", len(svc.Modules)),
			totalScenarios, pluralUnit("scenario", totalScenarios)))

		for _, mod := range svc.Modules {
			sb.WriteString(fmt.Sprintf("### %s (%d %s)\n",
				mod.Name, len(mod.Scenarios), pluralUnit("scenario", len(mod.Scenarios))))
			for _, s := range mod.Scenarios {
				keywords := strings.Join(s.Keywords, ",")
				sb.WriteString(fmt.Sprintf(
					"- %s | %s | %s#L%d\n",
					s.Title, keywords, s.File, s.LineStart,
				))
			}
		}
	}
	return sb.String()
}

// renderModuleLevel 模块级渲染：只展示 服务 > 模块 (N场景)
// 按服务逐个追加，超限时停止
func (c *Catalog) renderModuleLevel() string {
	var sb strings.Builder
	for _, svc := range c.Services {
		totalScenarios := 0
		for _, mod := range svc.Modules {
			totalScenarios += len(mod.Scenarios)
		}
		line := fmt.Sprintf("## %s (%d %s, %d %s)\n",
			svc.Name, len(svc.Modules), pluralUnit("module", len(svc.Modules)),
			totalScenarios, pluralUnit("scenario", totalScenarios))

		// 预估该服务的模块行大小
		modLines := ""
		for _, mod := range svc.Modules {
			modLines += fmt.Sprintf("- %s (%d %s)\n",
				mod.Name, len(mod.Scenarios), pluralUnit("scenario", len(mod.Scenarios)))
		}

		// 如果追加后会超限，跳过剩余服务
		if sb.Len()+len(line)+len(modLines) > maxCatalogBytes {
			remaining := c.countServicesFrom(c.Services, svc.Name) - 1
			if remaining > 0 {
				sb.WriteString(fmt.Sprintf("... (还有 %d %s未展示)\n", remaining, pluralUnit("service", remaining)))
			}
			break
		}

		sb.WriteString(line)
		sb.WriteString(modLines)
	}
	return sb.String()
}

// countServicesFrom 计算从指定服务名开始的剩余服务数
func (c *Catalog) countServicesFrom(services []ServiceEntry, fromName string) int {
	counting := false
	count := 0
	for _, svc := range services {
		if svc.Name == fromName {
			counting = true
		}
		if counting {
			count++
		}
	}
	return count
}

func pluralUnit(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// ReplaceEntries 替换指定文件的所有条目
// 先移除旧条目，再按 serviceName/moduleName 重新组织插入
func (c *Catalog) ReplaceEntries(filePath string, serviceName string, moduleName string, entries []ScenarioEntry) {
	// 移除该文件的所有旧条目
	c.removeFileEntries(filePath)

	if len(entries) == 0 {
		return
	}

	// 找到或创建 ServiceEntry
	var svc *ServiceEntry
	for i := range c.Services {
		if c.Services[i].Name == serviceName {
			svc = &c.Services[i]
			break
		}
	}
	if svc == nil {
		c.Services = append(c.Services, ServiceEntry{Name: serviceName})
		svc = &c.Services[len(c.Services)-1]
	}

	// 找到或创建 ModuleEntry
	var mod *ModuleEntry
	for i := range svc.Modules {
		if svc.Modules[i].Name == moduleName {
			mod = &svc.Modules[i]
			break
		}
	}
	if mod == nil {
		svc.Modules = append(svc.Modules, ModuleEntry{Name: moduleName})
		mod = &svc.Modules[len(svc.Modules)-1]
	}

	// 追加新条目（ID 在此统一赋值：此处是树重建的唯一插入口）
	for i := range entries {
		entries[i].ID = ScenarioID(serviceName, moduleName, entries[i].Title)
	}
	mod.Scenarios = append(mod.Scenarios, entries...)
}

// ScenarioLocation 场景在目录树中的定位
type ScenarioLocation struct {
	Service string
	Module  string
	Entry   *ScenarioEntry
}

// FindByID 按短 ID 查找场景
func (c *Catalog) FindByID(id string) *ScenarioLocation {
	for i := range c.Services {
		svc := &c.Services[i]
		for j := range svc.Modules {
			mod := &svc.Modules[j]
			for k := range mod.Scenarios {
				if mod.Scenarios[k].ID == id {
					return &ScenarioLocation{Service: svc.Name, Module: mod.Name, Entry: &mod.Scenarios[k]}
				}
			}
		}
	}
	return nil
}

// EnsureScenarioIDs 为历史无 ID 条目补算 ID，并检测 ID 冲突。
// 旧版 .catalog.json 没有该字段，构建时统一补齐。
func (c *Catalog) EnsureScenarioIDs() error {
	seen := make(map[string]string) // id -> "服务/模块/标题"，用于冲突定位
	for i := range c.Services {
		svc := &c.Services[i]
		for j := range svc.Modules {
			mod := &svc.Modules[j]
			for k := range mod.Scenarios {
				if mod.Scenarios[k].ID == "" {
					mod.Scenarios[k].ID = ScenarioID(svc.Name, mod.Name, mod.Scenarios[k].Title)
				}
				id := mod.Scenarios[k].ID
				owner := svc.Name + "/" + mod.Name + "/" + mod.Scenarios[k].Title
				if prev, dup := seen[id]; dup {
					return fmt.Errorf("场景 ID 冲突: %s 与 %s 同为 %s", prev, owner, id)
				}
				seen[id] = owner
			}
		}
	}
	return nil
}

// RemoveDeletedFiles 清理已删除文件的条目和空的 Service/Module
func (c *Catalog) RemoveDeletedFiles(existingFiles map[string]bool) {
	for i := range c.Services {
		svc := &c.Services[i]
		for j := len(svc.Modules) - 1; j >= 0; j-- {
			mod := &svc.Modules[j]
			kept := make([]ScenarioEntry, 0, len(mod.Scenarios))
			for _, s := range mod.Scenarios {
				if existingFiles[s.File] {
					kept = append(kept, s)
				}
			}
			if len(kept) == 0 {
				svc.Modules = append(svc.Modules[:j], svc.Modules[j+1:]...)
			} else {
				svc.Modules[j].Scenarios = kept
			}
		}
	}

	// 清理空的 Service
	surviving := make([]ServiceEntry, 0, len(c.Services))
	for _, svc := range c.Services {
		if len(svc.Modules) > 0 {
			surviving = append(surviving, svc)
		}
	}
	c.Services = surviving

	// 清理 FileHash 中已删除的文件
	for path := range c.FileHash {
		if !existingFiles[path] {
			delete(c.FileHash, path)
		}
	}
}

// removeFileEntries 移除指定文件的所有条目
func (c *Catalog) removeFileEntries(filePath string) {
	for i := range c.Services {
		svc := &c.Services[i]
		for j := range svc.Modules {
			mod := &svc.Modules[j]
			kept := make([]ScenarioEntry, 0, len(mod.Scenarios))
			for _, s := range mod.Scenarios {
				if s.File != filePath {
					kept = append(kept, s)
				}
			}
			svc.Modules[j].Scenarios = kept
		}
	}
}
