package engine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/holidays"
	"github.com/padalton/doormutex/hub/internal/schedule"
	"github.com/padalton/doormutex/hub/internal/status"
)

var vienna = mustLoc("Europe/Vienna")

func mustLoc(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// at returns a time on Wednesday 2026-10-28 in Vienna.
func at(hh, mm int) time.Time { return time.Date(2026, 10, 28, hh, mm, 0, 0, vienna) }

func ptr(t time.Time) *time.Time { return &t }

// base returns inputs with Mon–Fri 08:00–17:00, AT holidays and a fresh,
// unlocked companion as seen at now.
func base(now time.Time) Inputs {
	week, err := schedule.ParseWeek(map[string][]string{
		"monday": {"08:00-17:00"}, "tuesday": {"08:00-17:00"}, "wednesday": {"08:00-17:00"},
		"thursday": {"08:00-17:00"}, "friday": {"08:00-17:00"},
	})
	if err != nil {
		panic(err)
	}
	hol, _ := holidays.New("AT", nil)
	return Inputs{
		Location:  vienna,
		Rules:     DefaultRules(),
		WorkWeek:  week,
		Holidays:  hol,
		Companion: Companion{LastSeen: now.Add(-5 * time.Second)},
	}
}

func meeting(kind calendar.Kind, start, end time.Time) calendar.Event {
	return calendar.Event{ID: "e", Title: "secret title", Start: start, End: end, Kind: kind}
}

// heartbeat simulates the companion: report rep continuously from start to now.
func heartbeat(c *Companion, start, now time.Time, rep Report, r Rules) {
	for t := start; !t.After(now); t = t.Add(30 * time.Second) {
		c.Observe(t, rep, r)
	}
	c.Observe(now, rep, r)
}

type want struct {
	status   status.Status
	reason   string
	phase    status.Phase
	light    status.Light
	until    *time.Time // nil → expect unknown
	nextStat status.Status
	nextFree *time.Time // expected next_free.start; nil → expect none
	noCheck  bool       // skip until/next/next_free checks
}

func TestPriorityTable(t *testing.T) {
	cases := []struct {
		name  string
		now   time.Time
		setup func(in *Inputs, now time.Time)
		want  want
	}{
		// 7 – default
		{"7 default: free outside working hours", at(19, 0), nil,
			want{status: status.Free, reason: ReasonDefault, light: status.LightGreen,
				until: ptr(time.Date(2026, 10, 29, 8, 0, 0, 0, vienna)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},

		// 5 – working hours
		{"5 work hours: working, free at 17:00", at(10, 0), nil,
			want{status: status.Working, reason: ReasonWorkHours, light: status.LightYellow,
				until: ptr(at(17, 0)), nextStat: status.Free, nextFree: ptr(at(17, 0))}},
		{"5 holiday: Nationalfeiertag is free", time.Date(2026, 10, 26, 10, 0, 0, 0, vienna),
			func(in *Inputs, now time.Time) { in.Companion.LastSeen = now },
			want{status: status.Free, reason: ReasonDefault, noCheck: true}},
		{"5 holiday: meetings still count (holiday workers, self-employed)", time.Date(2026, 10, 26, 10, 10, 0, 0, vienna),
			func(in *Inputs, now time.Time) {
				in.Companion.LastSeen = now
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, now.Add(-10*time.Minute), now.Add(20*time.Minute))}
			},
			want{status: status.DND, reason: ReasonCalendar, noCheck: true}},
		{"5 all-day out of office: day off (vacation)", at(10, 0),
			func(in *Inputs, _ time.Time) {
				ooo := meeting(calendar.KindOutOfOffice, at(0, 0), at(0, 0).AddDate(0, 0, 1))
				ooo.AllDay = true
				in.Events = []calendar.Event{ooo}
			},
			want{status: status.Free, reason: ReasonDefault, light: status.LightGreen,
				until: ptr(time.Date(2026, 10, 29, 8, 0, 0, 0, vienna)), nextStat: status.Working, nextFree: ptr(time.Date(2026, 10, 27, 17, 0, 0, 0, vienna))}},

		// 4 – calendar
		{"4 calendar meeting: dnd until meeting end", at(10, 10),
			func(in *Inputs, _ time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
			},
			want{status: status.DND, reason: ReasonCalendar, light: status.LightRed,
				until: ptr(at(10, 30)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"4 calendar focus time: working", at(18, 30),
			func(in *Inputs, _ time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindFocus, at(18, 0), at(19, 0))}
			},
			want{status: status.Working, reason: ReasonCalendar, until: ptr(at(19, 0)), nextStat: status.Free, nextFree: ptr(at(19, 0))}},
		{"4 calendar timed out of office: dnd (private appointment blocked in the work calendar)", at(10, 0),
			func(in *Inputs, _ time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindOutOfOffice, at(9, 0), at(12, 0))}
			},
			want{status: status.DND, reason: ReasonCalendar, light: status.LightRed, until: ptr(at(12, 0)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"4 calendar overlap: the stricter status wins, not the later start", at(10, 15),
			func(in *Inputs, _ time.Time) {
				in.Events = []calendar.Event{
					meeting(calendar.KindVideoCall, at(10, 0), at(10, 30)),
					meeting(calendar.KindFocus, at(10, 10), at(11, 0)),
				}
			},
			want{status: status.DND, reason: ReasonCalendar, until: ptr(at(10, 30)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"4 calendar: all-day and transparent events are ignored", at(10, 0),
			func(in *Inputs, _ time.Time) {
				allDay := meeting(calendar.KindVideoCall, at(0, 0), at(0, 0).AddDate(0, 0, 1))
				allDay.AllDay = true
				free := meeting(calendar.KindVideoCall, at(9, 0), at(11, 0))
				free.Transparent = true
				in.Events = []calendar.Event{allDay, free}
			},
			want{status: status.Working, reason: ReasonWorkHours, noCheck: true}},
		{"4 calendar: other events count as working by default", at(19, 5),
			func(in *Inputs, _ time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindOther, at(19, 0), at(20, 0))}
			},
			want{status: status.Working, reason: ReasonCalendar, noCheck: true}},

		// 2 – presence
		{"2 screen locked: away, even during a meeting", at(10, 10),
			func(in *Inputs, now time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
				heartbeat(&in.Companion, now.Add(-5*time.Minute), now, Report{Locked: true}, in.Rules)
			},
			want{status: status.Away, reason: ReasonScreenLocked, light: status.LightOff, until: nil, nextFree: nil}},
		{"2 screen locked beats an override; the override survives", at(10, 0),
			func(in *Inputs, now time.Time) {
				in.Override = &Override{Status: status.DND, Start: now.Add(-time.Hour)}
				heartbeat(&in.Companion, now.Add(-time.Minute), now, Report{Locked: true}, in.Rules)
			},
			want{status: status.Away, reason: ReasonScreenLocked, light: status.LightOff, noCheck: true}},
		{"2 companion signed off (shutdown): off, no fault", at(10, 0),
			func(in *Inputs, now time.Time) {
				in.Companion.Observe(now.Add(-time.Hour), Report{Goodbye: GoodbyeShutdown}, in.Rules)
			},
			want{status: status.Away, reason: ReasonPCOff, phase: status.PhaseNormal, light: status.LightOff, until: nil, nextFree: nil}},
		{"2 companion silent without goodbye during work hours: fault", at(10, 0),
			func(in *Inputs, now time.Time) { in.Companion.LastSeen = now.Add(-2 * time.Minute) },
			want{status: status.Away, reason: ReasonCompanionLost, phase: status.PhaseFault, light: status.LightYellowBlink, until: nil, nextFree: nil}},
		{"2 companion lost: fault ends with the work day", at(17, 30),
			func(in *Inputs, _ time.Time) { in.Companion.LastSeen = at(16, 0) },
			want{status: status.Away, reason: ReasonCompanionOffline, phase: status.PhaseNormal, light: status.LightOff, until: nil, nextFree: nil}},
		{"2 companion silent outside work hours: off, no fault", at(19, 0),
			func(in *Inputs, now time.Time) { in.Companion.LastSeen = now.Add(-2 * time.Minute) },
			want{status: status.Away, reason: ReasonCompanionOffline, light: status.LightOff, until: nil, nextFree: nil}},
		{"2 companion never seen: away", at(10, 0),
			func(in *Inputs, _ time.Time) { in.Companion = Companion{} },
			want{status: status.Away, reason: ReasonCompanionOffline, until: nil, nextFree: nil}},
		{"2 companion not required: never seen is fine", at(10, 0),
			func(in *Inputs, _ time.Time) { in.Companion = Companion{}; in.Rules.RequireCompanion = false },
			want{status: status.Working, reason: ReasonWorkHours, noCheck: true}},

		// 1 – microphone / camera
		{"1 microphone active ≥10 s: dnd, beats a locked screen", at(10, 0),
			func(in *Inputs, now time.Time) {
				heartbeat(&in.Companion, now.Add(-time.Minute), now,
					Report{Locked: true, Microphone: true, MicApps: []string{"Firefox"}}, in.Rules)
			},
			want{status: status.DND, reason: ReasonMicrophone, light: status.LightRed, until: nil, nextFree: nil}},
		{"1 microphone active <10 s: not yet", at(10, 0),
			func(in *Inputs, now time.Time) {
				in.Companion.Observe(now.Add(-5*time.Second), Report{Microphone: true}, in.Rules)
				in.Companion.Observe(now, Report{Microphone: true}, in.Rules)
			},
			want{status: status.Working, reason: ReasonWorkHours, noCheck: true}},
		{"1 microphone stopped 1 min ago: still dnd (2 min hold)", at(10, 0),
			func(in *Inputs, now time.Time) {
				heartbeat(&in.Companion, now.Add(-10*time.Minute), now.Add(-time.Minute), Report{Microphone: true}, in.Rules)
				heartbeat(&in.Companion, now.Add(-time.Minute), now, Report{}, in.Rules)
			},
			want{status: status.DND, reason: ReasonMicrophone, until: ptr(at(10, 1)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"1 microphone stopped 3 min ago: hold expired", at(10, 0),
			func(in *Inputs, now time.Time) {
				heartbeat(&in.Companion, now.Add(-10*time.Minute), now.Add(-3*time.Minute), Report{Microphone: true}, in.Rules)
				heartbeat(&in.Companion, now.Add(-3*time.Minute), now, Report{}, in.Rules)
			},
			want{status: status.Working, reason: ReasonWorkHours, noCheck: true}},
		{"1 camera: dnd with reason camera", at(19, 0),
			func(in *Inputs, now time.Time) {
				heartbeat(&in.Companion, now.Add(-time.Minute), now, Report{Camera: true, CamApps: []string{"zoom"}}, in.Rules)
			},
			want{status: status.DND, reason: ReasonCamera, noCheck: true}},
		{"1 microphone during a calendar meeting: until meeting end", at(10, 10),
			func(in *Inputs, now time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
				heartbeat(&in.Companion, at(10, 0), now, Report{Microphone: true}, in.Rules)
			},
			want{status: status.DND, reason: ReasonMicrophone, until: ptr(at(10, 30)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},

		// 3 – manual
		{"1 microphone beats an override (spontaneous call)", at(10, 0),
			func(in *Inputs, now time.Time) {
				heartbeat(&in.Companion, now.Add(-time.Minute), now, Report{Microphone: true}, in.Rules)
				in.Override = &Override{Status: status.Free, Start: now.Add(-time.Minute)}
			},
			want{status: status.DND, reason: ReasonMicrophone, light: status.LightRed, until: nil, nextFree: nil}},
		{"3 override beats the calendar", at(10, 10),
			func(in *Inputs, now time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
				in.Override = &Override{Status: status.Free, Start: now.Add(-time.Minute)}
			},
			// Free right now, so the "next free slot" is the current one.
			want{status: status.Free, reason: ReasonOverride, light: status.LightGreen, until: nil, nextFree: ptr(at(10, 9))}},
		{"3 override with end", at(10, 0),
			func(in *Inputs, now time.Time) {
				in.Override = &Override{Status: status.DND, Start: now, Until: at(10, 30)}
			},
			want{status: status.DND, reason: ReasonOverride, until: ptr(at(10, 30)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"3 override expired: back to automatic", at(10, 31),
			func(in *Inputs, _ time.Time) {
				in.Override = &Override{Status: status.DND, Start: at(10, 0), Until: at(10, 30)}
			},
			want{status: status.Working, reason: ReasonWorkHours, noCheck: true}},

		// 6 – webhook signals
		{"6 signal: working outside work hours until TTL", at(19, 0),
			func(in *Inputs, now time.Time) {
				in.Signals = []Signal{{Name: "claude", Status: status.Working, Start: now.Add(-time.Minute), Expires: at(19, 29)}}
			},
			want{status: status.Working, reason: ReasonSignal, until: ptr(at(19, 29)), nextStat: status.Free, nextFree: ptr(at(19, 29))}},
		{"6 signal expired", at(19, 30),
			func(in *Inputs, _ time.Time) {
				in.Signals = []Signal{{Name: "claude", Status: status.Working, Start: at(19, 0), Expires: at(19, 29)}}
			},
			want{status: status.Free, reason: ReasonDefault, noCheck: true}},
		{"6 signal loses against calendar", at(19, 10),
			func(in *Inputs, now time.Time) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(19, 0), at(20, 0))}
				in.Signals = []Signal{{Name: "claude", Status: status.Working, Start: now, Expires: now.Add(time.Hour)}}
			},
			want{status: status.DND, reason: ReasonCalendar, noCheck: true}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base(c.now)
			if c.setup != nil {
				c.setup(&in, c.now)
			}
			got := Evaluate(c.now, in)
			check(t, got, c.want)
		})
	}
}

func TestPhases(t *testing.T) {
	sec := func(hh, mm, ss int) time.Time { return at(hh, mm).Add(time.Duration(ss) * time.Second) }
	evening := func(in *Inputs) { in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(19, 0), at(19, 30))} }
	morning := func(in *Inputs) { in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))} }
	lead := func(f func(*Inputs), d time.Duration) func(*Inputs) {
		return func(in *Inputs) { f(in); in.Rules.LastCall, in.Rules.EndingSoon = Lead(d), Lead(d) }
	}
	cases := []struct {
		name  string
		now   time.Time
		setup func(in *Inputs)
		want  want
	}{
		// From green: green blinks, then ≈3 s yellow, then red.
		{"stvo: green blinks 7–3 s before a meeting", sec(18, 59, 54), evening,
			want{status: status.Free, phase: status.PhaseLastCall, light: status.LightGreenBlink, noCheck: true}},
		{"stvo: yellow in the last 3 s", sec(18, 59, 58), evening,
			want{status: status.Free, phase: status.PhaseAmber, light: status.LightYellow, noCheck: true}},
		{"stvo: no warning 4 min before", at(18, 56), evening,
			want{status: status.Free, phase: status.PhaseNormal, light: status.LightGreen, noCheck: true}},
		{"lead 5m: green blinks 4 min before", at(18, 56), lead(evening, 5*time.Minute),
			want{status: status.Free, phase: status.PhaseLastCall, light: status.LightGreenBlink, until: ptr(at(19, 0)), nextStat: status.DND, nextFree: ptr(at(17, 0))}},
		{"lead 5m: still yellow in the last 3 s", sec(18, 59, 58), lead(evening, 5*time.Minute),
			want{status: status.Free, phase: status.PhaseAmber, light: status.LightYellow, noCheck: true}},
		{"lead 5m: no warning 10 min before", at(18, 50), lead(evening, 5*time.Minute),
			want{status: status.Free, phase: status.PhaseNormal, light: status.LightGreen, noCheck: true}},

		// From yellow: red and yellow alternate (the StVO has nothing for it).
		{"stvo: red/yellow alternate 4 s before a meeting during work hours", sec(9, 59, 57), morning,
			want{status: status.Working, phase: status.PhaseLastCall, light: status.LightRedYellowAlternate, noCheck: true}},
		{"lead 3m: red/yellow alternate 2 min before", at(9, 58), lead(morning, 3*time.Minute),
			want{status: status.Working, phase: status.PhaseLastCall, light: status.LightRedYellowAlternate, noCheck: true}},
		{"stvo: plain yellow a minute before", at(9, 59), morning,
			want{status: status.Working, phase: status.PhaseNormal, light: status.LightYellow, noCheck: true}},

		// Ending soon: red+yellow.
		{"stvo: red+yellow 1 s before the end", sec(10, 29, 59), morning,
			want{status: status.DND, phase: status.PhaseEndingSoon, light: status.LightRedYellow, noCheck: true}},
		{"stvo: plain red 3 min before the end", at(10, 27), morning,
			want{status: status.DND, phase: status.PhaseNormal, light: status.LightRed, noCheck: true}},
		{"lead 5m: red+yellow 3 min before the end", at(10, 27), lead(morning, 5*time.Minute),
			want{status: status.DND, phase: status.PhaseEndingSoon, light: status.LightRedYellow, until: ptr(at(10, 30)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"no ending soon for back-to-back meetings", at(10, 27),
			lead(func(in *Inputs) {
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30)), meeting(calendar.KindVideoCall, at(10, 30), at(11, 0))}
			}, 5*time.Minute),
			want{status: status.DND, phase: status.PhaseNormal, light: status.LightRed, until: ptr(at(11, 0)), nextStat: status.Working, nextFree: ptr(at(17, 0))}},
		{"no ending soon when the light goes off afterwards", at(10, 27),
			lead(func(in *Inputs) {
				in.Rules.CalendarStatus[calendar.KindOther] = status.Away
				in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30)), meeting(calendar.KindOther, at(10, 30), at(12, 0))}
			}, 5*time.Minute),
			want{status: status.DND, phase: status.PhaseNormal, noCheck: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base(c.now)
			c.setup(&in)
			check(t, Evaluate(c.now, in), c.want)
		})
	}
}

