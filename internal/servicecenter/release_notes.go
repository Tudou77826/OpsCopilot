package servicecenter

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
	protocol "opscopilot/pkg/servicecenter"
)

type portalRelease struct {
	protocol.ReleaseInfo
	BodyHTML string `json:"body_html"`
}

// Render downloaded Markdown locally. Raw HTML and images never enter the page.
// Images are excluded even when hosted internally, so notes cannot initiate requests.
func renderReleaseNotes(body string) string {
	if len(body) > 128*1024 {
		body = body[:128*1024]
	}
	source := []byte(body)
	markdown := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify))
	document := markdown.Parser().Parse(text.NewReader(source))
	var remove []ast.Node
	ast.Walk(document, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Image, *ast.RawHTML, *ast.HTMLBlock:
			remove = append(remove, n)
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			u, err := url.Parse(string(v.Destination))
			if err != nil || (u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http") || u.User != nil || strings.HasPrefix(string(v.Destination), "//") {
				v.Destination = nil
			}
		}
		return ast.WalkContinue, nil
	})
	for _, n := range remove {
		if parent := n.Parent(); parent != nil {
			parent.RemoveChild(parent, n)
		}
	}
	var output bytes.Buffer
	if markdown.Renderer().Render(&output, source, document) != nil {
		return ""
	}
	return output.String()
}
