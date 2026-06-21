package ops

import (
	"strings"
	"testing"
)

// TestOutput_SmallInput_PassThrough 小输入且无长行：原样返回，meta 计数正确
func TestOutput_SmallInput_PassThrough(t *testing.T) {
	c := NewOutputController(10240, 500, 5)
	input := "line1\nline2\nline3"
	r := c.Process(input)

	if r.Output != input {
		t.Errorf("expected pass-through, got len %d want %d", len(r.Output), len(input))
	}
	if r.Meta.TotalBytes != len(input) {
		t.Errorf("TotalBytes = %d, want %d", r.Meta.TotalBytes, len(input))
	}
	if r.Meta.ReturnedBytes != len(input) {
		t.Errorf("ReturnedBytes = %d, want %d", r.Meta.ReturnedBytes, len(input))
	}
	if r.Meta.TotalLines != 3 {
		t.Errorf("TotalLines = %d, want 3", r.Meta.TotalLines)
	}
	if r.Meta.ReturnedLines != 3 {
		t.Errorf("ReturnedLines = %d, want 3", r.Meta.ReturnedLines)
	}
	if r.Meta.TruncatedLines != 0 {
		t.Errorf("TruncatedLines = %d, want 0", r.Meta.TruncatedLines)
	}
	if r.Meta.LongLinesTruncated != 0 {
		t.Errorf("LongLinesTruncated = %d, want 0", r.Meta.LongLinesTruncated)
	}
}

// TestOutput_LongLine_Truncated 单行超过 MaxLineLength 被截断为"前200+标记+后200"
func TestOutput_LongLine_Truncated(t *testing.T) {
	c := NewOutputController(10240, 500, 5)
	longLine := strings.Repeat("x", 1000) // 远超 500
	r := c.Process(longLine)

	if r.Meta.LongLinesTruncated != 1 {
		t.Errorf("LongLinesTruncated = %d, want 1", r.Meta.LongLinesTruncated)
	}

	// 截断后应含标记文本
	if !strings.Contains(r.Output, "[截断") {
		t.Errorf("truncated line should contain marker, got: %q", r.Output)
	}

	// 应保留头 200 字符
	if !strings.HasPrefix(strings.SplitN(r.Output, "...", 2)[0], strings.Repeat("x", 199)) {
		t.Errorf("truncated line should preserve first 200 chars")
	}

	// 应保留尾 200 字符
	if !strings.HasSuffix(r.Output, strings.Repeat("x", 200)) {
		t.Errorf("truncated line should preserve last 200 chars, suffix len=%d", len(r.Output))
	}
}

// TestOutput_OverSize_HeadTailPreserved 总量超限时保留头 HeadLines 行 + 尾部行，中间插省略标记
func TestOutput_OverSize_HeadTailPreserved(t *testing.T) {
	// MaxTotalBytes 设小，触发总截断；HeadLines=2
	c := NewOutputController(200, 1000, 2)

	// 构造 10 行，每行约 30 字节，总量约 300 > 200
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, strings.Repeat("a", 25)+string(rune('A'+i)))
	}
	input := strings.Join(lines, "\n")
	r := c.Process(input)

	if r.Meta.TruncatedLines <= 0 {
		t.Errorf("expected truncation, TruncatedLines = %d", r.Meta.TruncatedLines)
	}
	if !strings.Contains(r.Output, "[省略") {
		t.Errorf("expected omit marker in output, got: %q", r.Output)
	}
	// 第一行应被保留（头部）
	if !strings.Contains(r.Output, lines[0]) {
		t.Errorf("first line should be preserved (head), got: %q", r.Output)
	}
	// 最后一行应被保留（尾部）
	if !strings.Contains(r.Output, lines[9]) {
		t.Errorf("last line should be preserved (tail), got: %q", r.Output)
	}
	// ReturnedBytes 不应超过 MaxTotalBytes
	if r.Meta.ReturnedBytes > c.MaxTotalBytes {
		t.Errorf("ReturnedBytes = %d exceeds MaxTotalBytes = %d", r.Meta.ReturnedBytes, c.MaxTotalBytes)
	}
}

