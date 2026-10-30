package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServesTheApp(t *testing.T) {
	srv := httptest.NewServer(Handler())
	defer srv.Close()
	for path, want := range map[string]string{
		"/":                     "<title>doormutex</title>",
		"/app.js":               "/api/v1/profiles",
		"/manifest.webmanifest": `"display": "standalone"`,
		"/sw.js":                "network only",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), want) {
			t.Errorf("%s: %d, missing %q", path, resp.StatusCode, want)
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
			t.Errorf("%s: CSP = %q", path, csp)
		}
		if path == "/manifest.webmanifest" && resp.Header.Get("Content-Type") != "application/manifest+json" {
			t.Errorf("manifest type = %s", resp.Header.Get("Content-Type"))
		}
	}
	for _, icon := range []string{"/icon.svg", "/icon-192.png", "/icon-512.png", "/apple-touch-icon.png"} {
		resp, err := http.Get(srv.URL + icon)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", icon, resp.StatusCode)
		}
	}
}

// The page must not depend on inline code: the CSP forbids it.
func TestNoInlineScriptsOrStyles(t *testing.T) {
	b, err := static.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	for _, bad := range []string{"<script>", "<style", " style=", "onclick=", "javascript:"} {
		if strings.Contains(html, bad) {
			t.Errorf("index.html contains %q", bad)
		}
	}
}
