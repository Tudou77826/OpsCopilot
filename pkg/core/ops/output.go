package ops

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// OutputController 输出控制器
type OutputController struct {
	MaxTotalBytes int
	MaxLineLength int
	HeadLines     int
}

// OutputMeta 输出元信息
type OutputMeta struct {
	TotalBytes         int `json:"total_bytes"`
	ReturnedBytes      int `json:"returned_bytes"`
	TotalLines         int `json:"total_lines"`
	ReturnedLines      int `json:"returned_lines"`
	TruncatedLines     int `json:"truncated_lines"`
	LongLinesTruncated int `json:"long_lines_truncated"`
}

// OutputResult 输出结果
type OutputResult struct {
	Output string
	Meta   OutputMeta
}

// NewOutputController 创建输出控制器
func NewOutputController(maxTotalBytes, maxLineLength, headLines int) *OutputController {
	return &OutputController{
		MaxTotalBytes: maxTotalBytes,
		MaxLineLength: maxLineLength,
		HeadLines:     headLines,
	}
}

// Process 处理输出
func (c *OutputController) Process(output string) *OutputResult {
	// Use local limits: callers may share the controller between requests.
	limits := *c
	if limits.MaxTotalBytes < 1 {
		limits.MaxTotalBytes = 10240
	}
	if limits.MaxLineLength < 1 {
		limits.MaxLineLength = 500
	}
	if limits.HeadLines < 0 {
		limits.HeadLines = 0
	}
	c = &limits
	output = strings.ToValidUTF8(output, "�")
	meta := OutputMeta{
		TotalBytes: len(output),
	}

	// Step 1: 单行截断
	lines := strings.Split(output, "\n")
	processedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		if len(line) > c.MaxLineLength {
			// 截断长行：前200字 + 标记 + 后200字
			keep := min(200, c.MaxLineLength/2)
			truncated := utf8Head(line, keep) +
				fmt.Sprintf("...[截断:原长度%d字]...", len(line)) +
				utf8Tail(line, keep)
			processedLines = append(processedLines, truncated)
			meta.LongLinesTruncated++
		} else {
			processedLines = append(processedLines, line)
		}
	}
	meta.TotalLines = len(processedLines)

	// Step 2: 总大小检查
	fullOutput := strings.Join(processedLines, "\n")
	if len(fullOutput) <= c.MaxTotalBytes {
		meta.ReturnedBytes = len(fullOutput)
		meta.ReturnedLines = len(processedLines)
		return &OutputResult{
			Output: fullOutput,
			Meta:   meta,
		}
	}

	// 需要截断
	var resultLines []string
	var usedBytes int

	// 保留头部
	for i := 0; i < c.HeadLines && i < len(processedLines); i++ {
		line := processedLines[i]
		if usedBytes+len(line)+1 > c.MaxTotalBytes {
			break
		}
		resultLines = append(resultLines, line)
		usedBytes += len(line) + 1
	}
	headCount := len(resultLines)

	// 添加省略标记
	omitCount := len(processedLines) - headCount
	omitMarker := fmt.Sprintf("\n...[省略 %d 行]...\n", omitCount)
	markerIncluded := false
	if usedBytes+len(omitMarker) < c.MaxTotalBytes {
		resultLines = append(resultLines, omitMarker)
		markerIncluded = true
		usedBytes += len(omitMarker)
	}

	// 从尾部添加行
	tailLines := make([]string, 0)
	remainingBytes := c.MaxTotalBytes - usedBytes

	for i := len(processedLines) - 1; i >= headCount && remainingBytes > 0; i-- {
		line := processedLines[i]
		if len(line)+1 > remainingBytes {
			break
		}
		tailLines = append([]string{line}, tailLines...)
		remainingBytes -= len(line) + 1
	}

	resultLines = append(resultLines, tailLines...)
	if markerIncluded {
		resultLines[headCount] = fmt.Sprintf("\n...[省略 %d 行]...\n", len(processedLines)-headCount-len(tailLines))
	}

	finalOutput := strings.Join(resultLines, "\n")
	if len(finalOutput) > c.MaxTotalBytes {
		finalOutput = utf8Head(finalOutput, c.MaxTotalBytes)
	}
	meta.ReturnedBytes = len(finalOutput)
	meta.ReturnedLines = headCount + len(tailLines)
	meta.TruncatedLines = meta.TotalLines - meta.ReturnedLines

	return &OutputResult{
		Output: finalOutput,
		Meta:   meta,
	}
}

func utf8Head(value string, size int) string {
	if size >= len(value) {
		return value
	}
	for size > 0 && !utf8.RuneStart(value[size]) {
		size--
	}
	return value[:size]
}
func utf8Tail(value string, size int) string {
	if size >= len(value) {
		return value
	}
	start := len(value) - size
	for start < len(value) && !utf8.RuneStart(value[start]) {
		start++
	}
	return value[start:]
}
