package fsscan

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"eincast/internal/domain/media"
)

func byRel(t *testing.T, items []media.Item, rel string) media.Item {
	t.Helper()
	for _, it := range items {
		if it.RelPath == rel {
			return it
		}
	}
	t.Fatalf("no item %q in %v", rel, relPaths(items))
	return media.Item{}
}

func TestScan_ReadsMetadataFromNames(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Movie.Name.2019.1080p.BluRay.mkv", "x")
	write(t, root, "Breaking Bad/Season 2/05 - Mandala.mkv", "x")
	write(t, root, "song.mp3", "x")

	items, err := scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if m := byRel(t, items, "Movie.Name.2019.1080p.BluRay.mkv").Meta; m.Title != "Movie Name" || m.Year != 2019 {
		t.Errorf("movie meta = %+v", m)
	}
	ep := byRel(t, items, "Breaking Bad/Season 2/05 - Mandala.mkv")
	if !ep.Meta.IsEpisode() || ep.Meta.Show != "Breaking Bad" || ep.Meta.Season != 2 || ep.Meta.Episode != 5 || ep.Meta.Title != "Mandala" {
		t.Errorf("episode meta = %+v", ep.Meta)
	}
	if ep.Name != "05 - Mandala.mkv" {
		t.Errorf("Name = %q: the original file name must survive", ep.Name)
	}
	if got := byRel(t, items, "song.mp3").Meta; got != (media.Meta{}) {
		t.Errorf("a song got meta %+v: only videos are read", got)
	}
}

func TestScan_NFOCorrectsTheName(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Heat.1995.mkv", "x")
	write(t, root, "Heat.1995.nfo", `<movie><title>Heat (Director's Cut)</title><year>1996</year><plot>A thief and a cop.</plot></movie>`)
	write(t, root, "Solo/Some File.mkv", "x")
	write(t, root, "Solo/movie.nfo", `<movie><title>The Solo Film</title></movie>`)
	write(t, root, "Many/A.mkv", "x")
	write(t, root, "Many/B.mkv", "x")
	write(t, root, "Many/movie.nfo", `<movie><title>Not Either Of Them</title></movie>`)
	write(t, root, "Bad/Bad Film.mkv", "x")
	write(t, root, "Bad/Bad Film.nfo", "this is not xml")

	items, err := scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if m := byRel(t, items, "Heat.1995.mkv").Meta; m.Title != "Heat (Director's Cut)" || m.Year != 1996 || m.Plot != "A thief and a cop." {
		t.Errorf("same-name nfo not applied: %+v", m)
	}
	if m := byRel(t, items, "Solo/Some File.mkv").Meta; m.Title != "The Solo Film" {
		t.Errorf("movie.nfo for a lone video not applied: %+v", m)
	}
	if m := byRel(t, items, "Many/A.mkv").Meta; m.Title != "A" {
		t.Errorf("movie.nfo was applied to a video in a folder of several: %+v", m)
	}
	if m := byRel(t, items, "Bad/Bad Film.mkv").Meta; m.Title != "Bad Film" {
		t.Errorf("an unusable nfo changed the title: %+v", m)
	}
}

func TestScan_OversizedNFOIsIgnored(t *testing.T) {
	root := t.TempDir()
	write(t, root, "A.mkv", "x")
	write(t, root, "A.nfo", "<movie><title>Big</title><plot>"+strings.Repeat("x", media.MaxNFOSize)+"</plot></movie>")
	items, _ := scan(context.Background(), root)
	if m := byRel(t, items, "A.mkv").Meta; m.Title != "A" || m.Plot != "" {
		t.Errorf("an oversized nfo was used: %+v", m)
	}
}

