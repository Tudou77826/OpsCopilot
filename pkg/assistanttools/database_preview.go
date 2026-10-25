package assistanttools

import (
	"fmt"
	"regexp"
	"strings"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/dbworkspace"
)

var credentialColumn = regexp.MustCompile(`(?i)(password|passwd|secret|token|api_?key|access_?key|private_?key|credential)`)

func databaseResultPreview(result dbworkspace.QueryResult) string {
	var out strings.Builder
	for i, set := range result.Sets {
		if i >= 3 {
			out.WriteString("[更多结果请查看 SQL 页签]\n")
			break
		}
		if len(set.Columns) == 0 {
			fmt.Fprintf(&out, "结果 %d：影响 %d 行\n", i+1, set.AffectedRows)
			continue
		}
		fmt.Fprintf(&out, "结果 %d：返回 %d 行", i+1, len(set.Rows))
		if set.Truncated {
			out.WriteString("（已截断）")
		}
		out.WriteByte('\n')
		n := min(len(set.Columns), 6)
		for column := 0; column < n; column++ {
			if column > 0 {
				out.WriteString(" | ")
			}
			out.WriteString(set.Columns[column].Name)
		}
		out.WriteByte('\n')
		for rowIndex, row := range set.Rows {
			if rowIndex >= 5 {
				out.WriteString("[更多行请查看 SQL 页签]\n")
				break
			}
			for column := 0; column < n; column++ {
				if column > 0 {
					out.WriteString(" | ")
				}
				value := "NULL"
				if column < len(row) && row[column] != nil {
					value = *row[column]
				}
				if credentialColumn.MatchString(set.Columns[column].Name) {
					value = "[已隐藏]"
				}
				value = strings.NewReplacer("\n", " ↵ ", "\r", "", "\t", " ").Replace(value)
				out.WriteString(assistantcontrol.DisplayExcerpt(value, 200))
			}
			out.WriteByte('\n')
		}
		if n < len(set.Columns) {
			out.WriteString("[部分列已省略]\n")
		}
	}
	if len(result.Sets) > 0 {
		fmt.Fprintf(&out, "耗时 %d ms", result.DurationMS)
	}
	return out.String()
}
