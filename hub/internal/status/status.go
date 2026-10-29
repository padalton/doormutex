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
	PhaseLastCall   Phase = "last_call"   // green blinks: a meeting starts soon
	PhaseEndingSoon Phase = "ending_soon" // red+yellow: the meeting ends soon
)

// Light is the lamp pattern the traffic light should show.
type Light string

const (
	LightOff         Light = "off"
	LightRed         Light = "red"
	LightYellow      Light = "yellow"
	LightGreen       Light = "green"
	LightGreenBlink  Light = "green_blink"
	LightRedYellow   Light = "red_yellow"
	LightYellowBlink Light = "yellow_blink" // failsafe, decided by the device itself
)

// LightFor maps a status and phase to the lamp pattern.
func LightFor(s Status, p Phase) Light {
	switch s {
	case DND:
		if p == PhaseEndingSoon {
			return LightRedYellow
		}
		return LightRed
	case Working:
		return LightYellow
	case Free:
		if p == PhaseLastCall {
			return LightGreenBlink
		}
		return LightGreen
	default:
		return LightOff
	}
}
