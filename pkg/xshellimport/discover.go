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
// 用户把数据目录改到其他盘时探测不到，由调用方回退到手工选择。
func DiscoverXshellSessionDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return discoverAssetDirsUnder(xshellDataRoots(home), "Sessions", "")
}

// DiscoverXshellQuickButtonDirs 探测本机 Xshell 的快捷按钮目录
// （<版本>\Xshell\QuickButton Files，每个 .qbl 是一套按钮），按版本从新到旧排序。
//
// 与会话目录不同，这里要求目录里至少有一个 .qbl 才列为来源：快捷按钮目录常常存在
// 但为空，列出来只会给用户一个导不出任何东西的空入口。
func DiscoverXshellQuickButtonDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return discoverAssetDirsUnder(xshellDataRoots(home), "QuickButton Files", ".qbl")
}

// xshellDataRoots 返回 Xshell 可能的数据根目录（NetSarang Computer 的父目录）。
//
// 用户文档目录可能被重定向到 OneDrive（中文系统下实体名可能是「文档」），
// 因此这些位置都要探测。根目录由调用方传入而不是在这里读 UserHomeDir，
// 是为了让目录探测逻辑可以脱离真实用户目录被测试。
func xshellDataRoots(home string) []string {
	return []string{
		filepath.Join(home, "Documents", "NetSarang Computer"),
		filepath.Join(home, "OneDrive", "Documents", "NetSarang Computer"),
		filepath.Join(home, "OneDrive", "文档", "NetSarang Computer"),
	}
}

// discoverAssetDirsUnder 找出所有 <root>/<版本>/Xshell/<leafDir>。
//
// requireExt 非空时要求目录内至少有一个该扩展名的文件（递归查找）。
func discoverAssetDirsUnder(roots []string, leafDir, requireExt string) []string {
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
			dir := filepath.Join(root, entry.Name(), "Xshell", leafDir)
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() || seen[dir] {
				continue
			}
			if requireExt != "" && !hasFileWithExt(dir, requireExt) {
				continue
			}
			seen[dir] = true
			found = append(found, dir)
		}
	}

	// 版本号大的排前面，便于默认选中最新版本（"10" 应排在 "8" 之前，
	// 所以按数值而非字符串比较）。
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && assetDirVersion(found[j]) > assetDirVersion(found[j-1]); j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}
	return found
}

// assetDirVersion 从 .../NetSarang Computer/<版本>/Xshell/<目录> 中取出 <版本>。
func assetDirVersion(dir string) int {
	versionDir := filepath.Base(filepath.Dir(filepath.Dir(dir)))
	n, err := strconv.Atoi(versionDir)
	if err != nil {
		return 0
	}
	return n
}

// hasFileWithExt 判断目录（含子目录）下是否存在指定扩展名的文件。
func hasFileWithExt(dir, ext string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(p), ext) {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
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
