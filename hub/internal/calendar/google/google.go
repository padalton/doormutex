// Package google reads events from the Google Calendar API, for several
// Google accounts per profile (e.g. private and work), and classifies them
// into the hub's event kinds.
//
// Only what the rules need is requested: times, event type, transparency,
// conference data, the own response, location/description (to find video
// links) and the mirror mark. Titles are never requested.
package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/padalton/doormutex/hub/internal/calendar"
)

// Scope is the only OAuth scope the hub asks for.
const Scope = "https://www.googleapis.com/auth/calendar.events.readonly"

// Google endpoints; tests replace them.
var (
	Endpoint = oauth2.Endpoint{
		AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:  "https://oauth2.googleapis.com/token",
		AuthStyle: oauth2.AuthStyleInParams,
	}
	APIBase = "https://www.googleapis.com/calendar/v3"
)

// Mirror handling for copies made by a mirroring script.
const (
	MirrorIgnore = "ignore" // the source account is connected: skip the copies
	MirrorUse    = "use"    // the source can't be connected: the mark carries the kind
)

// DefaultMirrorKey is the extendedProperties.private key a mirroring script
// sets on its copies.
const DefaultMirrorKey = "doormutex"

// Account is one Google account of a profile.
type Account struct {
	Name      string   // e.g. "private", "work"; also the token key
	Calendars []string // calendar IDs; default ["primary"]
	Mirror    string   // MirrorIgnore (default) or MirrorUse
}

// TokenStore persists OAuth tokens (the hub's SQLite store).
type TokenStore interface {
	Put(key string, v any) error
	Get(key string, v any) error
}

// TokenKey is the store key of an account's token.
func TokenKey(profile, account string) string {
	return "profiles/" + profile + "/google/" + account
}

// ErrNotConnected means nobody has signed in for an account yet.
var ErrNotConnected = errors.New("not connected")

// Options configure a Provider.
type Options struct {
	OAuth     *oauth2.Config // client id/secret, endpoint, scope
	Profile   string
	Accounts  []Account
	MirrorKey string // default DefaultMirrorKey
	Tokens    TokenStore
	Location  *time.Location // for all-day events
	HTTP      *http.Client   // base client for token refreshes and the API (tests)
	Logger    *slog.Logger
}

// Provider implements calendar.Provider for several Google accounts.
type Provider struct {
	o Options

	mu     sync.Mutex
	cache  map[string][]calendar.Event // last good events per account
	status map[string]*calendar.AccountStatus
}

// New creates a provider.
func New(o Options) *Provider {
	if o.MirrorKey == "" {
		o.MirrorKey = DefaultMirrorKey
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	p := &Provider{o: o, cache: map[string][]calendar.Event{}, status: map[string]*calendar.AccountStatus{}}
	for _, a := range o.Accounts {
		p.status[a.Name] = &calendar.AccountStatus{Name: a.Name}
	}
	return p
}

// Status returns the health of every account, in configuration order.
func (p *Provider) Status() []calendar.AccountStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]calendar.AccountStatus, 0, len(p.o.Accounts))
	for _, a := range p.o.Accounts {
		out = append(out, *p.status[a.Name])
	}
	return out
}

// Events implements calendar.Provider. Accounts are read independently: if
// one fails, its last good events are used and the error is returned
// together with the (partial) result.
func (p *Provider) Events(ctx context.Context, from, to time.Time) ([]calendar.Event, error) {
	var out []calendar.Event
	var errs []error
	for _, a := range p.o.Accounts {
		ev, err := p.account(ctx, a, from, to)
		p.mu.Lock()
		st := p.status[a.Name]
		if err != nil {
			st.Error = err.Error()
			st.Connected = !errors.Is(err, ErrNotConnected)
			errs = append(errs, fmt.Errorf("google account %s: %w", a.Name, err))
			ev = p.cache[a.Name]
		} else {
			st.Error, st.Connected, st.LastSync, st.Events = "", true, time.Now(), len(ev)
			p.cache[a.Name] = ev
		}
		p.mu.Unlock()
		out = append(out, ev...)
	}
	return out, errors.Join(errs...)
}

