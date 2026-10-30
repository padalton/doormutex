package googlelogin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeHub struct {
	redirect  string
	completed []string
}

func (h *fakeHub) StartGoogleLogin(_ context.Context, account, redirect string) (string, error) {
	if account != "work" {
		return "", errors.New("hub: unknown Google account")
	}
	h.redirect = redirect
	return "https://accounts.example/auth?state=st-1&redirect_uri=" + url.QueryEscape(redirect), nil
}

func (h *fakeHub) CompleteGoogleLogin(_ context.Context, account, state, code string) error {
	h.completed = append(h.completed, account+"/"+state+"/"+code)
	return nil
}

var texts = Texts{Opening: "Open:", Done: "Connected.", Page: "Signed in.", Failed: "Sign-in failed"}

// browser plays the user and Google: it follows the redirect with a code.
func browser(t *testing.T, query string, page *string) func(string) error {
	return func(auth string) error {
		u, _ := url.Parse(auth)
		redirect := u.Query().Get("redirect_uri")
		go func() {
			c := http.Client{Timeout: 5 * time.Second}
			// A stray request (favicon, wrong state) must not end the sign-in.
			if resp, err := c.Get(redirect + "favicon.ico"); err == nil {
				_ = resp.Body.Close()
			}
			resp, err := c.Get(redirect + "?" + query)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = resp.Body.Close() }()
			var b bytes.Buffer
			_, _ = b.ReadFrom(resp.Body)
			*page = b.String()
		}()
		return nil
	}
}

func TestRun(t *testing.T) {
	hub := &fakeHub{}
	var out bytes.Buffer
	var page string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Run(ctx, hub, "work", browser(t, "state=st-1&code=c0de", &page), &out, texts); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hub.redirect, "http://127.0.0.1:") || len(hub.completed) != 1 || hub.completed[0] != "work/st-1/c0de" {
		t.Fatalf("hub saw %q %v", hub.redirect, hub.completed)
	}
	if !strings.Contains(out.String(), "Connected.") {
		t.Errorf("output %q", out.String())
	}
}

func TestRunDenied(t *testing.T) {
	var page string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := Run(ctx, &fakeHub{}, "work", browser(t, "state=st-1&error=access_denied", &page), &bytes.Buffer{}, texts)
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("err = %v", err)
	}
	if err := Run(ctx, &fakeHub{}, "bank", func(string) error { return nil }, &bytes.Buffer{}, texts); err == nil {
		t.Fatal("unknown account accepted")
	}
}
