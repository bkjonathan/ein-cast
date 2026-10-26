package media

import (
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxNFOSize is the most of a .nfo file that is read. They are a few hundred
// bytes to a few kilobytes; a file bigger than this is not one.
const MaxNFOSize = 256 << 10

const (
	maxTitleRunes = 200
	maxPlotRunes  = 2000
)

// IsNFOFile reports whether a file name is a Kodi-style .nfo metadata file.
func IsNFOFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".nfo")
}

// NFO is what a Kodi-style .nfo file says about a video. Only the few fields
// EinCast shows are read.
type NFO struct {
	// Root is the kind of file: "movie", "episodedetails" or something else.
	Root      string
	Title     string
	ShowTitle string
	Plot      string
	Year      int
	Season    int
	Episode   int
}

// nfoXML is the shape of the part of the file that is read. Every field is
// optional, and the root element's name is not checked here: movie.nfo and
// episode nfo files share these names.
type nfoXML struct {
	XMLName   xml.Name
	Title     string `xml:"title"`
	ShowTitle string `xml:"showtitle"`
	Plot      string `xml:"plot"`
	Year      string `xml:"year"`
	Premiered string `xml:"premiered"`
	Aired     string `xml:"aired"`
	Season    string `xml:"season"`
	Episode   string `xml:"episode"`
}

// ParseNFO reads a .nfo file. It is lenient on purpose: these files are written
// by many tools, are often not quite valid XML, and sometimes have a URL after
// the closing tag. Whatever can be read is used, and only a file with no
// readable root element is an error.
//
// encoding/xml does not load external entities or DTDs, so a hostile file can do
// no more than hold wrong text; text is bounded, stripped of control characters,
// and shown by clients as text, never as HTML.
func ParseNFO(data []byte) (NFO, error) {
	if len(data) > MaxNFOSize {
		return NFO{}, errors.New("nfo file is too large")
	}
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	// Declared encodings other than UTF-8 are read as if they were UTF-8; the
	// text is repaired below. Not worth a charset library for a description.
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }

	var x nfoXML
	if err := dec.Decode(&x); err != nil && x.XMLName.Local == "" {
		return NFO{}, errors.New("not a readable nfo file")
	}

	n := NFO{
		Root:      strings.ToLower(x.XMLName.Local),
		Title:     cleanText(x.Title, maxTitleRunes),
		ShowTitle: cleanText(x.ShowTitle, maxTitleRunes),
		Plot:      cleanText(x.Plot, maxPlotRunes),
		Season:    atoiPositive(x.Season),
		Episode:   atoiPositive(x.Episode),
	}
	n.Year = yearOf(x.Year)
	if n.Year == 0 {
		n.Year = yearOf(x.Premiered)
	}
	if n.Year == 0 {
		n.Year = yearOf(x.Aired)
	}
	return n, nil
}

// Apply returns m corrected by what the file says. A field the file leaves empty
// keeps the guess from the name, so a nearly empty .nfo cannot make an item
// worse.
func (m Meta) Apply(n NFO) Meta {
	if n.Title != "" {
		m.Title = n.Title
	}
	if n.Year != 0 {
		m.Year = n.Year
	}
	if n.Plot != "" {
		m.Plot = n.Plot
	}
	if n.Root == "episodedetails" {
		if n.ShowTitle != "" {
			m.Show = n.ShowTitle
		}
		if n.Season > 0 {
			m.Season = n.Season
		}
		if n.Episode > 0 {
			m.Episode = n.Episode
		}
	}
	return m
}

// cleanText collapses whitespace, drops control characters and invalid UTF-8,
// and cuts the result to max runes.
func cleanText(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' ' // tabs and line breaks separate words
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

func atoiPositive(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > 9999 {
		return 0
	}
	return n
}

// yearOf reads the year from "2019" or "2019-04-02".
func yearOf(s string) int {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0
	}
	y, ok := asYear(s[:4])
	if !ok {
		return 0
	}
	return y
}
