// Package i18n holds the companion's texts (notifications, terminal) in
// JSON catalogs, one per language, like the hub's web UI. English is the
// fallback for missing keys and languages.
package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"strings"
)

//go:embed *.json
var files embed.FS

// Catalog translates keys into one language.
type Catalog struct {
	Lang     string
	texts    map[string]string
	fallback map[string]string
}

func read(lang string) map[string]string {
	b, err := files.ReadFile(lang + ".json")
	if err != nil {
		return nil
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// New returns the catalog for a language code such as "de"; unknown
// languages get English.
func New(lang string) *Catalog {
	en := read("en")
	c := &Catalog{Lang: "en", texts: en, fallback: en}
	if m := read(lang); m != nil && lang != "en" {
		c.Lang, c.texts = lang, m
	}
	return c
}

// FromEnv picks the language like gettext: LANGUAGE (a list), LC_ALL,
// LC_MESSAGES, LANG – e.g. "de_AT.UTF-8" → de.
func FromEnv() *Catalog {
	var wanted []string
	if v := os.Getenv("LANGUAGE"); v != "" {
		wanted = strings.Split(v, ":")
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			wanted = append(wanted, v)
			break
		}
	}
	for _, w := range wanted {
		parts := strings.FieldsFunc(w, func(r rune) bool { return r == '_' || r == '.' || r == '@' || r == '-' })
		if len(parts) == 0 {
			continue
		}
		code := strings.ToLower(parts[0])
		if code == "c" || code == "posix" {
			break
		}
		if read(code) != nil {
			return New(code)
		}
	}
	return New("en")
}

// T returns the text for key with {name} placeholders replaced by the
// pairs in args ("name", value, …). Unknown keys come back as they are.
func (c *Catalog) T(key string, args ...string) string {
	s, ok := c.texts[key]
	if !ok {
		if s, ok = c.fallback[key]; !ok {
			return key
		}
	}
	for i := 0; i+1 < len(args); i += 2 {
		s = strings.ReplaceAll(s, "{"+args[i]+"}", args[i+1])
	}
	return s
}

// Has reports whether key exists (in this language or English).
func (c *Catalog) Has(key string) bool {
	_, ok := c.fallback[key]
	return ok
}