func TestLeadText(t *testing.T) {
	for in, want := range map[string]Lead{"stvo": 0, "5m": Lead(5 * time.Minute), "3m0s": Lead(3 * time.Minute)} {
		var l Lead
		if err := l.UnmarshalText([]byte(in)); err != nil || l != want {
			t.Errorf("UnmarshalText(%q) = %v, %v", in, l, err)
		}
	}
	for _, bad := range []string{"-1m", "2h", "soon"} {
		var l Lead
		if err := l.UnmarshalText([]byte(bad)); err == nil {
			t.Errorf("UnmarshalText(%q) accepted", bad)
		}
	}
	if b, _ := Lead(0).MarshalText(); string(b) != "stvo" {
		t.Errorf("MarshalText(0) = %s", b)
	}
}

func TestPomodoro(t *testing.T) {
	in := base(at(18, 10))
	in.Rules.RequireCompanion = false // evaluated at several times without heartbeats
	in.Pomodoro = &Pomodoro{Start: at(18, 0), Work: 25 * time.Minute, Break: 5 * time.Minute, Cycles: 2}

	// Work phases are red by default.
	check(t, Evaluate(at(18, 10), in), want{status: status.DND, reason: ReasonPomodoro, light: status.LightRed,
		until: ptr(at(18, 25)), nextStat: status.Free, nextFree: ptr(at(18, 25))})
	check(t, Evaluate(at(18, 27), in), want{status: status.Free, reason: ReasonPomodoroBreak, noCheck: true})
	check(t, Evaluate(at(18, 40), in), want{status: status.DND, reason: ReasonPomodoro, until: ptr(at(18, 55)),
		nextStat: status.Free, nextFree: ptr(at(18, 55))})
	// Two cycles end at 19:00; after that the default applies.
	check(t, Evaluate(at(19, 5), in), want{status: status.Free, reason: ReasonDefault, noCheck: true})
	if end := in.Pomodoro.End(); !end.Equal(at(19, 0)) {
		t.Errorf("Pomodoro.End() = %v", end)
	}
	in.Pomodoro.WorkStatus = status.Working
	check(t, Evaluate(at(18, 10), in), want{status: status.Working, reason: ReasonPomodoro, noCheck: true})
}

