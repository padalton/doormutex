package i18n

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestFromEnv(t *testing.T) {
	for env, want := range map[[3]string]string{
		{"", "", "de_AT.UTF-8"}:      "de",
		{"", "en_GB.UTF-8", "de_AT"}: "en",
		{"fr:de", "", "en_US"}:       "de",
		{"", "C", "de_AT"}:           "en",
		{"", "", ""}:                 "en",
		{"", "", "sv_SE"}:            "en",
	} {
		t.Setenv("LANGUAGE", env[0])
		t.Setenv("LC_ALL", env[1])
		t.Setenv("LC_MESSAGES", "")
		t.Setenv("LANG", env[2])
		if got := FromEnv().Lang; got != want {
			t.Errorf("%v: %s, want %s", env, got, want)
		}
	}
}

func TestT(t *testing.T) {
	de := New("de")
	if got := de.T("until", "time", "14:30"); got != "bis 14:30" {
		t.Errorf("until = %q", got)
	}
	if got := de.T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key = %q", got)
	}
	if New("xx").Lang != "en" {
		t.Error("unknown language is not English")
	}
}

// Every catalog has only English keys and the same placeholders.
func TestCatalogs(t *testing.T) {
	en := read("en")
	ph := regexp.MustCompile(`\{\w+\}`)
	names, _ := fs.Glob(files, "*.json")
	for _, n := range names {
		lang := strings.TrimSuffix(n, ".json")
		cat := read(lang)
		if cat == nil {
			t.Fatalf("%s: invalid JSON", n)
		}
		for k, v := range cat {
			src, ok := en[k]
			if !ok {
				t.Errorf("%s: unknown key %q", n, k)
			} else if strings.Join(ph.FindAllString(src, -1), ",") != strings.Join(ph.FindAllString(v, -1), ",") {
				t.Errorf("%s: %s placeholders differ", n, k)
			}
		}
	}
}
