package dbworkspace

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"opscopilot/pkg/filetxn"
)

type JDBCDriver struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ClassName   string    `json:"className"`
	URLTemplate string    `json:"urlTemplate"`
	Dialect     string    `json:"dialect"`
	Version     string    `json:"version"`
	Jars        []JDBCJar `json:"jars"`
}
type JDBCJar struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}
type JDBCImport struct {
	Name        string   `json:"name"`
	ClassName   string   `json:"className"`
	URLTemplate string   `json:"urlTemplate"`
	Dialect     string   `json:"dialect"`
	Paths       []string `json:"paths"`
}
type JDBCStatus struct {
	Available bool         `json:"available"`
	Message   string       `json:"message"`
	Drivers   []JDBCDriver `json:"drivers"`
}

const zenithURLTemplate = "jdbc:zenith:@{host}:{port}"

func normalizeJDBC(d *JDBCDriver) {
	if d.ClassName == "com.huawei.gauss.jdbc.ZenithDriver" {
		d.URLTemplate = zenithURLTemplate
	}
}

func (s *Service) driverDir() string { return filepath.Join(filepath.Dir(s.path), "jdbc-drivers") }
func (s *Service) JDBCStatus() (JDBCStatus, error) {
	out := JDBCStatus{Drivers: []JDBCDriver{}}
	if _, _, err := s.jdbcRuntime(); err != nil {
		out.Message = err.Error()
	} else {
		out.Available = true
		out.Message = "Java 运行组件已就绪；仅 JDBC 连接时启动"
	}
	entries, err := os.ReadDir(s.driverDir())
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := uuid.Parse(entry.Name()); err != nil {
			continue
		}
		d, err := s.readDriver(entry.Name())
		if err != nil {
			if os.IsNotExist(err) {
				continue
			} // An import has not committed its manifest yet.
			return out, err
		}
		out.Drivers = append(out.Drivers, d)
	}
	return out, nil
}
func (s *Service) jdbcRuntime() (string, string, error) {
	root := s.JDBCRuntimeDir
	if root == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", "", err
		}
		root = filepath.Join(filepath.Dir(exe), "jdbc-runtime")
	}
	java := filepath.Join(root, "runtime", "bin", "java")
	if os.PathSeparator == '\\' {
		java += ".exe"
	}
	helper := filepath.Join(root, "jdbc-helper.jar")
	for _, path := range []string{java, helper} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", "", fmt.Errorf("未安装 JDBC 运行组件，请将发行包中的 jdbc-runtime 放到程序旁")
		}
	}
	return java, helper, nil
}
func (s *Service) readDriver(id string) (JDBCDriver, error) {
	var d JDBCDriver
	if _, err := uuid.Parse(id); err != nil {
		return d, fmt.Errorf("JDBC 驱动标识无效")
	}
	b, err := os.ReadFile(filepath.Join(s.driverDir(), id, "driver.json"))
	if err != nil {
		return d, fmt.Errorf("读取 JDBC 驱动失败: %w", err)
	}
	if len(b) > 65536 {
		return d, fmt.Errorf("驱动配置过大")
	}
	if err = json.Unmarshal(b, &d); err != nil {
		return d, err
	}
	if d.ID != id {
		return d, fmt.Errorf("驱动标识不匹配")
	}
	normalizeJDBC(&d)
	return d, validateJDBC(d)
}

