package google

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Login runs the OAuth flow for an installed ("desktop") client:
//
//  1. It listens on a random loopback port and prints the sign-in URL.
//  2. If the browser runs on the same machine, Google redirects to that
//     port and the flow finishes on its own.
//  3. Otherwise (the hub runs in Docker or on a server) the browser shows
//     an error page for 127.0.0.1 – the user copies the address from the
//     browser and pastes it into in.
//
// PKCE and a random state protect the exchange. The token is returned;
// the caller stores it.
func Login(ctx context.Context, conf oauth2.Config, in io.Reader, out io.Writer) (*oauth2.Token, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer func() { _ = ln.Close() }()
	conf.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port)
	if len(conf.Scopes) == 0 {
		conf.Scopes = []string{Scope}
	}
	state := random()
	verifier := oauth2.GenerateVerifier()
	authURL := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"))

	_, _ = fmt.Fprintf(out, "Open this address in a browser and sign in with the Google account:\n\n  %s\n\n", authURL)
	_, _ = fmt.Fprintln(out, "If the browser then shows an error page for 127.0.0.1, copy the full address")
	_, _ = fmt.Fprint(out, "from its address bar and paste it here: ")

	codes := make(chan string, 2)
	errc := make(chan error, 2)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := codeFrom(r.URL.Query(), state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			errc <- err
			return
		}
		_, _ = io.WriteString(w, "doormutex: signed in. You can close this tab.\n")
		codes <- code
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	go func() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			u, err := url.Parse(line)
			if err != nil {
				errc <- fmt.Errorf("not an address: %w", err)
				return
			}
			code, err := codeFrom(u.Query(), state)
			if err != nil {
				errc <- err
				return
			}
			codes <- code
			return
		}
	}()

	var code string
	select {
	case code = <-codes:
	case err := <-errc:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	_, _ = fmt.Fprintln(out)
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, errors.New("no refresh token from Google; remove the app's access in your Google account and sign in again")
	}
	return tok, nil
}

func codeFrom(q url.Values, state string) (string, error) {
	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("sign-in failed: %s", e)
	}
	if q.Get("state") != state {
		return "", errors.New("state mismatch – use the address from this sign-in attempt")
	}
	code := q.Get("code")
	if code == "" {
		return "", errors.New("the address contains no code")
	}
	return code, nil
}

func random() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
