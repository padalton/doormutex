package web

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func readCatalog(t *testing.T, name string) map[string]string {
	t.Helper()
	b, err := static.ReadFile("static/i18n/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return m
}

var placeholder = regexp.MustCompile(`\{\w+\}`)

// Every translation is listed, has only known keys and keeps the
// placeholders of the English text; missing keys fall back to English.
func TestCatalogs(t *testing.T) {
	en := readCatalog(t, "en.json")
	var langs map[string]string
	b, _ := static.ReadFile("static/i18n/languages.json")
	if err := json.Unmarshal(b, &langs); err != nil || langs["en"] == "" {
		t.Fatalf("languages.json: %v", err)
	}
	files, _ := fs.Glob(static, "static/i18n/*.json")
	for _, f := range files {
		name := strings.TrimPrefix(f, "static/i18n/")
		if name == "languages.json" {
			continue
		}
		code := strings.TrimSuffix(name, ".json")
		if langs[code] == "" {
			t.Errorf("%s is not listed in languages.json", name)
		}
		cat := readCatalog(t, name)
		var missing []string
		for k, v := range cat {
			src, ok := en[k]
			if !ok {
				t.Errorf("%s: unknown key %q", name, k)
				continue
			}
			want, got := placeholder.FindAllString(src, -1), placeholder.FindAllString(v, -1)
			sort.Strings(want)
			sort.Strings(got)
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Errorf("%s: %s has placeholders %v, English %v", name, k, got, want)
			}
		}
		for k := range en {
			if _, ok := cat[k]; !ok {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			t.Logf("%s: %d keys fall back to English: %v", name, len(missing), missing)
		}
	}
	for code := range langs {
		if _, err := static.ReadFile("static/i18n/" + code + ".json"); err != nil {
			t.Errorf("languages.json lists %s without a catalog", code)
		}
	}
}

// Keys used by the page and the script exist in the English catalog.
func TestUsedKeysExist(t *testing.T) {
	en := readCatalog(t, "en.json")
	html, _ := static.ReadFile("static/index.html")
	js, _ := static.ReadFile("static/app.js")
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`data-i18n(?:-\w+)?="([\w.]+)"`).FindAllStringSubmatch(string(html), -1) {
		used[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`\bt\("([\w.]+)"`).FindAllStringSubmatch(string(js), -1) {
		used[m[1]] = true
	}
	if len(used) < 40 {
		t.Fatalf("only %d keys found – did the patterns break?", len(used))
	}
	for k := range used {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q is used but not in en.json", k)
		}
	}
	for _, prefix := range []string{"status.", "reason.", "kind.", "color."} {
		n := 0
		for k := range en {
			if strings.HasPrefix(k, prefix) {
				n++
			}
		}
		if n < 4 {
			t.Errorf("en.json has only %d %s keys", n, prefix)
		}
	}
}
