package terminal

import (
	"regexp"
	"strings"
	"sync"
)

// CommandExtractor 从终端输出中提取命令（用于 Tab 补全修正）
type CommandExtractor struct {
	mu            sync.Mutex
	buffer        string
	pendingInput  string // 待匹配的输入（用于检测回显）
	promptPattern *regexp.Regexp
}

// NewCommandExtractor 创建命令提取器
func NewCommandExtractor() *CommandExtractor {
	return &CommandExtractor{
		// 常见提示符模式: [user@host dir]$ 或 user@host:~$ 或 # 或 $
		// 支持带 ANSI 颜色代码的提示符
		promptPattern: regexp.MustCompile(`(?:\x1b\[[0-9;]*m)*(?:\[?[^\r\n@\]]*@[^\]$>\r\n]*[\]$][^$>\r\n]*[$>]\s|[#\$]\s)`),
	}
}

// SetPendingInput 设置待匹配的输入（从 LineBuffer 获取）
// 当用户按下 Enter 时，将输入的命令前缀设置到提取器
func (e *CommandExtractor) SetPendingInput(input string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pendingInput = input
}

// GetPendingInput 获取当前待匹配的输入（用于调试）
func (e *CommandExtractor) GetPendingInput() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pendingInput
}

// ProcessOutput 处理输出，返回提取到的命令
// 返回: (命令内容, 是否为新命令)
// 策略：只有当有 pendingInput 时才进行前缀匹配（Tab 补全场景）
func (e *CommandExtractor) ProcessOutput(output string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 如果没有待匹配的输入，不处理
	if e.pendingInput == "" {
		// 只追加到缓冲区，不处理
		e.buffer += output
		// 限制缓冲区大小
		if len(e.buffer) > 10000 {
			e.buffer = e.buffer[len(e.buffer)-5000:]
		}
		return "", false
	}

	// 追加到缓冲区
	e.buffer += output

	// 查找命令行模式: 提示符 + 命令 + 换行
	lines := strings.Split(e.buffer, "\n")
	if len(lines) < 2 {
		return "", false
	}

	// 保留最后一行（未完成的）
	e.buffer = lines[len(lines)-1]

	// 检查已完成的行
	for _, line := range lines[:len(lines)-1] {
		cleanedLine := CleanInput(line)
		cleanedLine = strings.TrimSpace(cleanedLine)

		// 跳过空行
		if cleanedLine == "" {
			continue
		}

		// 前缀匹配（Tab 补全）
		if len(cleanedLine) >= len(e.pendingInput) {
			// 检查是否以前缀开头
			if strings.HasPrefix(cleanedLine, e.pendingInput) {
				cmd := cleanedLine
				e.pendingInput = ""
				e.buffer = "" // 清空缓冲区
				return cmd, true
			}

			// 尝试从提示符后提取命令
			if idx := e.promptPattern.FindStringIndex(cleanedLine); idx != nil {
				cmdAfterPrompt := strings.TrimSpace(cleanedLine[idx[1]:])
				if cmdAfterPrompt != "" && len(cmdAfterPrompt) >= len(e.pendingInput) && strings.HasPrefix(cmdAfterPrompt, e.pendingInput) {
					e.pendingInput = ""
					e.buffer = ""
					return cmdAfterPrompt, true
				}
			}
		}
	}

	return "", false
}

// Reset 重置提取器状态
func (e *CommandExtractor) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.buffer = ""
	e.pendingInput = ""
}
