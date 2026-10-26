// Package desktopruntime installs the desktop's bundled JDBC helper and Java.
// Versions are immutable and selected by the hash compiled into the executable;
// an older executable never accidentally starts a newer helper.
package desktopruntime

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/filetxn"
)

const BundleName = "jdbc-runtime.bundle"
const receipt = ".bundle-ready"
const maxExpanded = 1024 * 1024 * 1024

// ExpectedSHA256 is injected by the desktop release build. Empty means an
// unmanaged development/server build, which retains the legacy runtime layout.
var ExpectedSHA256 string
var ready sync.Map

func managedDir(appDir, digest string) string {
	return filepath.Join(appDir, "jdbc-runtime", "managed", digest)
}

// Resolve never falls back to a different version for a managed executable.
func Resolve(appDir string) (string, error) {
	if ExpectedSHA256 == "" {
		return filepath.Join(appDir, "jdbc-runtime"), nil
	}
	if !validDigest(ExpectedSHA256) {
		return "", fmt.Errorf("数据库组件版本标识无效，请重新下载完整安装包")
	}
	root := managedDir(appDir, ExpectedSHA256)
	if _, ok := ready.Load(root); !ok {
		return "", fmt.Errorf("数据库组件尚未就绪，请查看【设置－关于】中的准备状态")
	}
	b, err := os.ReadFile(filepath.Join(root, receipt))
	if err != nil || string(b) != ExpectedSHA256 {
		return "", fmt.Errorf("数据库组件尚未就绪，请查看【设置－关于】中的准备状态")
	}
	return root, nil
}

func validDigest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && strings.ToLower(s) == s
}

// Ensure retains both the legacy runtime and all previously committed versions.
// Only a fully validated staging directory is renamed into the selected path.
// A crash or failed extraction leaves no ready marker and is retried next time.
func Ensure(ctx context.Context, appDir, digest string, progress func(string)) error {
	return ensure(ctx, appDir, digest, progress, probeJava)
}

