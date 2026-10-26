package ops

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOutputSmallLimitsAndUnicodeDoNotPanicOrCorrupt(t *testing.T) {
	for _, line := range []int{1, 50, 100, 500} {
		for _, budget := range []int{1, 32, 200, 10240} {
			got := NewOutputController(budget, line, 5).Process(strings.Repeat("中文测试", 100))
			if len(got.Output) > budget || !utf8.ValidString(got.Output) {
				t.Fatalf("budget=%d line=%d size=%d valid=%v", budget, line, len(got.Output), utf8.ValidString(got.Output))
			}
		}
	}
}

func TestOutputOmittedSingleLineIsReportedAsTruncated(t *testing.T) {
	r := NewOutputController(100, 65536, 5).Process(strings.Repeat("x", 1000))
	if r.Meta.TruncatedLines != 1 || r.Meta.ReturnedLines != 0 {
		t.Fatalf("omitted content reported as returned: %+v", r.Meta)
	}
}