func TestGoodbyeThenHeartbeat(t *testing.T) {
	in := base(at(10, 0))
	in.Companion.Observe(at(9, 0), Report{Goodbye: GoodbyeSuspend}, in.Rules)
	check(t, Evaluate(at(10, 0), in), want{status: status.Away, reason: ReasonPCOff, noCheck: true})
	in.Companion.Observe(at(10, 0), Report{}, in.Rules)
	check(t, Evaluate(at(10, 0), in), want{status: status.Working, reason: ReasonWorkHours, noCheck: true})
}

func TestStateIsStableBetweenTicks(t *testing.T) {
	in := base(at(10, 0))
	a := Evaluate(at(10, 0), in)
	in.Companion.LastSeen = at(10, 0).Add(time.Second)
	b := Evaluate(at(10, 0).Add(time.Second), in)
	if !a.SameAs(b) {
		t.Errorf("state changed between ticks:\n%+v\n%+v", a, b)
	}
	if !a.Since.Equal(at(8, 0)) {
		t.Errorf("since = %v, want 08:00", a.Since)
	}
	// Never-seen companion: away since the (stable) window start.
	in.Companion = Companion{}
	c, d := Evaluate(at(10, 0), in), Evaluate(at(10, 0).Add(time.Second), in)
	if !c.SameAs(d) {
		t.Errorf("away state not stable:\n%+v\n%+v", c, d)
	}
}

