package httpapi

import (
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

func testTranslations(t *testing.T) *translations {
	t.Helper()
	tr, err := loadTranslations(fstest.MapFS{
		"locales/en.json": {Data: []byte(`{"a":"one","b":"two","c":"three"}`)},
		"locales/my.json": {Data: []byte(`{"a":"တစ်","c":"သုံး"}`)},
		"locales/README":  {Data: []byte("not a locale")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestTranslations_Negotiate(t *testing.T) {
	tr := testTranslations(t)
	tests := []struct {
		name, query, accept, want string
	}{
		{"nothing asked", "", "", "en"},
		{"browser asks for Burmese", "", "my", "my"},
		{"region is ignored", "", "my-MM,en;q=0.5", "my"},
		{"case is ignored", "", "MY-mm", "my"},
		{"unknown first, known second", "", "th-TH,my;q=0.8", "my"},
		{"quality beats header order", "", "en;q=0.4,my;q=0.9", "my"},
		{"equal quality keeps header order", "", "my,en", "my"},
		{"q=0 means not acceptable", "", "my;q=0,en", "en"},
		{"nothing we have", "", "th,fr;q=0.8", "en"},
		{"wildcard is not a language", "", "*", "en"},
		{"garbage", "", ";;q=,,my;q=x,", "en"},
		{"query wins over the header", "my", "en", "my"},
		{"query with region", "my-MM", "en", "my"},
		{"unknown query falls back to the header", "xx", "my", "my"},
		{"locales/README is not a language", "README", "", "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tr.negotiate(tt.query, tt.accept); got != tt.want {
				t.Errorf("negotiate(%q, %q) = %q, want %q", tt.query, tt.accept, got, tt.want)
			}
		})
	}
}

func TestTranslations_ALongHeaderIsBounded(t *testing.T) {
	tr := testTranslations(t)
	// Burmese is the 30th entry: past the limit, so it is never looked at.
	accept := strings.Repeat("xx,", 29) + "my"
	if got := tr.negotiate("", accept); got != "en" {
		t.Errorf("negotiate = %q, want en (entries past %d are ignored)", got, maxAcceptEntries)
	}
}

func TestTranslations_MissingKeysShowEnglish(t *testing.T) {
	tr := testTranslations(t)
	got := tr.messages("my")
	want := map[string]string{"a": "တစ်", "b": "two", "c": "သုံး"}
	if len(got) != len(want) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("messages[%q] = %q, want %q", k, got[k], v)
		}
	}
	// Asking for the messages must not change what English says.
	if tr.dicts["en"]["a"] != "one" {
		t.Error("messages(my) overwrote the English dictionary")
	}
}

func TestTranslations_LoadErrors(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"no locales folder": {},
		"no English":        {"locales/my.json": {Data: []byte(`{}`)}},
		"bad JSON":          {"locales/en.json": {Data: []byte(`{`)}},
		"not text":          {"locales/en.json": {Data: []byte(`{"a":1}`)}},
	}
	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadTranslations(files); err == nil {
				t.Error("loadTranslations succeeded, want an error")
			}
		})
	}
}

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

func placeholders(text string) []string {
	var names []string
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return names
}

// A translation may be unfinished, but it may not invent a key (a typo would
// silently never show) or lose a {placeholder} (the page would print the braces
// or drop a name). This is the check `tsc` does for the desktop window.
func TestTranslations_EveryLanguageMatchesEnglish(t *testing.T) {
	tr := mustLoadTranslations()
	english := tr.dicts[fallbackLang]
	for code, dict := range tr.dicts {
		for key, text := range dict {
			base, ok := english[key]
			if !ok {
				t.Errorf("%s.json has %q, which English does not", code, key)
				continue
			}
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s.json: %q is empty", code, key)
			}
			if !slices.Equal(placeholders(text), placeholders(base)) {
				t.Errorf("%s.json: %q has placeholders %v, English has %v", code, key, placeholders(text), placeholders(base))
			}
		}
	}
}

