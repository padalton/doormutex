// Package streamdeck is the doormutex Stream Deck plugin: keys that show
// the light live and switch it. It speaks the Stream Deck SDK's WebSocket
// protocol, so it runs in Elgato's app (Windows, macOS) and in OpenDeck
// (Linux) – as a native binary, no Node.js needed.
package streamdeck

import (
	"fmt"
	"strings"
	"time"

	"github.com/padalton/doormutex/companion/internal/hubclient"
)

// Lamp is one of the light's lamps, or "off" (away).
type Lamp string

const (
	Red    Lamp = "red"
	Yellow Lamp = "yellow"
	Green  Lamp = "green"
	Off    Lamp = "off"
)

// statusFor is the status a lamp key sets.
var statusFor = map[Lamp]string{Red: "dnd", Yellow: "working", Green: "free", Off: "away"}

// blinkPeriod is a half period of the blinking patterns, as on the light.
const blinkPeriod = 500 * time.Millisecond

// blinking reports whether a pattern changes over time.
func blinking(light string) bool {
	switch light {
	case "green_blink", "red_yellow_alternate", "yellow_blink":
		return true
	}
	return false
}

// litAt reports whether lamp is lit in the pattern light at time t, like
// the light itself: blinking patterns start with their lit step.
func litAt(light string, lamp Lamp, t time.Time) bool {
	even := (t.UnixMilli()/blinkPeriod.Milliseconds())%2 == 0
	switch light {
	case "red":
		return lamp == Red
	case "yellow":
		return lamp == Yellow
	case "green":
		return lamp == Green
	case "red_yellow":
		return lamp == Red || lamp == Yellow
	case "green_blink":
		return lamp == Green && even
	case "red_yellow_alternate":
		return (lamp == Red && even) || (lamp == Yellow && !even)
	case "yellow_blink":
		return lamp == Yellow && even
	}
	return false
}

// overrideRequest is what a lamp key sends, for a duration setting:
// "15m", "1h" … or one of the hub's modes.
func overrideRequest(lamp Lamp, duration string) hubclient.OverrideRequest {
	r := hubclient.OverrideRequest{Status: statusFor[lamp]}
	switch duration {
	case "end_of_event", "end_of_workday", "next_change", "indefinite":
		r.Mode = duration
	case "":
		r.Mode = "next_change"
	default:
		r.Mode, r.Duration = "duration", duration
	}
	return r
}

// pomodoroPhase is where a running pomodoro is at now: in a work or a
// break block, and how long until it ends.
func pomodoroPhase(p *hubclient.Pomodoro, now time.Time) (work bool, left time.Duration, ok bool) {
	if p == nil {
		return false, 0, false
	}
	w, err1 := time.ParseDuration(p.Work)
	b, err2 := time.ParseDuration(p.Break)
	if err1 != nil || err2 != nil || w <= 0 || b < 0 {
		return false, 0, false
	}
	elapsed := now.Sub(p.Start)
	if elapsed < 0 {
		return true, w, true
	}
	if p.End != nil && !now.Before(*p.End) {
		return false, 0, false
	}
	pos := elapsed % (w + b)
	if pos < w {
		return true, w - pos, true
	}
	return false, w + b - pos, true
}

// ---- texts

// texts in the light's wording (the Plasma widget's translations).
var texts = map[string]map[string]string{
	"en": {
		"free": "Come in", "working": "Working", "dnd": "Do not\ndisturb", "away": "Not here",
		"until": "until %s", "auto": "Auto", "offline": "Hub\noffline", "setup": "Set up\nhub",
		"work": "Work", "break": "Break", "start": "Start",
	},
	"de": {
		"free": "Herein", "working": "Ich\narbeite", "dnd": "Bitte nicht\nstören", "away": "Nicht da",
		"until": "bis %s", "auto": "Auto", "offline": "Hub nicht\nerreichbar", "setup": "Hub\neinrichten",
		"work": "Arbeit", "break": "Pause", "start": "Start",
	},
}

// language picks "de" for any German locale, "en" otherwise.
func language(code string) string {
	if strings.HasPrefix(strings.ToLower(code), "de") {
		return "de"
	}
	return "en"
}

func text(lang, key string) string {
	if t, ok := texts[lang][key]; ok {
		return t
	}
	return texts["en"][key]
}

// clock formats a time as on the light: HH:MM today, otherwise with the
// weekday.
func clock(t, now time.Time, lang string) string {
	t = t.In(now.Location())
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	day := t.Format("Mon")
	if lang == "de" {
		day = [...]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}[t.Weekday()]
	}
	return day + " " + t.Format("15:04")
}

// statusTitle is the status key's title: the status and until when.
func statusTitle(st *hubclient.State, now time.Time, lang string) string {
	s := text(lang, st.Status)
	if st.Until != nil {
		s += "\n" + fmt.Sprintf(text(lang, "until"), clock(*st.Until, now, lang))
	}
	return s
}

// lampTitle is a lamp key's title: until when, on the key that is set.
func lampTitle(st *hubclient.State, lamp Lamp, now time.Time, lang string) string {
	if st.Override == nil || st.Override.Status != statusFor[lamp] || st.Until == nil {
		return ""
	}
	return fmt.Sprintf(text(lang, "until"), clock(*st.Until, now, lang))
}

// pomodoroTitle: the time left in the current block, or the work length
// to start with.
func pomodoroTitle(st *hubclient.State, work string, now time.Time, lang string) string {
	if st != nil {
		if w, left, ok := pomodoroPhase(st.Pomodoro, now); ok {
			label := text(lang, "break")
			if w {
				label = text(lang, "work")
			}
			left = left.Round(time.Second)
			return fmt.Sprintf("%s\n%d:%02d", label, int(left.Minutes()), int(left.Seconds())%60)
		}
	}
	d, err := time.ParseDuration(work)
	if err != nil {
		d = 25 * time.Minute
	}
	return fmt.Sprintf("%d min", int(d.Minutes())) // emoji may lack a font on the key
}