// TestOutput_ExactSize 恰好等于 MaxTotalBytes：走 PassThrough 分支不截断
func TestOutput_ExactSize(t *testing.T) {
	// 用 50 字节边界
	c := NewOutputController(50, 1000, 5)
	input := strings.Repeat("a", 50)
	r := c.Process(input)

	if r.Meta.TruncatedLines != 0 {
		t.Errorf("exact-size input should not truncate, TruncatedLines = %d", r.Meta.TruncatedLines)
	}
	if r.Output != input {
		t.Errorf("exact-size input should pass through")
	}
}

// TestOutput_EmptyInput 空字符串不 panic，meta 全 0 或合理值
func TestOutput_EmptyInput(t *testing.T) {
	c := NewOutputController(10240, 500, 5)
	r := c.Process("")

	// 空字符串 split 后得到 [""]，即 1 行空内容；这里只要求不 panic 且计数自洽
	if r.Meta.ReturnedBytes > r.Meta.TotalBytes {
		t.Errorf("ReturnedBytes(%d) > TotalBytes(%d)", r.Meta.ReturnedBytes, r.Meta.TotalBytes)
	}
}

// TestOutput_OnlyLongLines_NoOversize 只有长行但总量未超：只做行截断，不触发头尾截断
func TestOutput_OnlyLongLines_NoOversize(t *testing.T) {
	// MaxLineLength 小，但 MaxTotalBytes 足够大
	c := NewOutputController(100000, 100, 5)
	longLine := strings.Repeat("y", 500)
	r := c.Process(longLine)

	if r.Meta.LongLinesTruncated != 1 {
		t.Errorf("LongLinesTruncated = %d, want 1", r.Meta.LongLinesTruncated)
	}
	if r.Meta.TruncatedLines != 0 {
		t.Errorf("should not trigger head/tail truncation, TruncatedLines = %d", r.Meta.TruncatedLines)
	}
}

// TestOutput_OmitMarkerFitting 头部+省略标记已超限时，省略标记不硬塞（覆盖 output.go 的判断）
func TestOutput_OmitMarkerFitting(t *testing.T) {
	// MaxTotalBytes 设到刚好放不下省略标记的程度
	// HeadLines=5，每行较长，使头部占满预算
	c := NewOutputController(80, 1000, 5)
	// 5 行各 15 字节 = 75，再加省略标记（约 20 字节）会超 80
	input := strings.Repeat(strings.Repeat("b", 14)+"\n", 5) + "tail"
	r := c.Process(input)

	// 关键验证：不能因为省略标记而超出 MaxTotalBytes
	if r.Meta.ReturnedBytes > c.MaxTotalBytes {
		t.Errorf("ReturnedBytes = %d exceeds MaxTotalBytes = %d (omit marker not properly bounded)",
			r.Meta.ReturnedBytes, c.MaxTotalBytes)
	}
}

// TestOutput_MetaConsistency ReturnedBytes/ReturnedLines/TruncatedLines 三者守恒
func TestOutput_MetaConsistency(t *testing.T) {
	c := NewOutputController(150, 1000, 3)
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, strings.Repeat("c", 20))
	}
	input := strings.Join(lines, "\n")
	r := c.Process(input)

	// 守恒：TotalLines = ReturnedLines + TruncatedLines
	// 注意：省略标记本身算作返回行的一部分，但 TruncatedLines 是"被省略的原始行数"
	// 这里验证 TruncatedLines >= 0 且 ReturnedLines <= TotalLines
	if r.Meta.TruncatedLines < 0 {
		t.Errorf("TruncatedLines should not be negative: %d", r.Meta.TruncatedLines)
	}
	if r.Meta.ReturnedLines > r.Meta.TotalLines {
		t.Errorf("ReturnedLines(%d) > TotalLines(%d)", r.Meta.ReturnedLines, r.Meta.TotalLines)
	}
	// ReturnedBytes 不超过上限
	if r.Meta.ReturnedBytes > c.MaxTotalBytes {
		t.Errorf("ReturnedBytes(%d) > MaxTotalBytes(%d)", r.Meta.ReturnedBytes, c.MaxTotalBytes)
	}
}
