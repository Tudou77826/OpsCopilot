package dbworkspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"opscopilot/pkg/filetxn"
	"opscopilot/pkg/secretstore"
)

const credentialService = "OpsCopilot-Database"

type storedProfile struct {
	Profile
	SecretID string `json:"secretId"`
}
type profileFile struct {
	Version  int             `json:"version"`
	Profiles []storedProfile `json:"profiles"`
	Groups   []string        `json:"groups,omitempty"`
}

func (s *Service) readProfiles() (profileFile, error) {
	b, err := filetxn.Read(s.path)
	if err != nil {
		return profileFile{}, fmt.Errorf("读取数据库连接失败: %w", err)
	}
	f := profileFile{Version: 1, Profiles: []storedProfile{}}
	if len(b) > 0 {
		if err = json.Unmarshal(b, &f); err != nil {
			return f, fmt.Errorf("解析数据库连接失败: %w", err)
		}
		if f.Version != 1 {
			return f, fmt.Errorf("数据库连接文件版本不兼容")
		}
	}
	// Older files only stored the group on each connection. Include those names
	// in the catalog so empty groups survive subsequent moves and deletions.
	seen := make(map[string]bool)
	groups := make([]string, 0, len(f.Groups))
	for _, name := range f.Groups {
		if name != "" && !seen[name] {
			groups = append(groups, name)
			seen[name] = true
		}
	}
	for _, p := range f.Profiles {
		if p.Group != "" && !seen[p.Group] {
			groups = append(groups, p.Group)
			seen[p.Group] = true
		}
	}
	f.Groups = groups
	return f, nil
}

func (s *Service) List() ([]Profile, error) {
	f, err := s.readProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(f.Profiles))
	for _, p := range f.Profiles {
		p.PasswordSet = p.SecretID != ""
		out = append(out, p.Profile)
	}
	return out, nil
}

func validateProfile(p *Profile) error {
	p.Name, p.Host, p.User = strings.TrimSpace(p.Name), strings.TrimSpace(p.Host), strings.TrimSpace(p.User)
	p.Environment, p.Database = strings.TrimSpace(p.Environment), strings.TrimSpace(p.Database)
	p.Group = strings.TrimSpace(p.Group)
	if len([]rune(p.Group)) > 100 || strings.ContainsAny(p.Group, "\x00\r\n") {
		return fmt.Errorf("分组名称应在 100 个字以内且不能包含换行")
	}
	if p.Engine == "" {
		p.Engine = "jdbc"
	}
	if p.TLS == "" {
		p.TLS = "off"
	}
	if p.GatewayMode == "" {
		p.GatewayMode = GatewayModeAuto
	}
	if p.GatewayMode != GatewayModeAuto && p.GatewayMode != GatewayModeForward && p.GatewayMode != GatewayModeCommand {
		return fmt.Errorf("不支持的 SSH 通道模式")
	}
	if p.GatewayMode == GatewayModeCommand && p.GatewayID == "" {
		return fmt.Errorf("远程命令兼容模式需要选择 SSH 连接")
	}
	if p.Port == 0 {
		if p.Engine == "mysql" {
			p.Port = 3306
		} else {
			return fmt.Errorf("请填写数据库端口")
		}
	}
	if p.Host == "" || p.User == "" {
		return fmt.Errorf("请填写数据库地址和用户名")
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("端口应在 1–65535 之间")
	}
	if p.TLS != "off" && p.TLS != "required" {
		return fmt.Errorf("不支持的 TLS 配置")
	}
	if strings.ContainsAny(p.Host, "\x00\r\n/\\") || len(p.Host) > 253 {
		return fmt.Errorf("数据库地址格式无效")
	}
	p.Host = strings.Trim(p.Host, "[]")
	if strings.Contains(p.Host, ":") && net.ParseIP(p.Host) == nil {
		return fmt.Errorf("地址中不要填写端口")
	}
	if p.Name == "" {
		p.Name = net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
		if p.Database != "" {
			p.Name = p.Database + " @ " + p.Name
		}
	}
	if len(p.Name) > 500 {
		return fmt.Errorf("连接名称过长")
	}
	return nil
}

