package google

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/schedule"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
	"github.com/padalton/doormutex/hub/internal/store"
)

// The whole chain: Google (fake) → provider → service → traffic light.
func TestHubUsesGoogleEvents(t *testing.T) {
	_, srv := setup(t)
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	_ = st.Put(TokenKey("office", "private"), oauth2.Token{AccessToken: "x", RefreshToken: "private", Expiry: time.Now().Add(-time.Minute)})
	_ = st.Put(TokenKey("office", "work"), oauth2.Token{AccessToken: "x", RefreshToken: "work", Expiry: time.Now().Add(-time.Minute)})

	week, _ := schedule.ParseWeek(map[string][]string{"wednesday": {"08:00-17:00"}})
	rules := engine.DefaultRules()
	rules.RequireCompanion = false
	now := time.Date(2026, 10, 28, 10, 10, 0, 0, vienna) // during the Meet call 10:00–10:30
	svc, err := service.New(service.Options{
		Profile: "office",
		Base:    engine.Inputs{Location: vienna, Rules: rules, WorkWeek: week},
		Store:   st, Now: func() time.Time { return now }, Logger: slog.New(slog.DiscardHandler),
		Calendar: New(Options{OAuth: oauthConf(srv), Profile: "office", Tokens: st, Location: vienna,
			Accounts: []Account{{Name: "private"}, {Name: "work"}}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.RefreshCalendar(context.Background())
	s := svc.Evaluate()
	if s.Status != status.DND || s.Reason != engine.ReasonCalendar || s.Detail != "video_call" || s.Until == nil || s.Until.Hour() != 10 {
		t.Fatalf("during the Meet call: %+v", s)
	}
	now = time.Date(2026, 10, 28, 13, 30, 0, 0, vienna) // work account: focus time 13:00–15:00
	if s := svc.Evaluate(); s.Status != status.Working || s.Detail != "focus" {
		t.Fatalf("during focus time: %+v", s)
	}
	now = time.Date(2026, 10, 28, 16, 10, 0, 0, vienna) // work account: Meet call answered "maybe"
	if s := svc.Evaluate(); s.Status != status.Working || s.Detail != "video_call" {
		t.Fatalf("during the tentative call: %+v", s)
	}
}
