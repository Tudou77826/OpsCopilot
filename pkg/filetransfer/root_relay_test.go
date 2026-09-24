package filetransfer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestParseFindOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Entry
		hasError bool
	}{
		{
			name: "typical directory listing",
			input: "directory\t4096\t1585405215\t.ssh\n" +
				"regular file\t1234\t1585405215\t.bashrc\n" +
				"regular file\t0\t1585405215\t.profile",
			expected: []Entry{
				{Name: ".ssh", IsDir: true, Size: 4096},
				{Name: ".bashrc", IsDir: false, Size: 1234},
				{Name: ".profile", IsDir: false, Size: 0},
			},
		},
		{
			name: "filters dot entries",
			input: "directory\t4096\t1585405215\t.\n" +
				"directory\t4096\t1585405215\t..",
			expected: []Entry{},
		},
		{
			name:  "empty input",
			input: "",
			// Empty input returns nil (triggers ls fallback)
			expected: nil,
		},
		{
			name:     "non-find format triggers nil",
			input:    "total 32\ndrwxr-xr-x 2 root root 4096 Mar 28 10:30 .ssh",
			expected: nil,
		},
		{
			name: "symbolic link",
			input: "symbolic link\t10\t1585405215\tlink_to_file",
			expected: []Entry{
				{Name: "link_to_file", IsDir: false, Size: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFindOutput(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
				return
			}
			for i, exp := range tt.expected {
				if result[i].Name != exp.Name {
					t.Errorf("entry[%d].Name = %q, want %q", i, result[i].Name, exp.Name)
				}
				if result[i].IsDir != exp.IsDir {
					t.Errorf("entry[%d].IsDir = %v, want %v", i, result[i].IsDir, exp.IsDir)
				}
				if result[i].Size != exp.Size {
					t.Errorf("entry[%d].Size = %d, want %d", i, result[i].Size, exp.Size)
				}
			}
		})
	}
}

func TestParseFindOutput_ModTime(t *testing.T) {
	input := "regular file\t1234\t1709251200\ttestfile.txt"
	result := parseFindOutput(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	expected := time.Unix(1709251200, 0)
	if !result[0].ModTime.Equal(expected) {
		t.Errorf("ModTime = %v, want %v", result[0].ModTime, expected)
	}
}

func TestParseLsOutput(t *testing.T) {
	input := `total 32
drwxr-xr-x  2 root root 4096 2026-03-28 10:30 .ssh
-rw-r--r--  1 root root 1234 2026-03-28 10:30 .bashrc
-rw-r--r--  1 root root  512 2026-03-27 14:20 test.sh
lrwxrwxrwx  1 root root   10 2026-03-28 10:30 link`

	result, err := parseLsOutput(input, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result))
	}

	// Check directory entry
	if result[0].Name != ".ssh" {
		t.Errorf("entry[0].Name = %q, want .ssh", result[0].Name)
	}
	if !result[0].IsDir {
		t.Errorf("entry[0].IsDir = false, want true")
	}
	if result[0].Path != "/root/.ssh" {
		t.Errorf("entry[0].Path = %q, want /root/.ssh", result[0].Path)
	}

	// Check file entry
	if result[1].Name != ".bashrc" {
		t.Errorf("entry[1].Name = %q, want .bashrc", result[1].Name)
	}
	if result[1].IsDir {
		t.Errorf("entry[1].IsDir = true, want false")
	}
	if result[1].Size != 1234 {
		t.Errorf("entry[1].Size = %d, want 1234", result[1].Size)
	}

	// Check symlink treated as non-dir
	if result[3].Name != "link" {
		t.Errorf("entry[3].Name = %q, want link", result[3].Name)
	}
}

func TestParseLsOutput_FiltersDots(t *testing.T) {
	input := `total 8
drwxr-xr-x  2 root root 4096 2026-03-28 10:30 .
drwxr-xr-x  3 root root 4096 2026-03-28 10:30 ..
-rw-r--r--  1 root root  100 2026-03-28 10:30 file.txt`

	result, err := parseLsOutput(input, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 entry (filtering . and ..), got %d", len(result))
	}
	if result[0].Name != "file.txt" {
		t.Errorf("entry[0].Name = %q, want file.txt", result[0].Name)
	}
}

