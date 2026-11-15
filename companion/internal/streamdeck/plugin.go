package streamdeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/padalton/doormutex/companion/internal/hubclient"
)

// Action UUIDs, as in the manifest.
const (
	ActionLamp     = "org.doormutex.trafficlight.lamp"
	ActionAuto     = "org.doormutex.trafficlight.auto"
	ActionStatus   = "org.doormutex.trafficlight.status"
	ActionPomodoro = "org.doormutex.trafficlight.pomodoro"
)

// Global are the plugin-wide settings, entered in the property inspector
// and kept by the Stream Deck software.
type Global struct {
	Hub     string `json:"hub"`
	Token   string `json:"token"`
	Profile string `json:"profile"`
}

func (g Global) complete() bool { return g.Hub != "" && g.Token != "" }

// KeySettings are one key's settings.
type KeySettings struct {
	Lamp     Lamp   `json:"lamp"`     // lamp keys: red, yellow, green, off
	Duration string `json:"duration"` // lamp keys: "30m", "end_of_event" …
	Work     string `json:"work"`     // pomodoro: "25m"
	Break    string `json:"break"`    // pomodoro: "5m"
}

type key struct {
	action, context string
	settings        KeySettings
	image, title    string // last sent, to skip repeats
}

// Args are what the Stream Deck software passes on the command line.
type Args struct {
	Port          int
	PluginUUID    string
	RegisterEvent string
	Info          string
}

// Plugin is a running plugin.
type Plugin struct {
	args Args
	lang string

	wmu  sync.Mutex // serialises writes to ws
	rmu  sync.Mutex // one render at a time, so an older picture never overtakes a newer one
	ws   *websocket.Conn
	hubF func(Global) hub // replaceable in tests

	mu      sync.Mutex
	global  Global
	keys    map[string]*key
	state   *hubclient.State
	offline bool
	stopHub context.CancelFunc
	hubConn hub
}

// hub is what the plugin needs from the hub client.
type hub interface {
	FollowErr(ctx context.Context, fn func(hubclient.State), onErr func(error))
	SetOverride(ctx context.Context, o hubclient.OverrideRequest) (hubclient.State, error)
	ClearOverride(ctx context.Context) error
	StartPomodoro(ctx context.Context, work, brk string, cycles int, workStatus string) (hubclient.State, error)
	StopPomodoro(ctx context.Context) error
}

// New prepares a plugin for the given launch arguments.
func New(a Args) *Plugin {
	p := &Plugin{args: a, keys: map[string]*key{}, lang: "en"}
	var info struct {
		Application struct {
			Language string `json:"language"`
		} `json:"application"`
	}
	if json.Unmarshal([]byte(a.Info), &info) == nil {
		p.lang = language(info.Application.Language)
	}
	p.hubF = func(g Global) hub {
		return &hubclient.Client{URL: strings.TrimSuffix(g.Hub, "/"), Token: g.Token, Profile: g.Profile}
	}
	return p
}

// Run connects to the Stream Deck software and handles its events until
// the connection ends (the software quits or unloads the plugin).
func (p *Plugin) Run(ctx context.Context) error {
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, fmt.Sprintf("ws://127.0.0.1:%d", p.args.Port), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close() // the handshake's; the connection stays open
	}
	if err != nil {
		return err
	}
	p.ws = ws
	defer func() { _ = ws.Close() }()
	p.send(map[string]any{"event": p.args.RegisterEvent, "uuid": p.args.PluginUUID})
	p.send(map[string]any{"event": "getGlobalSettings", "context": p.args.PluginUUID})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go p.tick(ctx)
	defer p.stop()
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		p.handle(ctx, msg)
	}
}

// event is an incoming message; only the fields the plugin uses.
type event struct {
	Event   string `json:"event"`
	Action  string `json:"action"`
	Context string `json:"context"`
	Payload struct {
		Settings json.RawMessage `json:"settings"`
	} `json:"payload"`
}

