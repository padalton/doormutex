// Package hubclient talks to the doormutex hub: heartbeats, goodbye and
// the live state stream (Server-Sent Events).
package hubclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Media is one device in a heartbeat.
type Media struct {
	Active bool     `json:"active"`
	Apps   []string `json:"apps,omitempty"`
}

// Report is the heartbeat body (POST /api/v1/signals/companion).
type Report struct {
	Locked     bool   `json:"locked"`
	Microphone Media  `json:"microphone"`
	Camera     Media  `json:"camera"`
	Goodbye    string `json:"goodbye,omitempty"` // "shutdown" or "suspend"
}

// Equal reports whether two reports carry the same information.
func (r Report) Equal(o Report) bool {
	a, _ := json.Marshal(r)
	b, _ := json.Marshal(o)
	return bytes.Equal(a, b)
}

// Override is the active manual status of the profile.
type Override struct {
	Status string `json:"status"`
}

// State is the part of the hub state the companion shows.
type State struct {
	Status   string     `json:"status"`
	Phase    string     `json:"phase"`
	Reason   string     `json:"reason"`
	Detail   string     `json:"detail"`
	Until    *time.Time `json:"until"`
	Override *Override  `json:"override"`
}

// Client is a hub client.
type Client struct {
	URL   string // e.g. http://hub:8080
	Token string
	HTTP  *http.Client
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.URL, "/")+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("hub: %s %s: %s %s", method, path, resp.Status, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Heartbeat sends a report and returns the new state.
func (c *Client) Heartbeat(ctx context.Context, r Report) (State, error) {
	var st State
	err := c.do(ctx, http.MethodPost, "/api/v1/signals/companion", r, &st)
	return st, err
}

// ClearOverride returns the light to automatic mode.
func (c *Client) ClearOverride(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/override", nil, nil)
}

// OverrideRequest is the body of PUT /api/v1/override.
type OverrideRequest struct {
	Status   string `json:"status"`             // free, working, dnd, away
	Mode     string `json:"mode"`               // duration, end_of_event, end_of_workday, next_change, indefinite
	Duration string `json:"duration,omitempty"` // with mode duration, e.g. "30m"
}

// SetOverride sets a manual status and returns the new state.
func (c *Client) SetOverride(ctx context.Context, o OverrideRequest) (State, error) {
	var st State
	err := c.do(ctx, http.MethodPut, "/api/v1/override", o, &st)
	return st, err
}

// Status returns the current state.
func (c *Client) Status(ctx context.Context) (State, error) {
	var st State
	err := c.do(ctx, http.MethodGet, "/api/v1/status", nil, &st)
	return st, err
}

// StartPomodoro starts work/break cycles; workStatus "" uses the profile's setting.
func (c *Client) StartPomodoro(ctx context.Context, work, brk string, cycles int, workStatus string) (State, error) {
	body := map[string]any{"work": work, "break": brk, "cycles": cycles}
	if workStatus != "" {
		body["work_status"] = workStatus
	}
	var st State
	err := c.do(ctx, http.MethodPut, "/api/v1/pomodoro", body, &st)
	return st, err
}

// StopPomodoro ends a running pomodoro.
func (c *Client) StopPomodoro(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/pomodoro", nil, nil)
}

// StartGoogleLogin asks the hub for Google's sign-in URL; Google will
// redirect the browser to redirectURI (the companion's loopback address).
func (c *Client) StartGoogleLogin(ctx context.Context, account, redirectURI string) (string, error) {
	var out struct {
		AuthURL string `json:"auth_url"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/calendar/accounts/"+url.PathEscape(account)+"/login",
		map[string]string{"redirect_uri": redirectURI}, &out)
	return out.AuthURL, err
}

// CompleteGoogleLogin hands Google's code to the hub, which stores the token.
func (c *Client) CompleteGoogleLogin(ctx context.Context, account, state, code string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/calendar/accounts/"+url.PathEscape(account)+"/login/complete",
		map[string]string{"state": state, "code": code}, nil)
}

// Follow streams the state until ctx is done, reconnecting with backoff.
// fn is called for every state event, starting with the current state.
func (c *Client) Follow(ctx context.Context, fn func(State)) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := c.stream(ctx, fn)
		if ctx.Err() != nil {
			return
		}
		_ = err // reconnect; the light shows a fault if the hub stays away
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (c *Client) stream(ctx context.Context, fn func(State)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.URL, "/")+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := (&http.Client{}).Do(req) // no timeout: the stream is long-lived
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub: events: %s", resp.Status)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: ") && event == "state":
			var st State
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &st) == nil {
				fn(st)
			}
		case line == "":
			event = ""
		}
	}
	return sc.Err()
}
