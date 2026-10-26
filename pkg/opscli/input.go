package opscli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"opscopilot/pkg/filetxn"
)

func requireOptions(pairs ...string) error {
	var missing []string
	for i := 0; i < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) == "" {
			missing = append(missing, pairs[i])
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("缺少必填参数：%s", strings.Join(missing, "、"))
	}
	return nil
}

func optionPresent(args []string, name string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		if !strings.HasPrefix(args[i], "-") {
			continue
		}
		key, _, hasValue := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		if key == name {
			return true
		}
		if !hasValue {
			i++
		}
	}
	return false
}

func readSQL(path string, input io.Reader) (string, error) {
	const maxSQLBytes = 1 << 20
	var data []byte
	var err error
	if path == "-" {
		if input == nil {
			return "", errors.New("无法读取 SQL 标准输入")
		}
		data, err = io.ReadAll(io.LimitReader(input, maxSQLBytes+1))
	} else {
		data, err = filetxn.ReadBounded(path, maxSQLBytes)
	}
	if err != nil || len(data) > maxSQLBytes {
		return "", errors.New("读取 SQL 失败：请提供可读普通文件或 stdin，内容不能超过 1 MiB")
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", errors.New("SQL 输入必须为 UTF-8 文本，不能包含 NUL 字符")
	}
	// Accept files saved with an UTF-8 BOM without passing it to the driver.
	return strings.TrimPrefix(string(data), "\ufeff"), nil
}
