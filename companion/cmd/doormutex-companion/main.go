// Command doormutex-companion runs on the desktop and tells the doormutex
// hub whether the microphone or camera is in use and whether the screen is
// locked. It signs off before suspend and shutdown and shows a
// notification on every change of the traffic light.
//
//	doormutex-companion [run] [-config path]   run (default; e.g. as a systemd user service)
//	doormutex-companion check [-config path]   print what it detects once and test the hub connection
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/padalton/doormutex/companion/internal/agent"
	"github.com/padalton/doormutex/companion/internal/camera"
	"github.com/padalton/doormutex/companion/internal/desktop"
	"github.com/padalton/doormutex/companion/internal/hubclient"
	"github.com/padalton/doormutex/companion/internal/pipewire"
)

var version = "dev"

// Config mirrors companion.example.yaml.
type Config struct {
	Hub       string        `yaml:"hub"`        // e.g. http://hub.lan:8080
	Token     string        `yaml:"token"`      // or token_file, or DOORMUTEX_TOKEN
	TokenFile string        `yaml:"token_file"` // e.g. ~/.config/doormutex/token
	Ignore    []string      `yaml:"ignore"`     // apps that never count (binary or app name)
	Notify    *bool         `yaml:"notify"`     // desktop notifications (default on)
	Poll      time.Duration `yaml:"poll"`
	Interval  time.Duration `yaml:"interval"`
}

func main() {
	args := os.Args[1:]
	cmd := "run"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", defaultConfig(), "config file")
	_ = fs.Parse(args)
	var err error
	switch cmd {
	case "run":
		err = run(*cfgPath)
	case "check":
		err = check(*cfgPath)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (want run, check, version)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "doormutex-companion:", err)
		os.Exit(1)
	}
}

func defaultConfig() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "companion.yaml"
	}
	return filepath.Join(dir, "doormutex", "companion.yaml")
}

func load(path string) (*Config, error) {
	c := &Config{Ignore: []string{"easyeffects", "pavucontrol", "pwvucontrol", "plasmashell"}}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w (see companion.example.yaml)", err)
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if v := os.Getenv("DOORMUTEX_TOKEN"); v != "" {
		c.Token = v
	}
	if c.Token == "" && c.TokenFile != "" {
		p := c.TokenFile
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			home, _ := os.UserHomeDir()
			p = filepath.Join(home, rest)
		}
		t, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("token_file: %w", err)
		}
		c.Token = strings.TrimSpace(string(t))
	}
	if c.Hub == "" || c.Token == "" {
		return nil, errors.New("config: hub and a token (token, token_file or DOORMUTEX_TOKEN) are required")
	}
	return c, nil
}

// sampler reads PipeWire, /proc and logind.
type sampler struct {
	ignore []string
	lock   *desktop.Logind // nil: lock unknown → unlocked
}

func (s *sampler) Sample(ctx context.Context) (hubclient.Report, error) {
	var r hubclient.Report
	streams, err := pipewire.Dump(ctx)
	if err != nil {
		return r, err
	}
	r.Microphone.Active, r.Microphone.Apps = pipewire.Active(streams, pipewire.AudioCapture, s.ignore)
	camOn, camApps := pipewire.Active(streams, pipewire.VideoCapture, s.ignore)
	for _, a := range camera.Users("/proc", s.ignore) {
		camOn = true
		camApps = appendNew(camApps, a)
	}
	r.Camera.Active, r.Camera.Apps = camOn, camApps
	if s.lock != nil {
		if r.Locked, err = s.lock.Locked(); err != nil {
			return r, fmt.Errorf("lock state: %w", err)
		}
	}
	return r, nil
}

func appendNew(list []string, v string) []string {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return list
		}
	}
	return append(list, v)
}

func run(path string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := load(path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := &sampler{ignore: cfg.Ignore}
	var power chan agent.Power
	if l, err := desktop.NewLogind(); err != nil {
		log.Warn("logind unavailable: no lock detection, no goodbye before suspend", "err", err)
	} else {
		defer l.Close()
		s.lock = l
		events, err := l.Watch()
		if err != nil {
			log.Warn("logind: no goodbye before suspend/shutdown", "err", err)
		} else {
			power = make(chan agent.Power)
			go forward(ctx, l, events, power)
		}
	}
	a := &agent.Agent{
		Sampler: s, Hub: &hubclient.Client{URL: cfg.Hub, Token: cfg.Token}, Power: power,
		Poll: cfg.Poll, Interval: cfg.Interval, Log: log,
	}
	if cfg.Notify == nil || *cfg.Notify {
		if n, err := desktop.NewNotifier(); err != nil {
			log.Warn("notifications unavailable", "err", err)
		} else {
			defer n.Close()
			a.Notifier = n
		}
	}
	log.Info("companion started", "version", version, "hub", cfg.Hub)
	return a.Run(ctx)
}

// forward turns logind events into agent events; the inhibitor is released
// after the goodbye and taken again after resume.
func forward(ctx context.Context, l *desktop.Logind, in <-chan desktop.Power, out chan<- agent.Power) {
	for ev := range in {
		var p agent.Power
		switch ev {
		case desktop.Suspend:
			p = agent.Power{Kind: "suspend", Ack: l.Release}
		case desktop.Shutdown:
			p = agent.Power{Kind: "shutdown", Ack: l.Release}
		case desktop.Resume:
			_ = l.Inhibit()
			p = agent.Power{Kind: "resume"}
		}
		select {
		case out <- p:
		case <-ctx.Done():
			return
		}
	}
}

func check(path string) error {
	cfg, err := load(path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := &sampler{ignore: cfg.Ignore}
	if l, err := desktop.NewLogind(); err != nil {
		fmt.Println("lock:       unknown –", err)
	} else {
		defer l.Close()
		s.lock = l
	}
	r, err := s.Sample(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("microphone: %v %v\ncamera:     %v %v\nlocked:     %v\n", r.Microphone.Active, r.Microphone.Apps, r.Camera.Active, r.Camera.Apps, r.Locked)
	st, err := (&hubclient.Client{URL: cfg.Hub, Token: cfg.Token}).Heartbeat(ctx, r)
	if err != nil {
		return fmt.Errorf("hub: %w", err)
	}
	summary, body := agent.Describe(st)
	fmt.Printf("hub:        %s – %s\n", summary, body)
	return nil
}
