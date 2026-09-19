package filetransfer

import (
	"context"
	"io"
	"testing"
	"time"
)

// 伪远端 shell：读到命令后按脚本回复，模拟真实 su 会话的 PTY 行为。
func fakeShell(t *testing.T, reply string) (stdin io.Writer, stdout io.Reader) {
	t.Helper()
	cmdR, cmdW := io.Pipe() // 我方写入命令 → 伪 shell 读取
	outR, outW := io.Pipe() // 伪 shell 写出输出 → 我方读取
	go func() {
		buf := make([]byte, 512)
		_, _ = cmdR.Read(buf) // 消费掉 id -u 命令
		_, _ = outW.Write([]byte(reply))
		_ = outW.Close()
		_ = cmdR.Close()
	}()
	return cmdW, outR
}

// 业务场景（#75）：跳板机 + sopuser 提权 root。su 密码错误时远端回落到
// 登录用户的 "$ " 提示符——旧实现仅凭提示符判定成功，整个 relay 静默以
// sopuser 身份运行，与后续重建的 root 会话形成"列表/上传权限不一致"。
func TestVerifyRootUID_RejectsSuFailureFallback(t *testing.T) {
	stdin, stdout := fakeShell(t, "su: Authentication failure\r\n"+
		"sopuser@biz-01:~$ \r\n"+
		"1000\r\n"+ // id -u 实际是登录用户
		"__RELAY_OK__\r\n")
	if err := verifyRootUID(context.Background(), stdin, stdout); err == nil {
		t.Fatal("su 失败回落到登录用户时必须报错，不能静默降级为 sopuser 身份")
	}
}

func TestVerifyRootUID_AcceptsRootSession(t *testing.T) {
	stdin, stdout := fakeShell(t, "root@biz-01:~# \r\n0\r\n__RELAY_OK__\r\n")
	if err := verifyRootUID(context.Background(), stdin, stdout); err != nil {
		t.Fatalf("root 会话应通过验证: %v", err)
	}
}

func TestVerifyRootUID_RejectsFailMarker(t *testing.T) {
	stdin, stdout := fakeShell(t, "__RELAY_FAIL__\r\n")
	if err := verifyRootUID(context.Background(), stdin, stdout); err == nil {
		t.Fatal("命令失败标记必须报错")
	}
}

func TestContainsUIDZero(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"0\r\n__RELAY_OK__\r\n", true},
		{"root@host:~# 0\r\n", false}, // 与提示符同行不认：整行严格匹配，避免误判
		{"1000\r\n__RELAY_OK__\r\n", false},
		{"id -u && echo __RELAY_OK__\r\n0\r\n", true}, // 带命令回显
		{"", false},
		{"100\r\n", false},
	}
	for _, c := range cases {
		if got := containsUIDZero(c.raw); got != c.want {
			t.Errorf("containsUIDZero(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
	_ = time.Second
}
