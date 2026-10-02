package api

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const notBuiltPage = `<!doctype html>
<html lang="de"><head><meta charset="utf-8"><title>dnsdeck</title>
<style>body{font-family:system-ui,sans-serif;background:#0a0a0a;color:#e5e5e5;
display:grid;place-items:center;min-height:100vh;margin:0}code{color:#a3e635}</style>
</head><body><div><h1>dnsdeck</h1>
<p>Das Frontend wurde nicht gebaut. Bitte <code>cd web &amp;&amp; npm run build</code>
ausführen und den Server neu bauen.</p><p>API: <code>/api</code>, Healthcheck: <code>/healthz</code></p>
</div></body></html>`

// spaHandler liefert Dateien aus dem Frontend-Build; unbekannte Pfade erhalten
// index.html, damit das clientseitige Routing funktioniert.
func spaHandler(static fs.FS) http.Handler {
	fileServer := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if st, err := fs.Stat(static, name); err == nil && !st.IsDir() {
				switch {
				case strings.HasPrefix(name, "assets/"):
					// Vite versieht Assets mit Content-Hash.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				case name == "sw.js":
					// Der Browser soll Updates des Service Workers sofort sehen.
					w.Header().Set("Cache-Control", "no-cache")
				case name == "manifest.webmanifest":
					w.Header().Set("Cache-Control", "no-cache")
					w.Header().Set("Content-Type", "application/manifest+json")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := fs.ReadFile(static, "index.html")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if errors.Is(err, fs.ErrNotExist) {
			w.Write([]byte(notBuiltPage))
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Write(index)
	})
}
