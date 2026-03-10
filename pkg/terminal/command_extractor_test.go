package terminal

import (
	"strings"
	"testing"
)

func TestCommandExtractor_TabCompletion(t *testing.T) {
	extractor := NewCommandExtractor()

	// 模拟用户输入 "cd s" 然后按 Tab
	extractor.SetPendingInput("cd s")

	// 模拟 Shell 回显补全后的命令
	output := "cd sopuser/\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract command from output")
	}
	if !strings.HasPrefix(cmd, "cd s") {
		t.Errorf("Expected command to start with 'cd s', got %q", cmd)
	}
	if cmd != "cd sopuser/" {
		t.Errorf("Expected 'cd sopuser/', got %q", cmd)
	}
}

func TestCommandExtractor_TabCompletionWithANSI(t *testing.T) {
	extractor := NewCommandExtractor()

	// 模拟用户输入 "ls -l"
	extractor.SetPendingInput("ls -l")

	// 模拟带 ANSI 颜色代码的输出
	output := "\x1b[0m\x1b[01;32mls -la\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract command from ANSI-colored output")
	}
	if !strings.HasPrefix(cmd, "ls -l") {
		t.Errorf("Expected command to start with 'ls -l', got %q", cmd)
	}
}

func TestCommandExtractor_PromptPattern(t *testing.T) {
	extractor := NewCommandExtractor()
	extractor.SetPendingInput("ls")

	// 模拟带提示符的输出
	output := "\x1b[0m\x1b[01;34muser@host\x1b[0m:~$ ls -la\r\ntotal 32\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract command from output with prompt")
	}
	if cmd != "ls -la" && !strings.HasPrefix(cmd, "ls") {
		t.Errorf("Expected 'ls -la' or similar, got %q", cmd)
	}
}

func TestCommandExtractor_MultipleLines(t *testing.T) {
	extractor := NewCommandExtractor()

	// 设置待匹配的输入
	extractor.SetPendingInput("cat ")

	// 模拟多行输出，命令在中间
	output := "some output\r\ncat /etc/passwd\r\nmore output\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract command from multi-line output")
	}
	if !strings.HasPrefix(cmd, "cat ") {
		t.Errorf("Expected command to start with 'cat ', got %q", cmd)
	}
}

func TestCommandExtractor_NoMatch(t *testing.T) {
	extractor := NewCommandExtractor()

	// 设置待匹配的输入
	extractor.SetPendingInput("cd s")

	// 输出不包含匹配的命令
	output := "some random output\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if found {
		t.Errorf("Expected no match, but got: %q", cmd)
	}
}

func TestCommandExtractor_EmptyPendingInput(t *testing.T) {
	extractor := NewCommandExtractor()

	// 不设置待匹配的输入
	output := "some output\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if found {
		t.Errorf("Expected no match without pending input, but got: %q", cmd)
	}
}

func TestCommandExtractor_Reset(t *testing.T) {
	extractor := NewCommandExtractor()

	extractor.SetPendingInput("test")
	extractor.Reset()

	if extractor.GetPendingInput() != "" {
		t.Error("Expected pending input to be cleared after reset")
	}
}

func TestCommandExtractor_PartialOutput(t *testing.T) {
	extractor := NewCommandExtractor()
	extractor.SetPendingInput("echo")

	// 分两部分发送输出（模拟网络分包）
	part1 := "echo "
	part2 := "hello\r\n"

	cmd, found := extractor.ProcessOutput(part1)
	if found {
		t.Errorf("Expected no match for partial output, got: %q", cmd)
	}

	cmd, found = extractor.ProcessOutput(part2)
	if !found {
		t.Error("Expected to extract command after complete output")
	}
	if !strings.HasPrefix(cmd, "echo") {
		t.Errorf("Expected command to start with 'echo', got %q", cmd)
	}
}

func TestCommandExtractor_SpecialPromptFormats(t *testing.T) {
	tests := []struct {
		name         string
		pendingInput string
		output       string
		expectedCmd  string
	}{
		{
			name:         "root prompt with #",
			pendingInput: "systemctl",
			output:       "\x1b[0m# systemctl status nginx\r\n",
			expectedCmd:  "systemctl status nginx",
		},
		{
			name:         "simple $ prompt",
			pendingInput: "pwd",
			output:       "$ pwd\r\n",
			expectedCmd:  "pwd",
		},
		{
			name:         "user@host format",
			pendingInput: "vim",
			output:       "user@server:~$ vim /etc/hosts\r\n",
			expectedCmd:  "vim /etc/hosts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCommandExtractor()
			extractor.SetPendingInput(tt.pendingInput)

			cmd, found := extractor.ProcessOutput(tt.output)

			if !found {
				t.Errorf("Expected to extract command, but got no match")
				return
			}
			if cmd != tt.expectedCmd {
				t.Errorf("Expected %q, got %q", tt.expectedCmd, cmd)
			}
		})
	}
}

func TestCommandExtractor_TabCompletionWithDirectory(t *testing.T) {
	extractor := NewCommandExtractor()

	// 模拟 Tab 补全目录
	extractor.SetPendingInput("cd s")

	// 补全后的输出
	output := "cd sopuser/\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract tab-completed command")
	}
	if cmd != "cd sopuser/" {
		t.Errorf("Expected 'cd sopuser/', got %q", cmd)
	}
}

func TestCommandExtractor_HistoryRecall(t *testing.T) {
	extractor := NewCommandExtractor()

	// 用户按上下箭头选择历史命令时，原始输入可能是 "sudo"
	// 但实际执行的可能是之前的完整命令
	extractor.SetPendingInput("sudo")

	// 历史命令回显
	output := "sudo systemctl restart nginx\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if !found {
		t.Error("Expected to extract history-recalled command")
	}
	if !strings.HasPrefix(cmd, "sudo") {
		t.Errorf("Expected command to start with 'sudo', got %q", cmd)
	}
}

func TestCommandExtractor_NoPendingInput(t *testing.T) {
	extractor := NewCommandExtractor()

	// 不设置 pendingInput，输出不应该被处理
	output := "user@host:~$ ls -la /tmp\r\ntotal 32\r\n"
	cmd, found := extractor.ProcessOutput(output)

	if found {
		t.Errorf("Expected no extraction without pending input, got: %q", cmd)
	}
}
