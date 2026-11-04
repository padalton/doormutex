package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/padalton/doormutex/hub/internal/status"
	"github.com/padalton/doormutex/hub/internal/store"
)

// as sends a request with the given token.
func (f *fixture) as(t *testing.T, token, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// TestReadOnlyToken walks every route with a read-only token: the read
// routes answer, everything else is refused before the handler runs.
func TestReadOnlyToken(t *testing.T) {
	f := newFixture(t)
	tok, err := f.store.CreateToken("couch", nil, store.RoleRead)
	if err != nil {
		t.Fatal(err)
	}
	before := f.svc.State()
	for _, route := range f.api.Routes() {
		method, path, _ := strings.Cut(route, " ")
		if path == "/api/v1/events" {
			continue // a stream; TestEventStreamAllProfiles uses a read token
		}
		path = strings.NewReplacer("{source}", "spec-test", "{account}", "private", "{name}", "test").Replace(path)
		resp, b := f.as(t, tok, method, path, "{}")
		switch {
		case readRoutes[route] && resp.StatusCode != http.StatusOK:
			t.Errorf("%s: read route answered %d %s", route, resp.StatusCode, b)
		case !readRoutes[route] && (resp.StatusCode != http.StatusForbidden || !strings.Contains(string(b), "read-only token")):
			t.Errorf("%s: answered %d %s, want 403 read-only token", route, resp.StatusCode, b)
		}
	}
	if after := f.svc.State(); after.Status != before.Status || after.Override != nil {
		t.Errorf("a read-only token changed the state: %+v", after)
	}
	_, b := f.as(t, tok, http.MethodGet, "/api/v1/me", "")
	var me Me
	if err := json.Unmarshal(b, &me); err != nil || me.Name != "couch" || me.Role != store.RoleRead || me.Admin ||
		!slices.Equal(me.Profiles, []string{"office", "teen"}) {
		t.Fatalf("me = %s", b)
	}
}

func TestTokenProfileList(t *testing.T) {
	f := newFixture(t)
	tok, _ := f.store.CreateToken("kitchen", []string{"teen", "office"}, store.RoleRead)
	// Default is the token's first profile, both are reachable.
	_, b := f.as(t, tok, http.MethodGet, "/api/v1/status", "")
	if st := decodeState(t, b); st.Status != status.Away {
		t.Fatalf("default profile should be teen (away): %+v", st)
	}
	if resp, _ := f.as(t, tok, http.MethodGet, "/api/v1/status?profile=office", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("office: %d", resp.StatusCode)
	}
	_, b = f.as(t, tok, http.MethodGet, "/api/v1/profiles", "")
	var list []ProfileInfo
	if err := json.Unmarshal(b, &list); err != nil || len(list) != 2 {
		t.Fatalf("profiles: %s", b)
	}
	one, _ := f.store.CreateToken("teen-only", []string{"teen"}, store.RoleRead)
	if resp, _ := f.as(t, one, http.MethodGet, "/api/v1/status?profile=office", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("teen-only reached office: %d", resp.StatusCode)
	}
}

func TestTokenAdmin(t *testing.T) {
	f := newFixture(t)

	// Create a read-only token for one profile, as the web UI does.
	resp, b := f.do(t, http.MethodPost, "/api/v1/tokens", `{"name":"couch","profiles":["teen"],"hub":"https://doormutex.example.com/"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, b)
	}
	var nt NewToken
	if err := json.Unmarshal(b, &nt); err != nil {
		t.Fatal(err)
	}
	if nt.Role != store.RoleRead || !slices.Equal(nt.Profiles, []string{"teen"}) || !strings.HasPrefix(nt.Token, "dm_") {
		t.Fatalf("new token: %s", b)
	}
	pair, err := url.Parse(nt.PairURL)
	if err != nil || pair.Scheme != "doormutex" || pair.Host != "pair" || pair.Query().Get("hub") != "https://doormutex.example.com" ||
		pair.Query().Get("token") != nt.Token || pair.Query().Get("name") != "couch" {
		t.Fatalf("pair url %q", nt.PairURL)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(nt.QR, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	if img, err := png.Decode(bytes.NewReader(raw)); err != nil || img.Bounds().Dx() < 200 {
		t.Fatalf("qr: %v", err)
	}
	// The new token works and is read-only.
	if resp, _ := f.as(t, nt.Token, http.MethodGet, "/api/v1/status", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("new token: %d", resp.StatusCode)
	}
	if resp, _ := f.as(t, nt.Token, http.MethodPut, "/api/v1/override", `{"status":"dnd","mode":"indefinite"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("new token could write: %d", resp.StatusCode)
	}

	// Without hub the link uses the request's host.
	_, b = f.do(t, http.MethodPost, "/api/v1/tokens", `{"name":"tablet","role":"full"}`)
	if err := json.Unmarshal(b, &nt); err != nil || nt.Role != store.RoleFull || len(nt.Profiles) != 0 ||
		!strings.Contains(nt.PairURL, url.QueryEscape(f.srv.URL)) {
		t.Fatalf("tablet: %s", b)
	}

	for _, c := range []struct{ body, want string }{
		{`{"name":"couch"}`, "409"},
		{`{"name":"x","profiles":["nobody"]}`, "400"},
		{`{"name":"x","role":"root"}`, "400"},
		{`{"name":"Bad Name"}`, "400"},
		{`{"name":"x","hub":"ftp://example.com"}`, "400"},
		{`{"name":"x","hub":"https://example.com/?token=1"}`, "400"},
	} {
		if resp, b := f.do(t, http.MethodPost, "/api/v1/tokens", c.body); resp.Status[:3] != c.want {
			t.Errorf("%s: %d %s, want %s", c.body, resp.StatusCode, b, c.want)
		}
	}

	_, b = f.do(t, http.MethodGet, "/api/v1/tokens", "")
	var list []store.TokenInfo
	if err := json.Unmarshal(b, &list); err != nil || len(list) != 3 || strings.Contains(string(b), "dm_") {
		t.Fatalf("list: %s", b)
	}

	// Only a full token for all profiles may manage tokens.
	scoped, _ := f.store.CreateToken("teen-phone", []string{"teen"}, store.RoleFull)
	if resp, _ := f.as(t, scoped, http.MethodGet, "/api/v1/tokens", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("scoped token listed tokens: %d", resp.StatusCode)
	}
	if resp, _ := f.as(t, scoped, http.MethodPost, "/api/v1/tokens", `{"name":"sneaky"}`); resp.StatusCode != http.StatusForbidden {
		t.Errorf("scoped token created a token: %d", resp.StatusCode)
	}

	// Revoke: not the token of the request itself, then the other one.
	if resp, _ := f.do(t, http.MethodDelete, "/api/v1/tokens/test", ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("revoked itself: %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, http.MethodDelete, "/api/v1/tokens/nobody", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown: %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, http.MethodDelete, "/api/v1/tokens/teen-phone", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %d", resp.StatusCode)
	}
	if resp, _ := f.as(t, scoped, http.MethodGet, "/api/v1/status", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token still works: %d", resp.StatusCode)
	}
}

func TestEventStreamAllProfiles(t *testing.T) {
	f := newFixture(t)
	read, _ := f.store.CreateToken("couch", nil, store.RoleRead)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.srv.URL+"/api/v1/events?profile=*", nil)
	req.Header.Set("Authorization", "Bearer "+read)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	next := func() ProfileState {
		t.Helper()
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var ps ProfileState
				if err := json.Unmarshal([]byte(v), &ps); err != nil {
					t.Fatalf("decode %s: %v", v, err)
				}
				return ps
			}
		}
		t.Fatalf("stream ended: %v", sc.Err())
		return ProfileState{}
	}
	// The current state of every profile first, in configuration order …
	if a, b := next(), next(); a.Profile != "office" || a.State.Status != status.Working || b.Profile != "teen" || b.State.Status != status.Away {
		t.Fatalf("first events: %+v %+v", a, b)
	}
	// … then each change, tagged with its profile.
	f.do(t, http.MethodPut, "/api/v1/override?profile=teen", `{"status":"dnd","mode":"indefinite"}`)
	if ps := next(); ps.Profile != "teen" || ps.State.Status != status.DND {
		t.Fatalf("change: %+v", ps)
	}
}

func TestEventStreamAllProfilesScoped(t *testing.T) {
	f := newFixture(t)
	f.api.lifetime = 300 * time.Millisecond
	tok, _ := f.store.CreateToken("teen-only", []string{"teen"}, store.RoleRead)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.srv.URL+"/api/v1/events?profile=*&access_token="+tok, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if n := strings.Count(string(b), "event: state"); n != 1 || !strings.Contains(string(b), `"profile":"teen"`) {
		t.Fatalf("scoped stream: %s", b)
	}
}