func TestParseLsOutput_Empty(t *testing.T) {
	input := `total 0`
	result, err := parseLsOutput(input, "/root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestParseStatOutput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		filePath  string
		expected  Entry
		hasError  bool
	}{
		{
			name:     "regular file",
			input:    "regular file\t12345\t1709251200",
			filePath: "/etc/passwd",
			expected: Entry{
				Name:    "passwd",
				IsDir:   false,
				Size:    12345,
				Path:    "/etc/passwd",
			},
		},
		{
			name:     "directory",
			input:    "directory\t4096\t1709251200",
			filePath: "/root",
			expected: Entry{
				Name:    "root",
				IsDir:   true,
				Size:    4096,
				Path:    "/root",
			},
		},
		{
			name:     "invalid input",
			input:    "something",
			filePath: "/test",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseStatOutput(tt.input, tt.filePath)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Name != tt.expected.Name {
				t.Errorf("Name = %q, want %q", result.Name, tt.expected.Name)
			}
			if result.IsDir != tt.expected.IsDir {
				t.Errorf("IsDir = %v, want %v", result.IsDir, tt.expected.IsDir)
			}
			if result.Size != tt.expected.Size {
				t.Errorf("Size = %d, want %d", result.Size, tt.expected.Size)
			}
			if result.Path != tt.expected.Path {
				t.Errorf("Path = %q, want %q", result.Path, tt.expected.Path)
			}
		})
	}
}

