package httpapi

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// fallbackLang is the language every other one is completed from, and the one
// used when the device asks for something we do not have.
const fallbackLang = "en"

// maxAcceptEntries bounds the work a hostile Accept-Language header can cause.
const maxAcceptEntries = 20

// translations are the player page's words, one flat JSON file per language in
// web/locales (ADR 0033). English is the source: another language may lack a
// key and then shows the English text, so a half-finished translation works.
type translations struct {
	dicts map[string]map[string]string // language code -> key -> text
}

// loadTranslations reads every locales/<code>.json in files.
func loadTranslations(files fs.FS) (*translations, error) {
	entries, err := fs.ReadDir(files, "locales")
	if err != nil {
		return nil, fmt.Errorf("read locales: %w", err)
	}
	t := &translations{dicts: make(map[string]map[string]string)}
	for _, e := range entries {
		code, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() {
			continue
		}
		raw, err := fs.ReadFile(files, "locales/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("read locale %q: %w", code, err)
		}
		dict := make(map[string]string)
		if err := json.Unmarshal(raw, &dict); err != nil {
			return nil, fmt.Errorf("parse locale %q: %w", code, err)
		}
		t.dicts[code] = dict
	}
	if t.dicts[fallbackLang] == nil {
		return nil, fmt.Errorf("locales has no %s.json", fallbackLang)
	}
	return t, nil
}

// messages is every key in the language, with English filling any gap.
func (t *translations) messages(lang string) map[string]string {
	out := make(map[string]string, len(t.dicts[fallbackLang]))
	for k, v := range t.dicts[fallbackLang] {
		out[k] = v
	}
	for k, v := range t.dicts[lang] {
		out[k] = v
	}
	return out
}

// text is the words for key in the language, with {name} filled from params.
// A key nobody wrote shows as itself, as on the page, so the gap is visible.
func (t *translations) text(lang, key string, params map[string]any) string {
	template, ok := t.dicts[lang][key]
	if !ok {
		template, ok = t.dicts[fallbackLang][key]
	}
	if !ok {
		return key
	}
	pairs := make([]string, 0, 2*len(params))
	for name, value := range params {
		pairs = append(pairs, "{"+name+"}", fmt.Sprint(value))
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

// plural is text for base.one when count is 1, else base.other, with {count}
// filled in. That is the rule of English and, with both forms written alike, of
// Burmese; a language that needs more forms needs a rule here (ADR 0033).
func (t *translations) plural(lang, base string, count int, params map[string]any) string {
	form := "other"
	if count == 1 {
		form = "one"
	}
	merged := map[string]any{"count": count}
	for k, v := range params {
		merged[k] = v
	}
	return t.text(lang, base+"."+form, merged)
}

// langOf is the language to answer a request in.
func (a *api) langOf(r *http.Request) string {
	return a.text.negotiate(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
}

// negotiate picks the language to answer in: an explicit ?lang= (for a device
// whose browser language cannot be changed, such as a TV), else the first
// Accept-Language entry we have, best-liked first. Regions are ignored, so
// "my-MM" and "en-GB" match "my" and "en".
func (t *translations) negotiate(query, acceptLanguage string) string {
	if code := primaryTag(query); t.dicts[code] != nil {
		return code
	}
	for _, tag := range rankedTags(acceptLanguage) {
		if code := primaryTag(tag); t.dicts[code] != nil {
			return code
		}
	}
	return fallbackLang
}

// primaryTag is the language part of a tag: "my-MM" -> "my".
func primaryTag(tag string) string {
	head, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(tag)), "-")
	return head
}

// rankedTags returns the tags of an Accept-Language header, highest quality
// first, in header order among equals. Entries with q=0 ("not acceptable") and
// the wildcard are dropped.
func rankedTags(header string) []string {
	type ranked struct {
		tag string
		q   float64
	}
	var list []ranked
	for i, part := range strings.Split(header, ",") {
		if i == maxAcceptEntries {
			break
		}
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		if q > 0 {
			list = append(list, ranked{tag, q})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].q > list[j].q })
	tags := make([]string, len(list))
	for i, r := range list {
		tags[i] = r.tag
	}
	return tags
}

type stringsJSON struct {
	// Lang is the language the messages are in, for <html lang>.
	Lang     string            `json:"lang"`
	Messages map[string]string `json:"messages"`
}

// handleStrings answers with the player page's words in the device's language.
// It holds nothing private, so it needs no profile.
func (a *api) handleStrings(w http.ResponseWriter, r *http.Request) {
	lang := a.langOf(r)
	h := w.Header()
	h.Set("Content-Language", lang)
	h.Add("Vary", "Accept-Language")
	h.Set("Cache-Control", "no-cache") // embedded files have no mtime; revalidate after upgrades
	writeJSON(w, http.StatusOK, stringsJSON{Lang: lang, Messages: a.text.messages(lang)})
}

// mustLoadTranslations reads the embedded locales. They are compiled in and
// TestTranslations_* parses every one, so a failure here cannot reach a user.
func mustLoadTranslations() *translations {
	t, err := loadTranslations(webFS())
	if err != nil {
		panic(err) // impossible: see above
	}
	return t
}
