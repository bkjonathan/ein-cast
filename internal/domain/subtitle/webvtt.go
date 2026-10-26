// Package subtitle converts subtitle text into WebVTT, the one format browsers
// show natively in a <track>. It is pure: bytes in, bytes out.
package subtitle

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// ToWebVTT turns the text of an .srt or .vtt file into WebVTT. text must already
// be UTF-8 (see Decode). The conversion is deliberately forgiving: subtitle files
// in the wild have mixed line endings, a byte-order mark, numbering gaps and
// stray blank lines, and a browser refuses a whole track for one bad header.
func ToWebVTT(text string) string {
	text = strings.TrimPrefix(text, "\xef\xbb\xbf") // UTF-8 byte-order mark
	text = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)

	if strings.HasPrefix(text, "WEBVTT") {
		return text // already WebVTT
	}

	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, block := range strings.Split(text, "\n\n") {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		// Drop the cue number, which is the line before the timing line.
		timing := -1
		for i, l := range lines {
			if strings.Contains(l, "-->") {
				timing = i
				break
			}
		}
		if timing < 0 {
			continue // a number with no timing, or stray text
		}
		b.WriteString(fixTiming(lines[timing]))
		b.WriteByte('\n')
		for _, l := range lines[timing+1:] {
			b.WriteString(l)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// fixTiming rewrites "00:01:02,500 --> 00:01:04,000 X1:10" as WebVTT wants it:
// a dot before the milliseconds, and hours present. SRT's position hints (after
// the end time) are dropped; they mean something different in WebVTT.
func fixTiming(line string) string {
	start, rest, _ := strings.Cut(line, "-->")
	end := strings.Fields(rest)
	endTime := ""
	if len(end) > 0 {
		endTime = end[0]
	}
	return vttTime(strings.TrimSpace(start)) + " --> " + vttTime(endTime)
}

func vttTime(t string) string {
	t = strings.Replace(t, ",", ".", 1)
	// "01:02.500" (no hours) is valid WebVTT; "1:02:03.500" with a one-digit hour is not.
	if parts := strings.Split(t, ":"); len(parts) == 3 && len(parts[0]) == 1 {
		t = "0" + t
	}
	return t
}

// Decode turns the bytes of a subtitle file into UTF-8 text. It understands a
// UTF-8 or UTF-16 byte-order mark, accepts plain UTF-8, and otherwise assumes
// the legacy single-byte Windows encoding most older .srt files use.
func Decode(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return decodeUTF16(raw[2:], false)
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return decodeUTF16(raw[2:], true)
	case utf8.Valid(raw):
		return string(raw)
	default:
		return decodeWindows1252(raw)
	}
}

func decodeUTF16(b []byte, bigEndian bool) string {
	runes := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		var u uint16
		if bigEndian {
			u = uint16(b[i])<<8 | uint16(b[i+1])
		} else {
			u = uint16(b[i+1])<<8 | uint16(b[i])
		}
		// Surrogate pairs: combine a high and a low half.
		if u >= 0xD800 && u < 0xDC00 && i+3 < len(b) {
			var lo uint16
			if bigEndian {
				lo = uint16(b[i+2])<<8 | uint16(b[i+3])
			} else {
				lo = uint16(b[i+3])<<8 | uint16(b[i+2])
			}
			if lo >= 0xDC00 && lo < 0xE000 {
				runes = append(runes, rune(u-0xD800)<<10+rune(lo-0xDC00)+0x10000)
				i += 2
				continue
			}
		}
		runes = append(runes, rune(u))
	}
	return string(runes)
}

// cp1252High maps bytes 0x80-0x9F, where Windows-1252 differs from Latin-1.
var cp1252High = [32]rune{
	0x20AC, 0x81, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x8D, 0x017D, 0x8F,
	0x90, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x9D, 0x017E, 0x0178,
}

func decodeWindows1252(b []byte) string {
	runes := make([]rune, len(b))
	for i, c := range b {
		if c >= 0x80 && c <= 0x9F {
			runes[i] = cp1252High[c-0x80]
		} else {
			runes[i] = rune(c)
		}
	}
	return string(runes)
}