func TestEventTitlesNeverLeak(t *testing.T) {
	in := base(at(10, 10))
	in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
	st := Evaluate(at(10, 10), in)
	if st.Detail == "secret title" || st.Reason == "secret title" {
		t.Fatal("event title leaked into the state")
	}
}

func TestTimeline(t *testing.T) {
	in := base(at(7, 0))
	in.Events = []calendar.Event{meeting(calendar.KindVideoCall, at(10, 0), at(10, 30))}
	// Live signals must not appear in the plan.
	heartbeat(&in.Companion, at(6, 50), at(7, 0), Report{Locked: true}, in.Rules)
	slots := Timeline(at(7, 0), at(7, 0), at(18, 0), in)
	wantSlots := []struct {
		st         status.Status
		start, end time.Time
	}{
		{status.Free, at(7, 0), at(8, 0)},
		{status.Working, at(8, 0), at(10, 0)},
		{status.DND, at(10, 0), at(10, 30)},
		{status.Working, at(10, 30), at(17, 0)},
		{status.Free, at(17, 0), at(18, 0)},
	}
	if len(slots) != len(wantSlots) {
		t.Fatalf("got %d slots: %+v", len(slots), slots)
	}
	for i, w := range wantSlots {
		s := slots[i]
		if s.Status != w.st || !s.Since.Equal(w.start) || !s.Until.Equal(w.end) {
			t.Errorf("slot %d = %s %v–%v, want %s %v–%v", i, s.Status, s.Since, *s.Until, w.st, w.start, w.end)
		}
	}
}

