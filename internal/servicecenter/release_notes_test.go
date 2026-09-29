package servicecenter

import (
	"encoding/json"
	protocol "opscopilot/pkg/servicecenter"
	"strings"
	"testing"
	"time"
)

func TestReleaseNotesRenderMarkdownWithoutActiveContent(t *testing.T) {
	body := "# v1.10.5\n\n## 修复\n\n- **侧边栏**不再白屏，使用 `Ctrl+K`。\n\n## What's Changed\n\n* 修复记录 https://github.com/Tudou77826/OpsCopilot/pull/77\n\n|项目|结果|\n|---|---|\n|连接|成功|\n\n![tracking](https://evil.example/track)\n\n<img src=\"https://evil.example/raw\" onerror=\"alert(1)\">\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert(1))\n\n```sh\necho '<script>'\n```\n"
	html := renderReleaseNotes(body)
	for _, want := range []string{"<h2>修复</h2>", "<ul>", "<strong>侧边栏</strong>", "<code>Ctrl+K</code>", "<table>", "https://github.com/Tudou77826/OpsCopilot/pull/77", "&lt;script&gt;"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	for _, bad := range []string{"<img", "<script", "javascript:", "onerror=", "evil.example"} {
		if strings.Contains(html, bad) {
			t.Fatalf("active content %q in %s", bad, html)
		}
	}
}

func TestReleaseListAddsRenderedNotesAndPreservesUpdaterShape(t *testing.T) {
	s := testServer(t)
	release := protocol.ReleaseInfo{TagName: "v1.10.5", Name: "v1.10.5", Body: "## 修复\n\n- 白屏问题已修复", PublishedAt: time.Now()}
	if err := s.put("releases", release.TagName, release); err != nil {
		t.Fatal(err)
	}
	if err := s.put("state", "latest", release); err != nil {
		t.Fatal(err)
	}
	var rows []portalRelease
	w := request(s, "GET", "/api/v1/releases", nil, false)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if rows[0].Body != release.Body || !strings.Contains(rows[0].BodyHTML, "<li>白屏问题已修复</li>") {
		t.Fatal("missing rendered or original notes")
	}
	latest := request(s, "GET", "/api/v1/releases/latest", nil, false)
	if strings.Contains(latest.Body.String(), "body_html") {
		t.Fatal("updater shape changed")
	}
}
