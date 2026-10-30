// Package store persists hub state in SQLite: API tokens, the active
// override/pomodoro timer per profile and the status history.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo needed
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS tokens (
	name       TEXT PRIMARY KEY,
	hash       TEXT NOT NULL UNIQUE,
	profile    TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_used  TEXT
);
CREATE TABLE IF NOT EXISTS kv (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS history (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	profile TEXT NOT NULL DEFAULT '',
	at      TEXT NOT NULL,
	status  TEXT NOT NULL,
	reason  TEXT NOT NULL,
	detail  TEXT NOT NULL DEFAULT ''
);`

// columns added after the first release of a table: table → column → DDL.
var addedColumns = []struct{ table, column, ddl string }{
	{"tokens", "profile", `ALTER TABLE tokens ADD COLUMN profile TEXT NOT NULL DEFAULT ''`},
	{"history", "profile", `ALTER TABLE history ADD COLUMN profile TEXT NOT NULL DEFAULT ''`},
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	for _, c := range addedColumns {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.column).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := db.Exec(c.ddl); err != nil {
				return err
			}
		}
	}
	return nil
}

// Open opens (and migrates) the database at path. Use ":memory:" for tests.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; also keeps :memory: a single database
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

var tokenName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

func hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CreateToken creates an API token for a client and returns it once.
// A token with a profile only works for that profile; an empty profile
// means all profiles. Tokens are random (256 bit), so a plain SHA-256 is
// enough to store them.
func (s *Store) CreateToken(name, profile string) (string, error) {
	if !tokenName.MatchString(name) {
		return "", fmt.Errorf("token name %q: use lowercase letters, digits, '-', '_' or '.'", name)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := "dm_" + base64.RawURLEncoding.EncodeToString(b)
	_, err := s.db.Exec(`INSERT INTO tokens(name, hash, profile, created_at) VALUES(?, ?, ?, ?)`,
		name, hash(tok), profile, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("create token %q: %w", name, err)
	}
	return tok, nil
}

// Client is an authenticated API client.
type Client struct {
	Name    string
	Profile string // empty: all profiles
}

// Authenticate returns the client for a token.
func (s *Store) Authenticate(token string) (Client, bool) {
	if token == "" {
		return Client{}, false
	}
	var c Client
	err := s.db.QueryRow(`SELECT name, profile FROM tokens WHERE hash = ?`, hash(token)).Scan(&c.Name, &c.Profile)
	if err != nil {
		return Client{}, false
	}
	_, _ = s.db.Exec(`UPDATE tokens SET last_used = ? WHERE name = ?`, time.Now().UTC().Format(time.RFC3339), c.Name)
	return c, true
}

// TokenInfo describes a token without its secret.
type TokenInfo struct {
	Name      string
	Profile   string
	CreatedAt string
	LastUsed  string
}

// ListTokens lists all tokens.
func (s *Store) ListTokens() ([]TokenInfo, error) {
	rows, err := s.db.Query(`SELECT name, profile, created_at, COALESCE(last_used, '') FROM tokens ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []TokenInfo
	for rows.Next() {
		var t TokenInfo
		if err := rows.Scan(&t.Name, &t.Profile, &t.CreatedAt, &t.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken deletes a token.
func (s *Store) RevokeToken(name string) error {
	res, err := s.db.Exec(`DELETE FROM tokens WHERE name = ?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Put stores v as JSON under key; a nil v deletes the key.
func (s *Store) Put(key string, v any) error {
	if v == nil {
		_, err := s.db.Exec(`DELETE FROM kv WHERE key = ?`, key)
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO kv(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, string(b))
	return err
}

// Get loads the JSON value under key into v. It returns ErrNotFound if missing.
func (s *Store) Get(key string, v any) error {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

// Keys returns the keys that start with prefix, sorted.
func (s *Store) Keys(prefix string) ([]string, error) {
	rows, err := s.db.Query(`SELECT key FROM kv WHERE substr(key, 1, ?) = ? ORDER BY key`, len(prefix), prefix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// HistoryEntry is one status change.
type HistoryEntry struct {
	Profile string    `json:"-"`
	At      time.Time `json:"at"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason"`
	Detail  string    `json:"detail,omitempty"`
}

// AppendHistory records a status change.
func (s *Store) AppendHistory(e HistoryEntry) error {
	_, err := s.db.Exec(`INSERT INTO history(profile, at, status, reason, detail) VALUES(?, ?, ?, ?, ?)`,
		e.Profile, e.At.UTC().Format(time.RFC3339Nano), e.Status, e.Reason, e.Detail)
	return err
}

// History returns the most recent status changes of a profile, newest first.
func (s *Store) History(profile string, limit int) ([]HistoryEntry, error) {
	rows, err := s.db.Query(`SELECT at, status, reason, detail FROM history WHERE profile = ? ORDER BY id DESC LIMIT ?`, profile, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []HistoryEntry
	for rows.Next() {
		e := HistoryEntry{Profile: profile}
		var at string
		if err := rows.Scan(&at, &e.Status, &e.Reason, &e.Detail); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	return out, rows.Err()
}
