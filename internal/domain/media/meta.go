package media

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Meta is what a video's name (and the folders above it) say about it: a movie's
// title and year, or an episode's show, season and number. It is read from names
// alone, with no network and no file contents (ADR 0026), so it is a best guess
// that a .nfo file can correct. The original file name is never lost: it stays
// in Item.Name.
type Meta struct {
	// Title is what to show for the item: a movie's title, an episode's own name,
	// or the cleaned file name when nothing better was found. Empty only for a
	// name that has no letters or digits at all.
	Title string
	// Year is the release year, or 0 when the name carries none.
	Year int
	// Show names the series an episode belongs to; empty for a movie or when no
	// series could be told.
	Show string
	// Season and Episode number an episode; 0 means unknown. A series without
	// seasons has Season 0 and numbered episodes.
	Season  int
	Episode int
	// Plot is a description, set only from a .nfo file.
	Plot string
}

// IsEpisode reports whether the item belongs to a series: it has an episode
// number and a show to put it under.
func (m Meta) IsEpisode() bool { return m.Show != "" && m.Episode > 0 }

// ParseMeta reads a video's metadata from its slash-separated path relative to
// the library root. The folders count: "Breaking Bad/Season 2/05 - Mandala.mkv"
// is episode 5 of season 2 of Breaking Bad.
func ParseMeta(relPath string) Meta {
	dir, file := path.Split(relPath)
	stem := strings.TrimSuffix(file, path.Ext(file))
	dirs := splitDirs(dir)
	s := normalize(stem)

	if m, ok := parseEpisode(s, dirs); ok {
		return m
	}
	return parseMovie(s, stem)
}

func splitDirs(dir string) []string {
	dir = strings.Trim(dir, "/")
	if dir == "" {
		return nil
	}
	return strings.Split(dir, "/")
}

var (
	bracketed    = regexp.MustCompile(`\[([^\]]*)\]`)          // [Group], [1080p]: release tags, dropped; [2010] is kept
	moreEpisodes = regexp.MustCompile(`(?i)^(?:e\d{1,3}\s*)+`) // the E02 of S01E01E02
	separators   = strings.NewReplacer(".", " ", "_", " ", "(", " ", ")", " ", "{", " ", "}", " ")
	spaces       = regexp.MustCompile(`\s+`)

	seasonEpisode = regexp.MustCompile(`(?i)\bs(\d{1,2})\s*e(\d{1,3})`)
	crossEpisode  = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	wordsEpisode  = regexp.MustCompile(`(?i)\bseason\s*(\d{1,2})\s*episode\s*(\d{1,3})`)
	loneEpisode   = regexp.MustCompile(`(?i)\b(?:episode|ep|e)\s*(\d{1,3})\b`)
	leadingNumber = regexp.MustCompile(`^(\d{1,3})(?:\s+-\s+|\s+|\s*-\s*)`)
	seasonFolder  = regexp.MustCompile(`(?i)^(?:(?:season|series|staffel|saison|temporada)\s*[._-]?\s*|s)(\d{1,2})$`)
)