func ensure(ctx context.Context, appDir, digest string, progress func(string), probe func(context.Context, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if digest == "" {
		return nil
	}
	if !validDigest(digest) {
		return fmt.Errorf("数据库组件版本标识无效")
	}
	if progress == nil {
		progress = func(string) {}
	}
	ready.Delete(managedDir(appDir, digest))
	lockCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	unlock, err := filetxn.LockContext(lockCtx, filepath.Join(appDir, ".jdbc-runtime-install"))
	if err != nil {
		return fmt.Errorf("其他实例正在准备数据库组件，请稍后重试: %w", err)
	}
	defer unlock()
	for _, p := range []string{appDir, filepath.Join(appDir, "jdbc-runtime"), filepath.Join(appDir, "jdbc-runtime", "managed")} {
		if info, e := os.Lstat(p); e == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return fmt.Errorf("数据库组件安装目录必须是普通目录")
		} else if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	progress("正在校验数据库组件…")
	archive, err := openBundle(filepath.Join(appDir, BundleName), digest)
	if err != nil {
		return err
	}
	defer archive.Close()
	target := managedDir(appDir, digest)
	if err = verifyInstalled(ctx, archive.File, target); err == nil {
		ready.Store(target, true)
		return nil
	}
	parent := filepath.Dir(target)
	if err = os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("创建数据库组件目录失败: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".prepare-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // The path is generated directly beneath our managed directory.
	progress("正在准备数据库组件…")
	for _, f := range archive.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = extractFile(f, stage); err != nil {
			return fmt.Errorf("解压数据库组件失败: %w", err)
		}
	}
	progress("正在检查 Java 运行环境…")
	if err = probe(ctx, stage); err != nil {
		return fmt.Errorf("数据库组件运行检查失败: %w", err)
	}
	if err = os.WriteFile(filepath.Join(stage, receipt), []byte(digest), 0644); err != nil {
		return err
	}
	// A damaged version is kept for diagnosis; never overwrite files of a running
	// Java process. A fresh process will select the committed replacement.
	if _, err = os.Lstat(target); err == nil {
		quarantine := target + ".damaged-" + fmt.Sprint(time.Now().UnixNano())
		if err = os.Rename(target, quarantine); err != nil {
			return fmt.Errorf("隔离损坏组件失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(stage, target); err != nil {
		return fmt.Errorf("启用数据库组件失败: %w", err)
	}
	ready.Store(target, true)
	return nil
}

type checkedBundle struct {
	*zip.Reader
	source *os.File
}

func (b *checkedBundle) Close() error { return b.source.Close() }

func openBundle(path, digest string) (*checkedBundle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("缺少数据库运行组件，请重新解压完整安装包: %w", err)
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("读取数据库组件失败: %w", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		f.Close()
		return nil, fmt.Errorf("数据库组件与主程序不匹配或文件损坏，请重新下载完整安装包")
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	z, err := zip.NewReader(f, info.Size())
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("打开数据库组件失败: %w", err)
	}
	if err = validateEntries(z.File); err != nil {
		f.Close()
		return nil, err
	}
	return &checkedBundle{Reader: z, source: f}, nil
}

func validateEntries(files []*zip.File) error {
	seen := map[string]bool{}
	var total uint64
	for _, f := range files {
		n := f.Name
		// Check Windows path rules on every platform, including ADS/device names.
		if n == "" || strings.ContainsAny(n, "\\:") || strings.HasPrefix(n, "/") || f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("数据库组件包含不安全的路径")
		}
		parts := strings.Split(strings.TrimSuffix(n, "/"), "/")
		for _, p := range parts {
			base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
			if p == "" || p == "." || p == ".." || strings.TrimRight(p, ". ") != p || base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
				return fmt.Errorf("数据库组件包含不安全的路径")
			}
		}
		key := strings.ToLower(strings.TrimSuffix(n, "/"))
		if seen[key] || key == receipt {
			return fmt.Errorf("数据库组件包含重复或保留路径")
		}
		seen[key] = true
		if f.UncompressedSize64 > maxExpanded || total > maxExpanded-f.UncompressedSize64 {
			return fmt.Errorf("数据库组件解压大小超限")
		}
		total += f.UncompressedSize64
	}
	for _, required := range []string{"jdbc-helper.jar", "runtime/bin/java.exe", "runtime/lib/modules"} {
		if !seen[required] {
			return fmt.Errorf("数据库组件缺少必需文件: %s", required)
		}
	}
	return nil
}

func extractFile(f *zip.File, root string) error {
	p := filepath.Join(root, filepath.FromSlash(f.Name))
	if f.FileInfo().IsDir() {
		return os.MkdirAll(p, 0755)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = io.CopyN(out, in, int64(f.UncompressedSize64))
	if err == nil {
		var extra [1]byte
		n, e := in.Read(extra[:])
		if n != 0 || e != io.EOF {
			err = fmt.Errorf("数据库组件校验失败")
		}
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func verifyInstalled(ctx context.Context, files []*zip.File, root string) error {
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		// Reject links in the entire ancestry, not just the leaf.
		for q := p; ; q = filepath.Dir(q) {
			i, err := os.Lstat(q)
			if err != nil {
				return err
			}
			if i.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("数据库组件路径不能是链接")
			}
			if q == root {
				break
			}
		}
		if f.FileInfo().IsDir() {
			continue
		}
		actual, err := os.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, actual)
		actual.Close()
		if err != nil {
			return err
		}
		expected, err := f.Open()
		if err != nil {
			return err
		}
		eh := sha256.New()
		_, err = io.Copy(eh, expected)
		expected.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != hex.EncodeToString(eh.Sum(nil)) {
			return fmt.Errorf("数据库组件文件校验失败: %s", f.Name)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, receipt))
	if err != nil || string(b) != filepath.Base(root) {
		return fmt.Errorf("数据库组件未提交")
	}
	return nil
}

func probeJava(ctx context.Context, root string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime", "bin", "java.exe"), "-version")
	hideWindow(cmd)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Java 无法启动，请检查组件完整性和目录执行权限: %w", err)
	}
	if !strings.Contains(string(b), `version "21.`) {
		return fmt.Errorf("需要随包提供的 Java 21")
	}
	// Load the helper and its shaded dependencies without connecting to a DB.
	cmd = exec.CommandContext(ctx, filepath.Join(root, "runtime", "bin", "java.exe"), "-jar", filepath.Join(root, "jdbc-helper.jar"))
	hideWindow(cmd)
	if b, err = cmd.CombinedOutput(); err != nil {
		// JVM output can echo JAVA_TOOL_OPTIONS containing user secrets.
		return fmt.Errorf("JDBC helper 无法启动，请检查组件完整性: %w", err)
	}
	return nil
}
