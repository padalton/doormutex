package google

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"time"

	"golang.org/x/oauth2"
)

// Sign-in through a client on the desktop (the companion): the browser
// runs there, so Google's redirect to 127.0.0.1 reaches the client, which
// passes the code on. The hub keeps the PKCE verifier and does the
// exchange – the client secret and the refresh token never leave it.
//
//  1. client: listens on 127.0.0.1:PORT, calls StartLogin with that address
//  2. hub:    returns Google's sign-in URL; the client opens the browser
//  3. Google: redirects to 127.0.0.1:PORT with code and state
//  4. client: calls CompleteLogin with both; the hub stores the token

// Errors of the remote sign-in.
var (
	ErrUnknownAccount = errors.New("unknown Google account")
	ErrBadRedirect    = errors.New("redirect_uri must be http://127.0.0.1:PORT/")
	ErrLoginExpired   = errors.New("sign-in expired or unknown – start again")
)

// LoginTTL is how long a started sign-in can be completed.
const LoginTTL = 10 * time.Minute

type pendingLogin struct {
	account, redirect, verifier string
	expires                     time.Time
}

// StartLogin begins a sign-in for an account and returns Google's
// sign-in URL. redirectURI is the client's loopback address.
func (p *Provider) StartLogin(account, redirectURI string) (string, error) {
	if !slices.ContainsFunc(p.o.Accounts, func(a Account) bool { return a.Name == account }) {
		return "", fmt.Errorf("%w %q", ErrUnknownAccount, account)
	}
	if err := checkLoopback(redirectURI); err != nil {
		return "", err
	}
	conf := p.conf(redirectURI)
	state, verifier := random(), oauth2.GenerateVerifier()
	now := time.Now()
	p.mu.Lock()
	for s, l := range p.logins {
		if now.After(l.expires) {
			delete(p.logins, s)
		}
	}
	p.logins[state] = pendingLogin{account: account, redirect: redirectURI, verifier: verifier, expires: now.Add(LoginTTL)}
	p.mu.Unlock()
	return conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent")), nil
}

// CompleteLogin exchanges the code from Google's redirect and stores the
// account's token. A state can be used once.
func (p *Provider) CompleteLogin(ctx context.Context, account, state, code string) error {
	p.mu.Lock()
	l, ok := p.logins[state]
	delete(p.logins, state)
	p.mu.Unlock()
	if !ok || l.account != account || time.Now().After(l.expires) {
		return ErrLoginExpired
	}
	if code == "" {
		return errors.New("code is missing")
	}
	if p.o.HTTP != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.o.HTTP)
	}
	conf := p.conf(l.redirect)
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(l.verifier))
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return errors.New("no refresh token from Google; remove the app's access in your Google account and sign in again")
	}
	if err := p.o.Tokens.Put(TokenKey(p.o.Profile, account), tok); err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	p.mu.Lock()
	if st := p.status[account]; st != nil {
		st.Connected, st.Error = true, ""
	}
	p.mu.Unlock()
	p.o.Logger.Info("google account connected", "account", account)
	return nil
}

func (p *Provider) conf(redirect string) oauth2.Config {
	conf := *p.o.OAuth
	conf.RedirectURL = redirect
	if len(conf.Scopes) == 0 {
		conf.Scopes = []string{Scope}
	}
	return conf
}

// checkLoopback accepts http://127.0.0.1:PORT/ (any path, no query).
func checkLoopback(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" ||
		u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return ErrBadRedirect
	}
	if _, err := net.LookupPort("tcp", u.Port()); err != nil {
		return ErrBadRedirect
	}
	return nil
}
