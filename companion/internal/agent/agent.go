// Package agent is the companion's main loop: sample microphone, camera
// and lock, send heartbeats to the hub, sign off before sleep and
// shutdown, and show a notification on every state change.
package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/padalton/doormutex/companion/internal/hubclient"
)

// Sampler reads the current desktop state.
type Sampler interface {
	Sample(ctx context.Context) (hubclient.Report, error)
}

// Hub is the part of the hub client the agent uses.
type Hub interface {
	Heartbeat(ctx context.Context, r hubclient.Report) (hubclient.State, error)
	ClearOverride(ctx context.Context) error
	Follow(ctx context.Context, fn func(hubclient.State))
}

// Notifier shows desktop notifications.
type Notifier interface {
	Notify(summary, body string, actions map[string]string, onAction func(string)) error
}

// Power is a sleep/shutdown event; Ack lets the system continue.
type Power struct {
	Kind string // "suspend", "resume" or "shutdown"
	Ack  func()
}

// Agent ties it together.
type Agent struct {
	Sampler  Sampler
	Hub      Hub
	Notifier Notifier // nil: no notifications
	Power    <-chan Power
	Poll     time.Duration // how often to sample (default 2 s)
	Interval time.Duration // heartbeat at least this often (default 30 s)
	Log      *slog.Logger
}

// Run loops until ctx is done or the system shuts down. On the way out it
// signs off at the hub.
func (a *Agent) Run(ctx context.Context) error {
	if a.Poll == 0 {
		a.Poll = 2 * time.Second
	}
	if a.Interval == 0 {
		a.Interval = 30 * time.Second
	}
	if a.Log == nil {
		a.Log = slog.Default()
	}
	if a.Notifier != nil {
		go a.Hub.Follow(ctx, a.notifier(ctx))
	}
	t := time.NewTicker(a.Poll)
	defer t.Stop()
	var last hubclient.Report
	var lastSent time.Time
	force := true
	for {
		select {
		case <-ctx.Done():
			a.goodbye(context.Background(), "shutdown")
			return nil
		case p, ok := <-a.Power:
			if !ok {
				a.Power = nil
				continue
			}
			switch p.Kind {
			case "suspend", "shutdown":
				a.goodbye(ctx, p.Kind)
				if p.Ack != nil {
					p.Ack()
				}
				if p.Kind == "shutdown" {
					return nil
				}
			case "resume":
				force = true
			}
		case <-t.C:
			rep, err := a.Sampler.Sample(ctx)
			if err != nil {
				a.Log.Warn("sample", "err", err)
				continue
			}
			if !force && rep.Equal(last) && time.Since(lastSent) < a.Interval {
				continue
			}
			if _, err := a.Hub.Heartbeat(ctx, rep); err != nil {
				a.Log.Warn("heartbeat", "err", err)
				force = true // retry on the next tick
				continue
			}
			last, lastSent, force = rep, time.Now(), false
		}
	}
}

func (a *Agent) goodbye(ctx context.Context, kind string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := a.Hub.Heartbeat(ctx, hubclient.Report{Goodbye: kind}); err != nil {
		a.Log.Warn("goodbye", "kind", kind, "err", err)
		return
	}
	a.Log.Info("signed off at the hub", "kind", kind)
}

// notifier returns the callback for the state stream: a notification on
// every change of status or reason, not for the first state.
func (a *Agent) notifier(ctx context.Context) func(hubclient.State) {
	var prev *hubclient.State
	return func(st hubclient.State) {
		if prev != nil && prev.Status == st.Status && prev.Reason == st.Reason && prev.Phase == st.Phase {
			return
		}
		first := prev == nil
		prev = &st
		if first || st.Phase == "last_call" || st.Phase == "amber" || st.Phase == "ending_soon" {
			return // the light shows the phases; a notification per phase would be noise
		}
		summary, body := Describe(st)
		var actions map[string]string
		if st.Override != nil {
			actions = map[string]string{"auto": "Back to automatic"}
		}
		err := a.Notifier.Notify(summary, body, actions, func(key string) {
			if key == "auto" {
				if err := a.Hub.ClearOverride(ctx); err != nil {
					a.Log.Warn("clear override", "err", err)
				}
			}
		})
		if err != nil {
			a.Log.Warn("notify", "err", err)
		}
	}
}

var statusText = map[string]string{
	"dnd": "🔴 Do not disturb", "working": "🟡 Working", "free": "🟢 Come in", "away": "⚫ Off",
}

var reasonText = map[string]string{
	"override": "set by hand", "pomodoro": "pomodoro", "pomodoro_break": "pomodoro break",
	"microphone": "microphone in use", "camera": "camera in use", "screen_locked": "screen locked",
	"pc_off": "PC off", "companion_lost": "companion not responding", "companion_offline": "PC not running",
	"calendar": "calendar", "work_hours": "working hours", "signal": "signal", "default": "nothing planned",
}

// Describe turns a state into a notification text, e.g.
// "🔴 Do not disturb" / "microphone in use – Firefox · until 14:30".
func Describe(st hubclient.State) (string, string) {
	summary := statusText[st.Status]
	if summary == "" {
		summary = st.Status
	}
	if st.Phase == "fault" {
		summary = "🟡 Fault"
	}
	parts := []string{reasonText[st.Reason]}
	if parts[0] == "" {
		parts[0] = st.Reason
	}
	if st.Detail != "" {
		parts[0] += " – " + strings.ReplaceAll(st.Detail, "_", " ")
	}
	if st.Until != nil {
		parts = append(parts, fmt.Sprintf("until %s", st.Until.Local().Format("15:04")))
	}
	return summary, strings.Join(parts, " · ")
}
