package media

import (
	"path"
	"slices"
	"strings"
)

// Subtitle is a sidecar subtitle file that belongs to a video: Movie.srt, or
// Movie.en.srt next to Movie.mkv.
type Subtitle struct {
	// Lang is the language tag from the file name ("en", "pt-BR"), or "".
	Lang string
	// Label is what a person sees in the player's menu ("English", "English (forced)").
	Label string
	// RelPath is slash-separated and relative to the library root.
	RelPath string
	// Path is the absolute location on disk. Like Item.Path it never leaves the
	// server: clients ask for a track by the video's ID and its position.
	Path string
}

// IsSubtitleFile reports whether a file name is a subtitle format EinCast can
// serve. Only text formats are supported; image formats (.sub/.idx, PGS) would
// need rendering.
func IsSubtitleFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".srt", ".vtt":
		return true
	default:
		return false
	}
}

// SubtitleMatch is a subtitle file name that belongs to a video, with what the
// name says about it.
type SubtitleMatch struct {
	Name  string
	Lang  string
	Label string
}

// MatchSubtitles picks, from the file names in one folder, the subtitles that
// belong to the video called videoName. A subtitle belongs when its name is the
// video's name with the extension swapped (Movie.srt) or with extra dot-separated
// tags before it (Movie.en.srt, Movie.pt-BR.forced.srt). Movie2.srt and
// Movie Extras.srt do not belong to Movie.mkv. The comparison ignores case, and
// the result is ordered by name so it is stable between scans.
func MatchSubtitles(videoName string, folder []string) []SubtitleMatch {
	stem := strings.ToLower(strings.TrimSuffix(videoName, path.Ext(videoName)))
	var out []SubtitleMatch
	for _, name := range folder {
		if !IsSubtitleFile(name) {
			continue
		}
		sub := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
		var tags string
		switch {
		case sub == stem:
		case strings.HasPrefix(sub, stem+"."):
			tags = sub[len(stem)+1:]
		default:
			continue
		}
		lang, label := describe(tags)
		out = append(out, SubtitleMatch{Name: name, Lang: lang, Label: label})
	}
	slices.SortFunc(out, func(a, b SubtitleMatch) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// languages names the codes people actually put in file names, both the
// two-letter and the three-letter forms.
var languages = map[string]string{
	"en": "English", "eng": "English",
	"fr": "French", "fre": "French", "fra": "French",
	"de": "German", "ger": "German", "deu": "German",
	"es": "Spanish", "spa": "Spanish",
	"it": "Italian", "ita": "Italian",
	"pt": "Portuguese", "por": "Portuguese",
	"ru": "Russian", "rus": "Russian",
	"ja": "Japanese", "jpn": "Japanese",
	"ko": "Korean", "kor": "Korean",
	"zh": "Chinese", "chi": "Chinese", "zho": "Chinese",
	"th": "Thai", "tha": "Thai",
	"vi": "Vietnamese", "vie": "Vietnamese",
	"my": "Burmese", "bur": "Burmese", "mya": "Burmese",
	"hi": "Hindi", "hin": "Hindi",
	"ar": "Arabic", "ara": "Arabic",
	"id": "Indonesian", "ind": "Indonesian",
	"nl": "Dutch", "dut": "Dutch", "nld": "Dutch",
	"tr": "Turkish", "tur": "Turkish",
}

// describe reads the tags between the video's name and the extension:
// "en", "en.forced", "pt-br.sdh", or "" for none.
func describe(tags string) (lang, label string) {
	if tags == "" {
		return "", "Subtitles"
	}
	parts := strings.Split(tags, ".")
	var flags []string
	for i, p := range parts {
		switch {
		case lang == "" && i == 0 && looksLikeLanguage(p):
			lang = p
		case p == "forced", p == "sdh", p == "cc", p == "hi":
			flags = append(flags, p)
		default:
			// An unknown tag ("commentary", "v2") is shown as it is.
			flags = append(flags, p)
		}
	}
	name := "Subtitles"
	if lang != "" {
		base, _, _ := strings.Cut(lang, "-")
		name = languages[base]
		if name == "" {
			name = strings.ToUpper(lang)
		}
		lang = canonicalLang(lang)
	}
	if len(flags) > 0 {
		name += " (" + strings.Join(flags, ", ") + ")"
	}
	return lang, name
}

// looksLikeLanguage accepts "en", "eng", "pt-br": two or three letters,
// optionally followed by a region.
func looksLikeLanguage(s string) bool {
	base, region, hasRegion := strings.Cut(s, "-")
	if len(base) < 2 || len(base) > 3 || !isLetters(base) {
		return false
	}
	return !hasRegion || (len(region) == 2 && isLetters(region))
}

func isLetters(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return s != ""
}

// canonicalLang writes a tag the way HTML's srclang wants it: "pt-BR".
func canonicalLang(tag string) string {
	base, region, ok := strings.Cut(tag, "-")
	if !ok {
		return base
	}
	return base + "-" + strings.ToUpper(region)
}
