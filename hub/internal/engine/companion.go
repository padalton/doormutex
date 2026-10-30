package engine

import (
	"strings"
	"time"

	"github.com/padalton/doormutex/hub/internal/status"
)

// Goodbye values: the companion signs off before the PC goes away.
const (
	GoodbyeShutdown = "shutdown"
	GoodbyeSuspend  = "suspend"
)

// Report is one heartbeat from the desktop companion. A non-empty Goodbye
// means the companion signs off; silence after that is expected.
type Report struct {
	Locked     bool
	Microphone bool
	Camera     bool
	MicApps    []string
	CamApps    []string
	Goodbye    string
}

// Activity tracks one media device over time to debounce and hold it.
type Activity struct {
	On        bool      `json:"on"`
	OnSince   time.Time `json:"on_since,omitzero"`
	HoldStart time.Time `json:"hold_start,omitzero"` // start of the last qualifying run
	HoldUntil time.Time `json:"hold_until,omitzero"` // keep the claim until then
	Apps      []string  `json:"apps,omitempty"`
}

func (a *Activity) observe(now time.Time, on bool, apps []string, r Rules) {
	switch {
	case on && !a.On:
		a.On, a.OnSince = true, now
	case !on && a.On:
		a.On = false
		if now.Sub(a.OnSince) >= r.MediaDebounce {
			a.HoldStart, a.HoldUntil = a.OnSince.Add(r.MediaDebounce), now.Add(r.MediaHold)
		}
	}
	if on {
		a.Apps = apps
	}
}

// Companion is the tracked state of the desktop companion.
type Companion struct {
	LastSeen    time.Time `json:"last_seen,omitzero"`
	Locked      bool      `json:"locked"`
	LockedSince time.Time `json:"locked_since,omitzero"`
	Mic         Activity  `json:"microphone"`
	Cam         Activity  `json:"camera"`
	SignedOff   string    `json:"signed_off,omitempty"` // goodbye reason; cleared by the next heartbeat
	SignedOffAt time.Time `json:"signed_off_at,omitzero"`
}

// Observe applies a heartbeat.
func (c *Companion) Observe(now time.Time, rep Report, r Rules) {
	if rep.Goodbye != "" {
		c.SignedOff, c.SignedOffAt = rep.Goodbye, now
		c.Mic.observe(now, false, nil, r)
		c.Cam.observe(now, false, nil, r)
		c.LastSeen = now
		return
	}
	c.SignedOff, c.SignedOffAt = "", time.Time{}
	if rep.Locked && (!c.Locked || c.Stale(now, r)) {
		c.LockedSince = now
	}
	c.Locked = rep.Locked
	c.Mic.observe(now, rep.Microphone, rep.MicApps, r)
	c.Cam.observe(now, rep.Camera, rep.CamApps, r)
	c.LastSeen = now
}

// Stale reports whether the companion has been silent too long.
func (c *Companion) Stale(now time.Time, r Rules) bool {
	return c.LastSeen.IsZero() || now.Sub(c.LastSeen) > r.StaleAfter
}

// mediaClaims returns dnd claims for an active (debounced) or recently
// stopped microphone/camera. An active run has no known end; if a calendar
// meeting is running, we assume the call ends with the meeting.
func (c *Companion) mediaClaims(now time.Time, r Rules, meetingEnd time.Time) []Claim {
	if c.Stale(now, r) || c.SignedOff != "" {
		return nil
	}
	var cs []Claim
	for _, m := range []struct {
		a      Activity
		reason string
	}{{c.Cam, ReasonCamera}, {c.Mic, ReasonMicrophone}} {
		a := m.a
		detail := strings.Join(a.Apps, ", ")
		if a.On && now.Sub(a.OnSince) >= r.MediaDebounce {
			end := time.Time{}
			if meetingEnd.After(now) {
				end = meetingEnd
			}
			cs = append(cs, Claim{Status: status.DND, Priority: PrioMedia, Reason: m.reason, Detail: detail,
				Start: a.OnSince.Add(r.MediaDebounce), End: end})
		}
		if a.HoldUntil.After(now) {
			cs = append(cs, Claim{Status: status.DND, Priority: PrioMedia, Reason: m.reason, Detail: detail,
				Start: a.HoldStart, End: a.HoldUntil})
		}
	}
	return cs
}

// presenceClaims returns away claims for a PC that is off, locked or
// whose companion went silent. Silence without a goodbye during working
// hours is a fault until the end of the work day, then plain away.
func (c *Companion) presenceClaims(now time.Time, r Rules, endOfWorkday func(time.Time) (time.Time, bool)) []Claim {
	switch {
	case c.SignedOff != "":
		return []Claim{{Status: status.Away, Priority: PrioPresence, Reason: ReasonPCOff, Detail: c.SignedOff,
			Start: c.SignedOffAt}}
	case c.Stale(now, r):
		if !r.RequireCompanion {
			return nil
		}
		if c.LastSeen.IsZero() { // never seen: away for the whole window
			return []Claim{{Status: status.Away, Priority: PrioPresence, Reason: ReasonCompanionOffline}}
		}
		start := c.LastSeen.Add(r.StaleAfter)
		if end, ok := endOfWorkday(start); ok {
			return []Claim{
				{Status: status.Away, Priority: PrioPresence, Reason: ReasonCompanionLost, Start: start, End: end},
				{Status: status.Away, Priority: PrioPresence, Reason: ReasonCompanionOffline, Start: end},
			}
		}
		return []Claim{{Status: status.Away, Priority: PrioPresence, Reason: ReasonCompanionOffline, Start: start}}
	case c.Locked:
		return []Claim{{Status: status.Away, Priority: PrioPresence, Reason: ReasonScreenLocked, Start: c.LockedSince}}
	}
	return nil
}