func check(t *testing.T, got State, w want) {
	t.Helper()
	if got.Status != w.status {
		t.Fatalf("status = %s (reason %s), want %s", got.Status, got.Reason, w.status)
	}
	if w.reason != "" && got.Reason != w.reason {
		t.Errorf("reason = %s, want %s", got.Reason, w.reason)
	}
	if w.phase != "" && got.Phase != w.phase {
		t.Errorf("phase = %s, want %s", got.Phase, w.phase)
	}
	if w.light != "" && got.Light != w.light {
		t.Errorf("light = %s, want %s", got.Light, w.light)
	}
	if w.noCheck {
		return
	}
	if !equalTimePtr(got.Until, w.until) {
		t.Errorf("until = %v, want %v", fmtPtr(got.Until), fmtPtr(w.until))
	}
	if w.nextStat != "" {
		if got.Next == nil || got.Next.Status != w.nextStat {
			t.Errorf("next = %+v, want status %s", got.Next, w.nextStat)
		}
	} else if got.Next != nil {
		t.Errorf("next = %+v, want none", got.Next)
	}
	var nf *time.Time
	if got.NextFree != nil {
		nf = &got.NextFree.Start
	}
	if !equalTimePtr(nf, w.nextFree) {
		t.Errorf("next_free.start = %v, want %v", fmtPtr(nf), fmtPtr(w.nextFree))
	}
}

