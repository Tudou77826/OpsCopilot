package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsPathAllowed(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefixes []string
		want     bool
	}{
		{
			name:     "exact match after clean",
			path:     "/var/log",
			prefixes: []string{"/var/log/"},
			want:     true, // /var/log matches /var/log (trailing slash cleaned)
		},
		{
			name:     "prefix match with trailing slash",
			path:     "/var/log/nginx/error.log",
			prefixes: []string{"/var/log/"},
			want:     true,
		},
		{
			name:     "prefix match without trailing slash in path",
			path:     "/etc/passwd",
			prefixes: []string{"/etc/"},
			want:     true,
		},
		{
			name:     "no match",
			path:     "/usr/bin/evil",
			prefixes: []string{"/var/log/", "/etc/"},
			want:     false,
		},
		{
			name:     "empty prefixes",
			path:     "/var/log/test",
			prefixes: []string{},
			want:     false,
		},
		{
			name:     "local staging dir",
			path:     "/tmp/opscopilot-mcp/error.log",
			prefixes: []string{"/tmp/opscopilot-mcp/"},
			want:     true,
		},
		{
			name:     "path traversal attempt",
			path:     "/tmp/opscopilot-mcp/../../../etc/shadow",
			prefixes: []string{"/tmp/opscopilot-mcp/"},
			want:     false, // Cleaned path is /etc/shadow, not under prefix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPathAllowed(tt.path, tt.prefixes)
			if got != tt.want {
				t.Errorf("isPathAllowed(%q, %v) = %v, want %v", tt.path, tt.prefixes, got, tt.want)
			}
		})
	}
}

func TestPathMatches(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		{
			name:    "exact match",
			path:    "/etc/shadow",
			pattern: "/etc/shadow",
			want:    true,
		},
		{
			name:    "prefix match directory",
			path:    "/etc/ssh/ssh_host_rsa_key",
			pattern: "/etc/ssh/",
			want:    true,
		},
		{
			name:    "glob with star segment",
			path:    "/home/alice/.ssh/id_rsa",
			pattern: "/home/*/.ssh/id_*",
			want:    true,
		},
		{
			name:    "glob different user",
			path:    "/home/bob/.ssh/id_ed25519",
			pattern: "/home/*/.ssh/id_*",
			want:    true,
		},
		{
			name:    "glob no match",
			path:    "/home/alice/.ssh/authorized_keys",
			pattern: "/home/*/.ssh/id_*",
			want:    false,
		},
		{
			name:    "root ssh key",
			path:    "/root/.ssh/id_rsa",
			pattern: "/root/.ssh/",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathMatches(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("pathMatches(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestSimpleGlob(t *testing.T) {
	tests := []struct {
		s       string
		pattern string
		want    bool
	}{
		{"id_rsa", "id_*", true},
		{"id_ed25519", "id_*", true},
		{"authorized_keys", "id_*", false},
		{"config", "id_*", false},
	}

	for _, tt := range tests {
		got := simpleGlob(tt.s, tt.pattern)
		if got != tt.want {
			t.Errorf("simpleGlob(%q, %q) = %v, want %v", tt.s, tt.pattern, got, tt.want)
		}
	}
}

func TestFileAccessChecker_CheckRead(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "file_access.json")

	checker, err := NewFileAccessChecker(configPath)
	if err != nil {
		t.Fatalf("NewFileAccessChecker() error = %v", err)
	}

	// 验证默认配置已加载
	cfg := checker.GetConfig()
	if cfg.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", cfg.Version)
	}
	if len(cfg.Policies) != 1 {
		t.Fatalf("Expected 1 policy, got %d", len(cfg.Policies))
	}

	policy := cfg.Policies[0]
	if policy.ID != "default" {
		t.Errorf("Expected policy ID 'default', got %s", policy.ID)
	}

	tests := []struct {
		name       string
		remotePath string
		localPath  string
		serverIP   string
		fileSize   int64
		wantAllow  bool
		wantReason string
	}{
		{
			name:       "read log file allowed",
			remotePath: "/var/log/nginx/error.log",
			localPath:  "/tmp/opscopilot-mcp/error.log",
			serverIP:   "192.168.1.100",
			fileSize:   1024,
			wantAllow:  true,
		},
		{
			name:       "read etc file allowed",
			remotePath: "/etc/nginx/nginx.conf",
			localPath:  "/tmp/opscopilot-mcp/nginx.conf",
			serverIP:   "10.0.0.1",
			fileSize:   4096,
			wantAllow:  true,
		},
		{
			name:       "read /etc/shadow denied",
			remotePath: "/etc/shadow",
			localPath:  "/tmp/opscopilot-mcp/shadow",
			serverIP:   "192.168.1.100",
			fileSize:   1024,
			wantAllow:  false,
			wantReason: "拒绝列表",
		},
		{
			name:       "read ssh host key denied",
			remotePath: "/etc/ssh/ssh_host_rsa_key",
			localPath:  "/tmp/opscopilot-mcp/key",
			serverIP:   "192.168.1.100",
			fileSize:   512,
			wantAllow:  false,
			wantReason: "拒绝列表",
		},
		{
			name:       "local path outside staging dir denied",
			remotePath: "/var/log/test.log",
			localPath:  "/home/user/test.log",
			serverIP:   "192.168.1.100",
			fileSize:   1024,
			wantAllow:  false,
			wantReason: "不在允许的目录中",
		},
		{
			name:       "file too large denied",
			remotePath: "/var/log/huge.log",
			localPath:  "/tmp/opscopilot-mcp/huge.log",
			serverIP:   "192.168.1.100",
			fileSize:   20 * 1024 * 1024, // 20MB > 10MB limit
			wantAllow:  false,
			wantReason: "超过下载上限",
		},
		{
			name:       "path not in read paths denied",
			remotePath: "/usr/bin/evil",
			localPath:  "/tmp/opscopilot-mcp/evil",
			serverIP:   "192.168.1.100",
			fileSize:   100,
			wantAllow:  false,
			wantReason: "不在允许读取的路径中",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckRead(tt.remotePath, tt.localPath, tt.serverIP, tt.fileSize)
			if result.Allowed != tt.wantAllow {
				t.Errorf("CheckRead() Allowed = %v, want %v, reason: %s", result.Allowed, tt.wantAllow, result.Reason)
			}
			if !tt.wantAllow && tt.wantReason != "" {
				if !containsString(result.Reason, tt.wantReason) {
					t.Errorf("CheckRead() Reason = %q, want to contain %q", result.Reason, tt.wantReason)
				}
			}
		})
	}
}

