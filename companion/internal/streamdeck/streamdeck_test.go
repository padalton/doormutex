package streamdeck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/padalton/doormutex/companion/internal/hubclient"
)

func at(ms int64) time.Time { return time.UnixMilli(ms) }

func TestLitAt(t *testing.T) {
	cases := []struct {
		light       string
		ms          int64
		red, yellow bool
		green       bool
	}{
		{"red", 0, true, false, false},
		{"red_yellow", 0, true, true, false},
		{"green", 0, false, false, true},
		{"green_blink", 0, false, false, true},
		{"green_blink", 500, false, false, false},
		{"red_yellow_alternate", 0, true, false, false},
		{"red_yellow_alternate", 500, false, true, false},
		{"yellow_blink", 1000, false, true, false},
		{"off", 0, false, false, false},
		{"something_new", 0, false, false, false},
	}
	for _, c := range cases {
		got := [3]bool{litAt(c.light, Red, at(c.ms)), litAt(c.light, Yellow, at(c.ms)), litAt(c.light, Green, at(c.ms))}
		if got != [3]bool{c.red, c.yellow, c.green} {
			t.Errorf("%s at %d ms: %v", c.light, c.ms, got)
		}
	}
	if !blinking("green_blink") || blinking("red") {
		t.Error("blinking")
	}
}

func TestOverrideRequest(t *testing.T) {
	for _, c := range []struct {
		lamp     Lamp
		duration string
		want     hubclient.OverrideRequest
	}{
		{Red, "30m", hubclient.OverrideRequest{Status: "dnd", Mode: "duration", Duration: "30m"}},
		{Yellow, "", hubclient.OverrideRequest{Status: "working", Mode: "next_change"}},
		{Green, "end_of_event", hubclient.OverrideRequest{Status: "free", Mode: "end_of_event"}},
		{Off, "indefinite", hubclient.OverrideRequest{Status: "away", Mode: "indefinite"}},
	} {
		if got := overrideRequest(c.lamp, c.duration); got != c.want {
			t.Errorf("%s %q: %+v", c.lamp, c.duration, got)
		}
	}
}

func TestPomodoroPhase(t *testing.T) {
	start := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	p := &hubclient.Pomodoro{Start: start, Work: "25m0s", Break: "5m0s", Cycles: 4}
	if w, left, ok := pomodoroPhase(p, start.Add(10*time.Minute)); !ok || !w || left != 15*time.Minute {
		t.Errorf("work: %v %v %v", w, left, ok)
	}
	if w, left, ok := pomodoroPhase(p, start.Add(27*time.Minute)); !ok || w || left != 3*time.Minute {
		t.Errorf("break: %v %v %v", w, left, ok)
	}
	if w, _, ok := pomodoroPhase(p, start.Add(31*time.Minute)); !ok || !w {
		t.Error("second cycle should be work")
	}
	end := start.Add(2 * time.Hour)
	p.End = &end
	if _, _, ok := pomodoroPhase(p, end); ok {
		t.Error("ended")
	}
	if _, _, ok := pomodoroPhase(nil, start); ok {
		t.Error("nil")
	}
	if got := pomodoroTitle(&hubclient.State{Pomodoro: &hubclient.Pomodoro{Start: start, Work: "25m0s", Break: "5m0s"}},
		"25m", start.Add(90*time.Second), "de"); got != "Arbeit\n23:30" {
		t.Errorf("title %q", got)
	}
	if got := pomodoroTitle(&hubclient.State{}, "50m", start, "en"); got != "50 min" {
		t.Errorf("idle title %q", got)
	}
}

func TestTitles(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.Local)
	until := time.Date(2026, 11, 14, 14, 30, 0, 0, time.Local)
	st := &hubclient.State{Status: "dnd", Light: "red", Until: &until, Override: &hubclient.Override{Status: "dnd"}}
	if got := statusTitle(st, now, "de"); got != "Bitte nicht\nstören\nbis 14:30" {
		t.Errorf("status %q", got)
	}
	if got := lampTitle(st, Red, now, "en"); got != "until 14:30" {
		t.Errorf("lamp %q", got)
	}
	if got := lampTitle(st, Green, now, "en"); got != "" {
		t.Errorf("other lamp %q", got)
	}
	tomorrow := until.Add(24 * time.Hour)
	if got := clock(tomorrow, now, "de"); got != "So 14:30" {
		t.Errorf("clock %q", got)
	}
	if language("de-AT") != "de" || language("en") != "en" || language("") != "en" {
		t.Error("language")
	}
}

func TestViewFor(t *testing.T) {
	now := time.Now()
	k := &key{action: ActionLamp, settings: KeySettings{Lamp: Green}}
	if v := viewFor(k, nil, false, false, now, "en"); v.title != "Set up\nhub" {
		t.Errorf("unconfigured: %q", v.title)
	}
	if v := viewFor(k, &hubclient.State{}, true, true, now, "de"); v.title != "Hub nicht\nerreichbar" {
		t.Errorf("offline: %q", v.title)
	}
	lit := viewFor(k, &hubclient.State{Status: "free", Light: "green"}, true, false, now, "en")
	dim := viewFor(k, &hubclient.State{Status: "dnd", Light: "red"}, true, false, now, "en")
	if lit.image == dim.image || !strings.HasPrefix(lit.image, "data:image/png;base64,") {
		t.Error("a lit and a dark lamp must differ")
	}
}