func fmtPtr(t *time.Time) string {
	if t == nil {
		return "<none>"
	}
	return t.Format("2006-01-02 15:04")
}

func TestPomodoroJSON(t *testing.T) {
	p := Pomodoro{Start: at(9, 0), Work: 25 * time.Minute, Break: 5 * time.Minute, Cycles: 2, WorkStatus: status.DND}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"work":"25m0s"`) || !strings.Contains(string(b), `"end":"2026-10-28T10:00:00+01:00"`) {
		t.Errorf("json = %s", b)
	}
	var back Pomodoro
	if err := json.Unmarshal(b, &back); err != nil || !back.Start.Equal(p.Start) || back.Work != p.Work || back.Break != p.Break || back.Cycles != p.Cycles {
		t.Errorf("round trip = %+v, %v", back, err)
	}
}

func TestTentative(t *testing.T) {
	now := at(10, 10)
	call := meeting(calendar.KindVideoCall, at(10, 0), at(11, 0))
	call.Tentative = true
	for setting, want := range map[string]status.Status{
		"":              status.Working, // default: yellow
		TentativeKind:   status.DND,     // like an accepted call
		TentativeIgnore: status.Working, // working hours
		"free":          status.Free,
	} {
		in := base(now)
		if setting != "" {
			in.Rules.Tentative = setting
		}
		in.Events = []calendar.Event{call}
		st := Evaluate(now, in)
		if st.Status != want {
			t.Errorf("tentative=%q: %s, want %s", setting, st.Status, want)
		}
		if setting == TentativeIgnore && st.Reason != ReasonWorkHours {
			t.Errorf("ignored tentative event still counts: %s", st.Reason)
		}
	}
	// A kind that is ignored stays ignored, even if tentative events are red.
	in := base(now)
	in.Rules.Tentative = "dnd"
	delete(in.Rules.CalendarStatus, calendar.KindVideoCall)
	in.Events = []calendar.Event{call}
	if st := Evaluate(now, in); st.Reason != ReasonWorkHours {
		t.Errorf("ignored kind: %+v", st)
	}
	for _, v := range []string{"kind", "ignore", "dnd"} {
		if CheckTentative(v) != nil {
			t.Errorf("%s rejected", v)
		}
	}
	if CheckTentative("maybe") == nil {
		t.Error("maybe accepted")
	}
}

func TestDefaultStatus(t *testing.T) {
	in := base(at(19, 0))
	in.Rules.Default = status.Away
	check(t, Evaluate(at(19, 0), in), want{status: status.Away, reason: ReasonDefault, light: status.LightOff, noCheck: true})
}