func TestFileAccessChecker_CheckWrite(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "file_access.json")

	checker, err := NewFileAccessChecker(configPath)
	if err != nil {
		t.Fatalf("NewFileAccessChecker() error = %v", err)
	}

	tests := []struct {
		name       string
		remotePath string
		localPath  string
		serverIP   string
		fileSize   int64
		wantAllow  bool
		wantReason string
	}{
		{
			name:       "write denied by default (no write paths)",
			remotePath: "/tmp/fix.sh",
			localPath:  "/tmp/opscopilot-mcp/fix.sh",
			serverIP:   "192.168.1.100",
			fileSize:   1024,
			wantAllow:  false,
			wantReason: "不在允许写入的路径中",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckWrite(tt.remotePath, tt.localPath, tt.serverIP, tt.fileSize)
			if result.Allowed != tt.wantAllow {
				t.Errorf("CheckWrite() Allowed = %v, want %v, reason: %s", result.Allowed, tt.wantAllow, result.Reason)
			}
			if !tt.wantAllow && tt.wantReason != "" {
				if !containsString(result.Reason, tt.wantReason) {
					t.Errorf("CheckWrite() Reason = %q, want to contain %q", result.Reason, tt.wantReason)
				}
			}
		})
	}
}

func TestFileAccessChecker_WriteWithConfiguredPaths(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "file_access.json")

	checker, err := NewFileAccessChecker(configPath)
	if err != nil {
		t.Fatalf("NewFileAccessChecker() error = %v", err)
	}

	// 配置写入路径
	cfg := checker.GetConfig()
	cfg.Policies[0].WritePaths = []string{"/tmp/", "/opt/app/"}
	if err := checker.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	tests := []struct {
		name       string
		remotePath string
		localPath  string
		serverIP   string
		fileSize   int64
		wantAllow  bool
	}{
		{
			name:       "write to /tmp allowed",
			remotePath: "/tmp/fix.sh",
			localPath:  "/tmp/opscopilot-mcp/fix.sh",
			serverIP:   "192.168.1.100",
			fileSize:   1024,
			wantAllow:  true,
		},
		{
			name:       "write to /opt/app allowed",
			remotePath: "/opt/app/config.yml",
			localPath:  "/tmp/opscopilot-mcp/config.yml",
			serverIP:   "192.168.1.100",
			fileSize:   2048,
			wantAllow:  true,
		},
		{
			name:       "write to /etc denied (no write path)",
			remotePath: "/etc/config.yml",
			localPath:  "/tmp/opscopilot-mcp/config.yml",
			serverIP:   "192.168.1.100",
			fileSize:   100,
			wantAllow:  false,
		},
		{
			name:       "write file too large",
			remotePath: "/tmp/big.tar.gz",
			localPath:  "/tmp/opscopilot-mcp/big.tar.gz",
			serverIP:   "192.168.1.100",
			fileSize:   10 * 1024 * 1024, // 10MB > 5MB limit
			wantAllow:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.CheckWrite(tt.remotePath, tt.localPath, tt.serverIP, tt.fileSize)
			if result.Allowed != tt.wantAllow {
				t.Errorf("CheckWrite() Allowed = %v, want %v, reason: %s", result.Allowed, tt.wantAllow, result.Reason)
			}
		})
	}
}

func TestFileAccessChecker_SaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "file_access.json")

	// 创建并保存配置
	checker, err := NewFileAccessChecker(configPath)
	if err != nil {
		t.Fatalf("NewFileAccessChecker() error = %v", err)
	}

	cfg := checker.GetConfig()
	cfg.Policies[0].WritePaths = []string{"/tmp/test/"}
	if err := checker.UpdateConfig(cfg); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	// 重新加载
	checker2, err := NewFileAccessChecker(configPath)
	if err != nil {
		t.Fatalf("NewFileAccessChecker() second load error = %v", err)
	}

	cfg2 := checker2.GetConfig()
	if len(cfg2.Policies[0].WritePaths) != 1 || cfg2.Policies[0].WritePaths[0] != "/tmp/test/" {
		t.Errorf("Reloaded config WritePaths = %v, want [/tmp/test/]", cfg2.Policies[0].WritePaths)
	}
}

func TestEnsureLocalStagingDir(t *testing.T) {
	tmpDir := t.TempDir()
	stagingDir := filepath.Join(tmpDir, "opscopilot-mcp")

	if err := EnsureLocalStagingDir(stagingDir); err != nil {
		t.Fatalf("EnsureLocalStagingDir() error = %v", err)
	}

	info, err := os.Stat(stagingDir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected directory to exist")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
