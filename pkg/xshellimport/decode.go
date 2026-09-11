// Package xshellimport 解析 Xshell 的落盘文件，并给出并入本地模型的导入计划。
//
// 它是一个纯叶子包：只依赖标准库与 x/text 的 GBK 解码器，不依赖项目内其他包。
// 写入目标通过端口注入，由宿主实现（桌面端用 connectionstore / config 适配），
// 因此本包可以脱离应用独立测试。
//
// 它承载两类导入，共用编码回退与 INI 解析：
//   - 会话（session）：单个 .xsh 文件、包含 .xsh 的目录树（子目录即分组，
//     Xshell 的 Sessions 目录就是这个形态）、.xts 备份包（ZIP，内含 xts.zcf
//     与 Xshell/<分组路径>/<会话>.xsh）
//   - 快捷命令（quickcmd）：单套 .qbl 文件，或包含多个 .qbl 的 QuickButton Files 目录
package xshellimport

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// Encoding 标识解码时实际使用的字符集。
type Encoding string

const (
	EncodingUTF8    Encoding = "utf-8"
	EncodingUTF16LE Encoding = "utf-16le"
	EncodingUTF16BE Encoding = "utf-16be"
	EncodingGBK     Encoding = "gbk"
)

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// DecodeText 把 Xshell 写出的文本字节解码为 UTF-8。
//
// 判定顺序：BOM → UTF-16LE/BE → 合法 UTF-8 → GBK 回退。依据实测与各版本行为：
// Xshell 8 写 UTF-16LE with BOM，中文环境的老版本写 GBK，ASCII/UTF-8 原样通过。
// 这个顺序是必要的——UTF-16LE 的中文内容在 UTF-8 校验下也常常"看似合法"的概率极低，
// 但反过来先判 UTF-8 会把 GBK 中文误判，所以必须靠 BOM 优先区分。
func DecodeText(raw []byte) (string, Encoding) {
	switch {
	case bytes.HasPrefix(raw, bomUTF8):
		return string(raw[len(bomUTF8):]), EncodingUTF8
	case bytes.HasPrefix(raw, bomUTF16LE):
		return decodeUTF16(raw[2:], true), EncodingUTF16LE
	case bytes.HasPrefix(raw, bomUTF16BE):
		return decodeUTF16(raw[2:], false), EncodingUTF16BE
	case utf8.Valid(raw):
		return string(raw), EncodingUTF8
	default:
		return decodeGBK(raw), EncodingGBK
	}
}

// decodeUTF16 把 UTF-16 字节序列解码为 UTF-8 字符串（含代理对）。
func decodeUTF16(b []byte, littleEndian bool) string {
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if littleEndian {
			units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		}
	}
	return string(utf16.Decode(units))
}

// decodeGBK 尽力按 GBK 解码。遇到个别非法字节序列时不放弃整段文本——
// 一条会话因为一个字节解不开就整条丢失是不可接受的，宁可局部出现替换字符。
func decodeGBK(raw []byte) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw)
	if err == nil {
		return string(decoded)
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD")
}

// looksLikeText 判断字节是否像可读文本，用于 <5.1 无校验和的解密结果做启发式判定。
func looksLikeText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r == utf8.RuneError {
			return false
		}
		// 允许制表符与换行；其余控制字符视为解密失败的特征。
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}
