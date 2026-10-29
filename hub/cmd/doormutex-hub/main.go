// Command doormutex-hub is the doormutex hub.
//
//	doormutex-hub serve [-config config.yaml]     run the hub (default)
//	doormutex-hub token create NAME [PROFILE]     create an API token (optionally for one profile)
//	doormutex-hub token list|revoke [NAME]        manage API tokens
//	doormutex-hub healthcheck                     exit 0 if the local hub answers (for Docker)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the distroless image has no zoneinfo

	"github.com/padalton/doormutex/hub/internal/api"
	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/config"
	"github.com/padalton/doormutex/hub/internal/mqttpub"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/store"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !isFlag(args[0]) {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "token":
		err = token(args)
	case "healthcheck":
		err = healthcheck(args)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (want serve, token, healthcheck, version)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "doormutex-hub:", err)
		os.Exit(1)
	}
}

func isFlag(s string) bool { return len(s) > 0 && s[0] == '-' }

func configFlag(fs *flag.FlagSet) *string {
	def := os.Getenv("DOORMUTEX_CONFIG")
	if def == "" {
		def = "/config/config.yaml"
	}
	return fs.String("config", def, "config file (env DOORMUTEX_CONFIG)")
}

func loadConfig(path string) (*config.Config, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		slog.Warn("config file not found, using defaults", "path", path)
		path = ""
	}
	return config.Load(path)
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := configFlag(fs)
	_ = fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.Database)
	if err != nil {
		return fmt.Errorf("database %s: %w", cfg.Database, err)
	}
	defer func() { _ = st.Close() }()

	var profiles []api.Profile
	for _, p := range cfg.Profiles {
		base, err := cfg.Inputs(p)
		if err != nil {
			return err
		}
		var cal calendar.Provider
		switch p.Calendar.Provider {
		case "none":
		case "fake":
			cal = calendar.NewFake(p.Calendar.FakeEvents...)
		default:
			return fmt.Errorf("profile %s: calendar.provider %q not supported (none, fake)", p.ID, p.Calendar.Provider)
		}
		svc, err := service.New(service.Options{Profile: p.ID, Base: base, Store: st, Calendar: cal, Logger: log.With("profile", p.ID)})
		if err != nil {
			return err
		}
		profiles = append(profiles, api.Profile{ID: p.ID, Name: p.Name, Mode: p.Mode, Svc: svc})
	}
	if tokens, _ := st.ListTokens(); len(tokens) == 0 {
		log.Warn("no API tokens yet – create one per client with: doormutex-hub token create <name> [<profile>]")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.MQTT.URL != "" {
		var mp []mqttpub.Profile
		for _, p := range profiles {
			mp = append(mp, mqttpub.Profile{ID: p.ID, Name: p.Name, Ctl: p.Svc})
		}
		client, err := mqttpub.Connect(mqttpub.Config{
			URL: cfg.MQTT.URL, Username: cfg.MQTT.Username, Password: cfg.MQTT.Password,
			ClientID: cfg.MQTT.ClientID, BaseTopic: cfg.MQTT.BaseTopic,
			DiscoveryPrefix: cfg.MQTT.DiscoveryPrefix, Discovery: cfg.DiscoveryEnabled(), Version: version,
		}, mp, log)
		if client == nil {
			return err
		}
		if err != nil {
			log.Warn("mqtt", "err", err)
		}
		defer client.Close()
		for _, p := range profiles {
			p.Svc.AddPublisher(client.Publisher(p.ID))
		}
	} else {
		log.Info("mqtt disabled (no mqtt.url)")
	}

	for _, p := range profiles {
		go p.Svc.Run(ctx, time.Second, time.Duration(cfg.CalendarPoll))
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(profiles, st, log),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("hub started", "version", version, "listen", cfg.Listen, "timezone", cfg.Timezone, "profiles", len(profiles))

	select {
	case <-ctx.Done():
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(sctx)
}

func token(args []string) error {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	cfgPath := configFlag(fs)
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New(tokenUsage)
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	switch {
	case rest[0] == "create" && (len(rest) == 2 || len(rest) == 3):
		profile := ""
		if len(rest) == 3 {
			profile = rest[2]
			if _, ok := cfg.Profile(profile); !ok {
				return fmt.Errorf("unknown profile %q", profile)
			}
		}
		tok, err := st.CreateToken(rest[1], profile)
		if err != nil {
			return err
		}
		fmt.Println(tok)
		fmt.Fprintln(os.Stderr, "Store this token now – it is not shown again.")
	case rest[0] == "list":
		list, err := st.ListTokens()
		if err != nil {
			return err
		}
		for _, t := range list {
			last := t.LastUsed
			if last == "" {
				last = "never"
			}
			scope := t.Profile
			if scope == "" {
				scope = "all profiles"
			}
			fmt.Printf("%-20s %-14s created %s  last used %s\n", t.Name, scope, t.CreatedAt, last)
		}
	case rest[0] == "revoke" && len(rest) == 2:
		return st.RevokeToken(rest[1])
	default:
		return errors.New(tokenUsage)
	}
	return nil
}

const tokenUsage = "usage: doormutex-hub token create NAME [PROFILE] | list | revoke NAME"

func healthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	addr := fs.String("url", "http://127.0.0.1:8080/healthz", "health endpoint")
	_ = fs.Parse(args)
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(*addr)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d", resp.StatusCode)
	}
	return nil
}
