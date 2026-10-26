package media

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewLibraryID(t *testing.T) {
	a := NewLibraryID("/media/films")
	if a != NewLibraryID("/media/films/") || a != NewLibraryID("/media//films") {
		t.Error("one folder written three ways gave different IDs")
	}
	if a == NewLibraryID("/media/music") {
		t.Error("different folders share an ID")
	}
	if len(a) != 16 {
		t.Errorf("len(%q) = %d, want 16", a, len(a))
	}
	// Pinned: removing and re-adding a library must bring its item IDs back, and
	// an upgrade must not renumber them.
	if runtime.GOOS == "windows" {
		return // the pinned value is for the slash form; Windows cleans to backslashes
	}
	if got, want := NewLibraryID("/media/films"), LibraryID("8c4b1834464271b1"); got != want {
		t.Errorf("NewLibraryID(/media/films) = %q, want %q", got, want)
	}
}

func TestNewLibrary(t *testing.T) {
	l := NewLibrary("/media/Home Videos/")
	if l.Root != filepath.Clean("/media/Home Videos") || l.Name != "Home Videos" || l.ID != NewLibraryID("/media/Home Videos") {
		t.Fatalf("NewLibrary() = %+v", l)
	}
}

func TestDefaultLibraryName(t *testing.T) {
	tests := map[string]string{
		"/media/films":  "films",
		"/media/films/": "films",
		"/":             "/",
		"films":         "films",
	}
	for root, want := range tests {
		if got := DefaultLibraryName(root); got != want {
			t.Errorf("DefaultLibraryName(%q) = %q, want %q", root, got, want)
		}
	}
}

func TestOverlaps(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"/media/films", "/media/films", true},
		{"/media/films", "/media/films/", true},
		{"/media/films", "/MEDIA/Films", true},
		{"/media/films", "/media/films/2020", true},
		{"/media/films/2020", "/media/films", true},
		{"/media/films", "/media/films-old", false},
		{"/media/films", "/media/music", false},
		{"/media", "/mediaserver", false},
		{"/a", "/b/a", false},
	}
	for _, tt := range tests {
		if got := Overlaps(tt.a, tt.b); got != tt.want {
			t.Errorf("Overlaps(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
