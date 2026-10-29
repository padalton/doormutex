package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/padalton/doormutex/hub/internal/calendar"
	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/schedule"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
	"github.com/padalton/doormutex/hub/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type fixture struct {
	srv   *httptest.Server
	svc   *service.Service
	clk   *clock
	cal   *calendar.Fake
	token string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	vienna, _ := time.LoadLocation("Europe/Vienna")
	clk := &clock{t: time.Date(2026, 10, 28, 10, 0, 0, 0, vienna)}
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	week, _ := schedule.ParseWeek(map[string][]string{"wednesday": {"08:00-17:00"}})
	rules := engine.DefaultRules()
	rules.RequireCompanion = false
	cal := calendar.NewFake()
	svc, err := service.New(service.Options{
		Base:  engine.Inputs{Location: vienna, Rules: rules, WorkWeek: week},
		Store: st, Calendar: cal, Now: clk.now, Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateToken("test")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(svc, st, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	return &fixture{srv: srv, svc: svc, clk: clk, cal: cal, token: tok}
}

func (f *fixture) do(t *testing.T, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func decodeState(t *testing.T, b []byte) engine.State {
	t.Helper()
	var st engine.State
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return st
}

func TestAuth(t *testing.T) {
	f := newFixture(t)
	resp, err := http.Get(f.srv.URL + "/api/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/v1/status", nil)
	req.Header.Set("Authorization", "Bearer dm_wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", resp.StatusCode)
	}
	// The query token only works for the event stream.
	resp, err = http.Get(f.srv.URL + "/api/v1/status?access_token=" + f.token)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query token on /status: %d", resp.StatusCode)
	}
	resp, err = http.Get(f.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %d", resp.StatusCode)
	}
}

func TestOverrideFlow(t *testing.T) {
	f := newFixture(t)
	_, b := f.do(t, http.MethodGet, "/api/v1/status", "")
	if st := decodeState(t, b); st.Status != status.Working {
		t.Fatalf("initial status %s", st.Status)
	}

	resp, b := f.do(t, http.MethodPut, "/api/v1/override", `{"status":"dnd","mode":"duration","duration":"30m"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT override: %d %s", resp.StatusCode, b)
	}
	st := decodeState(t, b)
	if st.Status != status.DND || st.Reason != engine.ReasonOverride || st.Until == nil || !st.Until.Equal(f.clk.now().Add(30*time.Minute)) {
		t.Fatalf("after override: %+v", st)
	}

	// The override expires on its own.
	f.clk.add(31 * time.Minute)
	if st := f.svc.Evaluate(); st.Status != status.Working {
		t.Fatalf("after expiry: %s", st.Status)
	}

	resp, _ = f.do(t, http.MethodPut, "/api/v1/override", `{"status":"free","mode":"end_of_workday"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("end_of_workday: %d", resp.StatusCode)
	}
	resp, b = f.do(t, http.MethodPut, "/api/v1/override", `{"status":"dnd","mode":"end_of_event"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("end_of_event without event: %d %s", resp.StatusCode, b)
	}
	_, b = f.do(t, http.MethodDelete, "/api/v1/override", "")
	if st := decodeState(t, b); st.Status != status.Working {
		t.Fatalf("after delete: %s", st.Status)
	}
}

func TestOverrideValidation(t *testing.T) {
	f := newFixture(t)
	for _, body := range []string{
		`{"status":"purple","mode":"indefinite"}`,
		`{"status":"dnd","mode":"forever"}`,
		`{"status":"dnd","mode":"duration","duration":"soon"}`,
		`{"status":"dnd","mode":"until"}`,
		`{"status":"dnd","mode":"until","until":"2020-01-01T00:00:00Z"}`,
		`{"status":"dnd","mode":"indefinite","extra":1}`,
		`not json`,
	} {
		resp, b := f.do(t, http.MethodPut, "/api/v1/override", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d %s", body, resp.StatusCode, b)
		}
	}
}

func TestEndOfEventOverride(t *testing.T) {
	f := newFixture(t)
	now := f.clk.now()
	f.cal.Set(calendar.Event{ID: "1", Start: now.Add(-10 * time.Minute), End: now.Add(20 * time.Minute), Kind: calendar.KindVideoCall})
	f.svc.RefreshCalendar(context.Background())
	resp, b := f.do(t, http.MethodPut, "/api/v1/override", `{"status":"free","mode":"end_of_event"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if st := decodeState(t, b); st.Until == nil || !st.Until.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("until = %v", st.Until)
	}
}

func TestCompanionAndSignals(t *testing.T) {
	f := newFixture(t)
	hb := `{"locked":false,"microphone":{"active":true,"apps":["Firefox"]},"camera":{"active":false}}`
	f.do(t, http.MethodPost, "/api/v1/signals/companion", hb)
	f.clk.add(15 * time.Second)
	_, b := f.do(t, http.MethodPost, "/api/v1/signals/companion", hb)
	st := decodeState(t, b)
	if st.Status != status.DND || st.Reason != engine.ReasonMicrophone || st.Detail != "Firefox" {
		t.Fatalf("microphone: %+v", st)
	}

	resp, b := f.do(t, http.MethodPost, "/api/v1/signals/companion", `{"locked":"yes"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad heartbeat: %d %s", resp.StatusCode, b)
	}

	// Webhook signals: validation and effect (after the work day).
	f.do(t, http.MethodPost, "/api/v1/signals/companion", `{"locked":false}`)
	f.clk.add(9 * time.Hour)
	f.do(t, http.MethodPost, "/api/v1/signals/companion", `{"locked":false}`)
	resp, b = f.do(t, http.MethodPost, "/api/v1/signals/claude", `{"status":"working","ttl":"30m"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signal: %d %s", resp.StatusCode, b)
	}
	if st := decodeState(t, b); st.Status != status.Working || st.Reason != engine.ReasonSignal || st.Detail != "claude" {
		t.Fatalf("signal state: %+v", st)
	}
	for path, body := range map[string]string{
		"/api/v1/signals/Claude":  `{"status":"working","ttl":"30m"}`,
		"/api/v1/signals/claude2": `{"status":"working","ttl":"48h"}`,
		"/api/v1/signals/claude3": `{"status":"working"}`,
	} {
		if resp, b := f.do(t, http.MethodPost, path, body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", path, body, resp.StatusCode, b)
		}
	}
	_, b = f.do(t, http.MethodDelete, "/api/v1/signals/claude", "")
	if st := decodeState(t, b); st.Status != status.Free {
		t.Fatalf("after delete signal: %+v", st)
	}
}

func TestCompanionGoodbye(t *testing.T) {
	f := newFixture(t)
	if resp, _ := f.do(t, http.MethodPost, "/api/v1/signals/companion", `{"goodbye":"reboot"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown goodbye accepted: %d", resp.StatusCode)
	}
	_, b := f.do(t, http.MethodPost, "/api/v1/signals/companion", `{"goodbye":"shutdown"}`)
	if st := decodeState(t, b); st.Status != status.Away || st.Reason != engine.ReasonPCOff || st.Light != status.LightOff {
		t.Fatalf("after goodbye: %+v", st)
	}
	// Silence after a goodbye is expected: still off, no fault.
	f.clk.add(10 * time.Minute)
	_, b = f.do(t, http.MethodGet, "/api/v1/status", "")
	if st := decodeState(t, b); st.Reason != engine.ReasonPCOff || st.Phase == status.PhaseFault {
		t.Fatalf("after silence: %+v", st)
	}
}

func TestPomodoroAndTimeline(t *testing.T) {
	f := newFixture(t)
	resp, b := f.do(t, http.MethodPut, "/api/v1/pomodoro", `{"work":"50m","break":"10m","cycles":1,"work_status":"dnd"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pomodoro: %d %s", resp.StatusCode, b)
	}
	if st := decodeState(t, b); st.Status != status.DND || st.Reason != engine.ReasonPomodoro {
		t.Fatalf("pomodoro state: %+v", st)
	}
	if resp, _ := f.do(t, http.MethodPut, "/api/v1/pomodoro", `{"work":"10s"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short pomodoro accepted: %d", resp.StatusCode)
	}

	from := url.QueryEscape(f.clk.now().Format(time.RFC3339))
	to := url.QueryEscape(f.clk.now().Add(8 * time.Hour).Format(time.RFC3339))
	resp, b = f.do(t, http.MethodGet, "/api/v1/timeline?from="+from+"&to="+to, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timeline: %d %s", resp.StatusCode, b)
	}
	var tl TimelineResponse
	if err := json.Unmarshal(b, &tl); err != nil || len(tl.Slots) < 3 {
		t.Fatalf("timeline %s: %v", b, err)
	}
	if tl.Slots[0].Status != status.DND || tl.Slots[1].Status != status.Free || tl.Slots[2].Status != status.Working {
		t.Fatalf("timeline slots: %+v", tl.Slots)
	}
	if resp, _ := f.do(t, http.MethodGet, "/api/v1/timeline?from=yesterday", ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad from: %d", resp.StatusCode)
	}
	_, b = f.do(t, http.MethodDelete, "/api/v1/pomodoro", "")
	if st := decodeState(t, b); st.Status != status.Working {
		t.Fatalf("after stop: %+v", st)
	}
}

func TestEventStream(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.srv.URL+"/api/v1/events?access_token="+f.token, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	next := func() engine.State {
		t.Helper()
		var ev string
		for sc.Scan() {
			line := sc.Text()
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				ev = v
			}
			if v, ok := strings.CutPrefix(line, "data: "); ok && ev == "state" {
				return decodeState(t, []byte(v))
			}
		}
		t.Fatalf("stream ended: %v", sc.Err())
		return engine.State{}
	}
	if st := next(); st.Status != status.Working {
		t.Fatalf("first event: %+v", st)
	}
	f.do(t, http.MethodPut, "/api/v1/override", `{"status":"dnd","mode":"indefinite"}`)
	if st := next(); st.Status != status.DND {
		t.Fatalf("second event: %+v", st)
	}
}

// TestSpecMatchesRoutes checks that every operation in api/openapi.yaml is
// served (no 404/405) and that the router serves nothing undocumented.
func TestSpecMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			if method == "parameters" {
				continue
			}
			m := strings.ToUpper(method)
			documented[m+" "+path] = true
			p := strings.ReplaceAll(path, "{source}", "spec-test")
			if path == "/api/v1/events" {
				continue // streams forever; covered by TestEventStream
			}
			resp, _ := f.do(t, m, p, "{}")
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
				t.Errorf("%s %s documented but not served (%d)", m, path, resp.StatusCode)
			}
		}
	}
	for _, route := range []string{
		"GET /healthz", "GET /api/v1/status", "GET /api/v1/timeline", "PUT /api/v1/override",
		"DELETE /api/v1/override", "PUT /api/v1/pomodoro", "DELETE /api/v1/pomodoro",
		"POST /api/v1/signals/companion", "POST /api/v1/signals/{source}",
		"DELETE /api/v1/signals/{source}", "GET /api/v1/events",
	} {
		if !documented[route] {
			t.Errorf("%s served but not documented in openapi.yaml", route)
		}
	}
}
