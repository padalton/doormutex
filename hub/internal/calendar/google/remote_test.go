package google

import (
	"context"
	"errors"
	"net/url"
	"testing"

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
