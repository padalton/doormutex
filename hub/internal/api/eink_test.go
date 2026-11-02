package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestEink(t *testing.T) {
	f := newFixture(t)
	resp, bin := f.do(t, http.MethodGet, "/api/v1/eink.bin", "")
	if resp.StatusCode != http.StatusOK || len(bin) != 5000 || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("bin: %d %d bytes %s", resp.StatusCode, len(bin), resp.Header.Get("Content-Type"))
	}
	etag := resp.Header.Get("ETag")
	// Unchanged frame: 304 for the device.
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/v1/eink.bin", nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("If-None-Match", etag)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match: %d", r2.StatusCode)
	}
	// A new message changes the frame.
	if resp, _ := f.do(t, http.MethodPut, "/api/v1/message", `{"text":"Back at 3"}`); resp.StatusCode != http.StatusOK {
		t.Fatal("message")
	}
	if resp, _ := f.do(t, http.MethodGet, "/api/v1/eink.bin", ""); resp.Header.Get("ETag") == etag {
		t.Fatal("frame did not change with the message")
	}
	// PNG preview with battery; language from the settings or the query.
	resp, b := f.do(t, http.MethodGet, "/api/v1/eink.png?battery=40&charging=true", "")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(string(b), "\x89PNG") {
		t.Fatalf("png: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.HasSuffix(resp.Header.Get("ETag"), `-png"`) {
		t.Errorf("png etag: %s", resp.Header.Get("ETag"))
	}
	_, en := f.do(t, http.MethodGet, "/api/v1/eink.bin", "")
	if resp, b := f.do(t, http.MethodPut, "/api/v1/settings", `{"eink_language":"de"}`); resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"eink_language":"de"`) {
		t.Fatalf("eink_language de: %d %s", resp.StatusCode, b)
	}
	_, de := f.do(t, http.MethodGet, "/api/v1/eink.bin", "")
	_, enQuery := f.do(t, http.MethodGet, "/api/v1/eink.bin?lang=en", "")
	if string(en) == string(de) || string(en) != string(enQuery) {
		t.Error("language: setting or ?lang= not applied")
	}
	// The offline screen and the teen's own frame.
	if resp, b := f.do(t, http.MethodGet, "/api/v1/eink.bin?screen=offline", ""); resp.StatusCode != http.StatusOK || len(b) != 5000 {
		t.Fatalf("offline: %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, http.MethodGet, "/api/v1/eink.png?profile=teen", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("teen: %d", resp.StatusCode)
	}
	for _, bad := range []string{"?battery=101", "?battery=x", "?lang=xx", "?screen=boot"} {
		if resp, _ := f.do(t, http.MethodGet, "/api/v1/eink.png"+bad, ""); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, resp.StatusCode)
		}
	}
	if resp, _ := f.do(t, http.MethodPut, "/api/v1/settings", `{"eink_language":"xx"}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("eink_language xx: %d", resp.StatusCode)
	}
}
