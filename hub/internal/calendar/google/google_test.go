package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/padalton/doormutex/hub/internal/calendar"
)

var vienna, _ = time.LoadLocation("Europe/Vienna")

// memStore is an in-memory TokenStore.
type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memStore) Put(key string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(v)
	s.m[key] = b
	return err
}

func (s *memStore) Get(key string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[key]
	if !ok {
		return errors.New("not found")
	}
	return json.Unmarshal(b, v)
}

func timed(start, end string) (eventTime, eventTime) {
	s, _ := time.Parse(time.RFC3339, start)
	e, _ := time.Parse(time.RFC3339, end)
	return eventTime{DateTime: s}, eventTime{DateTime: e}
}

func TestClassify(t *testing.T) {
	p := New(Options{Profile: "office", Location: vienna})
	ignore := Account{Name: "private"}
	use := Account{Name: "private", Mirror: MirrorUse}
	s, e := timed("2026-10-28T10:00:00+01:00", "2026-10-28T10:30:00+01:00")
	base := func(mod func(*item)) item {
		it := item{ID: "x", Status: "confirmed", EventType: "default", Start: s, End: e}
		if mod != nil {
			mod(&it)
		}
		return it
	}
	conf := func(key string) func(*item) {
		return func(it *item) {
			it.Conference = &struct {
				Solution *struct {
					Key struct {
						Type string `json:"type"`
					} `json:"key"`
				} `json:"conferenceSolution"`
				EntryPoints []struct {
					Type string `json:"entryPointType"`
					URI  string `json:"uri"`
				} `json:"entryPoints"`
			}{}
			it.Conference.Solution = &struct {
				Key struct {
					Type string `json:"type"`
				} `json:"key"`
			}{}
			it.Conference.Solution.Key.Type = key
		}
	}
	cases := []struct {
		name    string
		it      item
		account Account
		kind    calendar.Kind // "" → ignored
		allDay  bool
	}{
		{"plain event → other", base(nil), ignore, calendar.KindOther, false},
		{"Google Meet via hangoutLink", base(func(it *item) { it.HangoutLink = "https://meet.google.com/abc-defg-hij" }), ignore, calendar.KindVideoCall, false},
		{"Google Meet via conferenceData", base(conf("hangoutsMeet")), ignore, calendar.KindVideoCall, false},
		{"Zoom add-on", base(conf("addOn")), ignore, calendar.KindVideoCall, false},
		{"Zoom link in description", base(func(it *item) {
			it.Description = `Join: <a href="https://us02web.zoom.us/j/123456789?pwd=x">Zoom</a>`
		}), ignore, calendar.KindVideoCall, false},
		{"Teams link in location", base(func(it *item) {
			it.Location = "https://teams.microsoft.com/l/meetup-join/19%3ameeting_x"
		}), ignore, calendar.KindVideoCall, false},
		{"Jitsi link", base(func(it *item) { it.Description = "meet.jit.si/doormutex-standup" }), ignore, calendar.KindVideoCall, false},
		{"Webex link", base(func(it *item) { it.Location = "https://acme.webex.com/meet/someone" }), ignore, calendar.KindVideoCall, false},
		{"a zoom.us homepage link is no call", base(func(it *item) { it.Description = "see https://zoom.us/pricing" }), ignore, calendar.KindOther, false},
		{"focus time", base(func(it *item) { it.EventType = "focusTime" }), ignore, calendar.KindFocus, false},
		{"timed out of office", base(func(it *item) { it.EventType = "outOfOffice" }), ignore, calendar.KindOutOfOffice, false},
		{"all-day out of office", base(func(it *item) {
			it.EventType = "outOfOffice"
			it.Start, it.End = eventTime{Date: "2026-10-28"}, eventTime{Date: "2026-10-30"}
		}), ignore, calendar.KindOutOfOffice, true},
		{"working location ignored", base(func(it *item) { it.EventType = "workingLocation" }), ignore, "", false},
		{"birthday ignored", base(func(it *item) { it.EventType = "birthday" }), ignore, "", false},
		{"cancelled ignored", base(func(it *item) { it.Status = "cancelled" }), ignore, "", false},
		{"declined ignored", base(func(it *item) {
			it.Attendees = append(it.Attendees, struct {
				Self           bool   `json:"self"`
				ResponseStatus string `json:"responseStatus"`
			}{true, "declined"})
		}), ignore, "", false},
		{"mirror copy ignored when the source is connected", base(func(it *item) {
			it.Extended.Private = map[string]string{DefaultMirrorKey: "video_call"}
		}), ignore, "", false},
		{"mirror copy carries the kind", base(func(it *item) {
			it.Extended.Private = map[string]string{DefaultMirrorKey: "video_call"}
		}), use, calendar.KindVideoCall, false},
		{"mirror copy without a kind is classified normally", base(func(it *item) {
			it.Extended.Private = map[string]string{DefaultMirrorKey: "1"}
			it.EventType = "focusTime"
		}), use, calendar.KindFocus, false},
		{"zero length ignored", base(func(it *item) { it.End = it.Start }), ignore, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := p.classify(c.it, c.account)
			if c.kind == "" {
				if ok {
					t.Fatalf("expected ignored, got %+v", ev)
				}
				return
			}
			if !ok || ev.Kind != c.kind || ev.AllDay != c.allDay {
				t.Fatalf("got %+v ok=%v, want kind %s allDay %v", ev, ok, c.kind, c.allDay)
			}
			if c.allDay && !ev.Start.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, vienna)) {
				t.Errorf("all-day start = %v", ev.Start)
			}
		})
	}
	if ev, _ := p.classify(base(func(it *item) { it.Transparency = "transparent" }), ignore); !ev.Transparent {
		t.Error("transparency lost")
	}
	answer := func(rs string) func(*item) {
		return func(it *item) {
			it.Attendees = append(it.Attendees, struct {
				Self           bool   `json:"self"`
				ResponseStatus string `json:"responseStatus"`
			}{false, "declined"}, struct {
				Self           bool   `json:"self"`
				ResponseStatus string `json:"responseStatus"`
			}{true, rs})
		}
	}
	for rs, tentative := range map[string]bool{"tentative": true, "needsAction": true, "accepted": false} {
		if ev, ok := p.classify(base(answer(rs)), ignore); !ok || ev.Tentative != tentative {
			t.Errorf("%s: tentative = %v (ok %v)", rs, ev.Tentative, ok)
		}
	}
}