// Both the shipped languages are complete today; if one falls behind, the page
// still works (English fills in) but the gap should be a decision, not an accident.
func TestTranslations_ShippedLanguagesAreComplete(t *testing.T) {
	tr := mustLoadTranslations()
	for code, dict := range tr.dicts {
		for key := range tr.dicts[fallbackLang] {
			if _, ok := dict[key]; !ok {
				t.Errorf("%s.json lacks %q", code, key)
			}
		}
	}
}

var (
	jsKey    = regexp.MustCompile(`\bt\('([\w.]+)'`)
	jsPlural = regexp.MustCompile(`\btn\('([\w.]+)'`)
	htmlKey  = regexp.MustCompile(`data-i18n(?:-placeholder|-label)?="([\w.]+)"`)
)

// The page asks for keys by name in plain JavaScript, so nothing but this test
// notices a key that English lacks.
func TestPlayerPage_UsesOnlyKeysEnglishHas(t *testing.T) {
	english := mustLoadTranslations().dicts[fallbackLang]
	read := func(name string) string {
		b, err := webAssets.ReadFile("web/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	js, html := read("app.js"), read("index.html")

	var keys []string
	for _, m := range jsKey.FindAllStringSubmatch(js, -1) {
		keys = append(keys, m[1])
	}
	for _, m := range htmlKey.FindAllStringSubmatch(html, -1) {
		keys = append(keys, m[1])
	}
	for _, m := range jsPlural.FindAllStringSubmatch(js, -1) {
		keys = append(keys, m[1]+".other")
	}
	if len(keys) < 40 {
		t.Fatalf("found only %d keys in app.js and index.html; the pattern probably no longer matches", len(keys))
	}
	for _, k := range keys {
		if _, ok := english[k]; !ok {
			t.Errorf("the player page asks for %q, which locales/en.json lacks", k)
		}
	}

	// And the other way round: a key nobody asks for is dead weight sent to every phone.
	used := make(map[string]bool, len(keys))
	for _, k := range keys {
		used[k] = true
	}
	for k := range english {
		base := strings.TrimSuffix(strings.TrimSuffix(k, ".one"), ".other")
		if !used[k] && !used[base+".other"] {
			t.Errorf("locales/en.json has %q, which the player page never uses", k)
		}
	}
}

func TestStringsEndpoint(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name, path, accept, wantLang string
	}{
		{"default is English", "/api/strings", "", "en"},
		{"browser language", "/api/strings", "my-MM,my;q=0.9,en;q=0.5", "my"},
		{"?lang wins", "/api/strings?lang=my", "en", "my"},
		{"unknown language", "/api/strings?lang=xx", "th", "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers []string
			if tt.accept != "" {
				headers = []string{"Accept-Language", tt.accept}
			}
			resp := f.get(tt.path, headers...)
			body := readBody(t, resp)
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, body %.80q", resp.StatusCode, body)
			}
			if got := resp.Header.Get("Content-Language"); got != tt.wantLang {
				t.Errorf("Content-Language = %q, want %q", got, tt.wantLang)
			}
			if !strings.Contains(resp.Header.Get("Vary"), "Accept-Language") {
				t.Errorf("Vary = %q, want it to name Accept-Language", resp.Header.Get("Vary"))
			}
			var got stringsJSON
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			if got.Lang != tt.wantLang {
				t.Errorf("lang = %q, want %q", got.Lang, tt.wantLang)
			}
			english := mustLoadTranslations().dicts[fallbackLang]
			if len(got.Messages) != len(english) {
				t.Errorf("%d messages, want all %d of English's keys", len(got.Messages), len(english))
			}
			if tt.wantLang == "en" && got.Messages["item.play"] != "Play" {
				t.Errorf("item.play = %q", got.Messages["item.play"])
			}
			if tt.wantLang == "my" && got.Messages["item.play"] == "Play" {
				t.Error("item.play is still English in the Burmese answer")
			}
		})
	}
}

func TestStringsEndpoint_NeedsTheAccessToken(t *testing.T) {
	f, _ := newSecured(t)
	if resp := f.do("GET", "/api/strings", nil); resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401: only the static files are public", resp.StatusCode)
	}
}
