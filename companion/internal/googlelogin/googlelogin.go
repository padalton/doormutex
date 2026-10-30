// Package googlelogin signs a Google account in for the hub, from the
// desktop: the companion listens on a loopback port, the hub returns
// Google's sign-in URL for it, the browser opens and Google redirects back
// to the companion, which hands the code to the hub. Nothing needs to be
// copied or pasted, and the client secret and the refresh token stay on
// the hub.
package googlelogin

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"time"
)

// Hub is the part of the hub client the sign-in uses.
type Hub interface {
	StartGoogleLogin(ctx context.Context, account, redirectURI string) (string, error)
	CompleteGoogleLogin(ctx context.Context, account, state, code string) error
}

// Texts shown in the terminal and the browser.
type Texts struct {
	Opening, Done, Page, Failed string
}

// Run signs account in. open shows a URL in the browser (OpenBrowser if nil).
func Run(ctx context.Context, hub Hub, account string, open func(string) error, out io.Writer, t Texts) error {
	if open == nil {
		open = OpenBrowser
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	redirect := fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	authURL, err := hub.StartGoogleLogin(ctx, account, redirect)
	if err != nil {
		return err
	}
	u, err := url.Parse(authURL)
	if err != nil {
		return fmt.Errorf("sign-in URL from the hub: %w", err)
	}
	state := u.Query().Get("state")

	type result struct{ code, err string }
	results := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state || (q.Get("code") == "" && q.Get("error") == "") {
			http.Error(w, "doormutex: unexpected request", http.StatusBadRequest)
			return
		}
		var res result
		if e := q.Get("error"); e != "" {
			res.err = e
		} else {
			res.code = q.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		msg := t.Page
		if res.err != "" {
			msg = t.Failed + ": " + res.err
		}
		_, _ = fmt.Fprintf(w, "<!doctype html><meta charset=utf-8><title>doormutex</title><p style=\"font:1.2em sans-serif;margin:3em\">%s</p>", html.EscapeString(msg))
		select {
		case results <- res:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	_, _ = fmt.Fprintf(out, "%s\n\n  %s\n\n", t.Opening, authURL)
	if err := open(authURL); err != nil {
		_, _ = fmt.Fprintln(out, err) // the URL above still works
	}

	var res result
	select {
	case res = <-results:
	case <-ctx.Done():
		return ctx.Err()
	}
	if res.err != "" {
		return fmt.Errorf("%s: %s", t.Failed, res.err)
	}
	if err := hub.CompleteGoogleLogin(ctx, account, state, res.code); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, t.Done)
	return nil
}

// OpenBrowser opens a URL with xdg-open.
func OpenBrowser(u string) error {
	cmd := exec.Command("xdg-open", u)
	if err := cmd.Start(); err != nil {
		return errors.New("xdg-open failed – open the address yourself")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
