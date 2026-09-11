package xshellimport

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DiscoverXshellSessionDirs 探测本机 Xshell 的会话目录，按版本从新到旧排序。
//
// Xshell 按版本分子目录：%USERPROFILE%\Documents\NetSarang Computer\<版本>\Xshell\Sessions。
// 用户文档目录可能被重定向到 OneDrive（中文系统下实体名可能是「文档」），
// 因此这些位置都要探测。用户把数据目录改到其他盘时探测不到，由调用方回退到手工选择。
func DiscoverXshellSessionDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	roots := []string{
		filepath.Join(home, "Documents", "NetSarang Computer"),
		filepath.Join(home, "OneDrive", "Documents", "NetSarang Computer"),
		filepath.Join(home, "OneDrive", "文档", "NetSarang Computer"),
	}

	var found []string
	seen := make(map[string]bool)
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sessions := filepath.Join(root, entry.Name(), "Xshell", "Sessions")
			info, err := os.Stat(sessions)
			if err != nil || !info.IsDir() || seen[sessions] {
				continue
			}
			seen[sessions] = true
			found = append(found, sessions)
		}
	}

	// 版本号大的排前面，便于默认选中最新版本的会话目录（"10" 应排在 "8" 之前，
	// 所以按数值而非字符串比较）。
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && sessionDirVersion(found[j]) > sessionDirVersion(found[j-1]); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}
	return found
}

// sessionDirVersion 从 .../NetSarang Computer/<版本>/Xshell/Sessions 中取出 <版本>。
func sessionDirVersion(dir string) int {
	versionDir := filepath.Base(filepath.Dir(filepath.Dir(dir)))
	n, err := strconv.Atoi(versionDir)
	if err != nil {
		return 0
	}
	return n
}

// ParsePath 解析一个 .xsh 文件、包含 .xsh 的目录树，或 .xts 备份包。
//
// 返回值第二项是解析阶段的警告（单个文件读不了、目录里没有会话等），
// 这些不构成失败，但必须让用户看到。
func ParsePath(target string, opts ParseOptions) ([]*SessionRecord, []string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil, fs.ErrNotExist
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, nil, err
	}

	if !info.IsDir() {
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, nil, err
		}
		switch strings.ToLower(filepath.Ext(target)) {
		case ".xts":
			return ParseXTS(data, opts)
		case ".xsh":
			record := ParseXSH(data, sessionName(filepath.Base(target)), nil, opts)
			return []*SessionRecord{record}, nil, nil
		default:
			return nil, nil, fs.ErrInvalid
		}
	}
	return parseDirectory(target, opts)
}

func parseDirectory(root string, opts ParseOptions) ([]*SessionRecord, []string, error) {
	var records []*SessionRecord
	var warnings []string

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 单个条目不可读不应中断整次导入。
			warnings = append(warnings, "跳过无法读取的条目 "+p)
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".xsh") {
			return nil
		}

		rel, relErr := filepath.Rel(root, filepath.Dir(p))
		if relErr != nil {
			rel = "."
		}
		content, readErr := os.ReadFile(p)
		if readErr != nil {
			warnings = append(warnings, "读取失败 "+p)
			return nil
		}
		records = append(records, ParseXSH(content, sessionName(d.Name()), groupPathFromRel(rel), opts))
		return nil
	})
	if err != nil {
		return nil, warnings, err
	}
	if len(records) == 0 {
		warnings = append(warnings, "该目录下没有找到 .xsh 会话文件")
	}
	return records, warnings, nil
}

// groupPathFromRel 把相对目录转换为分组层级；根目录返回 nil。
func groupPathFromRel(rel string) []string {
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" || rel == "/" {
		return nil
	}
	var out []string
	for _, seg := range strings.Split(rel, "/") {
		if seg != "" && seg != "." {
			out = append(out, seg)
		}
	}
	return out
}
