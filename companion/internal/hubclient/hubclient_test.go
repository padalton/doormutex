package hubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHeartbeatAndFollow(t *testing.T) {
	var mu sync.Mutex
	var got []Report
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dm_test" {
			http.Error(w, `{"error":"missing or invalid token"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/signals/companion":
			var rep Report
			_ = json.NewDecoder(r.Body).Decode(&rep)
			mu.Lock()
			got = append(got, rep)
			mu.Unlock()
			_, _ = fmt.Fprint(w, `{"status":"dnd","reason":"microphone","detail":"Firefox"}`)
		case "/api/v1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, ": keep-alive\n\nevent: state\ndata: {\"status\":\"free\",\"reason\":\"default\"}\n\n")
			_, _ = fmt.Fprint(w, "event: state\ndata: {\"status\":\"dnd\",\"reason\":\"calendar\",\"detail\":\"video_call\"}\n\n")
		}
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL + "/", Token: "dm_test"}
	st, err := c.Heartbeat(context.Background(), Report{Microphone: Media{Active: true, Apps: []string{"Firefox"}}})
	if err != nil || st.Status != "dnd" || st.Detail != "Firefox" {
		t.Fatalf("heartbeat = %+v, %v", st, err)
	}
	if _, err := c.Heartbeat(context.Background(), Report{Goodbye: "shutdown"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(got) != 2 || !got[0].Microphone.Active || got[1].Goodbye != "shutdown" {
		t.Errorf("hub got %+v", got)
	}
	mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var states []State
	c.Follow(ctx, func(s State) {
		states = append(states, s)
		if len(states) == 2 {
			cancel()
		}
	})
	if len(states) < 2 || states[0].Status != "free" || states[1].Detail != "video_call" {
		t.Fatalf("states = %+v", states)
	}

	bad := &Client{URL: srv.URL, Token: "wrong"}
	if _, err := bad.Heartbeat(context.Background(), Report{}); err == nil {
		t.Error("expected an auth error")
	}
}

func TestReportEqual(t *testing.T) {
	a := Report{Microphone: Media{Active: true, Apps: []string{"x"}}}
	b := Report{Microphone: Media{Active: true, Apps: []string{"x"}}}
	if !a.Equal(b) || a.Equal(Report{}) {
		t.Error("Equal")
	}
}
