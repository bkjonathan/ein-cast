package library

import (
	"testing"
	"time"

	"eincast/internal/domain/media"
)

func TestSameFiles(t *testing.T) {
	mod := time.Unix(1_700_000_000, 0)
	base := func() []media.Item {
		return []media.Item{{ID: "a", Size: 1, ModTime: mod, Meta: media.Meta{Title: "A"}}}
	}
	tests := []struct {
		name   string
		change func([]media.Item)
		same   bool
	}{
		{"nothing changed", func([]media.Item) {}, true},
		{"size", func(i []media.Item) { i[0].Size = 2 }, false},
		{"a .nfo changed the title", func(i []media.Item) { i[0].Meta.Title = "B" }, false},
		{"a .nfo added a plot", func(i []media.Item) { i[0].Meta.Plot = "p" }, false},
		{"a poster appeared", func(i []media.Item) { i[0].Artwork = media.Artwork{Path: "/p.jpg", Size: 1, ModTime: mod} }, false},
		{"a subtitle appeared", func(i []media.Item) { i[0].Subtitles = []media.Subtitle{{RelPath: "a.srt"}} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := base()
			tt.change(b)
			if got := sameFiles(base(), b); got != tt.same {
				t.Errorf("sameFiles() = %v, want %v", got, tt.same)
			}
		})
	}
}