// ---- end to end: a fake Stream Deck host and a fake hub

type fakeHub struct {
	mu        sync.Mutex
	calls     []string
	follow    func(hubclient.State)
	followErr func(error)
	ready     chan struct{}
}

func (h *fakeHub) FollowErr(ctx context.Context, fn func(hubclient.State), onErr func(error)) {
	h.mu.Lock()
	h.follow, h.followErr = fn, onErr
	h.mu.Unlock()
	close(h.ready)
	<-ctx.Done()
}

func (h *fakeHub) record(s string) {
	h.mu.Lock()
	h.calls = append(h.calls, s)
	h.mu.Unlock()
}

func (h *fakeHub) SetOverride(_ context.Context, o hubclient.OverrideRequest) (hubclient.State, error) {
	h.record("set " + o.Status + " " + o.Mode + " " + o.Duration)
	return hubclient.State{}, nil
}
func (h *fakeHub) ClearOverride(context.Context) error { h.record("clear"); return nil }
func (h *fakeHub) StartPomodoro(_ context.Context, w, b string, _ int, _ string) (hubclient.State, error) {
	h.record("pomodoro " + w + " " + b)
	return hubclient.State{}, nil
}
func (h *fakeHub) StopPomodoro(context.Context) error { h.record("pomodoro stop"); return nil }

func TestEndToEnd(t *testing.T) {
	fh := &fakeHub{ready: make(chan struct{})}
	got := make(chan map[string]any, 100)
	var host *websocket.Conn
	connected := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		host = c
		close(connected)
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			var m map[string]any
			_ = json.Unmarshal(msg, &m)
			got <- m
		}
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])

	p := New(Args{Port: port, PluginUUID: "plugin-1", RegisterEvent: "registerPlugin", Info: `{"application":{"language":"de"}}`})
	var gotGlobal Global
	p.hubF = func(g Global) hub { gotGlobal = g; return fh }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Run(ctx) }()
	<-connected

	next := func(event string) map[string]any {
		t.Helper()
		timeout := time.After(3 * time.Second)
		for {
			select {
			case m := <-got:
				if m["event"] == event {
					return m
				}
			case <-timeout:
				t.Fatalf("no %s", event)
				return nil
			}
		}
	}
	hostSend := func(v any) {
		b, _ := json.Marshal(v)
		_ = host.WriteMessage(websocket.TextMessage, b)
	}

	if m := next("registerPlugin"); m["uuid"] != "plugin-1" {
		t.Fatalf("register: %v", m)
	}
	next("getGlobalSettings")
	hostSend(map[string]any{"event": "willAppear", "action": ActionLamp, "context": "k1",
		"payload": map[string]any{"settings": map[string]any{"lamp": "red", "duration": "30m"}}})
	if m := next("setTitle"); m["payload"].(map[string]any)["title"] != "Hub\neinrichten" {
		t.Fatalf("unconfigured title: %v", m)
	}
	hostSend(map[string]any{"event": "didReceiveGlobalSettings",
		"payload": map[string]any{"settings": map[string]any{"hub": " https://hub.example/ ", "token": "dm_x", "profile": "office"}}})
	<-fh.ready
	if gotGlobal.Hub != "https://hub.example/" || gotGlobal.Profile != "office" {
		t.Fatalf("global %+v", gotGlobal)
	}
	until := time.Now().Add(time.Hour)
	fh.follow(hubclient.State{Status: "dnd", Light: "red", Until: &until, Override: &hubclient.Override{Status: "dnd"}})
	for {
		m := next("setTitle")
		if strings.HasPrefix(m["payload"].(map[string]any)["title"].(string), "bis ") {
			break
		}
	}

	// Red is set: pressing red returns to automatic.
	hostSend(map[string]any{"event": "keyDown", "action": ActionLamp, "context": "k1"})
	// Then green comes from the hub, and pressing red sets red for 30 min.
	waitCall := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			fh.mu.Lock()
			calls := strings.Join(fh.calls, "|")
			fh.mu.Unlock()
			if strings.Contains(calls, want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("no call %q", want)
	}
	waitCall("clear")
	fh.follow(hubclient.State{Status: "free", Light: "green"})
	time.Sleep(50 * time.Millisecond)
	hostSend(map[string]any{"event": "keyDown", "action": ActionLamp, "context": "k1"})
	waitCall("set dnd duration 30m")

	// A broken stream shows on the key.
	fh.followErr(context.DeadlineExceeded)
	for {
		m := next("setTitle")
		if m["payload"].(map[string]any)["title"] == "Hub nicht\nerreichbar" {
			break
		}
	}
}
