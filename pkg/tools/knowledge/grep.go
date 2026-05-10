package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/tools"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepTool 知识库文件内容搜索工具
type GrepTool struct {
	knowledgeDir string
}

// NewGrepTool 创建 Grep 工具
func NewGrepTool(knowledgeDir string) *GrepTool {
	return &GrepTool{knowledgeDir: knowledgeDir}
}

// Name 返回工具名称
func (t *GrepTool) Name() string {
	return "grep_knowledge"
}

// Description 返回工具描述（给 LLM 看的说明）
func (t *GrepTool) Description() string {
	return "Search for a pattern in knowledge base documents. Supports regex (e.g. 'timeout|504|超时' to match any). Returns matching lines with file name, line number, and content. Use this to locate relevant sections before reading with read_knowledge_file."
}

// Parameters 返回 JSON Schema 格式的参数定义
func (t *GrepTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {"type": "string", "description": "Search pattern. Supports regex OR for multi-keyword: 'timeout|504|超时'. Case-insensitive."},
			"path": {"type": "string", "description": "Optional file path to limit search scope, e.g. 'payment_system_sop.md'"},
			"max_results": {"type": "integer", "description": "Maximum number of matching lines to return (default 20, max 100)", "minimum": 1, "maximum": 100}
		},
		"required": ["pattern"],
		"additionalProperties": false
	}`)
}

// Execute 执行 grep 搜索
func (t *GrepTool) Execute(ctx context.Context, args map[string]interface{}, emitStatus tools.StatusEmitter) (string, error) {
	pattern, _ := args["pattern"].(string)
	pathFilter, _ := args["path"].(string)
	maxResults := 20

	if mr, ok := toInt(args["max_results"]); ok && mr > 0 {
		maxResults = mr
		if maxResults > 100 {
			maxResults = 100
		}
	}

	if pattern == "" {
		return "", fmt.Errorf("pattern 参数不能为空")
	}

	if emitStatus != nil {
		emitStatus("grepping", fmt.Sprintf("正在搜索关键词: %s...", pattern))
	}

	// 尝试编译为正则（支持 OR: "timeout|504|超时"），失败则退化为子串匹配
	re, regexErr := regexp.Compile("(?i)" + pattern)
	if regexErr != nil {
		log.Printf("[Grep] Regex compile failed, falling back to substring: pattern=%q err=%v", pattern, regexErr)
	} else {
		log.Printf("[Grep] Using regex match: pattern=%q files=%d", pattern, 0)
	}
	matchLine := func(line string) bool {
		if regexErr == nil {
			return re.MatchString(line)
		}
		return strings.Contains(strings.ToLower(line), strings.ToLower(pattern))
	}

	var results []string
	totalMatches := 0

	// 确定搜索范围：单文件 or 全部 .md
	var files []string
	var err error

	if pathFilter != "" {
		// 安全校验
		cleanRel := filepath.ToSlash(filepath.Clean(pathFilter))
		if strings.Contains(cleanRel, "..") || strings.HasPrefix(cleanRel, "/") {
			return "", fmt.Errorf("invalid file path: traversal detected")
		}
		files = []string{cleanRel}
	} else {
		files, err = knowledge.ListFiles(t.knowledgeDir)
		if err != nil {
			return "", fmt.Errorf("列出知识库文件失败: %w", err)
		}
	}

	for _, relPath := range files {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		content, err := knowledge.ReadFile(t.knowledgeDir, relPath)
		if err != nil {
			log.Printf("[Grep] Skip file %s: %v", relPath, err)
			continue
		}

		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if matchLine(line) {
				totalMatches++
				if len(results) < maxResults {
					results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
				}
			}
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No matches found for pattern: %q", pattern), nil
	}

	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(r)
		sb.WriteByte('\n')
	}

	if totalMatches > maxResults {
		sb.WriteString(fmt.Sprintf("... (%d more matches, showing first %d)", totalMatches-maxResults, maxResults))
	}

	log.Printf("[Grep] pattern=%q files=%d matches=%d returned=%d", pattern, len(files), totalMatches, len(results))
	return sb.String(), nil
}

// toInt 从 map[string]interface{} 中提取 int 值
// JSON 反序列化可能产生 float64，需要兼容处理
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}
