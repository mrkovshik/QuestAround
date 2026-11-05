// Package webui раздаёт собранный фронтенд (web/ → dist/), вшитый в бинарник.
//
// Сборка: `make web` (нужен Node) кладёт файлы в dist/app.
// Без неё отдаётся заглушка с подсказкой.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const placeholder = `<!doctype html><meta charset="utf-8"><title>Фронтенд не собран</title>
<p style="font:16px system-ui;margin:2rem">Фронтенд не собран: выполните <code>make web</code> и пересоберите сервер.</p>`

// Handler отдаёт статику, а для остальных путей — index.html (SPA).
func Handler() http.Handler {
	dist, _ := fs.Sub(distFS, "dist/app")
	files := http.FileServer(http.FS(dist))
	_, err := fs.Stat(dist, "index.html")
	built := err == nil

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !built {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(placeholder))
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		switch {
		case p == "sw.js":
			// Service worker должен обновляться сразу.
			w.Header().Set("Cache-Control", "no-cache")
		case strings.HasPrefix(p, "assets/"):
			// Имена с хэшем содержимого — можно кэшировать навсегда.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if p != "" {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(p, "assets/") || path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
		}
		// Маршруты SPA (/g/КОД, /g/КОД/join/ИНВАЙТ) — index.html.
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		files.ServeHTTP(w, r2)
	})
}
