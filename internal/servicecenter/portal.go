package servicecenter

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed portal
var portalFiles embed.FS

func (s *Server) portalRoutes() {
	assets, err := fs.Sub(portalFiles, "portal")
	if err != nil {
		panic(err)
	}
	s.mux.Handle("GET /portal/", http.StripPrefix("/portal/", http.FileServer(http.FS(assets))))
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		html, err := portalFiles.ReadFile("portal/index.html")
		if err != nil {
			http.Error(w, "页面读取失败", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
}
