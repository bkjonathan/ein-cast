package media

import (
	"errors"
	"testing"
)

func TestDetectKind(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    Kind
		wantErr error
	}{
		{"mp4", "movie.mp4", KindVideo, nil},
		{"mkv", "movie.mkv", KindVideo, nil},
		{"uppercase extension", "MOVIE.MKV", KindVideo, nil},
		{"mixed case", "clip.Mp4", KindVideo, nil},
		{"audio mp3", "song.mp3", KindAudio, nil},
		{"audio flac", "song.flac", KindAudio, nil},
		{"image jpg", "photo.jpg", KindImage, nil},
		{"image jpeg", "photo.JPEG", KindImage, nil},
		{"nested slash path", "shows/s01/e01.mkv", KindVideo, nil},
		{"dots in name", "my.holiday.2024.mp4", KindVideo, nil},
		{"text file", "notes.txt", "", ErrUnsupported},
		{"no extension", "README", "", ErrUnsupported},
		{"hidden file without extension", ".DS_Store", "", ErrUnsupported},
		{"empty", "", "", ErrUnsupported},
		{"extension only inside dir name", "movie.mp4/readme", "", ErrUnsupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectKind(tt.file)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DetectKind(%q) error = %v, want %v", tt.file, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("DetectKind(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestNewID(t *testing.T) {
	t.Run("is 16 hex chars", func(t *testing.T) {
		id := NewID("movies/a.mp4")
		if len(id) != 16 {
			t.Fatalf("len(%q) = %d, want 16", id, len(id))
		}
		for _, c := range id {
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
				t.Fatalf("%q contains non-hex rune %q", id, c)
			}
		}
	})

	t.Run("is stable", func(t *testing.T) {
		if NewID("a/b.mp4") != NewID("a/b.mp4") {
			t.Fatal("same path produced different IDs")
		}
	})

	t.Run("known value", func(t *testing.T) {
		// First 8 bytes of sha256("a.mp4") (checked with shasum); pins the scheme so a change
		// would break every bookmarked URL and must be deliberate.
		if got, want := NewID("a.mp4"), ID("e6d48eabd560bc60"); got != want {
			t.Fatalf("NewID(a.mp4) = %q, want %q", got, want)
		}
	})

	t.Run("differs per path", func(t *testing.T) {
		if NewID("a.mp4") == NewID("b.mp4") {
			t.Fatal("different paths produced the same ID")
		}
	})
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name  string
		items []Item
		want  Stats
	}{
		{"empty", nil, Stats{Counts: map[Kind]int{}}},
		{
			"mixed",
			[]Item{
				{Kind: KindVideo, Size: 100},
				{Kind: KindVideo, Size: 50},
				{Kind: KindAudio, Size: 5},
			},
			Stats{Total: 3, TotalSize: 155, Counts: map[Kind]int{KindVideo: 2, KindAudio: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.items)
			if got.Total != tt.want.Total || got.TotalSize != tt.want.TotalSize {
				t.Fatalf("Summarize() = %+v, want %+v", got, tt.want)
			}
			for _, k := range []Kind{KindVideo, KindAudio, KindImage} {
				if got.Counts[k] != tt.want.Counts[k] {
					t.Errorf("Counts[%s] = %d, want %d", k, got.Counts[k], tt.want.Counts[k])
				}
			}
		})
	}
}

func TestIgnored(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{".hidden", true},
		{".DS_Store", true},
		{"._clip.mp4", true},
		{"Thumbs.db", true},
		{"DESKTOP.INI", true},
		{"$RECYCLE.BIN", true},
		{"System Volume Information", true},
		{"__MACOSX", true},
		{"@eaDir", true},
		{"movie.mkv", false},
		{"Season 1", false},
		{"my.hidden.movie.mkv", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Ignored(tt.name); got != tt.want {
				t.Fatalf("Ignored(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
