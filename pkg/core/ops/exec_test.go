package ops

import (
	"strings"
	"testing"
)

// TestBuildSudoCommand_BasicFormat 验证基本拼接格式：echo 'pwd' | su -c 'cmd' -
func TestBuildSudoCommand_BasicFormat(t *testing.T) {
	got := buildSudoCommand("ls -la", "s3cret")
	want := "echo 's3cret' | su -c 'ls -la' -"
	if got != want {
		t.Errorf("buildSudoCommand basic = %q, want %q", got, want)
	}
}

// TestBuildSudoCommand_EmptyPassword 空密码仍会拼接（调用方负责判断是否传入）
func TestBuildSudoCommand_EmptyPassword(t *testing.T) {
	got := buildSudoCommand("whoami", "")
	if !strings.Contains(got, "echo ''") {
		t.Errorf("empty password should produce empty echo, got %q", got)
	}
}

// TestBuildSudoCommand_SingleQuoteInCommand 命令含单引号时必须转义为 '\''，安全嵌入
func TestBuildSudoCommand_SingleQuoteInCommand(t *testing.T) {
	got := buildSudoCommand("grep 'error' /var/log/syslog", "pwd")

	if !strings.Contains(got, `grep '\''error'\''`) {
		t.Errorf("single quote not properly escaped, got %q", got)
	}
	if !strings.HasPrefix(got, "echo 'pwd' | su -c '") {
		t.Errorf("password wrapper broken, got %q", got)
	}
}

// TestBuildSudoCommand_PasswordWithSpecialChars 密码含特殊字符时直接嵌入单引号内
func TestBuildSudoCommand_PasswordWithSpecialChars(t *testing.T) {
	got := buildSudoCommand("ls", "P@ss!w0rd#$%")
	if !strings.HasPrefix(got, "echo 'P@ss!w0rd#$%' | su -c 'ls' -") {
		t.Errorf("password with special chars not embedded correctly, got %q", got)
	}
}

// TestBuildSudoCommand_DashPreserved 尾部的 ' -'（su 的目标用户）必须保留
func TestBuildSudoCommand_DashPreserved(t *testing.T) {
	got := buildSudoCommand("id", "x")
	if !strings.HasSuffix(got, "' -") {
		t.Errorf("trailing ' -' (su target) must be preserved, got %q", got)
	}
}