func (p *Plugin) handle(ctx context.Context, msg []byte) {
	var e event
	if err := json.Unmarshal(msg, &e); err != nil {
		return
	}
	switch e.Event {
	case "didReceiveGlobalSettings":
		var g Global
		_ = json.Unmarshal(e.Payload.Settings, &g)
		p.setGlobal(g)
	case "willAppear", "didReceiveSettings":
		var s KeySettings
		_ = json.Unmarshal(e.Payload.Settings, &s)
		p.mu.Lock()
		k := p.keys[e.Context]
		if k == nil {
			k = &key{action: e.Action, context: e.Context}
			p.keys[e.Context] = k
		}
		k.settings = s
		k.image, k.title = "", "" // the software forgets them; send again
		p.mu.Unlock()
		p.render(time.Now())
	case "willDisappear":
		p.mu.Lock()
		delete(p.keys, e.Context)
		p.mu.Unlock()
	case "keyDown":
		p.mu.Lock()
		k := p.keys[e.Context]
		p.mu.Unlock()
		if k != nil {
			go p.press(ctx, k.action, k.context)
		}
	case "systemDidWakeUp":
		// The stream may have died in sleep; start over at once.
		p.mu.Lock()
		g := p.global
		p.global = Global{}
		p.mu.Unlock()
		p.setGlobal(g)
	}
}

// setGlobal (re)connects to the hub when its settings change.
func (p *Plugin) setGlobal(g Global) {
	g.Hub = strings.TrimSpace(g.Hub)
	g.Token = strings.TrimSpace(g.Token)
	g.Profile = strings.TrimSpace(g.Profile)
	p.mu.Lock()
	if g == p.global && p.stopHub != nil {
		p.mu.Unlock()
		return
	}
	if p.stopHub != nil {
		p.stopHub()
		p.stopHub = nil
	}
	p.global, p.state, p.offline, p.hubConn = g, nil, false, nil
	if !g.complete() {
		log.Printf("hub: not set up yet")
	} else {
		log.Printf("hub: following %s, profile %q", g.Hub, g.Profile) // never the token
		ctx, cancel := context.WithCancel(context.Background())
		p.stopHub = cancel
		h := p.hubF(g)
		p.hubConn = h
		go h.FollowErr(ctx, func(st hubclient.State) {
			p.mu.Lock()
			if ctx.Err() == nil {
				p.state, p.offline = &st, false
			}
			p.mu.Unlock()
			p.render(time.Now())
		}, func(err error) {
			log.Printf("hub: %v", err)
			p.mu.Lock()
			if ctx.Err() == nil {
				p.offline = true
			}
			p.mu.Unlock()
			p.render(time.Now())
		})
	}
	p.mu.Unlock()
	p.render(time.Now())
}

func (p *Plugin) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopHub != nil {
		p.stopHub()
	}
}

// tick redraws what changes with time: blinking lamps, the pomodoro
// countdown, "until" across midnight.
func (p *Plugin) tick(ctx context.Context) {
	t := time.NewTicker(blinkPeriod / 2)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.render(now)
		}
	}
}