func TestScan_PostersBelongToVideosAndLeaveTheListOfPhotos(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Heat (1995)/Heat.mkv", "x")
	write(t, root, "Heat (1995)/poster.jpg", "pic")
	write(t, root, "Many/A.mkv", "x")
	write(t, root, "Many/B.mkv", "x")
	write(t, root, "Many/B-poster.jpg", "pic")
	write(t, root, "Many/poster.jpg", "belongs to neither")
	write(t, root, "Photos/holiday.jpg", "x")
	write(t, root, "Photos/poster.jpg", "a photo that happens to be called poster")

	items, err := scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	heat := byRel(t, items, "Heat (1995)/Heat.mkv")
	if !heat.Artwork.Present() || filepath.Base(heat.Artwork.Path) != "poster.jpg" || heat.Artwork.Size != 3 {
		t.Errorf("Heat artwork = %+v", heat.Artwork)
	}
	if byRel(t, items, "Many/A.mkv").Artwork.Present() {
		t.Error("a folder's poster went to a video in a folder of several")
	}
	if b := byRel(t, items, "Many/B.mkv"); filepath.Base(b.Artwork.Path) != "B-poster.jpg" {
		t.Errorf("B artwork = %+v, want its own poster", b.Artwork)
	}

	rels := relPaths(items)
	for _, gone := range []string{"Heat (1995)/poster.jpg", "Many/B-poster.jpg"} {
		if contains(rels, gone) {
			t.Errorf("%s is still listed as a photo although it is a poster", gone)
		}
	}
	for _, kept := range []string{"Many/poster.jpg", "Photos/holiday.jpg", "Photos/poster.jpg"} {
		if !contains(rels, kept) {
			t.Errorf("%s was hidden but belongs to no video", kept)
		}
	}
}

func TestScan_EpisodesShareTheSeriesPoster(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Fargo/poster.jpg", "series")
	write(t, root, "Fargo/Season 1/S01E01.mkv", "x")
	write(t, root, "Fargo/Season 1/S01E02.mkv", "x")
	write(t, root, "Fargo/Season 2/S02E01.mkv", "x")
	write(t, root, "Fargo/Season 2/poster.jpg", "season")

	items, err := scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if a := byRel(t, items, "Fargo/Season 1/S01E02.mkv").Artwork; filepath.ToSlash(a.Path) != filepath.ToSlash(filepath.Join(root, "Fargo", "poster.jpg")) {
		t.Errorf("season 1 artwork = %+v, want the series poster", a)
	}
	if a := byRel(t, items, "Fargo/Season 2/S02E01.mkv").Artwork; filepath.Base(filepath.Dir(a.Path)) != "Season 2" {
		t.Errorf("season 2 artwork = %+v, want the season's own poster", a)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// The Phase 15 goal in one folder: twenty episodes and ten films come out as ten
// films and one series with its seasons, from names alone.
func TestScan_TwentyEpisodesAndTenFilmsMakeTenFilmsAndOneSeries(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 10; i++ {
		write(t, root, "Films/"+filmName(i)+".2010.1080p.BluRay.x264-GRP.mkv", "x")
	}
	for season := 1; season <= 2; season++ {
		for ep := 1; ep <= 10; ep++ {
			write(t, root, "Show Name/Season "+itoa(season)+"/Show.Name.S0"+itoa(season)+"E"+pad(ep)+".720p.mkv", "x")
		}
	}

	items, err := scan(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	films, episodes := 0, 0
	seasons := map[int]int{}
	for _, it := range items {
		switch {
		case it.Meta.IsEpisode():
			episodes++
			seasons[it.Meta.Season]++
			if it.Meta.Show != "Show Name" {
				t.Errorf("%s: show = %q", it.RelPath, it.Meta.Show)
			}
		default:
			films++
			if it.Meta.Year != 2010 || it.Meta.Title == "" || strings.Contains(it.Meta.Title, "1080p") {
				t.Errorf("%s: meta = %+v", it.RelPath, it.Meta)
			}
		}
	}
	if films != 10 || episodes != 20 || seasons[1] != 10 || seasons[2] != 10 {
		t.Errorf("films=%d episodes=%d seasons=%v, want 10 films and 20 episodes in 2 seasons of 10", films, episodes, seasons)
	}
}

func filmName(i int) string { return "Film" + string(rune('A'+i-1)) + " Title" }
func itoa(n int) string     { return string(rune('0' + n)) }
func pad(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return "10"
}
