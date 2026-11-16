package google

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRemoteLogin(t *testing.T) {
	_, srv := setup(t)
	store := &memStore{m: map[string][]byte{}}
	p := New(Options{OAuth: oauthConf(srv), Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "private"}, {Name: "work"}}})

	for _, bad := range []string{"https://127.0.0.1:1/", "http://localhost:1/", "http://127.0.0.1/", "http://127.0.0.1:1/?x=1", "http://evil.example:80/"} {
		if _, err := p.StartLogin("private", bad); !errors.Is(err, ErrBadRedirect) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, err := p.StartLogin("bank", "http://127.0.0.1:1/"); !errors.Is(err, ErrUnknownAccount) {
		t.Errorf("unknown account: %v", err)
	}

	auth, err := p.StartLogin("work", "http://127.0.0.1:43117/")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if q.Get("redirect_uri") != "http://127.0.0.1:43117/" || q.Get("code_challenge_method") != "S256" ||
		q.Get("access_type") != "offline" || q.Get("scope") != Scope || q.Get("client_secret") != "" {
		t.Fatalf("auth URL %s", auth)
	}
	state := q.Get("state")
	if err := p.CompleteLogin(context.Background(), "private", state, "the-code"); !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("state for another account: %v", err)
	}
	// That attempt used the state up: start again.
	auth, _ = p.StartLogin("work", "http://127.0.0.1:43117/")
	u, _ = url.Parse(auth)
	state = u.Query().Get("state")
	if err := p.CompleteLogin(context.Background(), "work", state, "the-code"); err != nil {
		t.Fatal(err)
	}
	var tok oauth2.Token
	if err := store.Get(TokenKey("office", "work"), &tok); err != nil || tok.RefreshToken != "new" {
		t.Fatalf("token not stored: %+v %v", tok, err)
	}
	if err := p.CompleteLogin(context.Background(), "work", state, "the-code"); !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("state reused: %v", err)
	}
	if st := p.Status(); !st[1].Connected {
		t.Errorf("status = %+v", st)
	}
}

func TestRelayLogin(t *testing.T) {
	f, srv := setup(t)
	store := &memStore{m: map[string][]byte{}}
	web := oauthConf(srv)
	web.ClientID, web.ClientSecret = "web-id", "web-secret"
	const relay = "https://relay.example/oauth/"
	p := New(Options{OAuth: oauthConf(srv), Web: web, Relay: relay, Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "private"}, {Name: "work"}}})
	if p.LoginRelay() != relay {
		t.Fatalf("relay = %q", p.LoginRelay())
	}
	for _, bad := range []string{"https://relay.example/oauth", "https://relay.example/oauth/?x=1", "https://evil.example/oauth/"} {
		if _, err := p.StartLogin("work", bad); !errors.Is(err, ErrBadRedirect) {
			t.Errorf("%s: %v", bad, err)
		}
	}

	auth, err := p.StartLogin("work", relay)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(auth)
	q := u.Query()
	if q.Get("client_id") != "web-id" || q.Get("redirect_uri") != relay || q.Get("code_challenge_method") != "S256" || q.Get("client_secret") != "" {
		t.Fatalf("auth URL %s", auth)
	}
	if err := p.CompleteLogin(context.Background(), "work", q.Get("state"), "the-code"); err != nil {
		t.Fatal(err)
	}
	var st storedToken
	if err := store.Get(TokenKey("office", "work"), &st); err != nil || st.Client != ClientWeb || st.RefreshToken != "new" {
		t.Fatalf("stored token: %+v %v", st, err)
	}

	// Refreshing uses the client that issued the token: web for work,
	// desktop for private (a token stored before there was a web client).
	expired := time.Now().Add(-time.Hour)
	_ = store.Put(TokenKey("office", "private"), oauth2.Token{AccessToken: "old", RefreshToken: "private", Expiry: expired})
	_ = store.Put(TokenKey("office", "work"), storedToken{Token: oauth2.Token{AccessToken: "old", RefreshToken: "work", Expiry: expired}, Client: ClientWeb})
	f.mu.Lock()
	f.clients = nil
	f.mu.Unlock()
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, vienna)
	if _, err := p.Events(context.Background(), from, from.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	clients := strings.Join(f.clients, ",")
	f.mu.Unlock()
	if clients != "id,web-id" {
		t.Errorf("refresh clients = %s, want id,web-id", clients)
	}
	// The refreshed token keeps its mark.
	if err := store.Get(TokenKey("office", "work"), &st); err != nil || st.Client != ClientWeb || st.AccessToken != "access-work" {
		t.Errorf("refreshed token: %+v %v", st, err)
	}
}

func TestOnlyOneClient(t *testing.T) {
	_, srv := setup(t)
	store := &memStore{m: map[string][]byte{}}
	// Desktop client only: no relay.
	p := New(Options{OAuth: oauthConf(srv), Relay: "https://relay.example/oauth/", Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "work"}}})
	if p.LoginRelay() != "" {
		t.Errorf("relay without web client: %q", p.LoginRelay())
	}
	if _, err := p.StartLogin("work", "https://relay.example/oauth/"); !errors.Is(err, ErrBadRedirect) {
		t.Errorf("relay without web client: %v", err)
	}

	// Web client only: no loopback, and a desktop token needs a new sign-in.
	p = New(Options{Web: oauthConf(srv), Relay: "https://relay.example/oauth/", Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "work"}}})
	if _, err := p.StartLogin("work", "http://127.0.0.1:43117/"); !errors.Is(err, ErrNoClient) {
		t.Errorf("loopback without desktop client: %v", err)
	}
	_ = store.Put(TokenKey("office", "work"), oauth2.Token{AccessToken: "old", RefreshToken: "work", Expiry: time.Now().Add(-time.Hour)})
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, vienna)
	if _, err := p.Events(context.Background(), from, from.Add(24*time.Hour)); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("desktop token without desktop client: %v", err)
	}
	if st := p.Status(); !st[0].LoginExpired {
		t.Errorf("status = %+v", st)
	}
}
