package sessionimport

import (
	"os"
	"os/user"
	"strings"
)

// LocalCredentials 返回本机凭据：当前 Windows 账户的 SID 与账户名。
//
// 同机导入是本能力的前提。Xshell 5.3+ 的会话密码密钥由"导出时的账户 SID +
// Windows 账户名"派生，换机器或换账户就无法解开——这是密钥派生方式决定的，
// 实现上无法绕过，只能提示用户补充源机器 SID 或使用主密码。
//
// 实现要点（已在本机实测核对）：
//   - os/user.Current().Uid 在 Windows 上就是 SID 字符串（形如 S-1-5-21-...），
//     不需要 cgo 或 advapi32 调用。
//   - 密钥里的账户名是 Windows 账户名（如 15802），不是会话里的 SSH 登录名
//     （如 root）。用 SSH 登录名代入必然解不开。
func LocalCredentials() (Credentials, error) {
	current, err := user.Current()
	if err != nil {
		return Credentials{}, err
	}

	sid := current.Uid
	if !strings.HasPrefix(sid, "S-1-") {
		sid = "" // 非 Windows 平台或取不到 SID
	}

	name := os.Getenv("USERNAME")
	if name == "" {
		name = current.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
	}

	return Credentials{SID: sid, WindowsUser: name}, nil
}

// MaskSID 遮蔽 SID 中段，用于在 UI 上展示"已检测到本机凭据"而不暴露完整标识。
func MaskSID(sid string) string {
	if sid == "" {
		return ""
	}
	lastDash := strings.LastIndex(sid, "-")
	if lastDash <= 0 {
		return sid
	}
	prefix, rid := sid[:lastDash], sid[lastDash:]
	if parts := strings.Split(prefix, "-"); len(parts) > 4 {
		prefix = strings.Join(parts[:4], "-") + "-****-****"
	}
	return prefix + rid
}
