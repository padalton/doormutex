package agent

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/padalton/doormutex/companion/internal/hubclient"
	"github.com/padalton/doormutex/companion/internal/i18n"
)

type fakeSampler struct {
	mu  sync.Mutex
	rep hubclient.Report
}

func (f *fakeSampler) set(r hubclient.Report) { f.mu.Lock(); f.rep = r; f.mu.Unlock() }
func (f *fakeSampler) Sample(context.Context) (hubclient.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rep, nil
}

type fakeHub struct {
	mu      sync.Mutex
	sent    []hubclient.Report
	cleared int
	states  chan hubclient.State
}

func (h *fakeHub) Heartbeat(_ context.Context, r hubclient.Report) (hubclient.State, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, r)
	return hubclient.State{}, nil
}
func (h *fakeHub) ClearOverride(context.Context) error {
	h.mu.Lock()
	h.cleared++
	h.mu.Unlock()
	return nil
}
func (h *fakeHub) Follow(ctx context.Context, fn func(hubclient.State)) {
	for {
		select {
		case <-ctx.Done():
			return
		case s := <-h.states:
			fn(s)
		}
	}
}
func (h *fakeHub) reports() []hubclient.Report {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]hubclient.Report(nil), h.sent...)
}

type fakeNotifier struct {
	mu    sync.Mutex
	shown []string
	act   func(string)
}

func (n *fakeNotifier) Notify(summary, body string, _ map[string]string, on func(string)) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.shown = append(n.shown, summary+" | "+body)
	n.act = on
	return nil
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHeartbeatsOnChangeAndInterval(t *testing.T) {
	s := &fakeSampler{}
	h := &fakeHub{states: make(chan hubclient.State)}
	power := make(chan Power, 1)
	a := &Agent{Sampler: s, Hub: h, Power: power, Poll: 5 * time.Millisecond, Interval: time.Hour, Log: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- a.Run(ctx) }()

	waitFor(t, "first heartbeat", func() bool { return len(h.reports()) == 1 })
	time.Sleep(30 * time.Millisecond)
	if n := len(h.reports()); n != 1 {
		t.Fatalf("unchanged state was resent: %d", n)
	}
	s.set(hubclient.Report{Microphone: hubclient.Media{Active: true, Apps: []string{"Firefox"}}})
	waitFor(t, "change", func() bool { r := h.reports(); return len(r) == 2 && r[1].Microphone.Active })

	// Suspend: goodbye, then the system may continue.
	acked := make(chan struct{})
	power <- Power{Kind: "suspend", Ack: func() { close(acked) }}
	<-acked
	if r := h.reports(); r[len(r)-1].Goodbye != "suspend" {
		t.Fatalf("no goodbye before suspend: %+v", r)
	}
	// Resume: heartbeat right away even though nothing changed.
	power <- Power{Kind: "resume"}
	waitFor(t, "heartbeat after resume", func() bool { r := h.reports(); return r[len(r)-1].Goodbye == "" && len(r) == 4 })

	// Stop (SIGTERM, logout): sign off as shutdown.
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if r := h.reports(); r[len(r)-1].Goodbye != "shutdown" {
		t.Fatalf("no goodbye on exit: %+v", r)
	}
}

func TestNotifications(t *testing.T) {
	h := &fakeHub{states: make(chan hubclient.State)}
	n := &fakeNotifier{}
	a := &Agent{Sampler: &fakeSampler{}, Hub: h, Notifier: n, Poll: time.Hour, Log: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = a.Run(ctx) }()

	until := time.Date(2026, 10, 28, 14, 30, 0, 0, time.Local)
	h.states <- hubclient.State{Status: "working", Reason: "work_hours"}                     // first: no notification
	h.states <- hubclient.State{Status: "working", Reason: "work_hours", Phase: "last_call"} // phase: none
	h.states <- hubclient.State{Status: "dnd", Reason: "microphone", Detail: "Firefox"}      // change
	h.states <- hubclient.State{Status: "dnd", Reason: "microphone", Detail: "Firefox"}      // same: none
	h.states <- hubclient.State{Status: "free", Reason: "override", Until: &until, Override: &hubclient.Override{Status: "free"}}
	waitFor(t, "two notifications", func() bool { n.mu.Lock(); defer n.mu.Unlock(); return len(n.shown) == 2 })
	n.mu.Lock()
	if n.shown[0] != "🔴 Do not disturb | microphone in use – Firefox" || n.shown[1] != "🟢 Come in | set by hand · until 14:30" {
		t.Errorf("shown = %q", n.shown)
	}
	act := n.act
	n.mu.Unlock()
	act("auto")
	waitFor(t, "override cleared", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.cleared == 1 })
}

func TestDescribeFault(t *testing.T) {
	s, b := Describe(hubclient.State{Status: "away", Phase: "fault", Reason: "companion_lost"})
	if s != "🟡 Fault" || b != "companion not responding" {
		t.Errorf("%q %q", s, b)
	}
}

func TestDescribeGerman(t *testing.T) {
	Texts = i18n.New("de")
	defer func() { Texts = i18n.New("en") }()
	s, b := Describe(hubclient.State{Status: "dnd", Reason: "calendar", Detail: "video_call"})
	if s != "🔴 Bitte nicht stören" || b != "Kalender – Videocall" {
		t.Errorf("%q %q", s, b)
	}
}
