// Command doormutex-hub is the doormutex hub.
//
//	doormutex-hub serve [-config config.yaml]     run the hub (default)
//	doormutex-hub token create NAME [PROFILE]     create an API token (optionally for one profile)
//	doormutex-hub token list|revoke [NAME]        manage API tokens
//	doormutex-hub google login|logout PROFILE ACCOUNT  connect a Google account on a headless hub
//	                                              (on a desktop: doormutex-companion google login ACCOUNT)
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
	"slices"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the distroless image has no zoneinfo

	"golang.org/x/oauth2"

	"github.com/padalton/doormutex/hub/internal/api"
	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/calendar/google"
	"github.com/padalton/doormutex/hub/internal/config"
	"github.com/padalton/doormutex/hub/internal/mqttpub"
	"github.com/padalton/doormutex/hub/internal/secret"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/store"
	"github.com/padalton/doormutex/hub/internal/web"
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
	case "google":
		err = googleCmd(args)
	case "healthcheck":
		err = healthcheck(args)
	case "version":
		fmt.Println(version)
	default:
		err = fmt.Errorf("unknown command %q (want serve, token, google, healthcheck, version)", cmd)
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
	tokens, err := secretStore(cfg, st, log)
	if err != nil {
		return err
	}

	var profiles []api.Profile
	for _, p := range cfg.Profiles {
		base, err := cfg.Inputs(p)
		if err != nil {
			return err
		}
		var cal calendar.Provider
		var login api.CalendarLogin
		switch p.Calendar.Provider {
		case "fake":
			cal = calendar.NewFake(p.Calendar.FakeEvents...)
		case "google":
			if err := cfg.GoogleReady(); err != nil {
				return fmt.Errorf("profile %s: %w", p.ID, err)
			}
			g := google.New(google.Options{
				OAuth: oauthConfig(cfg), Profile: p.ID, Accounts: googleAccounts(p), MirrorKey: cfg.Google.MirrorKey,
				Tokens: tokens, Location: base.Location, Logger: log.With("profile", p.ID),
			})
			cal, login = g, g
		}
		svc, err := service.New(service.Options{Profile: p.ID, Base: base, Store: st, Calendar: cal, Logger: log.With("profile", p.ID)})
		if err != nil {
			return err
		}
		profiles = append(profiles, api.Profile{ID: p.ID, Name: p.Name, Mode: p.Mode, Calendar: p.Calendar.Provider, Svc: svc, Login: login})
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

	handler := api.New(profiles, st, log)
	handler.Mount("GET /", web.Handler())
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
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

func oauthConfig(cfg *config.Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID: cfg.Google.ClientID, ClientSecret: cfg.Google.ClientSecret,
		Endpoint: google.Endpoint, Scopes: []string{google.Scope},
	}
}

func googleAccounts(p config.Profile) []google.Account {
	out := make([]google.Account, 0, len(p.Calendar.Accounts))
	for _, a := range p.Calendar.Accounts {
		out = append(out, google.Account{Name: a.Name, Calendars: a.Calendars, Mirror: a.Mirror})
	}
	return out
}

// secretStore wraps the store so that Google tokens are encrypted, and
// encrypts tokens written by an older hub.
func secretStore(cfg *config.Config, st *store.Store, log *slog.Logger) (secret.Store, error) {
	box, src, err := secret.Load(cfg.Database)
	if err != nil {
		return secret.Store{}, err
	}
	s := secret.Store{KV: st, Box: box}
	keys, err := st.Keys("profiles/")
	if err != nil {
		return s, err
	}
	keys = slices.DeleteFunc(keys, func(k string) bool { return !strings.Contains(k, "/google/") })
	n, err := s.Migrate(keys)
	if err != nil {
		return s, fmt.Errorf("encrypt stored tokens: %w", err)
	}
	if log != nil {
		log.Info("token encryption", "key", src, "encrypted_now", n)
	}
	return s, nil
}

const googleUsage = "usage: doormutex-hub google login|logout PROFILE ACCOUNT"

// googleCmd signs a Google account in or out. The running hub picks up the
// token at its next calendar refresh.
func googleCmd(args []string) error {
	fs := flag.NewFlagSet("google", flag.ExitOnError)
	cfgPath := configFlag(fs)
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 3 || (rest[0] != "login" && rest[0] != "logout") {
		return errors.New(googleUsage)
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	p, ok := cfg.Profile(rest[1])
	if !ok {
		return fmt.Errorf("unknown profile %q", rest[1])
	}
	if p.Calendar.Provider != "google" || !slices.ContainsFunc(p.Calendar.Accounts, func(a config.GoogleAccount) bool { return a.Name == rest[2] }) {
		return fmt.Errorf("profile %s has no Google account %q in calendar.accounts", p.ID, rest[2])
	}
	st, err := store.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	tokens, err := secretStore(cfg, st, nil)
	if err != nil {
		return err
	}
	key := google.TokenKey(p.ID, rest[2])
	if rest[0] == "logout" {
		return tokens.Put(key, nil)
	}
	if err := cfg.GoogleReady(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	tok, err := google.Login(ctx, *oauthConfig(cfg), os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	if err := tokens.Put(key, tok); err != nil {
		return err
	}
	fmt.Printf("Google account %q of profile %q is connected. The hub reads it at the next calendar refresh.\n", rest[2], p.ID)
	return nil
}

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
