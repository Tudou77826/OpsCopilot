package sessionimport

import (
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// legacyKeySeed 是 Xshell < 5.1 使用的固定 RC4 密钥种子（与 SID 无关）。
const legacyKeySeed = "!X@s#h$e%l^l&"

// Credentials 是解密会话密码所需的凭据。
//
// 同机导入时 SID 与 WindowsUser 由 LocalCredentials 自动取得；MasterPassword
// 仅在用户设置了 Xshell 主密码时需要。跨机器导入必须由用户提供源机器的 SID，
// 否则密码在密码学上不可解——这是密钥派生方式决定的，不是实现缺陷。
type Credentials struct {
	SID            string
	WindowsUser    string
	MasterPassword string
}

// KeyCandidate 是一个候选 RC4 密钥及其来源说明（用于报告与排错）。
type KeyCandidate struct {
	Label string
	Key   []byte
}

// KeyCandidates 生成全部已知的密钥派生候选。
//
// 这里刻意不按 Version 硬编码单条规则，而是枚举候选 + 用密文尾部校验和判定：
// 各开源实现对 Xshell 7.x/8.x 的描述互相矛盾（实测 Xshell 8.1 用的是
// reverse(SID)+Windows账户名，而不少资料仍写作 UserName+SID，且此处的 UserName
// 指的是 Windows 账户名而非会话里的 SSH 登录名）。枚举 + 校验既能覆盖新旧数据，
// 也不怕将来规则再变，代价只是几条 SHA256。
func KeyCandidates(c Credentials) []KeyCandidate {
	rev := func(s string) string {
		r := []rune(s)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r)
	}
	sid, user := c.SID, c.WindowsUser
	rsid, ruser := rev(sid), rev(user)

	var out []KeyCandidate
	add := func(label, material string) {
		if material == "" {
			return
		}
		sum := sha256.Sum256([]byte(material))
		out = append(out, KeyCandidate{Label: label, Key: sum[:]})
	}

	// 按"现代版本优先"排序，命中通常发生在第一条。
	add("reverse(SID)+Windows账户名 (Xshell 7.1+/8.x)", rsid+user)
	add("Windows账户名+SID (Xshell 5.3-7.0)", user+sid)
	add("SID (Xshell 5.1-5.2)", sid)
	// 冗余候选：覆盖各实现对拼接顺序与反转范围的分歧。
	add("reverse(Windows账户名)+SID", ruser+sid)
	add("reverse(Windows账户名+SID)", rev(user+sid))
	add("reverse(SID+Windows账户名)", rev(sid+user))
	add("Windows账户名", user)

	if c.MasterPassword != "" {
		add("主密码（启用了 Xshell 主密码）", c.MasterPassword)
	}

	// 最老的固定密钥，用的是 MD5 而非 SHA256。
	legacy := md5.Sum([]byte(legacyKeySeed))
	out = append(out, KeyCandidate{Label: "固定密钥 (Xshell <5.1)", Key: legacy[:]})
	return out
}

// DecryptResult 是一次成功解密的产物。
type DecryptResult struct {
	Plaintext string
	Rule      string
}

// DecryptPassword 尝试解密 base64 编码的 Xshell 密码。
//
// 密文结构（>=5.1）：RC4(明文) || SHA256(明文)[32]。尾部校验和让密钥判定成为
// 确定性判断——错误密钥通过校验的概率是 2^-256，因此不需要按版本猜规则。
// 密文短于 32 字节时走 <5.1 的无校验路径，只认固定密钥并要求结果像可读文本。
func DecryptPassword(encoded string, candidates []KeyCandidate) (DecryptResult, bool) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || len(candidates) == 0 {
		return DecryptResult{}, false
	}

	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		blob, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return DecryptResult{}, false
		}
	}

	if len(blob) >= sha256.Size {
		ciphertext := blob[:len(blob)-sha256.Size]
		want := blob[len(blob)-sha256.Size:]
		for _, cand := range candidates {
			plaintext, err := rc4XOR(cand.Key, ciphertext)
			if err != nil {
				continue
			}
			got := sha256.Sum256(plaintext)
			if subtle.ConstantTimeCompare(got[:], want) == 1 {
				return DecryptResult{Plaintext: string(plaintext), Rule: cand.Label}, true
			}
		}
		return DecryptResult{}, false
	}

	for _, cand := range candidates {
		if !strings.Contains(cand.Label, "<5.1") {
			continue
		}
		plaintext, err := rc4XOR(cand.Key, blob)
		if err != nil || !looksLikeText(plaintext) {
			continue
		}
		return DecryptResult{Plaintext: string(plaintext), Rule: cand.Label}, true
	}
	return DecryptResult{}, false
}

func rc4XOR(key, data []byte) ([]byte, error) {
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.XORKeyStream(out, data)
	return out, nil
}
