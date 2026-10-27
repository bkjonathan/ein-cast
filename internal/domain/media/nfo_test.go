package media

import (
	"strings"
	"testing"
)

func TestParseNFO(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want NFO
	}{
		{"movie", `<?xml version="1.0" encoding="utf-8"?><movie><title>Heat</title><year>1995</year><plot>A thief and a cop.</plot></movie>`,
			NFO{Root: "movie", Title: "Heat", Year: 1995, Plot: "A thief and a cop."}},
		{"episode", `<episodedetails><title>Mandala</title><showtitle>Breaking Bad</showtitle><season>2</season><episode>5</episode><aired>2009-04-12</aired></episodedetails>`,
			NFO{Root: "episodedetails", Title: "Mandala", ShowTitle: "Breaking Bad", Season: 2, Episode: 5, Year: 2009}},
		{"premiered when there is no year", `<movie><title>X</title><premiered>1999-03-31</premiered></movie>`,
			NFO{Root: "movie", Title: "X", Year: 1999}},
		{"a URL after the closing tag, as some scrapers write", `<movie><title>X</title></movie>
https://www.themoviedb.org/movie/1`, NFO{Root: "movie", Title: "X"}},
		{"HTML entities that XML does not define", `<movie><title>Fish&nbsp;&amp;&nbsp;Chips</title></movie>`,
			NFO{Root: "movie", Title: "Fish & Chips"}},
		{"a charset we do not decode is read as it comes", `<?xml version="1.0" encoding="ISO-8859-1"?><movie><title>Cafe</title></movie>`,
			NFO{Root: "movie", Title: "Cafe"}},
		{"unknown fields ignored, missing fields empty", `<movie><sorttitle>z</sorttitle></movie>`, NFO{Root: "movie"}},
		{"whitespace and line breaks collapse", "<movie><plot>\n  one\n\n  two\t three </plot></movie>", NFO{Root: "movie", Plot: "one two three"}},
		{"nonsense numbers are zero", `<episodedetails><season>abc</season><episode>-4</episode><year>20</year></episodedetails>`, NFO{Root: "episodedetails"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNFO([]byte(tt.in))
			if err != nil || got != tt.want {
				t.Fatalf("ParseNFO() = %+v, %v\nwant %+v", got, err, tt.want)
			}
		})
	}
}

func TestParseNFO_Rejects(t *testing.T) {
	for name, in := range map[string]string{
		"empty":      "",
		"not xml":    "just some text",
		"too large":  "<movie><plot>" + strings.Repeat("x", MaxNFOSize) + "</plot></movie>",
		"half a tag": "<mov",
	} {
		if _, err := ParseNFO([]byte(in)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseNFO_BoundsLongText(t *testing.T) {
	long := strings.Repeat("é", maxPlotRunes+500)
	n, err := ParseNFO([]byte("<movie><title>A</title><plot>" + long + "</plot></movie>"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(n.Plot)); got != maxPlotRunes {
		t.Errorf("Plot has %d runes, want it cut to %d", got, maxPlotRunes)
	}
}

func TestParseNFO_AControlCharacterLosesTheRestNotTheFile(t *testing.T) {
	// XML 1.0 forbids them and the decoder stops there; what came before is kept.
	n, err := ParseNFO([]byte("<movie><title>Heat</title><plot>a\x00b</plot></movie>"))
	if err != nil || n.Title != "Heat" {
		t.Fatalf("ParseNFO() = %+v, %v; want the title that came before", n, err)
	}
}

func TestCleanText(t *testing.T) {
	for in, want := range map[string]string{
		"A\x07B\u200bC":  "AB\u200bC", // control characters go; zero-width characters are not control
		"  a \n b\t\tc ": "a b c",
		"bad\xffbyte":    "badbyte",
		"":               "",
	} {
		if got := cleanText(in, 100); got != want {
			t.Errorf("cleanText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cleanText("abcdef", 3); got != "abc" {
		t.Errorf("cleanText cut = %q", got)
	}
}

func TestParseNFO_MarkupIsKeptAsText(t *testing.T) {
	// Clients show this with textContent; the parser's job is only to not
	// invent markup. Escaped tags stay literal characters.
	n, err := ParseNFO([]byte(`<movie><plot>&lt;script&gt;alert(1)&lt;/script&gt;</plot></movie>`))
	if err != nil || n.Plot != "<script>alert(1)</script>" {
		t.Fatalf("Plot = %q, %v", n.Plot, err)
	}
}

func TestParseNFO_ExternalEntitiesAreNotLoaded(t *testing.T) {
	in := `<?xml version="1.0"?><!DOCTYPE m [<!ENTITY x SYSTEM "file:///etc/passwd">]><movie><title>&x;</title></movie>`
	n, _ := ParseNFO([]byte(in))
	if strings.Contains(n.Title, "root:") {
		t.Fatalf("Title = %q: an external entity was read", n.Title)
	}
}

func TestMeta_Apply(t *testing.T) {
	guess := Meta{Title: "Heat", Year: 1995}
	t.Run("a movie file corrects title, year and plot", func(t *testing.T) {
		got := guess.Apply(NFO{Root: "movie", Title: "Heat (Director's Cut)", Year: 1996, Plot: "p"})
		if got != (Meta{Title: "Heat (Director's Cut)", Year: 1996, Plot: "p"}) {
			t.Errorf("Apply() = %+v", got)
		}
	})
	t.Run("an empty file changes nothing", func(t *testing.T) {
		if got := guess.Apply(NFO{Root: "movie"}); got != guess {
			t.Errorf("Apply() = %+v", got)
		}
	})
	t.Run("an episode file corrects the series and numbers", func(t *testing.T) {
		ep := Meta{Title: "S01E02", Show: "Show", Season: 1, Episode: 2}
		got := ep.Apply(NFO{Root: "episodedetails", Title: "Pilot", ShowTitle: "The Show", Season: 1, Episode: 3})
		if got != (Meta{Title: "Pilot", Show: "The Show", Season: 1, Episode: 3}) {
			t.Errorf("Apply() = %+v", got)
		}
	})
	t.Run("a movie file cannot turn a movie into an episode", func(t *testing.T) {
		got := guess.Apply(NFO{Root: "movie", ShowTitle: "X", Season: 1, Episode: 1})
		if got.IsEpisode() {
			t.Errorf("Apply() = %+v", got)
		}
	})
}
