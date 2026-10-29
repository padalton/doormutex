// Package calendar abstracts calendar sources. The Google Calendar
// implementation follows later; for now there is an in-memory fake.
package calendar

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Kind classifies an event. Providers decide the kind (e.g. Google:
// attendees or a conference link → meeting, eventType focusTime → focus).
type Kind string

const (
	KindMeeting     Kind = "meeting"
	KindFocus       Kind = "focus"
	KindOutOfOffice Kind = "out_of_office"
	KindOther       Kind = "other"
)

// Event is a calendar entry. Title never leaves the hub.
type Event struct {
	ID          string    `json:"id" yaml:"id"`
	Title       string    `json:"title,omitempty" yaml:"title"`
	Start       time.Time `json:"start" yaml:"start"`
	End         time.Time `json:"end" yaml:"end"`
	AllDay      bool      `json:"all_day,omitempty" yaml:"all_day"`
	Transparent bool      `json:"transparent,omitempty" yaml:"transparent"` // shown as "free"
	Kind        Kind      `json:"kind" yaml:"kind"`
}

// Provider lists events overlapping [from, to).
type Provider interface {
	Events(ctx context.Context, from, to time.Time) ([]Event, error)
}

// Fake is an in-memory provider for tests and demos.
type Fake struct {
	mu     sync.Mutex
	events []Event
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