var jdbcClass = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*(\.[A-Za-z_$][A-Za-z0-9_$]*)+$`)

func validateJDBC(d JDBCDriver) error {
	if strings.TrimSpace(d.Name) == "" || len(d.Name) > 200 || len(d.ClassName) > 500 || !jdbcClass.MatchString(d.ClassName) {
		return fmt.Errorf("请填写驱动名称和完整驱动类名")
	}
	if d.Dialect != "mysql" && d.Dialect != "generic" {
		return fmt.Errorf("不支持的 JDBC 方言")
	}
	if !strings.HasPrefix(d.URLTemplate, "jdbc:") || len(d.URLTemplate) > 2048 || strings.ContainsAny(d.URLTemplate, "\r\n\x00") {
		return fmt.Errorf("JDBC URL 模板无效")
	}
	if !strings.Contains(d.URLTemplate, "{host}") || !strings.Contains(d.URLTemplate, "{port}") {
		return fmt.Errorf("URL 模板需要包含 {host} 和 {port}")
	}
	if d.Dialect == "mysql" && d.URLTemplate != "jdbc:mysql://{host}:{port}/{database}" {
		return fmt.Errorf("MySQL 请使用标准 URL 模板 jdbc:mysql://{host}:{port}/{database}，连接属性由程序管理")
	}
	// Credentials are separate keyring-backed properties, never URL parameters.
	lower := strings.ToLower(d.URLTemplate)
	if strings.Contains(lower, "password") || strings.Contains(lower, "user=") {
		return fmt.Errorf("请勿在 URL 模板中填写账号密码")
	}
	if len(d.Jars) == 0 || len(d.Jars) > 8 {
		return fmt.Errorf("请选择 1–8 个 JDBC JAR 文件")
	}
	for _, j := range d.Jars {
		if filepath.Base(j.Name) != j.Name || !strings.HasSuffix(strings.ToLower(j.Name), ".jar") || len(j.SHA256) != 64 {
			return fmt.Errorf("驱动文件信息无效")
		}
	}
	return nil
}
func (s *Service) ImportJDBC(ctx context.Context, in JDBCImport) (JDBCDriver, error) {
	d := JDBCDriver{ID: uuid.NewString(), Name: strings.TrimSpace(in.Name), ClassName: strings.TrimSpace(in.ClassName), URLTemplate: strings.TrimSpace(in.URLTemplate), Dialect: in.Dialect, Jars: []JDBCJar{}}
	normalizeJDBC(&d)
	if len(in.Paths) == 0 || len(in.Paths) > 8 {
		return d, fmt.Errorf("请选择 1–8 个 JAR 文件")
	}
	dir := filepath.Join(s.driverDir(), d.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return d, err
	}
	success := false
	// dir is always a newly generated UUID beneath the owned driver directory.
	defer func() {
		if !success {
			_ = os.RemoveAll(dir)
		}
	}()
	var total int64
	seen := map[string]bool{}
	for _, path := range in.Paths {
		name := filepath.Base(path)
		key := strings.ToLower(name)
		if seen[key] || !strings.HasSuffix(key, ".jar") {
			return d, fmt.Errorf("请选择不重名的 JAR 文件")
		}
		seen[key] = true
		source, err := os.Open(path)
		if err != nil {
			return d, err
		}
		info, err := source.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			source.Close()
			return d, fmt.Errorf("每个 JAR 应为不超过 64 MiB 的普通文件")
		}
		total += info.Size()
		if total > 128<<20 {
			source.Close()
			return d, fmt.Errorf("驱动文件总大小超过 128 MiB")
		}
		target, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			source.Close()
			return d, err
		}
		hash := sha256.New()
		_, err = io.Copy(io.MultiWriter(target, hash), io.LimitReader(source, 64<<20+1))
		source.Close()
		closeErr := target.Close()
		if err != nil {
			return d, err
		}
		if closeErr != nil {
			return d, closeErr
		}
		archive, err := zip.OpenReader(filepath.Join(dir, name))
		if err != nil {
			return d, fmt.Errorf("不是有效的 JAR 文件: %w", err)
		}
		archive.Close()
		d.Jars = append(d.Jars, JDBCJar{Name: name, SHA256: hex.EncodeToString(hash.Sum(nil))})
	}
	if err := validateJDBC(d); err != nil {
		return d, err
	}
	probe, err := s.startJDBC(ctx, d)
	if err != nil {
		return d, err
	}
	defer probe.close()
	var loaded struct {
		Version string `json:"version"`
	}
	if err = probe.load(ctx, s.driverDir(), d, &loaded); err != nil {
		return d, fmt.Errorf("验证 JDBC 驱动失败: %w", err)
	}
	d.Version = loaded.Version
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return d, err
	}
	if err = filetxn.Write(filepath.Join(dir, "driver.json"), b); err != nil {
		return d, err
	}
	success = true
	return d, nil
}
func (s *Service) DeleteJDBC(id string) error {
	release, err := filetxn.Lock(s.path)
	if err != nil {
		return err
	}
	defer release()
	d, err := s.readDriver(id)
	if err != nil {
		return err
	}
	profiles, err := s.List()
	if err != nil {
		return err
	}
	for _, p := range profiles {
		if p.DriverID == id {
			return fmt.Errorf("驱动仍被连接「%s」使用，请先更换或删除连接", p.Name)
		}
	}
	return os.RemoveAll(filepath.Join(s.driverDir(), d.ID))
}
func jdbcURL(d JDBCDriver, p Profile) string {
	host := p.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return strings.NewReplacer("{host}", host, "{port}", fmt.Sprint(p.Port), "{database}", url.PathEscape(p.Database)).Replace(d.URLTemplate)
}
func (s *Service) adapter(p Profile) (Connector, error) {
	if p.Engine == "jdbc" {
		d, err := s.readDriver(p.DriverID)
		if err != nil {
			return nil, err
		}
		if d.Dialect == "generic" && p.TLS != "off" {
			return nil, fmt.Errorf("通用 JDBC 的 TLS 参数需要厂商适配，暂不支持此选项")
		}
		if d.Dialect == "generic" && p.ReadOnly {
			return nil, fmt.Errorf("通用 JDBC 的只读事务尚未适配；请使用数据库只读账号，并取消程序的只读执行设置")
		}
		return JDBC{service: s, definition: d}, nil
	}
	a := s.connectors[p.Engine]
	if a == nil {
		return nil, fmt.Errorf("请先导入 JDBC 驱动，并在连接设置中选择该驱动；内置连接方式已移除，原配置和凭据仍保留")
	}
	return a, nil
}
