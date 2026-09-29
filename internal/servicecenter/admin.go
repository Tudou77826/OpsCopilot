package servicecenter

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed admin
var adminFiles embed.FS

func (s *Server) adminPages() {
	assets, err := fs.Sub(adminFiles, "admin")
	if err != nil {
		panic(err)
	}
	s.mux.Handle("GET /admin-assets/", http.StripPrefix("/admin-assets/", http.FileServer(http.FS(assets))))
	s.mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		html, err := adminFiles.ReadFile("admin/index.html")
		if err != nil {
			http.Error(w, "页面读取失败", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(html)
	})
}
