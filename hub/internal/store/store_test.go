package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	tok, err := s.CreateToken("companion", "office")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "dm_") || len(tok) < 40 {
		t.Fatalf("unexpected token format %q", tok)
	}
	if c, ok := s.Authenticate(tok); !ok || c.Name != "companion" || c.Profile != "office" {
		t.Fatalf("Authenticate = %+v %v", c, ok)
	}
	if _, ok := s.Authenticate(tok + "x"); ok {
		t.Fatal("wrong token accepted")
	}
	if _, ok := s.Authenticate(""); ok {
		t.Fatal("empty token accepted")
	}
	if _, err := s.CreateToken("companion", ""); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err := s.CreateToken("Bad Name", ""); err == nil {
		t.Fatal("invalid name accepted")
	}
	list, err := s.ListTokens()
	if err != nil || len(list) != 1 || list[0].LastUsed == "" {
		t.Fatalf("ListTokens = %+v, %v", list, err)
	}
	if err := s.RevokeToken("companion"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate(tok); ok {
		t.Fatal("revoked token accepted")
	}
	if err := s.RevokeToken("companion"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RevokeToken twice: %v", err)
	}
}

func TestKVAndHistoryPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	type obj struct{ A string }
	if err := s.Put("override", obj{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{Profile: "office", At: time.Now(), Status: "dnd", Reason: "calendar"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var got obj
	if err := s.Get("override", &got); err != nil || got.A != "x" {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := s.Put("override", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Get("override", &got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: %v", err)
	}
	h, err := s.History("office", 10)
	if err != nil || len(h) != 1 || h[0].Status != "dnd" {
		t.Fatalf("History = %+v, %v", h, err)
	}
	if h, _ := s.History("teen", 10); len(h) != 0 {
		t.Fatalf("history leaked into another profile: %+v", h)
	}
}

func TestMigrateAddsProfileColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// The M0 schema without profile columns.
	_, err = db.Exec(`CREATE TABLE tokens (name TEXT PRIMARY KEY, hash TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL, last_used TEXT);
		CREATE TABLE history (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT NOT NULL, status TEXT NOT NULL, reason TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '');`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateToken("widget", "office"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{Profile: "office", At: time.Now(), Status: "free", Reason: "default"}); err != nil {
		t.Fatal(err)
	}
}
