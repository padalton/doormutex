// Package calendar abstracts calendar sources.
package calendar

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Kind classifies an event. Providers decide the kind (e.g. Google:
// a Meet/Zoom/Teams link → video call, eventType focusTime → focus).
type Kind string

const (
	KindVideoCall   Kind = "video_call"
	KindFocus       Kind = "focus"
	KindOutOfOffice Kind = "out_of_office"
	KindOther       Kind = "other"
)

// Kinds lists every kind in a stable order.
var Kinds = []Kind{KindVideoCall, KindFocus, KindOutOfOffice, KindOther}

// Event is a calendar entry. Title never leaves the hub.
type Event struct {
	ID          string    `json:"id" yaml:"id"`
	Title       string    `json:"title,omitempty" yaml:"title"`
	Start       time.Time `json:"start" yaml:"start"`
	End         time.Time `json:"end" yaml:"end"`
	AllDay      bool      `json:"all_day,omitempty" yaml:"all_day"`
	Transparent bool      `json:"transparent,omitempty" yaml:"transparent"` // shown as "free"
	Tentative   bool      `json:"tentative,omitempty" yaml:"tentative"`     // answered "maybe" or not yet
	Kind        Kind      `json:"kind" yaml:"kind"`
}

// Provider lists events overlapping [from, to). A provider with several
// sources may return a partial result together with an error.
type Provider interface {
	Events(ctx context.Context, from, to time.Time) ([]Event, error)
}

// AccountStatus reports the health of one calendar account.
type AccountStatus struct {
	Name      string    `json:"name"`
	Connected bool      `json:"connected"`
	LastSync  time.Time `json:"last_sync,omitzero"`
	Events    int       `json:"events"`
	Error     string    `json:"error,omitempty"`
	// LoginExpired is set when Google refused the stored sign-in: the
	// account must sign in again.
	LoginExpired bool `json:"login_expired,omitempty"`
}

// Problems a calendar can have, worst first – see Problem.
const (
	ProblemLoginExpired = "login_expired" // an account must sign in again
	ProblemNotConnected = "not_connected" // an account was never signed in
	ProblemUnavailable  = "unavailable"   // reading failed otherwise, e.g. network
)

// Problem sums up the accounts' health in one word for clients that may
// not see the details (read-only tokens): "" if every account is fine,
// otherwise the worst of the Problem* constants. The timeline is then
// missing events, or built from older ones.
func Problem(accounts []AccountStatus) string {
	worst, rank := "", 0
	for _, a := range accounts {
		var p string
		var r int
		switch {
		case a.Error == "": // fine, or not read yet
		case a.LoginExpired:
			p, r = ProblemLoginExpired, 3
		case !a.Connected:
			p, r = ProblemNotConnected, 2
		default:
			p, r = ProblemUnavailable, 1
		}
		if r > rank {
			worst, rank = p, r
		}
	}
	return worst
}

// StatusReporter is implemented by providers with accounts (Google).
type StatusReporter interface {
	Status() []AccountStatus
}

// Fake is an in-memory provider for tests and demos.
type Fake struct {
	mu       sync.Mutex
	events   []Event
	accounts []AccountStatus
}

// NewFake returns a fake provider with the given events.
func NewFake(events ...Event) *Fake {
	f := &Fake{}
	f.Set(events...)
	return f
}

// Set replaces all events.
func (f *Fake) Set(events ...Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append([]Event(nil), events...)
	sort.Slice(f.events, func(i, j int) bool { return f.events[i].Start.Before(f.events[j].Start) })
}

// Events implements Provider.
func (f *Fake) Events(_ context.Context, from, to time.Time) ([]Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Event
	for _, e := range f.events {
		if e.End.After(from) && e.Start.Before(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

// SetStatus sets what Status reports, e.g. an expired sign-in.
func (f *Fake) SetStatus(accounts ...AccountStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts = append([]AccountStatus(nil), accounts...)
}

// Status implements StatusReporter; nil unless SetStatus was called.
func (f *Fake) Status() []AccountStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AccountStatus(nil), f.accounts...)
}
