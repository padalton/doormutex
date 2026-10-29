// Package api serves the REST and SSE API described in api/openapi.yaml.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/padalton/doormutex/hub/internal/engine"
	"github.com/padalton/doormutex/hub/internal/service"
	"github.com/padalton/doormutex/hub/internal/status"
	"github.com/padalton/doormutex/hub/internal/store"
)

// Authenticator maps a bearer token to a client.
type Authenticator interface {
	Authenticate(token string) (store.Client, bool)
}

// Profile is one person and light served by the API.
type Profile struct {
	ID   string
	Name string
	Mode string
	Svc  *service.Service
}

// Server is the HTTP API.
type Server struct {
	profiles  []Profile // the first one is the default for unscoped tokens
	auth      Authenticator
	log       *slog.Logger
	keepalive time.Duration
	mux       *http.ServeMux
	routes    []string
}

// New builds the API handler. profiles must not be empty.
func New(profiles []Profile, auth Authenticator, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{profiles: profiles, auth: auth, log: log, keepalive: 25 * time.Second, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.handle("GET /api/v1/profiles", s.getProfiles)
	s.handle("GET /api/v1/status", s.getStatus)
	s.handle("GET /api/v1/history", s.getHistory)
	s.handle("PUT /api/v1/message", s.putMessage)
	s.handle("DELETE /api/v1/message", s.deleteMessage)
	s.handle("GET /api/v1/timeline", s.getTimeline)
	s.handle("PUT /api/v1/override", s.putOverride)
	s.handle("DELETE /api/v1/override", s.deleteOverride)
	s.handle("PUT /api/v1/pomodoro", s.putPomodoro)
	s.handle("DELETE /api/v1/pomodoro", s.deletePomodoro)
	s.handle("POST /api/v1/signals/companion", s.postCompanion)
	s.handle("POST /api/v1/signals/{source}", s.postSignal)
	s.handle("DELETE /api/v1/signals/{source}", s.deleteSignal)
	s.handle("GET /api/v1/events", s.events)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

type (
	clientKey  struct{}
	profileKey struct{}
)

// Client returns the authenticated client of a request.
func Client(r *http.Request) store.Client {
	c, _ := r.Context().Value(clientKey{}).(store.Client)
	return c
}

// svc returns the profile's service the request is about.
func svc(r *http.Request) *service.Service {
	return r.Context().Value(profileKey{}).(*Profile).Svc
}

// resolve picks the profile of a request: a scoped token always gets its
// own profile, other tokens choose with ?profile= (default: the first).
func (s *Server) resolve(c store.Client, want string) (*Profile, int, string) {
	if c.Profile != "" {
		if want != "" && want != c.Profile {
			return nil, http.StatusForbidden, "this token is limited to profile " + c.Profile
		}
		want = c.Profile
	}
	if want == "" {
		return &s.profiles[0], 0, ""
	}
	for i := range s.profiles {
		if s.profiles[i].ID == want {
			return &s.profiles[i], 0, ""
		}
	}
	return nil, http.StatusNotFound, "unknown profile " + want
}

// Routes lists the authenticated routes (for the spec test).
func (s *Server) Routes() []string { return slices.Clone(s.routes) }

func (s *Server) handle(pattern string, h http.HandlerFunc) {
	s.routes = append(s.routes, pattern)
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == r.Header.Get("Authorization") {
			tok = ""
		}
		// Browsers' EventSource cannot set headers, so the event stream
		// also accepts the token as a query parameter.
		if tok == "" && r.URL.Path == "/api/v1/events" {
			tok = r.URL.Query().Get("access_token")
		}
		c, ok := s.auth.Authenticate(tok)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="doormutex"`)
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		p, code, msg := s.resolve(c, r.URL.Query().Get("profile"))
		if p == nil {
			writeError(w, code, msg)
			return
		}
		ctx := context.WithValue(context.WithValue(r.Context(), clientKey{}, c), profileKey{}, p)
		h(w, r.WithContext(ctx))
	})
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ProfileInfo is one entry of GET /api/v1/profiles.
type ProfileInfo struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Mode  string       `json:"mode"`
	State engine.State `json:"state"`
}

func (s *Server) getProfiles(w http.ResponseWriter, r *http.Request) {
	c := Client(r)
	out := []ProfileInfo{}
	for _, p := range s.profiles {
		if c.Profile == "" || c.Profile == p.ID {
			out = append(out, ProfileInfo{ID: p.ID, Name: p.Name, Mode: p.Mode, State: p.Svc.State()})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, svc(r).State())
}

func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit: 1 to 500")
			return
		}
		limit = n
	}
	h, err := svc(r).History(limit)
	if err != nil {
		s.respond(w, r, engine.State{}, err)
		return
	}
	if h == nil {
		h = []store.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, h)
}

// MessageBody is the body of PUT /api/v1/message.
type MessageBody struct {
	Text string `json:"text"`
	TTL  string `json:"ttl,omitempty"`
}

func (s *Server) putMessage(w http.ResponseWriter, r *http.Request) {
	var b MessageBody
	if !decode(w, r, &b) {
		return
	}
	var ttl time.Duration
	if b.TTL != "" {
		var err error
		if ttl, err = time.ParseDuration(b.TTL); err != nil {
			writeError(w, http.StatusBadRequest, "ttl: want a duration like \"2h\"")
			return
		}
	}
	st, err := svc(r).SetMessage(b.Text, ttl)
	s.respond(w, r, st, err)
}

func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	st, err := svc(r).SetMessage("", 0)
	s.respond(w, r, st, err)
}

// TimelineResponse is the body of GET /api/v1/timeline.
type TimelineResponse struct {
	From  time.Time     `json:"from"`
	To    time.Time     `json:"to"`
	Slots []engine.Slot `json:"slots"`
}

func (s *Server) getTimeline(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	from, to := now, now.Add(24*time.Hour)
	var err error
	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "from: want RFC 3339 timestamp")
			return
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "to: want RFC 3339 timestamp")
			return
		}
	}
	if !to.After(from) || to.Sub(from) > 14*24*time.Hour {
		writeError(w, http.StatusBadRequest, "to must be after from, at most 14 days")
		return
	}
	writeJSON(w, http.StatusOK, TimelineResponse{From: from, To: to, Slots: svc(r).Timeline(from, to)})
}

// OverrideBody is the body of PUT /api/v1/override.
type OverrideBody struct {
	Status   string     `json:"status"`
	Mode     string     `json:"mode"`
	Duration string     `json:"duration,omitempty"`
	Until    *time.Time `json:"until,omitempty"`
}

func (s *Server) putOverride(w http.ResponseWriter, r *http.Request) {
	var b OverrideBody
	if !decode(w, r, &b) {
		return
	}
	req := service.OverrideRequest{Status: status.Status(b.Status), Mode: b.Mode}
	if b.Mode == service.ModeDuration {
		d, err := time.ParseDuration(b.Duration)
		if err != nil {
			writeError(w, http.StatusBadRequest, "duration: want e.g. \"30m\" or \"1h30m\"")
			return
		}
		req.Duration = d
	}
	if b.Mode == service.ModeUntil {
		if b.Until == nil {
			writeError(w, http.StatusBadRequest, "until is required for mode until")
			return
		}
		req.Until = *b.Until
	}
	st, err := svc(r).SetOverride(req)
	s.respond(w, r, st, err)
}

func (s *Server) deleteOverride(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, svc(r).ClearOverride())
}

// PomodoroBody is the body of PUT /api/v1/pomodoro.
type PomodoroBody struct {
	Work       string `json:"work,omitempty"`
	Break      string `json:"break,omitempty"`
	Cycles     *int   `json:"cycles,omitempty"`
	WorkStatus string `json:"work_status,omitempty"`
}

func (s *Server) putPomodoro(w http.ResponseWriter, r *http.Request) {
	b := PomodoroBody{Work: "25m", Break: "5m"}
	if !decode(w, r, &b) {
		return
	}
	work, err1 := time.ParseDuration(b.Work)
	brk, err2 := time.ParseDuration(b.Break)
	if err := errors.Join(err1, err2); err != nil {
		writeError(w, http.StatusBadRequest, "work/break: want durations like \"25m\"")
		return
	}
	cycles := 4
	if b.Cycles != nil {
		cycles = *b.Cycles
	}
	st, err := svc(r).SetPomodoro(service.PomodoroRequest{Work: work, Break: brk, Cycles: cycles, WorkStatus: status.Status(b.WorkStatus)})
	s.respond(w, r, st, err)
}

func (s *Server) deletePomodoro(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, svc(r).ClearPomodoro())
}

// MediaBody describes one media device in a companion heartbeat.
type MediaBody struct {
	Active bool     `json:"active"`
	Apps   []string `json:"apps,omitempty"`
}

// CompanionBody is the body of POST /api/v1/signals/companion.
type CompanionBody struct {
	Locked     bool      `json:"locked"`
	Microphone MediaBody `json:"microphone"`
	Camera     MediaBody `json:"camera"`
	Goodbye    string    `json:"goodbye,omitempty"` // "shutdown" or "suspend": the companion signs off
}

func (s *Server) postCompanion(w http.ResponseWriter, r *http.Request) {
	var b CompanionBody
	if !decode(w, r, &b) {
		return
	}
	switch b.Goodbye {
	case "", engine.GoodbyeShutdown, engine.GoodbyeSuspend:
	default:
		writeError(w, http.StatusBadRequest, "goodbye must be shutdown or suspend")
		return
	}
	st := svc(r).CompanionReport(engine.Report{
		Locked:     b.Locked,
		Microphone: b.Microphone.Active, MicApps: limitApps(b.Microphone.Apps),
		Camera: b.Camera.Active, CamApps: limitApps(b.Camera.Apps),
		Goodbye: b.Goodbye,
	})
	writeJSON(w, http.StatusOK, st)
}

func limitApps(apps []string) []string {
	if len(apps) > 8 {
		apps = apps[:8]
	}
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		if len(a) > 64 {
			a = a[:64]
		}
		out = append(out, a)
	}
	return out
}

// SignalBody is the body of POST /api/v1/signals/{source}.
type SignalBody struct {
	Status string `json:"status"`
	TTL    string `json:"ttl"`
	Detail string `json:"detail,omitempty"`
}

func (s *Server) postSignal(w http.ResponseWriter, r *http.Request) {
	var b SignalBody
	if !decode(w, r, &b) {
		return
	}
	ttl, err := time.ParseDuration(b.TTL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ttl: want a duration like \"30m\"")
		return
	}
	st, err := svc(r).SetSignal(r.PathValue("source"), status.Status(b.Status), ttl, b.Detail)
	s.respond(w, r, st, err)
}

func (s *Server) deleteSignal(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, svc(r).ClearSignal(r.PathValue("source")))
}

// events streams the state as Server-Sent Events: one "state" event on
// connect and one per change, plus keep-alive comments.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: don't buffer
	ch, cancel := svc(r).Subscribe()
	defer cancel()
	ka := time.NewTicker(s.keepalive)
	defer ka.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case st := <-ch:
			b, _ := json.Marshal(st)
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b); err != nil {
				return
			}
			fl.Flush()
		case <-ka.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) respond(w http.ResponseWriter, r *http.Request, st engine.State, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, st)
	case errors.Is(err, service.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.log.Error("request failed", "path", r.URL.Path, "client", Client(r).Name, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// ErrorBody is returned for every error.
type ErrorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, ErrorBody{Error: msg})
}
