package assistantcontrol

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDisplayExcerptFiltersCommonCredentialFormsBeforeClipping(t *testing.T) {
	text := "\x1b[31m密码结果\x1b[0m\n{\"password\":\"hidden-1\"}\nTOKEN='hidden-2'\nBearer hidden-3\nhttps://u:hidden-4@host/path\n-----BEGIN PRIVATE KEY-----\nhidden-5\n-----END PRIVATE KEY-----"
	got := DisplayExcerpt(text, 4096)
	for _, value := range []string{"hidden-1", "hidden-2", "hidden-3", "hidden-4", "hidden-5", "\x1b"} {
		if strings.Contains(got, value) {
			t.Fatal("display excerpt retained credential/control", value)
		}
	}
	got = DisplayExcerpt(strings.Repeat("中文", 5000), 100)
	if !utf8.ValidString(got) || !strings.Contains(got, "已截断") {
		t.Fatal("invalid clipped display")
	}
}
