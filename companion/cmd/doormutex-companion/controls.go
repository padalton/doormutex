package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/padalton/doormutex/companion/internal/agent"
	"github.com/padalton/doormutex/companion/internal/hubclient"
)

// Commands for keyboard shortcuts, Stream Deck "run command" keys and
// scripts: they use the hub address and token of the companion config.

const setUsage = `usage: doormutex-companion set LIGHT [UNTIL]
  LIGHT  red | yellow | green | off | auto  (or dnd, working, free, away)
  UNTIL  next (default: until the next automatic change) | 30m, 1h30m … |
         event (end of the current event) | workday | forever`

// parseSet turns "set" arguments into an override request; clear is true
// for "auto".
func parseSet(args []string) (o hubclient.OverrideRequest, clear bool, err error) {
	if len(args) < 1 || len(args) > 2 {
		return o, false, errors.New(setUsage)
	}
	switch strings.ToLower(args[0]) {
	case "auto", "automatic":
		if len(args) > 1 {
			return o, false, errors.New("auto takes no duration")
		}
		return o, true, nil
	case "red", "dnd":
		o.Status = "dnd"
	case "yellow", "working":
		o.Status = "working"
	case "green", "free":
		o.Status = "free"
	case "off", "away":
		o.Status = "away"
	default:
		return o, false, fmt.Errorf("unknown light %q\n%s", args[0], setUsage)
	}
	o.Mode = "next_change"
	if len(args) == 1 {
		return o, false, nil
	}
	switch u := strings.ToLower(args[1]); u {
	case "next":
	case "event":
		o.Mode = "end_of_event"
	case "workday", "day":
		o.Mode = "end_of_workday"
	case "forever", "indefinite":
		o.Mode = "indefinite"
	default:
		d, perr := time.ParseDuration(u)
		if perr != nil || d <= 0 {
			return o, false, fmt.Errorf("unknown duration %q\n%s", args[1], setUsage)
		}
		o.Mode, o.Duration = "duration", d.String()
	}
	return o, false, nil
}

func setCmd(path string, args []string) error {
	o, clear, err := parseSet(args)
	if err != nil {
		return err
	}
	hub, err := client(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if clear {
		if err := hub.ClearOverride(ctx); err != nil {
			return err
		}
		return printStatus(ctx, hub)
	}
	st, err := hub.SetOverride(ctx, o)
	if err != nil {
		return err
	}
	return describe(st)
}

func statusCmd(path string) error {
	hub, err := client(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return printStatus(ctx, hub)
}

// pomodoroCmd: "pomodoro 25" (25/5), "pomodoro 50" (50/10), "pomodoro
// WORK BREAK" with durations, or "pomodoro stop". A trailing "yellow" or
// "red" picks the light for the work phases.
func pomodoroCmd(path string, args []string) error {
	work, brk, ws, stop, err := parsePomodoro(args)
	if err != nil {
		return err
	}
	hub, err := client(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if stop {
		if err := hub.StopPomodoro(ctx); err != nil {
			return err
		}
		return printStatus(ctx, hub)
	}
	st, err := hub.StartPomodoro(ctx, work, brk, 4, ws)
	if err != nil {
		return err
	}
	return describe(st)
}

const pomodoroUsage = "usage: doormutex-companion pomodoro 25 | 50 | WORK BREAK | stop  [red | yellow]"

func parsePomodoro(args []string) (work, brk, workStatus string, stop bool, err error) {
	if n := len(args); n > 0 {
		switch strings.ToLower(args[n-1]) {
		case "red", "dnd":
			workStatus, args = "dnd", args[:n-1]
		case "yellow", "working":
			workStatus, args = "working", args[:n-1]
		}
	}
	switch {
	case len(args) == 1 && strings.EqualFold(args[0], "stop"):
		return "", "", "", true, nil
	case len(args) == 1:
		m, aerr := strconv.Atoi(args[0])
		if aerr != nil || m <= 0 {
			return "", "", "", false, errors.New(pomodoroUsage)
		}
		// Classic ratio: a fifth of the work time as break.
		return fmt.Sprintf("%dm", m), fmt.Sprintf("%dm", max(1, m/5)), workStatus, false, nil
	case len(args) == 2:
		for _, a := range args {
			if d, perr := time.ParseDuration(a); perr != nil || d <= 0 {
				return "", "", "", false, errors.New(pomodoroUsage)
			}
		}
		return args[0], args[1], workStatus, false, nil
	}
	return "", "", "", false, errors.New(pomodoroUsage)
}

func client(path string) (*hubclient.Client, error) {
	cfg, err := load(path)
	if err != nil {
		return nil, err
	}
	return &hubclient.Client{URL: cfg.Hub, Token: cfg.Token}, nil
}

func printStatus(ctx context.Context, hub *hubclient.Client) error {
	st, err := hub.Status(ctx)
	if err != nil {
		return err
	}
	return describe(st)
}

func describe(st hubclient.State) error {
	summary, body := agent.Describe(st)
	fmt.Printf("%s – %s\n", summary, body)
	return nil
}
