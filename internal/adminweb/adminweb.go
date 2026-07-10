package adminweb

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

func Register(mux *http.ServeMux) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("GET /admin/", noCache(http.StripPrefix("/admin/", fileServer)))
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusFound)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
}
