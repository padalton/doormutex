package store

import (
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

	tok, err := s.CreateToken("companion")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "dm_") || len(tok) < 40 {
		t.Fatalf("unexpected token format %q", tok)
	}
	if name, ok := s.Authenticate(tok); !ok || name != "companion" {
		t.Fatalf("Authenticate = %q %v", name, ok)
	}
	if _, ok := s.Authenticate(tok + "x"); ok {
		t.Fatal("wrong token accepted")
	}
	if _, ok := s.Authenticate(""); ok {
		t.Fatal("empty token accepted")
	}
	if _, err := s.CreateToken("companion"); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err := s.CreateToken("Bad Name"); err == nil {
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
	if err := s.AppendHistory(HistoryEntry{At: time.Now(), Status: "dnd", Reason: "calendar"}); err != nil {
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
	h, err := s.History(10)
	if err != nil || len(h) != 1 || h[0].Status != "dnd" {
		t.Fatalf("History = %+v, %v", h, err)
	}
}
