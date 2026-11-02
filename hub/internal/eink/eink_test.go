package eink

import (
	"bytes"
	"encoding/json"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/status"
)

var update = flag.Bool("update", false, "rewrite the golden images in testdata/")

var (
	vienna, _ = time.LoadLocation("Europe/Vienna")
	now       = time.Date(2026, 10, 30, 14, 12, 0, 0, vienna)
)

func at(h, m int) *time.Time {
	t := time.Date(2026, 10, 30, h, m, 0, 0, vienna)
	return &t
}

func state(s status.Status, reason, detail string, until *time.Time) engine.State {
	return engine.State{Status: s, Phase: status.PhaseNormal, Reason: reason, Detail: detail, Since: now.Add(-time.Hour), Until: until, Updated: now}
}

func TestGolden(t *testing.T) {
	stamp := time.Date(2026, 10, 30, 14, 0, 0, 0, vienna)
	monday := time.Date(2026, 11, 2, 8, 0, 0, 0, vienna)
	call := state(status.DND, engine.ReasonCalendar, "video_call", at(15, 0))
	call.NextFree = &engine.FreeSlot{Start: *at(15, 0)}
	working := state(status.Working, engine.ReasonWorkHours, "", at(17, 0))
	working.NextFree = &engine.FreeSlot{Start: *at(15, 30)}
	free := state(status.Free, engine.ReasonDefault, "", at(15, 30))
	away := state(status.Away, engine.ReasonPCOff, "shutdown", nil)
	away.NextFree = &engine.FreeSlot{Start: monday}
	fault := state(status.Away, engine.ReasonCompanionLost, "", nil)
	fault.Phase = status.PhaseFault
	msg := state(status.DND, engine.ReasonOverride, "", at(16, 0))
	msg.Message = "Recording a podcast – please knock only if the house is on fire"
	msg.NextFree = &engine.FreeSlot{Start: *at(16, 0)}
	short := state(status.Working, engine.ReasonPomodoro, "", at(14, 25))
	short.Message = "Back at 3"
	short.NextFree = &engine.FreeSlot{Start: *at(14, 25)}

	cases := []struct {
		name string
		in   Input
	}{
		{"dnd-video-call-en", Input{State: call, Lang: "en", Stamp: stamp}},
		{"dnd-video-call-de", Input{State: call, Lang: "de", Stamp: stamp}},
		{"working-en", Input{State: working, Lang: "en", Stamp: stamp, Battery: &Battery{Percent: 76}}},
		{"working-de", Input{State: working, Lang: "de", Stamp: stamp, Battery: &Battery{Percent: 76}}},
		{"free-en", Input{State: free, Lang: "en", Stamp: stamp, Battery: &Battery{Percent: 100, Charging: true}}},
		{"free-de", Input{State: free, Lang: "de", Stamp: stamp, Battery: &Battery{Percent: 8}}},
		{"away-weekend-en", Input{State: away, Lang: "en", Stamp: stamp}},
		{"away-weekend-de", Input{State: away, Lang: "de", Stamp: stamp}},
		{"fault-en", Input{State: fault, Lang: "en", Stamp: stamp}},
		{"fault-de", Input{State: fault, Lang: "de", Stamp: stamp}},
		{"message-long-en", Input{State: msg, Lang: "en", Stamp: stamp, Battery: &Battery{Percent: 42, Charging: true}}},
		{"message-short-de", Input{State: short, Lang: "de", Stamp: stamp, Battery: &Battery{Percent: 55}}},
		{"no-footer-en", Input{State: working, Lang: "en"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			golden(t, tc.name, PNG(Render(tc.in)))
		})
	}
	t.Run("offline-en", func(t *testing.T) { golden(t, "offline-en", PNG(Offline("en"))) })
	t.Run("offline-de", func(t *testing.T) { golden(t, "offline-de", PNG(Offline("de"))) })
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".png")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/eink -update)", err)
	}
	// Compare pixels, not bytes: a different zlib must not fail the test.
	a, err1 := png.Decode(bytes.NewReader(got))
	b, err2 := png.Decode(bytes.NewReader(want))
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	for y := range Height {
		for x := range Width {
			if a.At(x, y) != b.At(x, y) {
				t.Fatalf("%s differs at (%d,%d); look at the image and run with -update if the change is intended", path, x, y)
			}
		}
	}
}