func TestParseStatOutput_ModTime(t *testing.T) {
	input := "regular file\t100\t1709251200"
	result, err := parseStatOutput(input, "/test/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Unix(1709251200, 0)
	if !result.ModTime.Equal(expected) {
		t.Errorf("ModTime = %v, want %v", result.ModTime, expected)
	}
}

func TestExtractMarked(t *testing.T) {
	tests := []struct {
		name   string
		output string
		marker string
		expect string
	}{
		{
			// 常态：提示符残留 + 回显行在 START 之前，全部丢弃
			name:   "prompt junk and echo before start",
			output: "$ echo __RELAY\"\"_START__; stat 'x' && echo __RELAY\"\"_OK__\r\n__RELAY_START__\r\nregular file\t12\t1700000001\tu\tu\r\n__RELAY_OK__\r\n",
			marker: "__RELAY_OK__",
			expect: "regular file\t12\t1700000001\tu\tu",
		},
		{
			// 单行输出：不再被"丢第一行"的旧逻辑误伤
			name:   "single line output survives",
			output: "junk\r\n__RELAY_START__\r\nresult\r\n__RELAY_OK__\r\n",
			marker: "__RELAY_OK__",
			expect: "result",
		},
		{
			// su 失败后的报错与提示符粘连在输出前
			name:   "su failure junk",
			output: "su: Authentication failure\r\n$ __RELAY_START__\r\n0\r\n__RELAY_OK__\r\n",
			marker: "__RELAY_OK__",
			expect: "0",
		},
		{
			name:   "fail marker",
			output: "$ cmd\r\n__RELAY_START__\r\nsome error\r\n__RELAY_FAIL__\r\n",
			marker: "__RELAY_FAIL__",
			expect: "some error",
		},
		{
			// 兜底：未见 START 时退化为截取终止标记之前的内容
			name:   "no start marker falls back",
			output: "data\r\n__RELAY_OK__",
			marker: "__RELAY_OK__",
			expect: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMarked(tt.output, tt.marker)
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestShellSingleQuoteInRelay(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"/root/file", "'/root/file'"},
		{"", "''"},
		{"/root/file's name", "'/root/file'\\''s name'"},
	}
	for _, tt := range tests {
		result := shellSingleQuote(tt.input)
		if result != tt.expect {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", tt.input, result, tt.expect)
		}
	}
}

func TestPrepareRelayDirPath(t *testing.T) {
	// Test that relay dir path format is correct
	// We can't actually run prepareRelayDir without SSH, but we can test the path construction
	// The UUID is 8 chars, and the path should be /tmp/opscopilot/<uuid>/
	relayDir := defaultRelayBaseDir + "/abcd1234/"
	expected := "/tmp/opscopilot/abcd1234/"
	if relayDir != expected {
		t.Errorf("relayDir = %q, want %q", relayDir, expected)
	}
}

func TestComputeLocalMD5(t *testing.T) {
	// Create a temp file with known content
	tmpFile, err := os.CreateTemp("", "md5test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	content := []byte("hello world")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	hash, err := computeLocalMD5(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Known MD5 of "hello world"
	expected := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	if hash != expected {
		t.Errorf("got %q, want %q", hash, expected)
	}
}

func TestComputeLocalMD5_EmptyFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "md5test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if err := tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	hash, err := computeLocalMD5(tmpFile.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MD5 of empty string
	expected := "d41d8cd98f00b204e9800998ecf8427e"
	if hash != expected {
		t.Errorf("got %q, want %q", hash, expected)
	}
}

func TestComputeLocalMD5_Nonexistent(t *testing.T) {
	_, err := computeLocalMD5("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMaxBase64DirectBytes(t *testing.T) {
	if maxBase64DirectBytes != 10*1024*1024 {
		t.Errorf("maxBase64DirectBytes = %d, want %d", maxBase64DirectBytes, 10*1024*1024)
	}
}

// TestShellTransport_ListStatAsLoginUser 覆盖 SCP 降级场景的 shell 回退：
// loginUser 为空（登录用户身份，无 su）时，List/Stat 走 find/stat 的
// 解析路径。此前这些能力只有 root-relay 消费，登录用户 SCP 会话
// 在 UI 层被迫使用表单；现在面板统一走 FilePane，本测试守住解析链路。
func TestShellTransport_ListStatAsLoginUser(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	// loginUser == "" ⇒ 不 su，直接用当前 shell（SCP 降级的登录用户路径）。
	tr := NewRootRelayTransport(client, "", "")
	defer tr.Close()

	ctx := context.Background()
	entries, err := tr.List(ctx, "/data")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List 条目数: got %d, want 1 (hello.txt)", len(entries))
	}
	if entries[0].Name != "hello.txt" || entries[0].IsDir {
		t.Fatalf("List 结果: %+v", entries[0])
	}

	entry, err := tr.Stat(ctx, "/data/hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "hello.txt" || entry.Size != 12 {
		t.Fatalf("Stat 结果: %+v", entry)
	}
}

// TestShellTransport_SuWithEmptyPasswordFailsFast 守护 v1.10.4 修复：
// 登录用户会话（没有 root 密码）绝不能进入 su 流程——loginUser 非空 +
// 空 rootPassword 的组合必然认证失败，且提示符等待会把每次操作拖住
// 十几秒（v1.10.3 上传前同名检查误触发此路径，表现为"点了上传没反应"）。
// 修复后这类传输以 loginUser="" 直连执行；本测试锁定"su + 空密码"
// 组合必须快速得到明确的认证错误，而不是长时间挂起。
func TestShellTransport_SuWithEmptyPasswordFailsFast(t *testing.T) {
	root := t.TempDir()
	srv := newTestSSHServer(t, testSSHServerOptions{RootDir: root})
	defer srv.Close()

	client, err := ssh.Dial("tcp", srv.Addr(), srv.ClientConfig())
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	defer client.Close()

	// loginUser 非空 + rootPassword 为空：v1.10.3 回归的准确形态。
	tr := NewRootRelayTransport(client, "", "u")
	defer tr.Close()

	start := time.Now()
	_, err = tr.List(context.Background(), "/data")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("su + 空密码应当失败，却成功了")
	}
	var te *TransferError
	if !errors.As(err, &te) || te.Code != ErrorCodeAuthFailed {
		t.Fatalf("应为认证类错误，实际: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("失败耗时 %v，存在提示符等待挂起的嫌疑", elapsed)
	}
}
