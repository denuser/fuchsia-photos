// Package web serves the frontend build embedded in the binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler returns a handler for the embedded frontend build.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	return spa(sub), nil
}

// spa serves files from fsys and falls back to index.html, so that the
// frontend router can handle paths that are not files.
func spa(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if info, err := fs.Stat(fsys, name); name == "" || err != nil || info.IsDir() {
			name = "index.html"
		}
		if _, err := fs.Stat(fsys, name); err != nil {
			http.Error(w, "frontend is not built", http.StatusNotFound)
			return
		}

		// Vite puts a content hash in the names of everything under assets/.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFileFS(w, r, fsys, name)
	})
}