func TestPack(t *testing.T) {
	img := Render(Input{State: state(status.Free, engine.ReasonDefault, "", nil), Lang: "en"})
	bits := Pack(img)
	if len(bits) != 5000 {
		t.Fatalf("len = %d, want 5000", len(bits))
	}
	// The top-left corner is white (bit set); spot-check a few pixels.
	for _, p := range [][2]int{{0, 0}, {30, 30}, {100, 61}, {199, 199}} {
		x, y := p[0], p[1]
		black := img.Pix[y*img.Stride+x] == 1
		bit := bits[y*25+x/8]&(0x80>>(x%8)) != 0
		if bit == black {
			t.Errorf("pixel (%d,%d): black=%v but bit=%v", x, y, black, bit)
		}
	}
}

func TestDisplayStamp(t *testing.T) {
	var d Display
	st := state(status.Working, engine.ReasonWorkHours, "", at(17, 0))
	f1 := d.Frame(st, "en", nil)
	// A later evaluation with the same content keeps the frame (and ETag).
	st.Updated = now.Add(5 * time.Minute)
	if f2 := d.Frame(st, "en", nil); f2.ETag != f1.ETag {
		t.Fatal("frame changed although only Updated did")
	}
	// New content: new frame, stamped with the new time.
	st.Message = "Lunch"
	if f3 := d.Frame(st, "en", nil); f3.ETag == f1.ETag {
		t.Fatal("frame did not change with the message")
	}
	if d.since != st.Updated {
		t.Fatalf("since = %v, want %v", d.since, st.Updated)
	}
	// Unknown languages fall back to English.
	if d.Frame(st, "xx", nil).ETag != d.Frame(st, "en", nil).ETag {
		t.Fatal("unknown language should render English")
	}
}

func TestClock(t *testing.T) {
	cases := []struct {
		lang string
		t    time.Time
		want string
	}{
		{"en", *at(15, 0), "15:00"},
		{"de", time.Date(2026, 11, 2, 8, 0, 0, 0, vienna), "Mo 08:00"},
		{"en", time.Date(2026, 11, 2, 8, 0, 0, 0, vienna), "Mon 08:00"},
		{"en", time.Date(2026, 11, 20, 8, 0, 0, 0, vienna), "Nov 20 08:00"},
		{"de", time.Date(2026, 11, 20, 8, 0, 0, 0, vienna), "20.11. 08:00"},
	}
	for _, c := range cases {
		if got := clock(c.lang, c.t, now); got != c.want {
			t.Errorf("clock(%s, %v) = %q, want %q", c.lang, c.t, got, c.want)
		}
	}
}

// TestCatalogs: every language has the English keys and keeps the
// placeholders.
func TestCatalogs(t *testing.T) {
	ph := regexp.MustCompile(`\{[a-z_]+\}`)
	en := catalogs["en"]
	for lang, cat := range catalogs {
		for k, v := range cat {
			ev, ok := en[k]
			if !ok {
				t.Errorf("%s: unknown key %q", lang, k)
				continue
			}
			a, b := ph.FindAllString(ev, -1), ph.FindAllString(v, -1)
			slices.Sort(a)
			slices.Sort(b)
			if !slices.Equal(a, b) {
				t.Errorf("%s: %q has placeholders %v, English %v", lang, k, b, a)
			}
		}
		if lang == "de" {
			for k := range en {
				if _, ok := cat[k]; !ok {
					t.Errorf("de: missing key %q", k)
				}
			}
		}
	}
	// Every status and reason the engine knows has a text.
	for _, s := range []status.Status{status.Free, status.Working, status.DND, status.Away} {
		if _, ok := en["status."+string(s)]; !ok {
			t.Errorf("no text for status %s", s)
		}
	}
	b, _ := json.Marshal(Languages())
	if string(b) != `["de","en"]` {
		t.Errorf("Languages() = %s", b)
	}
}
