package subtitle

import (
	"strings"
	"testing"
)

func TestToWebVTT(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			"basic srt",
			"1\n00:00:01,000 --> 00:00:02,500\nHello\n\n2\n00:00:03,000 --> 00:00:04,000\nWorld\nline two\n",
			"WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nHello\n\n00:00:03.000 --> 00:00:04.000\nWorld\nline two\n\n",
		},
		{
			"windows line endings and BOM",
			"\xef\xbb\xbf1\r\n00:00:01,000 --> 00:00:02,000\r\nHi\r\n\r\n",
			"WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n\n",
		},
		{
			"position hints are dropped",
			"1\n00:00:01,000 --> 00:00:02,000 X1:100 X2:200 Y1:10 Y2:20\nHi\n",
			"WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n\n",
		},
		{
			"one-digit hour",
			"1\n1:00:01,000 --> 1:00:02,000\nHi\n",
			"WEBVTT\n\n01:00:01.000 --> 01:00:02.000\nHi\n\n",
		},
		{
			"missing or odd numbering",
			"00:00:01,000 --> 00:00:02,000\nNo number\n\n7\n00:00:03,000 --> 00:00:04,000\nSeven\n",
			"WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nNo number\n\n00:00:03.000 --> 00:00:04.000\nSeven\n\n",
		},
		{
			"blocks without timing are skipped",
			"1\njust text\n\n2\n00:00:03,000 --> 00:00:04,000\nKept\n",
			"WEBVTT\n\n00:00:03.000 --> 00:00:04.000\nKept\n\n",
		},
		{"empty", "", "WEBVTT\n\n"},
		{"already webvtt", "WEBVTT\n\n00:01.000 --> 00:02.000\nHi\n", "WEBVTT\n\n00:01.000 --> 00:02.000\nHi\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToWebVTT(tt.in); got != tt.want {
				t.Fatalf("ToWebVTT() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestToWebVTT_KeepsTagsAndNonLatinText(t *testing.T) {
	got := ToWebVTT("1\n00:00:01,000 --> 00:00:02,000\n<i>မင်္ဂလာပါ</i>\n")
	if !strings.Contains(got, "<i>မင်္ဂလာပါ</i>") {
		t.Fatalf("text was altered: %q", got)
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"utf-8", []byte("café"), "café"},
		{"utf-8 with BOM keeps the BOM for ToWebVTT to strip", []byte("\xef\xbb\xbfhi"), "\xef\xbb\xbfhi"},
		{"utf-16 le", []byte{0xFF, 0xFE, 'h', 0, 0xE9, 0}, "hé"},
		{"utf-16 be", []byte{0xFE, 0xFF, 0, 'h', 0, 0xE9}, "hé"},
		{"utf-16 surrogate pair", []byte{0xFF, 0xFE, 0x3D, 0xD8, 0x00, 0xDE}, "😀"},
		{"windows-1252", []byte{'c', 'a', 'f', 0xE9, ' ', 0x93, 'x', 0x94, ' ', 0x80}, "café “x” €"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decode(tt.in); got != tt.want {
				t.Fatalf("Decode() = %q, want %q", got, tt.want)
			}
		})
	}
}
