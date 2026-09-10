package sessionimport

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
)

// ParseXTS 解析 Xshell 的 .xts 备份包（本质是 ZIP）。
//
// 包内结构：xts.zcf（元数据，INI 格式）、Xshell/<分组路径>/<会话>.xsh，
// 部分版本带 Xshell/Sessions/ 前缀。若 xts.zcf 里 [SessionInfo] Master=1，
// 表示整包被 Xshell 主密码加密，标准库无法读取——此时给出明确指引，
// 让用户改走本机会话目录（那条路径不需要解包）。
func ParseXTS(data []byte, opts ParseOptions) ([]*SessionRecord, []string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("打开备份包失败（可能不是有效的 .xts 文件）: %w", err)
	}

	var warnings []string

	if err := checkMasterEncrypted(reader); err != nil {
		return nil, nil, err
	}

	var records []*SessionRecord
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := zipEntryName(file)
		if !strings.EqualFold(path.Ext(name), ".xsh") {
			continue
		}
		groupPath, ok := groupPathFromZipName(name)
		if !ok {
			continue // 不是 Xshell/ 下的会话，跳过
		}
		content, err := readZipEntry(file)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("读取 %s 失败: %v", name, err))
			continue
		}
		records = append(records, ParseXSH(content, sessionName(path.Base(name)), groupPath, opts))
	}

	if len(records) == 0 {
		warnings = append(warnings, "该备份包内没有找到 .xsh 会话文件")
	}
	return records, warnings, nil
}

// checkMasterEncrypted 检测整包是否被主密码加密。
func checkMasterEncrypted(reader *zip.Reader) error {
	for _, file := range reader.File {
		if !strings.EqualFold(path.Base(zipEntryName(file)), "xts.zcf") {
			continue
		}
		content, err := readZipEntry(file)
		if err != nil {
			return nil // 元数据读不到时不阻断，交给后续逐文件解析报错
		}
		text, _ := DecodeText(content)
		if ParseINI(text).Get("SessionInfo", "Master") == "1" {
			return fmt.Errorf("该备份包启用了主密码（整包加密），无法直接读取。" +
				"请改用「从本机 Xshell 会话目录导入」；或在 Xshell 中关闭主密码后重新导出")
		}
		return nil
	}
	return nil
}

// zipEntryName 取 ZIP 成员的名称。
//
// 中文环境下 Xshell 写 ZIP 成员名用 GBK 且不置 UTF-8 标志位，此时 Go 返回的
// f.Name 是未解码的原始字节。必须按原始字节重新解码，否则中文分组名会变成乱码。
func zipEntryName(f *zip.File) string {
	if f.NonUTF8 {
		text, _ := DecodeText([]byte(f.Name))
		return text
	}
	return f.Name
}

// groupPathFromZipName 从成员路径推出分组层级。
// "Xshell/生产/华东/web-1.xsh" -> ["生产","华东"]；"Xshell/Sessions/a.xsh" -> nil。
func groupPathFromZipName(name string) ([]string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	idx := strings.Index(strings.ToLower(name), "xshell/")
	if idx < 0 {
		return nil, false
	}
	rest := name[idx+len("xshell/"):]

	const sessionsPrefix = "sessions/"
	if len(rest) >= len(sessionsPrefix) && strings.EqualFold(rest[:len(sessionsPrefix)], sessionsPrefix) {
		rest = rest[len(sessionsPrefix):]
	}

	var out []string
	for _, seg := range strings.Split(path.Dir(rest), "/") {
		if seg != "" && seg != "." {
			out = append(out, seg)
		}
	}
	return out, true
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// sessionName 去掉扩展名——Xshell 用文件名作为会话显示名。
func sessionName(base string) string {
	return strings.TrimSuffix(base, path.Ext(base))
}