func (p *Provider) account(ctx context.Context, a Account, from, to time.Time) ([]calendar.Event, error) {
	key := TokenKey(p.o.Profile, a.Name)
	var tok oauth2.Token
	if err := p.o.Tokens.Get(key, &tok); err != nil {
		return nil, fmt.Errorf("%w – run: doormutex-hub google login %s %s", ErrNotConnected, p.o.Profile, a.Name)
	}
	if p.o.HTTP != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.o.HTTP)
	}
	src := &savingSource{src: p.o.OAuth.TokenSource(ctx, &tok), store: p.o.Tokens, key: key, last: tok.AccessToken}
	client := oauth2.NewClient(ctx, src)
	cals := a.Calendars
	if len(cals) == 0 {
		cals = []string{"primary"}
	}
	var out []calendar.Event
	for _, cal := range cals {
		items, err := list(ctx, client, cal, from, to)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if e, ok := p.classify(it, a); ok {
				e.ID = a.Name + "/" + cal + "/" + e.ID
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// savingSource persists refreshed tokens (Google may rotate them).
type savingSource struct {
	src   oauth2.TokenSource
	store TokenStore
	key   string

	mu   sync.Mutex
	last string
}

func (s *savingSource) Token() (*oauth2.Token, error) {
	t, err := s.src.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.AccessToken != s.last {
		s.last = t.AccessToken
		if err := s.store.Put(s.key, t); err != nil {
			return nil, fmt.Errorf("save refreshed token: %w", err)
		}
	}
	return t, nil
}

// item is the subset of a Google Calendar event the hub reads.
type item struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	EventType    string    `json:"eventType"`
	Transparency string    `json:"transparency"`
	Start        eventTime `json:"start"`
	End          eventTime `json:"end"`
	Location     string    `json:"location"`
	Description  string    `json:"description"`
	HangoutLink  string    `json:"hangoutLink"`
	Conference   *struct {
		Solution *struct {
			Key struct {
				Type string `json:"type"`
			} `json:"key"`
		} `json:"conferenceSolution"`
		EntryPoints []struct {
			Type string `json:"entryPointType"`
			URI  string `json:"uri"`
		} `json:"entryPoints"`
	} `json:"conferenceData"`
	Attendees []struct {
		Self           bool   `json:"self"`
		ResponseStatus string `json:"responseStatus"`
	} `json:"attendees"`
	Extended struct {
		Private map[string]string `json:"private"`
	} `json:"extendedProperties"`
}

type eventTime struct {
	Date     string    `json:"date"`     // all-day: 2026-10-28
	DateTime time.Time `json:"dateTime"` // timed
}

// fields limits the response to what classify needs – no titles.
const fields = "nextPageToken,items(id,status,eventType,transparency,start,end,location,description,hangoutLink," +
	"conferenceData(conferenceSolution/key/type,entryPoints(entryPointType,uri)),attendees(self,responseStatus),extendedProperties/private)"

func list(ctx context.Context, client *http.Client, calendarID string, from, to time.Time) ([]item, error) {
	var out []item
	page := ""
	for range 20 { // at most 20 pages of 250 events
		q := url.Values{
			"timeMin":      {from.Format(time.RFC3339)},
			"timeMax":      {to.Format(time.RFC3339)},
			"singleEvents": {"true"},
			"orderBy":      {"startTime"},
			"maxResults":   {"250"},
			"eventTypes":   {"default", "focusTime", "outOfOffice", "fromGmail"},
			"fields":       {fields},
		}
		if page != "" {
			q.Set("pageToken", page)
		}
		u := APIBase + "/calendars/" + url.PathEscape(calendarID) + "/events?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		var body struct {
			Items         []item `json:"items"`
			NextPageToken string `json:"nextPageToken"`
			Error         *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			msg := resp.Status
			if body.Error != nil {
				msg += ": " + body.Error.Message
			}
			return nil, fmt.Errorf("calendar %s: %s", calendarID, msg)
		}
		if err != nil {
			return nil, fmt.Errorf("calendar %s: %w", calendarID, err)
		}
		out = append(out, body.Items...)
		if body.NextPageToken == "" {
			return out, nil
		}
		page = body.NextPageToken
	}
	return out, nil
}

// videoLink matches links of common video conferencing services.
var videoLink = regexp.MustCompile(`(?i)\b(?:https?://)?(?:[a-z0-9-]+\.)*(?:` +
	`zoom\.us/(?:j|my|w)/|` +
	`teams\.microsoft\.com/l/meetup-join|teams\.live\.com/meet|` +
	`webex\.com/(?:meet|join|[a-z0-9.-]*/j\.php)|` +
	`meet\.google\.com/[a-z]|` +
	`meet\.jit\.si/|` +
	`whereby\.com/|` +
	`(?:global\.)?gotomeeting\.com/join|meet\.goto\.com/)`)

// classify turns a Google event into a hub event; ok is false for events
// the hub ignores (cancelled, declined, working location, birthdays,
// mirrored copies of a connected account). Invitations answered with
// "maybe" or not answered yet are marked tentative.
func (p *Provider) classify(it item, a Account) (calendar.Event, bool) {
	if it.Status == "cancelled" {
		return calendar.Event{}, false
	}
	for _, at := range it.Attendees {
		if at.Self && at.ResponseStatus == "declined" {
			return calendar.Event{}, false
		}
	}
	e := calendar.Event{ID: it.ID, Transparent: it.Transparency == "transparent"}
	for _, at := range it.Attendees {
		if at.Self && (at.ResponseStatus == "tentative" || at.ResponseStatus == "needsAction") {
			e.Tentative = true
		}
	}
	if it.Start.Date != "" {
		s, err1 := time.ParseInLocation("2006-01-02", it.Start.Date, p.o.Location)
		en, err2 := time.ParseInLocation("2006-01-02", it.End.Date, p.o.Location)
		if err1 != nil || err2 != nil {
			return calendar.Event{}, false
		}
		e.Start, e.End, e.AllDay = s, en, true
	} else {
		e.Start, e.End = it.Start.DateTime, it.End.DateTime
	}
	if !e.End.After(e.Start) {
		return calendar.Event{}, false
	}

	if mark, ok := it.Extended.Private[p.o.MirrorKey]; ok {
		if a.Mirror != MirrorUse {
			return calendar.Event{}, false
		}
		if k := calendar.Kind(mark); slices.Contains(calendar.Kinds, k) {
			e.Kind = k
			return e, true
		}
	}

	switch it.EventType {
	case "outOfOffice":
		e.Kind = calendar.KindOutOfOffice
	case "focusTime":
		e.Kind = calendar.KindFocus
	case "workingLocation", "birthday":
		return calendar.Event{}, false
	default:
		e.Kind = calendar.KindOther
		if isVideoCall(it) {
			e.Kind = calendar.KindVideoCall
		}
	}
	return e, true
}

func isVideoCall(it item) bool {
	if it.HangoutLink != "" {
		return true
	}
	if c := it.Conference; c != nil {
		if c.Solution != nil && c.Solution.Key.Type != "" {
			return true // hangoutsMeet, addOn (Zoom, Teams, … installed as add-on)
		}
		for _, ep := range c.EntryPoints {
			if ep.Type == "video" {
				return true
			}
		}
	}
	return videoLink.MatchString(it.Location) || videoLink.MatchString(strings.ReplaceAll(it.Description, "&amp;", "&"))
}
