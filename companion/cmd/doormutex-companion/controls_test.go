package main

import (
	"testing"

	"github.com/padalton/doormutex/companion/internal/hubclient"
)

func TestParseSet(t *testing.T) {
	for _, c := range []struct {
		args  []string
		want  hubclient.OverrideRequest
		clear bool
		err   bool
	}{
		{args: []string{"red"}, want: hubclient.OverrideRequest{Status: "dnd", Mode: "next_change"}},
		{args: []string{"Yellow", "30m"}, want: hubclient.OverrideRequest{Status: "working", Mode: "duration", Duration: "30m0s"}},
		{args: []string{"green", "1h30m"}, want: hubclient.OverrideRequest{Status: "free", Mode: "duration", Duration: "1h30m0s"}},
		{args: []string{"off", "event"}, want: hubclient.OverrideRequest{Status: "away", Mode: "end_of_event"}},
		{args: []string{"dnd", "workday"}, want: hubclient.OverrideRequest{Status: "dnd", Mode: "end_of_workday"}},
		{args: []string{"free", "forever"}, want: hubclient.OverrideRequest{Status: "free", Mode: "indefinite"}},
		{args: []string{"red", "next"}, want: hubclient.OverrideRequest{Status: "dnd", Mode: "next_change"}},
		{args: []string{"auto"}, clear: true},
		{args: []string{"auto", "1h"}, err: true},
		{args: []string{"blue"}, err: true},
		{args: []string{"red", "soon"}, err: true},
		{args: []string{"red", "-5m"}, err: true},
		{args: nil, err: true},
	} {
		got, clear, err := parseSet(c.args)
		if (err != nil) != c.err || clear != c.clear || (!c.err && got != c.want) {
			t.Errorf("parseSet(%q) = %+v, %v, %v; want %+v, %v, err %v", c.args, got, clear, err, c.want, c.clear, c.err)
		}
	}
}

func TestParsePomodoro(t *testing.T) {
	for _, c := range []struct {
		args              []string
		work, brk, status string
		stop, err         bool
	}{
		{args: []string{"25"}, work: "25m", brk: "5m"},
		{args: []string{"50", "yellow"}, work: "50m", brk: "10m", status: "working"},
		{args: []string{"45m", "15m", "red"}, work: "45m", brk: "15m", status: "dnd"},
		{args: []string{"stop"}, stop: true},
		{args: []string{"3"}, work: "3m", brk: "1m"},
		{args: []string{"abc"}, err: true},
		{args: []string{"25m", "x"}, err: true},
		{args: nil, err: true},
	} {
		w, b, s, stop, err := parsePomodoro(c.args)
		if (err != nil) != c.err || stop != c.stop || w != c.work || b != c.brk || s != c.status {
			t.Errorf("parsePomodoro(%q) = %q %q %q %v %v", c.args, w, b, s, stop, err)
		}
	}
}
