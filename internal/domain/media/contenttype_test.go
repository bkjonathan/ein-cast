package media

import "testing"

func TestContentType(t *testing.T) {
	tests := map[string]string{
		"a.mkv":  "video/x-matroska",
		"A.MKV":  "video/x-matroska",
		"a.mp4":  "video/mp4",
		"a.ts":   "video/mp2t",
		"a.flac": "audio/flac",
		"a.mp3":  "audio/mpeg",
		"a.heic": "image/heic",
		"a.png":  "image/png",
		"a.zzzz": "application/octet-stream",
		"noext":  "application/octet-stream",
	}
	for name, want := range tests {
		if got := ContentType(name); got != want {
			t.Errorf("ContentType(%q) = %q, want %q", name, got, want)
		}
	}
}
