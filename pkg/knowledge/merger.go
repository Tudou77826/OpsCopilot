package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"opscopilot/pkg/knowledge/patchstore"
)

// RebuildFromPatches 根据补丁列表重建所有 service_module.md 文件
// 返回重建的文件数量和错误
func RebuildFromPatches(knowledgeDir string, patches []patchstore.Patch) (int, error) {
	if len(patches) == 0 {
		return 0, nil
	}

	// 按 service+module 分组
	groups := groupPatchesByServiceModule(patches)

	archiveDir := filepath.Join(knowledgeDir, "archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return 0, fmt.Errorf("create archive dir: %w", err)
	}

	count := 0
	for key, groupPatches := range groups {
		service, module := splitGroupKey(key)
		content := buildServiceFile(service, module, groupPatches)

		fileName := sanitizeFileName(service, module) + ".md"
		targetPath := filepath.Join(archiveDir, fileName)

		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return count, fmt.Errorf("write %s: %w", fileName, err)
		}
		count++
	}

	return count, nil
}

// buildServiceFile 将一组补丁拼接为完整的 service_module.md
func buildServiceFile(service, module string, patches []patchstore.Patch) string {
	sort.Slice(patches, func(i, j int) bool {
		return patches[i].Timestamp.Before(patches[j].Timestamp)
	})

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "service: %q\n", service)
	fmt.Fprintf(&sb, "module: %q\n", module)
	sb.WriteString("type: sop\n")
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "# %s - %s 运维文档\n\n", service, module)
	sb.WriteString("## 服务信息\n\n")
	fmt.Fprintf(&sb, "| 字段 | 值 |\n|------|----|\n| 微服务 | %s |\n| 模块 | %s |\n\n", service, module)

	for _, p := range patches {
		sb.WriteString(p.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// PatchToArchiveRecord 从 Patch 生成可追加到本地文件的记录文本
// 格式与 buildArchiveRecord 一致，确保本地归档和补丁重建后的文件格式兼容
func PatchToArchiveRecord(patch patchstore.Patch) string {
	return "\n" + patch.Content + "\n"
}

// groupPatchesByServiceModule 按 "service|module" 分组
func groupPatchesByServiceModule(patches []patchstore.Patch) map[string][]patchstore.Patch {
	groups := make(map[string][]patchstore.Patch)
	for _, p := range patches {
		key := p.Service + "|" + p.Module
		groups[key] = append(groups[key], p)
	}
	return groups
}

// splitGroupKey 拆分 "service|module" 为 service, module
func splitGroupKey(key string) (string, string) {
	service, module, ok := strings.Cut(key, "|")
	if !ok {
		return key, "默认模块"
	}
	return service, module
}
