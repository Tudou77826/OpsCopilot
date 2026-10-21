package assistantskills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Library owns installed packages and launch templates, not their business logic.
// Callers serialize mutations with assistant turns so active packages cannot change.
type Library struct{ Directory string }
type Installed struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Enabled      bool     `json:"enabled"`
	Files        []string `json:"files"`
}
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}
type Command struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Skill       string  `json:"skill"`
	Template    string  `json:"template"`
	Fields      []Field `json:"fields"`
}
type LibraryState struct {
	Skills   []Installed `json:"skills"`
	Commands []Command   `json:"commands"`
}
type libraryConfig struct {
	Disabled map[string]bool `json:"disabled"`
	Commands []Command       `json:"commands"`
}
type packageFile struct {
	name string
	data []byte
	mode fs.FileMode
}
type skillPackage struct {
	info  Installed
	files []packageFile
}

var libraryMu sync.Mutex

func (l Library) lock() (func(), error) {
	libraryMu.Lock()
	if l.Directory == "" {
		return libraryMu.Unlock, nil
	}
	release, err := filetxn.Lock(filepath.Join(l.Directory, "library.json"))
	if err != nil {
		libraryMu.Unlock()
		return nil, err
	}
	return func() { release(); libraryMu.Unlock() }, nil
}

