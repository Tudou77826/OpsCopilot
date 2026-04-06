package filetransfer

import (
	"os"
	"testing"
	"time"
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

func TestExtractBeforeMarker(t *testing.T) {
	tests := []struct {
		name   string
		output string
		marker string
		expect string
	}{
		{
			name:   "simple output",
			output: "cmd arg\nline1\nline2\n__RELAY_OK__\n",
			marker: "__RELAY_OK__",
			expect: "line1\nline2",
		},
		{
			name:   "single line output",
			output: "cmd arg\nresult\n__RELAY_OK__\n",
			marker: "__RELAY_OK__",
			expect: "result",
		},
		{
			name:   "fail marker",
			output: "cmd arg\nerro msg\n__RELAY_FAIL__\n",
			marker: "__RELAY_FAIL__",
			expect: "erro msg",
		},
		{
			name:   "no newline before marker",
			output: "data__RELAY_OK__",
			marker: "__RELAY_OK__",
			expect: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBeforeMarker(tt.output, tt.marker)
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
	if maxBase64DirectBytes != 300*1024 {
		t.Errorf("maxBase64DirectBytes = %d, want %d", maxBase64DirectBytes, 300*1024)
	}
}