func (s *Service) Save(in ProfileInput) (Profile, error) {
	p := in.Profile
	if err := validateProfile(&p); err != nil {
		return Profile{}, err
	}
	if in.Password != nil && len(*in.Password) > 8192 {
		return Profile{}, fmt.Errorf("密码过长")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return Profile{}, err
	}
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return Profile{}, err
	}
	defer release()
	// Serialize reference creation with driver removal under the profile file lock.
	if _, err := s.adapter(p); err != nil {
		return Profile{}, err
	}
	f, err := s.readProfiles()
	if err != nil {
		return Profile{}, err
	}
	index := -1
	for i := range f.Profiles {
		if f.Profiles[i].ID == p.ID {
			index = i
			break
		}
	}
	if p.ID != "" && index < 0 {
		return Profile{}, fmt.Errorf("数据库连接已被删除，请刷新")
	}
	oldSecret := ""
	if index >= 0 {
		if p.Revision != f.Profiles[index].Revision {
			return Profile{}, fmt.Errorf("连接已被修改，请刷新后重试")
		}
		oldSecret = f.Profiles[index].SecretID
	} else {
		p.ID = uuid.NewString()
	}
	p.Revision++
	nextSecret := oldSecret
	if in.Password != nil {
		nextSecret = ""
		if *in.Password != "" {
			nextSecret = uuid.NewString()
			if err = s.secrets.Set(credentialService, nextSecret, *in.Password); err != nil {
				return Profile{}, fmt.Errorf("保存数据库凭据失败: %w", err)
			}
		}
	}
	p.PasswordSet = nextSecret != ""
	if p.Group != "" && !hasGroup(f.Groups, p.Group) {
		f.Groups = append(f.Groups, p.Group)
	}
	next := storedProfile{Profile: p, SecretID: nextSecret}
	if index < 0 {
		f.Profiles = append(f.Profiles, next)
	} else {
		f.Profiles[index] = next
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err == nil {
		err = filetxn.Write(s.path, b)
	}
	if err != nil {
		if nextSecret != "" && nextSecret != oldSecret {
			_ = s.secrets.Delete(credentialService, nextSecret)
		}
		return Profile{}, fmt.Errorf("保存数据库连接失败: %w", err)
	}
	if oldSecret != "" && oldSecret != nextSecret {
		_ = s.secrets.Delete(credentialService, oldSecret)
	}
	return p, nil
}

func (s *Service) Delete(id string, revision int) error {
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return err
	}
	defer release()
	f, err := s.readProfiles()
	if err != nil {
		return err
	}
	for i, p := range f.Profiles {
		if p.ID != id {
			continue
		}
		if p.Revision != revision {
			return fmt.Errorf("连接已被修改，请刷新后重试")
		}
		f.Profiles = append(f.Profiles[:i], f.Profiles[i+1:]...)
		b, e := json.MarshalIndent(f, "", "  ")
		if e != nil {
			return e
		}
		if e = filetxn.Write(s.path, b); e != nil {
			return e
		}
		if p.SecretID != "" {
			_ = s.secrets.Delete(credentialService, p.SecretID)
		}
		s.disconnectProfile(id)
		return nil
	}
	return fmt.Errorf("数据库连接不存在")
}

func (s *Service) profile(id string) (Profile, string, error) {
	f, err := s.readProfiles()
	if err != nil {
		return Profile{}, "", err
	}
	for _, p := range f.Profiles {
		if p.ID != id {
			continue
		}
		password := ""
		if p.SecretID != "" {
			password, err = s.secrets.Get(credentialService, p.SecretID)
			if errors.Is(err, secretstore.ErrNotFound) {
				return Profile{}, "", fmt.Errorf("数据库凭据丢失，请编辑连接重新填写密码")
			}
			if err != nil {
				return Profile{}, "", fmt.Errorf("读取数据库凭据失败: %w", err)
			}
		}
		return p.Profile, password, nil
	}
	return Profile{}, "", fmt.Errorf("数据库连接不存在")
}
