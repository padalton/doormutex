// Package status defines the vocabulary shared by all hub components:
// the four presence states, the signal phases and the lamp patterns.
package status

import "fmt"

// Status is what the family should do.
type Status string

const (
	Free    Status = "free"    // green: come in
	Working Status = "working" // yellow: only if it's important
	DND     Status = "dnd"     // red: meeting, do not disturb
	Away    Status = "away"    // off: not in the office
)

// All lists every status in a stable order (used for HA enum options).
var All = []Status{Free, Working, DND, Away}

// Parse validates a status string.
func Parse(s string) (Status, error) {
	for _, st := range All {
		if string(st) == s {
			return st, nil
		}
	}
	return "", fmt.Errorf("unknown status %q (want one of free, working, dnd, away)", s)
}

// Phase refines a status with what is about to happen.
type Phase string

const (
	PhaseNormal     Phase = "normal"
	PhaseLastCall   Phase = "last_call"   // a meeting starts soon: green blinks (from free) or red/yellow alternate (from working)
	PhaseAmber      Phase = "amber"       // yellow shows briefly before red, like on the road
	PhaseEndingSoon Phase = "ending_soon" // red+yellow: the meeting ends soon
	PhaseFault      Phase = "fault"       // the companion went silent without signing off
)

// Light is the lamp pattern the traffic light should show.
type Light string

const (
	LightOff                Light = "off"
	LightRed                Light = "red"
	LightYellow             Light = "yellow"
	LightGreen              Light = "green"
	LightGreenBlink         Light = "green_blink"
	LightRedYellow          Light = "red_yellow"
	LightRedYellowAlternate Light = "red_yellow_alternate"
	LightYellowBlink        Light = "yellow_blink" // fault; the device also falls back to it when the hub is gone
)

// LightFor maps a status and phase to the lamp pattern.
func LightFor(s Status, p Phase) Light {
	if p == PhaseFault {
		return LightYellowBlink
	}
	switch s {
	case DND:
		if p == PhaseEndingSoon {
			return LightRedYellow
		}
		return LightRed
	case Working:
		if p == PhaseLastCall {
			return LightRedYellowAlternate
		}
		return LightYellow
	case Free:
		switch p {
		case PhaseLastCall:
			return LightGreenBlink
		case PhaseAmber:
			return LightYellow
		}
		return LightGreen
	default:
		return LightOff
	}
}

// Severity orders statuses from most to least restrictive; it decides
// between claims of the same priority (e.g. overlapping calendar events).
func Severity(s Status) int {
	switch s {
	case DND:
		return 3
	case Working:
		return 2
	case Away:
		return 1
	}
	return 0
}
