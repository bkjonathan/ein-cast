package media

import (
	"reflect"
	"testing"
)

func TestIsSubtitleFile(t *testing.T) {
	for name, want := range map[string]bool{
		"a.srt": true, "a.SRT": true, "a.en.vtt": true,
		"a.sub": false, "a.idx": false, "a.mkv": false, "srt": false, "": false,
	} {
		if got := IsSubtitleFile(name); got != want {
			t.Errorf("IsSubtitleFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMatchSubtitles(t *testing.T) {
	folder := []string{
		"Movie.mkv", "Movie.srt", "Movie.en.srt", "Movie.pt-BR.forced.srt", "movie.FR.vtt",
		"Movie2.srt", "Movie Extras.srt", "Other.srt", "Movie.sub", "Movie.commentary.srt",
	}
	got := MatchSubtitles("Movie.mkv", folder)
	want := []SubtitleMatch{
		{"Movie.commentary.srt", "", "Subtitles (commentary)"},
		{"Movie.en.srt", "en", "English"},
		{"Movie.pt-BR.forced.srt", "pt-BR", "Portuguese (forced)"},
		{"Movie.srt", "", "Subtitles"},
		{"movie.FR.vtt", "fr", "French"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MatchSubtitles() =\n %+v\nwant\n %+v", got, want)
	}
}

func TestMatchSubtitles_NothingMatches(t *testing.T) {
	if got := MatchSubtitles("a.mp4", []string{"b.srt", "a.txt"}); len(got) != 0 {
		t.Fatalf("MatchSubtitles() = %+v, want none", got)
	}
	if got := MatchSubtitles("a.mp4", nil); len(got) != 0 {
		t.Fatalf("MatchSubtitles(nil) = %+v, want none", got)
	}
}

func TestMatchSubtitles_UnknownLanguageCodeIsShownUppercase(t *testing.T) {
	got := MatchSubtitles("a.mp4", []string{"a.xyz.srt"})
	if len(got) != 1 || got[0].Lang != "xyz" || got[0].Label != "XYZ" {
		t.Fatalf("MatchSubtitles() = %+v", got)
	}
}