// normalize turns a file name stem into space-separated words: dots and
// underscores are separators, bracketed tags are dropped, parentheses are
// removed but their contents (a year) are kept.
func normalize(stem string) string {
	s := bracketed.ReplaceAllStringFunc(stem, func(tag string) string {
		if _, ok := asYear(tag[1 : len(tag)-1]); ok {
			return " " + tag[1:len(tag)-1] + " " // "[2010]" is a year written with brackets
		}
		return " "
	})
	s = separators.Replace(s)
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

// parseEpisode recognises the usual ways of numbering an episode. s is the
// normalized file name; dirs are the folders above it, outermost first.
func parseEpisode(s string, dirs []string) (Meta, bool) {
	season, episode, start, end, ok := findEpisode(s)
	folderSeason, inSeasonFolder := seasonOf(dirs)
	if !ok && inSeasonFolder {
		// "05 - Title" inside "Season 2": the folder supplies the season.
		if m := leadingNumber.FindStringSubmatch(s); m != nil {
			episode, _ = strconv.Atoi(m[1])
			start, end, ok = 0, len(m[0]), true
		}
	}
	if !ok {
		return Meta{}, false
	}
	if season == 0 {
		season = folderSeason
	}

	title, _ := cutAtJunk(moreEpisodes.ReplaceAllString(strings.TrimSpace(s[end:]), ""))
	showText, year := cutAtJunk(s[:start])
	show := strings.TrimSpace(trimDashes(showText))
	if show == "" {
		show, year = showFromFolders(dirs)
	}
	if title = trimDashes(title); title == "" {
		title = episodeLabel(season, episode)
	}
	return Meta{Title: title, Year: year, Show: show, Season: season, Episode: episode}, true
}

// findEpisode returns the season and episode numbers and where the marker sits in
// s. The season is 0 when the marker has none ("Episode 5").
func findEpisode(s string) (season, episode, start, end int, ok bool) {
	for _, re := range []*regexp.Regexp{seasonEpisode, wordsEpisode, crossEpisode} {
		if m := re.FindStringSubmatchIndex(s); m != nil {
			season, _ = strconv.Atoi(s[m[2]:m[3]])
			episode, _ = strconv.Atoi(s[m[4]:m[5]])
			return season, episode, m[0], m[1], true
		}
	}
	if m := loneEpisode.FindStringSubmatchIndex(s); m != nil {
		episode, _ = strconv.Atoi(s[m[2]:m[3]])
		return 0, episode, m[0], m[1], true
	}
	return 0, 0, 0, 0, false
}

// seasonOf returns the season named by the folder directly above the file.
func seasonOf(dirs []string) (int, bool) {
	if len(dirs) == 0 {
		return 0, false
	}
	if m := seasonFolder.FindStringSubmatch(strings.TrimSpace(normalize(dirs[len(dirs)-1]))); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, true
	}
	return 0, false
}

// showFromFolders names the series after the nearest folder that is not a season
// folder: "Breaking Bad (2008)/Season 1/…" gives "Breaking Bad" and 2008.
func showFromFolders(dirs []string) (string, int) {
	for i := len(dirs) - 1; i >= 0; i-- {
		name := normalize(dirs[i])
		if seasonFolder.MatchString(name) {
			continue
		}
		show, year := cutAtJunk(name)
		return strings.TrimSpace(trimDashes(show)), year
	}
	return "", 0
}

func episodeLabel(season, episode int) string {
	if season == 0 {
		return fmt.Sprintf("E%02d", episode)
	}
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

// parseMovie reads a title and year. Whatever the name has after them is release
// noise ("1080p BluRay x264") and is dropped.
func parseMovie(s, stem string) Meta {
	title, year := cutAtJunk(s)
	title = trimDashes(title)
	if title == "" {
		// Nothing but noise (a file called "1080p.mkv"): show the name rather than nothing.
		title = trimDashes(s)
	}
	if title == "" {
		title = strings.TrimSpace(stem)
	}
	return Meta{Title: title, Year: year}
}

// cutAtJunk returns the words of s that come before the release noise, and the
// year found there. The year is the last year-shaped word before the first noise
// word, so "Blade Runner 2049 2017 1080p" is Blade Runner 2049 from 2017; a name
// that is only a year ("2012") is a title, not a year.
func cutAtJunk(s string) (title string, year int) {
	words := strings.Fields(s)
	end := len(words)
	for i, w := range words {
		if isNoise(w) {
			end = i
			break
		}
	}
	cut := end
	for i := end - 1; i > 0; i-- {
		if y, ok := asYear(words[i]); ok {
			cut, year = i, y
			break
		}
	}
	return strings.Join(words[:cut], " "), year
}

func asYear(w string) (int, bool) {
	if len(w) != 4 {
		return 0, false
	}
	n, err := strconv.Atoi(w)
	if err != nil || n < 1900 || n > 2099 {
		return 0, false
	}
	return n, true
}

var noiseWord = regexp.MustCompile(`(?i)^(?:\d{3,4}[pi]|[48]k|u?hd|fhd|` +
	`blu-?ray|bd-?rip|br-?rip|bd|web|web-?dl|web-?rip|hd-?rip|dvd-?rip|dvd|dvdscr|hdtv|pdtv|remux|cam|hdcam|telesync|` +
	`extended|unrated|remastered|repack|proper|internal|imax|hdr|hdr10|` +
	`x26[45]|h26[45]|hevc|avc|xvid|divx|av1|10-?bit|8-?bit|` +
	`aac|ac3|eac3|dts|dts-?hd|truehd|atmos|(?:dd|ddp)\d*|flac|mp3)$`)

func isNoise(w string) bool { return noiseWord.MatchString(w) }

// trimDashes removes the separators left at the ends of a cut piece of a name:
// "Show - " becomes "Show".
func trimDashes(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '–' || r == '—' || r == ':'
	})
}