var commandName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)
var fieldName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,31}$`)
var placeholders = regexp.MustCompile(`\{\{([a-zA-Z][a-zA-Z0-9_]*)\}\}`)

func (l Library) config() (libraryConfig, error) {
	c := libraryConfig{Disabled: map[string]bool{}, Commands: []Command{}}
	if l.Directory == "" {
		return c, nil
	}
	b, err := os.ReadFile(filepath.Join(l.Directory, "library.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil || len(b) > 512<<10 {
		return c, errors.New("无法读取助手能力配置")
	}
	if json.Unmarshal(b, &c) != nil {
		return c, errors.New("助手能力配置格式无效")
	}
	if c.Disabled == nil {
		c.Disabled = map[string]bool{}
	}
	if c.Commands == nil {
		c.Commands = []Command{}
	}
	for i := range c.Commands {
		if c.Commands[i].Fields == nil {
			c.Commands[i].Fields = []Field{}
		}
	}
	return c, nil
}
func (l Library) save(c libraryConfig) error {
	if err := os.MkdirAll(l.Directory, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return filetxn.Write(filepath.Join(l.Directory, "library.json"), b)
}
func readPackage(directory string) (skillPackage, error) {
	p := skillPackage{info: Installed{Enabled: true, Files: []string{}}}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return p, errors.New("无法读取 Skill 目录")
	}
	defer root.Close()
	total, entries := 0, 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		entries++
		if walkErr != nil {
			return walkErr
		}
		if entries > 512 {
			return errors.New("Skill 文件数量超过 512")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("Skill 不支持符号链接")
		}
		if entry.IsDir() {
			return nil
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("Skill 只能包含普通文件")
		}
		if info.Size() > 8<<20 || int64(total)+info.Size() > 32<<20 {
			return errors.New("Skill 单文件不能超过 8 MB，总大小不能超过 32 MB")
		}
		file, e := root.Open(name)
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(file, (8<<20)+1))
		_ = file.Close()
		if e != nil {
			return e
		}
		total += len(data)
		if len(data) > 8<<20 || total > 32<<20 {
			return errors.New("Skill 超过大小限制")
		}
		p.files = append(p.files, packageFile{name, data, info.Mode().Perm()})
		p.info.Files = append(p.info.Files, name)
		if name == "SKILL.md" {
			if len(data) > maxFileBytes || !utf8.Valid(data) {
				return errors.New("SKILL.md 必须是 64 KB 以内的 UTF-8 文本")
			}
			skill, e := parse(strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n"))
			if e != nil {
				return e
			}
			p.info.Name, p.info.Description, p.info.Instructions = skill.Name, skill.Description, skill.Instructions
		}
		return nil
	})
	if err != nil {
		return p, err
	}
	if p.info.Name == "" {
		return p, errors.New("请选择直接包含 SKILL.md 的目录")
	}
	return p, nil
}
func (l Library) packages(c libraryConfig) ([]skillPackage, error) {
	entries, err := os.ReadDir(l.Directory)
	if errors.Is(err, fs.ErrNotExist) {
		return []skillPackage{}, nil
	}
	if err != nil {
		return nil, err
	}
	packs := []skillPackage{}
	total := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("Skill 目录不能包含符号链接")
		}
		if !entry.IsDir() {
			continue
		}
		p, e := readPackage(filepath.Join(l.Directory, entry.Name()))
		if e != nil {
			return nil, fmt.Errorf("Skill %s: %w", entry.Name(), e)
		}
		if p.info.Name != entry.Name() {
			return nil, errors.New("Skill 名称必须与目录一致")
		}
		for _, f := range p.files {
			total += len(f.data)
		}
		if total > 64<<20 {
			return nil, errors.New("Skill 总大小不能超过 64 MB")
		}
		p.info.Enabled = !c.Disabled[p.info.Name]
		packs = append(packs, p)
		if len(packs) > 32 {
			return nil, errors.New("最多导入 32 个 Skill")
		}
	}
	return packs, nil
}
func (l Library) Read() (LibraryState, error) {
	release, err := l.lock()
	if err != nil {
		return LibraryState{}, err
	}
	defer release()
	c, err := l.config()
	if err != nil {
		return LibraryState{}, err
	}
	packs, err := l.packages(c)
	if err != nil {
		return LibraryState{}, err
	}
	s := LibraryState{Skills: []Installed{}, Commands: c.Commands}
	for _, p := range packs {
		s.Skills = append(s.Skills, p.info)
	}
	return s, nil
}
func writePackage(directory string, p skillPackage) error {
	for _, f := range p.files {
		target := filepath.Join(directory, filepath.FromSlash(f.name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, f.data, 0600|f.mode&0100); err != nil {
			return err
		}
	}
	return nil
}
func (l Library) Import(source string, replace bool) error { return l.ImportAs(source, replace, "") }
func (l Library) ImportAs(source string, replace bool, expected string) error {
	release, err := l.lock()
	if err != nil {
		return err
	}
	defer release()
	p, err := readPackage(source)
	if err != nil {
		return err
	}
	if expected != "" && p.info.Name != expected {
		return errors.New("所选目录的 Skill 名称与待更新的 Skill 不一致")
	}
	c, err := l.config()
	if err != nil {
		return err
	}
	packs, err := l.packages(c)
	if err != nil {
		return err
	}
	target := filepath.Join(l.Directory, p.info.Name)
	_, statErr := os.Stat(target)
	exists := statErr == nil
	if exists && !replace {
		return errors.New("同名 Skill 已存在，请选择更新")
	}
	if !exists && len(packs) >= 32 {
		return errors.New("最多导入 32 个 Skill")
	}
	if err = os.MkdirAll(l.Directory, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(l.Directory, ".import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = writePackage(stage, p); err != nil {
		return err
	}
	backup := stage + "-old"
	if exists {
		if err = os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err = os.Rename(stage, target); err != nil {
		if exists {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if exists {
		_ = os.RemoveAll(backup)
	}
	return nil
}
func (l Library) Enable(name string, enabled bool) error {
	release, err := l.lock()
	if err != nil {
		return err
	}
	defer release()
	if !validName.MatchString(name) || len(name) > 64 {
		return errors.New("Skill 名称无效")
	}
	if _, err := readPackage(filepath.Join(l.Directory, name)); err != nil {
		return err
	}
	c, err := l.config()
	if err != nil {
		return err
	}
	if enabled {
		delete(c.Disabled, name)
	} else {
		c.Disabled[name] = true
	}
	return l.save(c)
}
func (l Library) Remove(name string) error {
	release, err := l.lock()
	if err != nil {
		return err
	}
	defer release()
	if !validName.MatchString(name) || len(name) > 64 {
		return errors.New("Skill 名称无效")
	}
	c, err := l.config()
	if err != nil {
		return err
	}
	for _, cmd := range c.Commands {
		if cmd.Skill == name {
			return errors.New("请先移除或修改关联此 Skill 的快捷命令")
		}
	}
	// Validated single path component; never remove the library root.
	target := filepath.Join(l.Directory, name)
	if err = os.RemoveAll(target); err != nil {
		return err
	}
	delete(c.Disabled, name)
	return l.save(c)
}
func ValidateCommand(c Command) error {
	if !commandName.MatchString(c.Name) || strings.TrimSpace(c.Description) == "" || len(c.Description) > 1024 || len(c.Template) > 8192 || strings.TrimSpace(c.Template) == "" || len(c.Fields) > 12 {
		return errors.New("请填写有效的命令名称、用途和提示词（最多 8 KB、12 个空位）")
	}
	remaining := placeholders.ReplaceAllString(c.Template, "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return errors.New("空位请使用 {{字母或数字名称}} 格式")
	}
	found := map[string]bool{}
	for _, m := range placeholders.FindAllStringSubmatch(c.Template, -1) {
		found[m[1]] = true
	}
	seen := map[string]bool{}
	for _, f := range c.Fields {
		if !fieldName.MatchString(f.Key) || seen[f.Key] || !found[f.Key] || strings.TrimSpace(f.Label) == "" || len(f.Label) > 80 || (f.Kind != "text" && f.Kind != "connection") {
			return errors.New("空位需有唯一名称、显示名称和正确类型，并出现在提示词中")
		}
		seen[f.Key] = true
	}
	if len(seen) != len(found) {
		return errors.New("每个 {{空位}} 都需要对应的填写项定义")
	}
	return nil
}
func (l Library) SaveCommand(command Command) error {
	release, err := l.lock()
	if err != nil {
		return err
	}
	defer release()
	if err := ValidateCommand(command); err != nil {
		return err
	}
	if command.Fields == nil {
		command.Fields = []Field{}
	}
	c, err := l.config()
	if err != nil {
		return err
	}
	if command.Skill != "" {
		if !validName.MatchString(command.Skill) {
			return errors.New("Skill 名称无效")
		}
		if _, err = readPackage(filepath.Join(l.Directory, command.Skill)); err != nil {
			return err
		}
	}
	for i, old := range c.Commands {
		if strings.EqualFold(old.Name, command.Name) {
			c.Commands[i] = command
			return l.save(c)
		}
	}
	if len(c.Commands) >= 128 {
		return errors.New("快捷命令不能超过 128 个")
	}
	c.Commands = append(c.Commands, command)
	sort.Slice(c.Commands, func(i, j int) bool { return c.Commands[i].Name < c.Commands[j].Name })
	return l.save(c)
}
func (l Library) RemoveCommand(name string) error {
	release, err := l.lock()
	if err != nil {
		return err
	}
	defer release()
	c, err := l.config()
	if err != nil {
		return err
	}
	for i, command := range c.Commands {
		if command.Name == name {
			c.Commands = append(c.Commands[:i], c.Commands[i+1:]...)
			return l.save(c)
		}
	}
	return errors.New("快捷命令不存在")
}

// NativeSnapshot exports complete enabled packages to an isolated conversation.
// Content-addressed directories remain immutable for the conversation's lifetime.
func (l Library) NativeSnapshot(destination string) ([]string, string, error) {
	release, err := l.lock()
	if err != nil {
		return nil, "", err
	}
	defer release()
	c, err := l.config()
	if err != nil {
		return nil, "", err
	}
	packs, err := l.packages(c)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.New()
	total := 0
	for _, p := range packs {
		if !p.info.Enabled {
			continue
		}
		fmt.Fprintf(hash, "%s\x00", p.info.Name)
		for _, f := range p.files {
			total += len(f.data)
			if total > 64<<20 {
				return nil, "", errors.New("启用的 Skill 总大小不能超过 64 MB")
			}
			fmt.Fprintf(hash, "%s\x00%d\x00", f.name, len(f.data))
			hash.Write(f.data)
		}
	}
	revision := hex.EncodeToString(hash.Sum(nil))
	directory := filepath.Join(destination, revision)
	paths := []string{}
	for _, p := range packs {
		if !p.info.Enabled {
			continue
		}
		path := filepath.Join(directory, p.info.Name)
		paths = append(paths, path)
	}
	if _, err = os.Stat(directory); err == nil {
		return paths, revision, nil
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return nil, "", err
	}
	stage, err := os.MkdirTemp(destination, ".snapshot-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(stage)
	for _, p := range packs {
		if p.info.Enabled {
			if err = writePackage(filepath.Join(stage, p.info.Name), p); err != nil {
				return nil, "", err
			}
		}
	}
	if err = os.Rename(stage, directory); err != nil {
		return nil, "", err
	}
	return paths, revision, nil
}
