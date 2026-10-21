// Package assistantskills reads the application's explicit, text-only Skill catalog.
// Snapshotting keeps Chrys away from source paths and fixes content per conversation.
package assistantskills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxSkills       = 32
	maxFileBytes    = 64 * 1024
	maxCatalogBytes = 512 * 1024
	maxResources    = 16
)

type Resource struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
type Skill struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Instructions string     `json:"instructions"`
	Resources    []Resource `json:"resources"`
}
type Catalog struct {
	Skills   []Skill
	Revision string
}

var validName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Load accepts only immediate <name>/SKILL.md entries and text files under
// references/. Scripts, executable metadata and arbitrary paths are not exported.
// A missing directory is an empty catalog; malformed selected content fails closed.
func Load(directory string) (Catalog, error) {
	catalog := Catalog{Skills: []Skill{}}
	if directory == "" {
		return catalog, nil
	}
	root, err := os.OpenRoot(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return catalog, errors.New("无法读取助手 Skill 目录")
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil || len(entries) > 128 {
		return catalog, errors.New("助手 Skill 目录无法读取或条目过多")
	}
	total := 0
	readText := func(filename string) (string, error) {
		info, err := root.Lstat(filename)
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("必须为普通文本文件")
		}
		file, err := root.Open(filename)
		if err != nil {
			return "", errors.New("文件无法读取")
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("必须为普通文本文件")
		}
		data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
		total += len(data)
		if err != nil || len(data) > maxFileBytes || total > maxCatalogBytes {
			return "", errors.New("Skill 文本超过大小限制")
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", errors.New("Skill 必须为 UTF-8 文本")
		}
		return strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n"), nil
	}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return catalog, errors.New("Skill 目录不能包含符号链接")
		}
		if !entry.IsDir() {
			continue
		}
		filename := path.Join(entry.Name(), "SKILL.md")
		info, err := root.Lstat(filename)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return catalog, fmt.Errorf("Skill %q 的 SKILL.md 必须为普通文件", entry.Name())
		}
		text, err := readText(filename)
		if err != nil {
			return catalog, fmt.Errorf("Skill %q: %w", entry.Name(), err)
		}
		skill, err := parse(text)
		if err != nil {
			return catalog, fmt.Errorf("Skill %q: %w", entry.Name(), err)
		}
		if skill.Name != entry.Name() || seen[skill.Name] {
			return catalog, errors.New("Skill 名称必须与目录一致且不能重复")
		}
		seen[skill.Name] = true
		if len(catalog.Skills) >= maxSkills {
			return catalog, errors.New("助手 Skill 数量不能超过 32 个")
		}
		refs := path.Join(entry.Name(), "references")
		if _, err = root.Lstat(refs); !errors.Is(err, fs.ErrNotExist) {
			visited := 0
			err = fs.WalkDir(root.FS(), refs, func(filename string, entry fs.DirEntry, walkErr error) error {
				visited++
				if walkErr != nil || visited > 128 {
					return errors.New("参考资料无法读取或条目过多")
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("参考资料不能包含符号链接")
				}
				if entry.IsDir() {
					return nil
				}
				ext := strings.ToLower(path.Ext(filename))
				if ext != ".md" && ext != ".txt" {
					return nil
				}
				if len(skill.Resources) >= maxResources {
					return errors.New("每个 Skill 最多包含 16 份参考资料")
				}
				content, err := readText(filename)
				if err != nil {
					return err
				}
				skill.Resources = append(skill.Resources, Resource{Name: strings.TrimPrefix(filename, skill.Name+"/"), Content: content})
				return nil
			})
			if err != nil {
				return catalog, fmt.Errorf("Skill %q: %w", skill.Name, err)
			}
		}
		catalog.Skills = append(catalog.Skills, skill)
	}
	data, _ := json.Marshal(catalog.Skills)
	digest := sha256.Sum256(data)
	catalog.Revision = hex.EncodeToString(digest[:])
	return catalog, nil
}

func parse(text string) (Skill, error) {
	skill := Skill{Resources: []Resource{}}
	if !strings.HasPrefix(text, "---\n") {
		return skill, errors.New("缺少 YAML 名称与描述")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return skill, errors.New("YAML 头部没有结束")
	}
	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if yaml.Unmarshal([]byte(text[4:4+end]), &metadata) != nil {
		return skill, errors.New("名称与描述格式无效")
	}
	metadata.Description = strings.TrimSpace(metadata.Description)
	if len(metadata.Name) > 64 || !validName.MatchString(metadata.Name) || len(metadata.Description) == 0 || len(metadata.Description) > 1024 {
		return skill, errors.New("名称需为小写字母、数字及单连字符，最长 64 字符；描述不能为空且不超过 1024 字节")
	}
	skill.Name, skill.Description = metadata.Name, metadata.Description
	skill.Instructions = strings.TrimSpace(text[4+end+5:])
	if skill.Instructions == "" {
		return skill, errors.New("Skill 正文不能为空")
	}
	return skill, nil
}
