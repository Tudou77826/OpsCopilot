package main

import "testing"

// 完成消息里的传输方式必须是用户可读文案，内部代号（sftp(login) 等）只留给日志。
func TestFriendlyTransportLabel(t *testing.T) {
	cases := map[string]string{
		"sftp":             "SFTP · 密码登录",
		"sftp(login)":      "SFTP · 密码登录",
		"sftp(key)":        "SFTP · 密钥登录",
		"sftp(root)":       "SFTP · Root 直连",
		"sftp(root-relay)": "SFTP · Root 中转",
		"scp(login)":       "SCP · 兼容模式",
		"scp(fallback)":    "SCP · 兼容模式",
		"scp(root)":        "SCP · Root 直连",
		// relay 自报的用户可读标签原样透传
		"Base64 直传": "Base64 直传",
		"Root 中转":   "Root 中转",
	}
	for code, want := range cases {
		if got := friendlyTransportLabel(code); got != want {
			t.Errorf("friendlyTransportLabel(%q) = %q, want %q", code, got, want)
		}
	}
}