// fakeGoogle serves the token endpoint and the events API for two accounts.
type fakeGoogle struct {
	t        *testing.T
	mu       sync.Mutex
	refreshs int
	queries  []url.Values
	failWork bool
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/token":
		_ = r.ParseForm()
		f.mu.Lock()
		f.refreshs++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("grant_type") {
		case "refresh_token":
			// The refresh token says which account it is.
			_, _ = fmt.Fprintf(w, `{"access_token":"access-%s","token_type":"Bearer","expires_in":3600}`, r.Form.Get("refresh_token"))
		case "authorization_code":
			if r.Form.Get("code") != "the-code" || r.Form.Get("code_verifier") == "" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprint(w, `{"access_token":"access-new","refresh_token":"new","token_type":"Bearer","expires_in":3600}`)
		}
	case strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/"):
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.Query())
		fail := f.failWork
		f.mu.Unlock()
		switch r.Header.Get("Authorization") {
		case "Bearer access-private":
			if r.URL.Query().Get("pageToken") == "" {
				_, _ = fmt.Fprint(w, `{"nextPageToken":"p2","items":[{"id":"a","status":"confirmed","eventType":"default",
					"hangoutLink":"https://meet.google.com/x","start":{"dateTime":"2026-10-28T10:00:00+01:00"},"end":{"dateTime":"2026-10-28T10:30:00+01:00"}}]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"items":[{"id":"b","status":"confirmed","eventType":"default","extendedProperties":{"private":{"doormutex":"1"}},
				"start":{"dateTime":"2026-10-28T11:00:00+01:00"},"end":{"dateTime":"2026-10-28T12:00:00+01:00"}}]}`)
		case "Bearer access-work":
			if fail {
				http.Error(w, `{"error":{"message":"backend error"}}`, http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprint(w, `{"items":[{"id":"c","status":"confirmed","eventType":"focusTime",
				"start":{"dateTime":"2026-10-28T13:00:00+01:00"},"end":{"dateTime":"2026-10-28T15:00:00+01:00"}},
				{"id":"d","status":"confirmed","eventType":"default","hangoutLink":"https://meet.google.com/y",
				"attendees":[{"self":true,"responseStatus":"tentative"}],
				"start":{"dateTime":"2026-10-28T16:00:00+01:00"},"end":{"dateTime":"2026-10-28T16:30:00+01:00"}}]}`)
		default:
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
		}
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (*fakeGoogle, *httptest.Server) {
	t.Helper()
	f := &fakeGoogle{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	oldAPI := APIBase
	APIBase = srv.URL + "/calendar/v3"
	t.Cleanup(func() { APIBase = oldAPI })
	return f, srv
}

func oauthConf(srv *httptest.Server) *oauth2.Config {
	return &oauth2.Config{ClientID: "id", ClientSecret: "secret", Scopes: []string{Scope},
		Endpoint: oauth2.Endpoint{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
}

func TestEventsFromTwoAccounts(t *testing.T) {
	f, srv := setup(t)
	store := &memStore{m: map[string][]byte{}}
	expired := time.Now().Add(-time.Hour)
	_ = store.Put(TokenKey("office", "private"), oauth2.Token{AccessToken: "old", RefreshToken: "private", Expiry: expired})
	_ = store.Put(TokenKey("office", "work"), oauth2.Token{AccessToken: "old", RefreshToken: "work", Expiry: expired})
	p := New(Options{OAuth: oauthConf(srv), Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "private"}, {Name: "work"}}})

	from := time.Date(2026, 10, 28, 0, 0, 0, 0, vienna)
	ev, err := p.Events(context.Background(), from, from.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// "b" is a mirrored copy and the source (work) is connected → ignored.
	got := map[string]calendar.Kind{}
	for _, e := range ev {
		got[e.ID] = e.Kind
	}
	want := map[string]calendar.Kind{"private/primary/a": calendar.KindVideoCall, "work/primary/c": calendar.KindFocus, "work/primary/d": calendar.KindVideoCall}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	// Refreshed tokens were saved.
	var tok oauth2.Token
	if err := store.Get(TokenKey("office", "work"), &tok); err != nil || tok.AccessToken != "access-work" {
		t.Errorf("refreshed token not saved: %+v %v", tok, err)
	}
	// The request asks only for what the rules need – never the title.
	q := f.queries[0]
	if strings.Contains(q.Get("fields"), "summary") || q.Get("singleEvents") != "true" || len(q["eventTypes"]) != 4 {
		t.Errorf("query = %v", q)
	}
	if st := p.Status(); len(st) != 2 || !st[0].Connected || st[0].Events != 1 || st[1].Error != "" {
		t.Errorf("status = %+v", st)
	}

	// The work account fails: its last good events are kept, the error is reported.
	f.mu.Lock()
	f.failWork = true
	f.mu.Unlock()
	ev, err = p.Events(context.Background(), from, from.Add(48*time.Hour))
	if err == nil || !strings.Contains(err.Error(), "backend error") || len(ev) != 3 {
		t.Fatalf("partial failure: %d events, err %v", len(ev), err)
	}
	if st := p.Status(); st[1].Error == "" || !st[1].Connected {
		t.Errorf("status after failure = %+v", st)
	}
}

func TestNotConnected(t *testing.T) {
	_, srv := setup(t)
	store := &memStore{m: map[string][]byte{}}
	_ = store.Put(TokenKey("office", "private"), oauth2.Token{AccessToken: "access-private", RefreshToken: "private", Expiry: time.Now().Add(time.Hour)})
	p := New(Options{OAuth: oauthConf(srv), Profile: "office", Tokens: store, Location: vienna,
		Accounts: []Account{{Name: "private"}, {Name: "work"}}})
	from := time.Date(2026, 10, 28, 0, 0, 0, 0, vienna)
	ev, err := p.Events(context.Background(), from, from.Add(24*time.Hour))
	if !errors.Is(err, ErrNotConnected) || !strings.Contains(err.Error(), "google login office work") {
		t.Fatalf("err = %v", err)
	}
	if len(ev) != 1 {
		t.Fatalf("the connected account must still deliver: %+v", ev)
	}
	if st := p.Status(); st[1].Connected {
		t.Errorf("work should not be connected: %+v", st)
	}
}

var urlRe = regexp.MustCompile(`https?://\S+/auth\?\S+`)

func TestLoginLoopback(t *testing.T) {
	_, srv := setup(t)
	out := &syncBuffer{}
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	done := make(chan struct{})
	var tok *oauth2.Token
	var err error
	go func() {
		tok, err = Login(context.Background(), *oauthConf(srv), pr, out)
		close(done)
	}()
	auth := waitForURL(t, out)
	q := auth.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" || q.Get("scope") != Scope {
		t.Errorf("auth URL = %s", auth)
	}
	// The browser follows Google's redirect to the loopback listener.
	resp, gerr := http.Get(q.Get("redirect_uri") + "?code=the-code&state=" + url.QueryEscape(q.Get("state")))
	if gerr != nil {
		t.Fatal(gerr)
	}
	_ = resp.Body.Close()
	<-done
	if err != nil || tok.RefreshToken != "new" {
		t.Fatalf("Login = %+v, %v", tok, err)
	}
}

func TestLoginPaste(t *testing.T) {
	_, srv := setup(t)
	out := &syncBuffer{}
	pr, pw := io.Pipe()
	done := make(chan struct{})
	var tok *oauth2.Token
	var err error
	go func() {
		tok, err = Login(context.Background(), *oauthConf(srv), pr, out)
		close(done)
	}()
	q := waitForURL(t, out).Query()
	// The hub runs elsewhere: the user pastes the address of the error page.
	_, _ = fmt.Fprintf(pw, "http://127.0.0.1:1/?state=%s&code=the-code&scope=x\n", url.QueryEscape(q.Get("state")))
	<-done
	if err != nil || tok.RefreshToken != "new" {
		t.Fatalf("Login = %+v, %v", tok, err)
	}
}

func TestLoginRejectsWrongState(t *testing.T) {
	_, srv := setup(t)
	out := &syncBuffer{}
	pr, pw := io.Pipe()
	done := make(chan struct{})
	var err error
	go func() {
		_, err = Login(context.Background(), *oauthConf(srv), pr, out)
		close(done)
	}()
	waitForURL(t, out)
	_, _ = fmt.Fprintln(pw, "http://127.0.0.1:1/?state=forged&code=the-code")
	<-done
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("err = %v", err)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitForURL(t *testing.T, out *syncBuffer) *url.URL {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := urlRe.FindString(out.String()); m != "" {
			u, err := url.Parse(m)
			if err != nil {
				t.Fatal(err)
			}
			return u
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no sign-in URL printed: %q", out.String())
	return nil
}
