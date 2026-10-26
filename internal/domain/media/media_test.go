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
	lib := NewLibraryID("/media/films")

	t.Run("is 16 hex chars", func(t *testing.T) {
		id := NewID(lib, "movies/a.mp4")
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
		if NewID(lib, "a/b.mp4") != NewID(lib, "a/b.mp4") {
			t.Fatal("same path produced different IDs")
		}
	})

	t.Run("known value", func(t *testing.T) {
		// First 8 bytes of sha256("lib1|a.mp4"); pins the scheme so a change would
		// break every bookmarked URL and must be deliberate.
		if got, want := NewID("lib1", "a.mp4"), ID("88dde587a9371bd0"); got != want {
			t.Fatalf("NewID(lib1, a.mp4) = %q, want %q", got, want)
		}
	})

	t.Run("differs per path", func(t *testing.T) {
		if NewID(lib, "a.mp4") == NewID(lib, "b.mp4") {
			t.Fatal("different paths produced the same ID")
		}
	})

	t.Run("differs per library: the same file name in two libraries is two items", func(t *testing.T) {
		if NewID("lib1", "a.mp4") == NewID("lib2", "a.mp4") {
			t.Fatal("two libraries produced the same ID for the same relative path")
		}
	})
}
