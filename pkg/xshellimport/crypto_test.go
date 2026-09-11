package xshellimport

import (
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// encryptXshell 按 Xshell 的密文结构（RC4(明文) || SHA256(明文)）加密，
// 仅供测试构造夹具，不参与生产逻辑。
func encryptXshell(t *testing.T, plaintext string, key []byte) string {
	t.Helper()
	cipher, err := rc4.NewCipher(key)
	if err != nil {
		t.Fatalf("构造 RC4 失败: %v", err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.XORKeyStream(ciphertext, []byte(plaintext))
	sum := sha256.Sum256([]byte(plaintext))
	return base64.StdEncoding.EncodeToString(append(ciphertext, sum[:]...))
}

func testCredentials() Credentials {
	return Credentials{
		SID:         "S-1-5-21-111-222-333-1001",
		WindowsUser: "alice",
	}
}

// 候选密钥必须两两不同，否则"用某候选密钥加密再用同一批候选解密"的
// 往返测试就无法定位到底是哪条规则命中的。
func TestKeyCandidatesAreDistinct(t *testing.T) {
	cands := KeyCandidates(testCredentials())
	seen := make(map[string]string, len(cands))
	for _, c := range cands {
		fingerprint := hex.EncodeToString(c.Key)
		if prev, dup := seen[fingerprint]; dup {
			t.Errorf("候选密钥重复: %q 与 %q 相同", c.Label, prev)
		}
		seen[fingerprint] = c.Label
	}
	if len(cands) < 5 {
		t.Fatalf("候选密钥过少，实际 %d 条", len(cands))
	}
}

// 每条候选密钥都必须能解开用它加密的密文，且报告正确的规则名。
func TestDecryptPassword_RoundTripForEveryCandidate(t *testing.T) {
	creds := testCredentials()
	cands := KeyCandidates(creds)

	for i, cand := range cands {
		plaintext := "pw-" + cand.Label
		encoded := encryptXshell(t, plaintext, cand.Key)

		got, ok := DecryptPassword(encoded, cands)
		if !ok {
			t.Errorf("候选 %d (%s) 无法解开用它加密的密文", i, cand.Label)
			continue
		}
		if got.Plaintext != plaintext {
			t.Errorf("候选 %d 明文不符: got %q", i, got.Plaintext)
		}
		if got.Rule != cand.Label {
			t.Errorf("候选 %d 报告的规则不符: got %q want %q", i, got.Rule, cand.Label)
		}
	}
}

// 现代版本规则必须排在首位：实测 Xshell 8.1 用的就是
// SHA256(reverse(SID) + Windows账户名)，排首位让常见情况一次命中。
func TestKeyCandidates_ModernRuleFirst(t *testing.T) {
	cands := KeyCandidates(testCredentials())
	if len(cands) == 0 {
		t.Fatal("没有候选密钥")
	}
	want := "reverse(SID)+Windows账户名 (Xshell 7.1+/8.x)"
	if cands[0].Label != want {
		t.Errorf("首条候选应为 %q，实际 %q", want, cands[0].Label)
	}

	// 独立算出该规则应得的密钥，确认实现没有被反转范围写错。
	creds := testCredentials()
	reversed := reverseString(creds.SID) + creds.WindowsUser
	wantKey := sha256.Sum256([]byte(reversed))
	if hex.EncodeToString(cands[0].Key) != hex.EncodeToString(wantKey[:]) {
		t.Errorf("首条候选密钥与 SHA256(reverse(SID)+账户名) 不一致")
	}
}

// 密钥里的账户名是 Windows 账户名，不是会话里的 SSH 登录名。
// 用 SSH 登录名代入必须解不开——这是实测踩过的坑，固化成回归用例。
func TestDecryptPassword_RejectsSSHLoginNameAsKeyMaterial(t *testing.T) {
	creds := testCredentials()
	encoded := encryptXshell(t, "secret", sha256Sum("SSH-Login-Name-not-Windows-account"))

	if _, ok := DecryptPassword(encoded, KeyCandidates(creds)); ok {
		t.Error("用无关密钥加密的密文不应被判定为解密成功")
	}
}

// 主密码模式下密钥是 SHA256(主密码)，与 SID/账户名无关。
func TestDecryptPassword_MasterPasswordRule(t *testing.T) {
	cands := KeyCandidates(Credentials{MasterPassword: "hunter2"})
	encoded := encryptXshell(t, "master-protected", sha256Sum("hunter2"))

	got, ok := DecryptPassword(encoded, cands)
	if !ok || got.Plaintext != "master-protected" {
		t.Fatalf("主密码规则解密失败: ok=%v plaintext=%q", ok, got.Plaintext)
	}
	if got.Rule != "主密码（启用了 Xshell 主密码）" {
		t.Errorf("规则名不符: %q", got.Rule)
	}
}

// 未提供主密码时，主密码加密的密文必须判定为失败，而不是给出乱码。
func TestDecryptPassword_MasterPasswordMissingFails(t *testing.T) {
	encoded := encryptXshell(t, "master-protected", sha256Sum("hunter2"))
	if got, ok := DecryptPassword(encoded, KeyCandidates(testCredentials())); ok {
		t.Errorf("缺少主密码时不应解密成功，却得到 %q", got.Plaintext)
	}
}

// <5.1 的密文没有尾部校验和，走固定密钥 + 可读文本启发式判定。
func TestDecryptPassword_LegacyShortCiphertext(t *testing.T) {
	legacyKey := md5.Sum([]byte(legacyKeySeed))
	cipher, err := rc4.NewCipher(legacyKey[:])
	if err != nil {
		t.Fatalf("rc4: %v", err)
	}
	plaintext := "pw"
	blob := make([]byte, len(plaintext))
	cipher.XORKeyStream(blob, []byte(plaintext))
	encoded := base64.StdEncoding.EncodeToString(blob)

	got, ok := DecryptPassword(encoded, KeyCandidates(testCredentials()))
	if !ok {
		t.Fatal("短密文应走 <5.1 的固定密钥路径并成功")
	}
	if got.Plaintext != plaintext {
		t.Errorf("明文不符: %q", got.Plaintext)
	}
}

func TestDecryptPassword_RejectsInvalidInput(t *testing.T) {
	cands := KeyCandidates(testCredentials())
	for _, encoded := range []string{"", "   ", "not-base64!!!", "c2hvcnQ="} {
		if _, ok := DecryptPassword(encoded, cands); ok {
			t.Errorf("输入 %q 不应被判定为解密成功", encoded)
		}
	}
	if _, ok := DecryptPassword("YWJj", nil); ok {
		t.Error("没有候选密钥时不应成功")
	}
}

func TestMaskSID(t *testing.T) {
	got := MaskSID("S-1-5-21-111-222-333-1001")
	if got == "" || got == "S-1-5-21-111-222-333-1001" {
		t.Errorf("SID 应被部分遮蔽，实际 %q", got)
	}
	if got[len(got)-5:] != "-1001" {
		t.Errorf("末段 RID 应保留，实际 %q", got)
	}
	if MaskSID("") != "" {
		t.Error("空 SID 应返回空串")
	}
}

// 真实机器端到端：读本机 Xshell 会话目录，确认带密码的会话能实际解密。
//
// 这是唯一能证明密钥规则正确的验证——只看"导入了几条"无法发现解密失败。
// 本机没有 Xshell 会话时跳过，因此不影响其他环境。
func TestRealMachineXshellSessionsDecrypt(t *testing.T) {
	dirs := DiscoverXshellSessionDirs()
	if len(dirs) == 0 {
		t.Skip("本机未检测到 Xshell 会话目录")
	}
	creds, err := LocalCredentials()
	if err != nil {
		t.Fatalf("取本机凭据失败: %v", err)
	}
	if creds.SID == "" {
		t.Skip("本机取不到 Windows SID")
	}
	t.Logf("本机凭据: SID=%s 账户名=%s", MaskSID(creds.SID), creds.WindowsUser)

	records, warnings, err := ParsePath(dirs[0], ParseOptions{Credentials: creds, DecryptPassword: true})
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", dirs[0], err)
	}
	t.Logf("会话目录 %s：解析到 %d 条会话", dirs[0], len(records))
	for _, w := range warnings {
		t.Logf("警告: %s", w)
	}

	encrypted, decrypted := 0, 0
	for _, rec := range records {
		if !rec.PasswordEncrypted {
			continue
		}
		encrypted++
		if rec.Password != "" {
			decrypted++
		}
		// 只报告是否成功与所用规则，绝不输出明文密码。
		t.Logf("会话 %q：解密=%v 规则=%s", rec.Name, rec.Password != "", rec.PasswordRule)
	}

	if encrypted > 0 && decrypted == 0 {
		t.Fatalf("本机 %d 条带密码的会话全部解密失败，密钥规则不正确", encrypted)
	}
	if encrypted == 0 {
		t.Log("本机会话均未保存密码，无法验证解密路径")
	}
}

func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
