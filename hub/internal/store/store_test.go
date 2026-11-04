package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
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

	tok, err := s.CreateToken("companion", []string{"office"}, RoleFull)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "dm_") || len(tok) < 40 {
		t.Fatalf("unexpected token format %q", tok)
	}
	if c, ok := s.Authenticate(tok); !ok || c.Name != "companion" || !slices.Equal(c.Profiles, []string{"office"}) || c.Role != RoleFull || c.Admin() {
		t.Fatalf("Authenticate = %+v %v", c, ok)
	}
	if _, ok := s.Authenticate(tok + "x"); ok {
		t.Fatal("wrong token accepted")
	}
	if _, ok := s.Authenticate(""); ok {
		t.Fatal("empty token accepted")
	}
	if _, err := s.CreateToken("companion", nil, RoleFull); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := s.CreateToken("Bad Name", nil, RoleFull); err == nil {
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

func TestTokenRolesAndProfiles(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	for _, c := range []struct {
		name     string
		profiles []string
		role     Role
		admin    bool
		sees     map[string]bool
	}{
		{"admin", nil, RoleFull, true, map[string]bool{"office": true, "teen": true}},
		{"couch", nil, RoleRead, false, map[string]bool{"office": true, "teen": true}},
		{"kitchen", []string{"office", "teen"}, RoleRead, false, map[string]bool{"office": true, "teen": true, "other": false}},
		{"teen-phone", []string{"teen"}, RoleFull, false, map[string]bool{"office": false, "teen": true}},
	} {
		tok, err := s.CreateToken(c.name, c.profiles, c.role)
		if err != nil {
			t.Fatal(err)
		}
		cl, ok := s.Authenticate(tok)
		if !ok || cl.Role != c.role || cl.Admin() != c.admin || !slices.Equal(cl.Profiles, c.profiles) {
			t.Errorf("%s: Authenticate = %+v", c.name, cl)
		}
		for p, want := range c.sees {
			if cl.Sees(p) != want {
				t.Errorf("%s: Sees(%s) = %v", c.name, p, !want)
			}
		}
	}
	if _, err := s.CreateToken("x", nil, "root"); err == nil {
		t.Error("unknown role accepted")
	}
	if _, err := s.CreateToken("y", []string{"a,b"}, RoleRead); err == nil {
		t.Error("profile with a comma accepted")
	}
	list, err := s.ListTokens()
	if err != nil || len(list) != 4 || list[1].Name != "couch" || list[1].Role != RoleRead || len(list[1].Profiles) != 0 ||
		!slices.Equal(list[2].Profiles, []string{"office", "teen"}) {
		t.Fatalf("ListTokens = %+v, %v", list, err)
	}
}

func TestMigrateKeepsOldTokensFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	// Tokens before roles: one scoped, one unscoped.
	_, err = db.Exec(`CREATE TABLE tokens (name TEXT PRIMARY KEY, hash TEXT NOT NULL UNIQUE, profile TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, last_used TEXT);
		INSERT INTO tokens(name, hash, profile, created_at) VALUES('widget', ?, 'office', '2026-11-01T00:00:00Z'), ('admin', ?, '', '2026-11-01T00:00:00Z');`,
		hash("dm_widget"), hash("dm_admin"))
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if c, ok := s.Authenticate("dm_widget"); !ok || c.Role != RoleFull || !slices.Equal(c.Profiles, []string{"office"}) {
		t.Errorf("widget = %+v %v", c, ok)
	}
	if c, ok := s.Authenticate("dm_admin"); !ok || !c.Admin() {
		t.Errorf("admin = %+v %v", c, ok)
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
	if _, err := s.CreateToken("widget", []string{"office"}, RoleRead); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(HistoryEntry{Profile: "office", At: time.Now(), Status: "free", Reason: "default"}); err != nil {
		t.Fatal(err)
	}
}