// press runs a key's action against the hub.
func (p *Plugin) press(ctx context.Context, action, keyCtx string) {
	p.mu.Lock()
	k := p.keys[keyCtx]
	h, g, st := p.hubConn, p.global, p.state
	var s KeySettings
	if k != nil {
		s = k.settings
	}
	p.mu.Unlock()
	if action == ActionStatus {
		if g.Hub != "" {
			p.send(map[string]any{"event": "openUrl", "payload": map[string]string{"url": strings.TrimSuffix(g.Hub, "/") + "/"}})
		}
		return
	}
	if h == nil {
		p.send(map[string]any{"event": "showAlert", "context": keyCtx})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error
	switch action {
	case ActionLamp:
		lamp := s.Lamp
		if lamp == "" {
			lamp = Red
		}
		// Pressing the key that's set already returns to automatic.
		if st != nil && st.Override != nil && st.Override.Status == statusFor[lamp] {
			err = h.ClearOverride(ctx)
		} else {
			_, err = h.SetOverride(ctx, overrideRequest(lamp, s.Duration))
		}
	case ActionAuto:
		err = h.ClearOverride(ctx)
	case ActionPomodoro:
		if st != nil && st.Pomodoro != nil {
			err = h.StopPomodoro(ctx)
		} else {
			_, err = h.StartPomodoro(ctx, orDefault(s.Work, "25m"), orDefault(s.Break, "5m"), 4, "")
		}
	}
	if err != nil {
		log.Printf("%s: %v", action, err)
		p.send(map[string]any{"event": "showAlert", "context": keyCtx})
	}
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return strings.TrimSpace(s)
}

// view is what one key should show.
type view struct{ image, title string }

// viewFor decides a key's picture and title. Pure apart from rendering.
func viewFor(k *key, st *hubclient.State, configured, offline bool, now time.Time, lang string) view {
	switch {
	case !configured:
		return view{lightImage("", now, true, false).dataURL(), text(lang, "setup")}
	case st == nil || offline:
		title := text(lang, "offline")
		if st == nil && !offline {
			title = "…"
		}
		switch k.action {
		case ActionLamp:
			return view{lampImage(lampOf(k), false, false).dataURL(), title}
		case ActionStatus:
			return view{statusImage("", now, true).dataURL(), title}
		}
		return view{lightImage("", now, true, false).dataURL(), title}
	}
	switch k.action {
	case ActionLamp:
		lamp := lampOf(k)
		lit := false
		if lamp == Off {
			lit = st.Status == "away"
		} else {
			lit = litAt(st.Light, lamp, now)
		}
		selected := st.Override != nil && st.Override.Status == statusFor[lamp]
		return view{lampImage(lamp, lit, selected).dataURL(), lampTitle(st, lamp, now, lang)}
	case ActionAuto:
		return view{lightImage(st.Light, now, false, st.Override == nil).dataURL(), text(lang, "auto")}
	case ActionStatus:
		return view{statusImage(st.Light, now, false).dataURL(), statusTitle(st, now, lang)}
	case ActionPomodoro:
		work, left, running := pomodoroPhase(st.Pomodoro, now)
		frac := 0.0
		if running {
			total, _ := time.ParseDuration(st.Pomodoro.Work)
			if !work {
				total, _ = time.ParseDuration(st.Pomodoro.Break)
			}
			if total > 0 {
				frac = float64(left) / float64(total)
			}
		}
		return view{pomodoroImage(running, work, frac).dataURL(), pomodoroTitle(st, k.settings.Work, now, lang)}
	}
	return view{}
}

func lampOf(k *key) Lamp {
	switch k.settings.Lamp {
	case Red, Yellow, Green, Off:
		return k.settings.Lamp
	}
	return Red
}

// render sends every key whose picture or title changed.
func (p *Plugin) render(now time.Time) {
	p.rmu.Lock()
	defer p.rmu.Unlock()
	p.mu.Lock()
	st, offline, configured, lang := p.state, p.offline, p.global.complete(), p.lang
	type out struct {
		context string
		v       view
		img     bool
		title   bool
	}
	var outs []out
	for _, k := range p.keys {
		v := viewFor(k, st, configured, offline, now, lang)
		o := out{context: k.context, v: v, img: v.image != k.image, title: v.title != k.title}
		k.image, k.title = v.image, v.title
		if o.img || o.title {
			outs = append(outs, o)
		}
	}
	p.mu.Unlock()
	for _, o := range outs {
		if o.img {
			p.send(map[string]any{"event": "setImage", "context": o.context,
				"payload": map[string]any{"image": o.v.image, "target": 0}})
		}
		if o.title {
			p.send(map[string]any{"event": "setTitle", "context": o.context,
				"payload": map[string]any{"title": o.v.title, "target": 0}})
		}
	}
}

func (p *Plugin) send(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if p.ws == nil {
		return
	}
	if err := p.ws.WriteMessage(websocket.TextMessage, b); err != nil && !errors.Is(err, websocket.ErrCloseSent) {
		log.Printf("send: %v", err)
	}
}
