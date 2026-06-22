package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticRoot embed.FS

// AssetsHandler serves everything under /static/* from the embedded FS.
// It sets long-lived cache headers and falls back to 404 for missing files.
func AssetsHandler() http.Handler {
	sub, err := fs.Sub(staticRoot, "static")
	if err != nil {
		panic(err) // embed layout is compile-time fixed; panic is safe
	}
	files := http.FS(sub)
	fs := http.FileServer(files)
	return http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		if !assetExists(sub, r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
		fs.ServeHTTP(w, r)
	}))
}

func assetExists(fsys fs.FS, name string) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
