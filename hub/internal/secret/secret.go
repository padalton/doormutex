// Package secret encrypts values at rest (Google refresh tokens) with
// AES-256-GCM. The key comes from DOORMUTEX_SECRET_KEY or from a key file
// next to the database that is created on first start.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvKey is the environment variable with the base64-encoded 32-byte key.
const EnvKey = "DOORMUTEX_SECRET_KEY"

// KeyFile is the name of the key file next to the database.
const KeyFile = "secret.key"

const prefix = "enc:v1:"

// ErrDecrypt means a value can't be decrypted – usually a different key.
var ErrDecrypt = errors.New("cannot decrypt (different " + EnvKey + " or " + KeyFile + "?)")

// Box seals and opens values.
type Box struct{ aead cipher.AEAD }

// New returns a box for a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key: want 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Load returns the box for the hub: the key from EnvKey, else from the key
// file in the database's directory, which is created (mode 0600) if it
// doesn't exist. An in-memory database gets a random key.
func Load(dbPath string) (*Box, string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvKey)); v != "" {
		key, err := decodeKey(v)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", EnvKey, err)
		}
		b, err := New(key)
		return b, "env " + EnvKey, err
	}
	if dbPath == ":memory:" {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		b, err := New(key)
		return b, "random (in-memory database)", err
	}
	path := filepath.Join(filepath.Dir(dbPath), KeyFile)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		_, _ = rand.Read(key)
		enc := base64.StdEncoding.EncodeToString(key) + "\n"
		// O_EXCL: two processes starting at once must not both write a key.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, err = f.WriteString(enc)
			err = errors.Join(err, f.Close())
			if err != nil {
				return nil, "", fmt.Errorf("write %s: %w", path, err)
			}
			b, err := New(key)
			return b, "new key file " + path, err
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", fmt.Errorf("create %s: %w", path, err)
		}
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
	} else if err != nil {
		return nil, "", err
	}
	key, err := decodeKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	b, err := New(key)
	return b, "key file " + path, err
}

// NewKey returns a fresh key, base64-encoded (for DOORMUTEX_SECRET_KEY).
func NewKey() string {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(s); err == nil {
			if len(k) != 32 {
				return nil, fmt.Errorf("want 32 bytes, got %d", len(k))
			}
			return k, nil
		}
	}
	return nil, errors.New("not base64 – create one with: openssl rand -base64 32")
}

// Seal encrypts plain into a printable string.
func (b *Box) Seal(plain []byte) string {
	nonce := make([]byte, b.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return prefix + base64.StdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, plain, nil))
}

// Open decrypts a string made by Seal.
func (b *Box) Open(s string) ([]byte, error) {
	enc, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return nil, errors.New("not an encrypted value")
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return nil, ErrDecrypt
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// KV is a JSON key-value store (the hub's SQLite store).
type KV interface {
	Put(key string, v any) error
	Get(key string, v any) error
}

// Store encrypts every value it puts into a KV. Plaintext values written
// before encryption existed are read and re-written encrypted.
type Store struct {
	KV  KV
	Box *Box
}

// Put stores v encrypted; a nil v deletes the key.
func (s Store) Put(key string, v any) error {
	if v == nil {
		return s.KV.Put(key, nil)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.KV.Put(key, s.Box.Seal(b))
}

// Get loads and decrypts the value under key into v.
func (s Store) Get(key string, v any) error {
	var raw json.RawMessage
	if err := s.KV.Get(key, &raw); err != nil {
		return err
	}
	var sealed string
	if json.Unmarshal(raw, &sealed) == nil && strings.HasPrefix(sealed, prefix) {
		plain, err := s.Box.Open(sealed)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		return json.Unmarshal(plain, v)
	}
	// Plaintext from before encryption: use it and encrypt it now.
	if err := json.Unmarshal(raw, v); err != nil {
		return err
	}
	return s.KV.Put(key, s.Box.Seal(raw))
}

// Migrate encrypts the plaintext values under keys; encrypted values and
// missing keys are left alone. It returns how many were encrypted.
func (s Store) Migrate(keys []string) (int, error) {
	n := 0
	for _, k := range keys {
		var raw json.RawMessage
		if err := s.KV.Get(k, &raw); err != nil {
			continue
		}
		var sealed string
		if json.Unmarshal(raw, &sealed) == nil && strings.HasPrefix(sealed, prefix) {
			continue
		}
		if err := s.KV.Put(k, s.Box.Seal(raw)); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
