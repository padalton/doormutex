// Package web serves the web UI: a small installable web app (PWA) for
// phones and desktops, embedded into the hub binary. It talks to the hub
// only through the public API with the token the user entered.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var static embed.FS

// csp allows nothing but the hub itself: no inline scripts, no CDNs.
const csp = "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; connect-src 'self'; " +
	"manifest-src 'self'; worker-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// Handler serves the embedded files.
func Handler() http.Handler {
	sub, _ := fs.Sub(static, "static")
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		switch {
		case r.URL.Path == "/sw.js":
			// The service worker controls the whole site and must not be cached long.
			h.Set("Cache-Control", "no-cache")
			h.Set("Service-Worker-Allowed", "/")
		case strings.HasSuffix(r.URL.Path, ".webmanifest"):
			h.Set("Content-Type", "application/manifest+json")
			h.Set("Cache-Control", "no-cache")
		default:
			h.Set("Cache-Control", "no-cache") // small files; always revalidate so updates show up
		}
		files.ServeHTTP(w, r)
	})
}
