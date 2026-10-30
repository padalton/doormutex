package secret

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/padalton/doormutex/hub/internal/store"
)

type token struct {
	RefreshToken string `json:"refresh_token"`
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func box(t *testing.T) *Box {
	t.Helper()
	b, err := New([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRoundTrip(t *testing.T) {
	st := openStore(t)
	s := Store{KV: st, Box: box(t)}
	if err := s.Put("profiles/office/google/private", token{"refresh-me"}); err != nil {
		t.Fatal(err)
	}
	// On disk there is no plaintext.
	var raw string
	if err := st.Get("profiles/office/google/private", &raw); err != nil || !strings.HasPrefix(raw, prefix) || strings.Contains(raw, "refresh-me") {
		t.Fatalf("stored value %q (%v)", raw, err)
	}
	var got token
	if err := s.Get("profiles/office/google/private", &got); err != nil || got.RefreshToken != "refresh-me" {
		t.Fatalf("Get = %+v %v", got, err)
	}
	if err := s.Get("missing", &got); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	if err := s.Put("profiles/office/google/private", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Get("profiles/office/google/private", &got); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestWrongKey(t *testing.T) {
	st := openStore(t)
	if err := (Store{KV: st, Box: box(t)}).Put("k", token{"x"}); err != nil {
		t.Fatal(err)
	}
	other, _ := New([]byte(strings.Repeat("o", 32)))
	var got token
	err := Store{KV: st, Box: other}.Get("k", &got)
	if !errors.Is(err, ErrDecrypt) || !strings.Contains(err.Error(), EnvKey) {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestMigration(t *testing.T) {
	st := openStore(t)
	_ = st.Put("profiles/office/google/private", token{"plain-1"}) // written by an older hub
	_ = st.Put("profiles/office/google/work", token{"plain-2"})
	s := Store{KV: st, Box: box(t)}
	keys, _ := st.Keys("profiles/office/google/")
	if n, err := s.Migrate(keys); err != nil || n != 2 {
		t.Fatalf("Migrate = %d, %v", n, err)
	}
	if n, _ := s.Migrate(keys); n != 0 {
		t.Fatalf("second Migrate encrypted %d again", n)
	}
	var got token
	if err := s.Get("profiles/office/google/work", &got); err != nil || got.RefreshToken != "plain-2" {
		t.Fatalf("after migration: %+v %v", got, err)
	}
	// A plaintext value that slipped through is encrypted on first read.
	_ = st.Put("profiles/teen/google/private", token{"plain-3"})
	if err := s.Get("profiles/teen/google/private", &got); err != nil || got.RefreshToken != "plain-3" {
		t.Fatalf("read plaintext: %+v %v", got, err)
	}
	var raw string
	if err := st.Get("profiles/teen/google/private", &raw); err != nil || !strings.HasPrefix(raw, prefix) {
		t.Fatalf("not re-encrypted: %q %v", raw, err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hub.db")
	t.Setenv(EnvKey, "")
	b1, src, err := Load(db)
	if err != nil || !strings.Contains(src, "new key file") {
		t.Fatalf("first Load: %v %q", err, src)
	}
	fi, err := os.Stat(filepath.Join(dir, KeyFile))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
	b2, src, err := Load(db)
	if err != nil || !strings.HasPrefix(src, "key file") {
		t.Fatalf("second Load: %v %q", err, src)
	}
	if plain, err := b2.Open(b1.Seal([]byte("same key"))); err != nil || string(plain) != "same key" {
		t.Fatalf("key changed between starts: %v", err)
	}

	t.Setenv(EnvKey, NewKey())
	if _, src, err := Load(db); err != nil || src != "env "+EnvKey {
		t.Fatalf("env key: %v %q", err, src)
	}
	t.Setenv(EnvKey, "c2hvcnQ=")
	if _, _, err := Load(db); err == nil {
		t.Fatal("short key accepted")
	}
}
