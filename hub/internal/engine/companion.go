package engine

import (
	"strings"
	"time"

	"github.com/padalton/office-traffic-light/hub/internal/status"
)

// Report is one heartbeat from the desktop companion.
type Report struct {
	Locked     bool
	Microphone bool
	Camera     bool
	MicApps    []string
	CamApps    []string
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
}

// Observe applies a heartbeat.
func (c *Companion) Observe(now time.Time, rep Report, r Rules) {
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
	if c.Stale(now, r) {
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

// presenceClaim returns an away claim when the screen is locked or the
// companion went silent.
func (c *Companion) presenceClaim(now time.Time, r Rules) *Claim {
	if c.Stale(now, r) {
		if !r.RequireCompanion {
			return nil
		}
		var start time.Time // never seen: away for the whole window
		if !c.LastSeen.IsZero() {
			start = c.LastSeen.Add(r.StaleAfter)
		}
		return &Claim{Status: status.Away, Priority: PrioPresence, Reason: ReasonCompanionOffline, Start: start}
	}
	if c.Locked {
		return &Claim{Status: status.Away, Priority: PrioPresence, Reason: ReasonScreenLocked, Start: c.LockedSince}
	}
	return nil
}
