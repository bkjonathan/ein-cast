package fsscan

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"eincast/internal/domain/media"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newScanner() *Scanner { return New(slog.New(slog.DiscardHandler)) }

// scan scans root as a library named after it, the way the app does.
func scan(ctx context.Context, root string) ([]media.Item, error) {
	return newScanner().Scan(ctx, media.NewLibrary(root))
}

func relPaths(items []media.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.RelPath
	}
	slices.Sort(out)
	return out
}

func TestScan_FindsMediaAndSkipsTheRest(t *testing.T) {
	root := t.TempDir()
	write(t, root, "movie.mp4", "12345")
	write(t, root, "shows/s01/e01.MKV", "x")
	write(t, root, "music/song.flac", "x")
	write(t, root, "photos/a.jpg", "x")

	write(t, root, "notes.txt", "unsupported")
	write(t, root, ".hidden.mp4", "hidden file")
	write(t, root, "._movie.mp4", "appledouble")
	write(t, root, ".trash/old.mp4", "hidden dir")
	write(t, root, "@eaDir/thumb.jpg", "nas metadata")
	write(t, root, "__MACOSX/x.mp4", "archive junk")
	write(t, root, "Thumbs.db", "windows")

	got, err := scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	want := []string{"movie.mp4", "music/song.flac", "photos/a.jpg", "shows/s01/e01.MKV"}
	if g := relPaths(got); !slices.Equal(g, want) {
		t.Fatalf("scanned = %v, want %v", g, want)
	}
}

func TestScan_ItemFields(t *testing.T) {
	root := t.TempDir()
	write(t, root, "tv/ep1.mkv", "12345")

	got, err := scan(context.Background(), root)
	if err != nil || len(got) != 1 {
		t.Fatalf("Scan() = %v, %v", got, err)
	}
	it := got[0]

	if it.RelPath != "tv/ep1.mkv" || it.Name != "ep1.mkv" {
		t.Errorf("names: rel=%q name=%q", it.RelPath, it.Name)
	}
	lib := media.NewLibrary(root)
	if it.ID != media.NewID(lib.ID, "tv/ep1.mkv") {
		t.Errorf("ID = %q, want hash of the library and the slash-separated relative path", it.ID)
	}
	if it.LibraryID != lib.ID {
		t.Errorf("LibraryID = %q, want %q", it.LibraryID, lib.ID)
	}
	if abs, _ := filepath.Abs(root); it.Root != abs {
		t.Errorf("Root = %q, want %q: every item remembers where its library is", it.Root, abs)
	}
	if it.Kind != media.KindVideo || it.Size != 5 || it.ModTime.IsZero() {
		t.Errorf("kind=%q size=%d modtime=%v", it.Kind, it.Size, it.ModTime)
	}
	if !filepath.IsAbs(it.Path) || filepath.Base(it.Path) != "ep1.mkv" {
		t.Errorf("Path = %q, want absolute path to the file", it.Path)
	}
}

func TestScan_SkipsSymlinks(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, "secret.mp4", "outside the library")
	root := t.TempDir()
	write(t, root, "real.mp4", "x")

	if err := os.Symlink(filepath.Join(outside, "secret.mp4"), filepath.Join(root, "link.mp4")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got, err := scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if g := relPaths(got); !slices.Equal(g, []string{"real.mp4"}) {
		t.Fatalf("scanned = %v, want only real.mp4 (symlinks must not be followed)", g)
	}
}

func TestScan_EmptyFolder(t *testing.T) {
	got, err := scan(context.Background(), t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan(empty) = %v, %v; want no items and no error", got, err)
	}
}

func TestScan_BadRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file.mp4")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, root string }{
		{"missing", filepath.Join(t.TempDir(), "nope")},
		{"is a file", file},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := scan(context.Background(), tt.root); err == nil {
				t.Fatal("Scan() error = nil, want error")
			}
		})
	}
}

func TestScan_CancelledContext(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.mp4", "x")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := scan(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan() error = %v, want context.Canceled", err)
	}
}

func TestScan_AttachesSubtitlesToTheirVideo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "tv/Show S01E01.mkv", "v")
	write(t, root, "tv/Show S01E01.en.srt", "s")
	write(t, root, "tv/Show S01E01.fr.vtt", "s")
	write(t, root, "tv/Show S01E02.mkv", "v")
	write(t, root, "tv/Show S01E02.srt", "s")
	write(t, root, "tv/Stranger.srt", "belongs to no video here")
	write(t, root, "music/song.mp3", "a")
	write(t, root, "music/song.srt", "lyrics are not subtitles of audio")
	write(t, root, "other/Show S01E01.en.srt", "another folder")

	got, err := scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if g := relPaths(got); !slices.Equal(g, []string{"music/song.mp3", "tv/Show S01E01.mkv", "tv/Show S01E02.mkv"}) {
		t.Fatalf("subtitle files must not become items: %v", g)
	}
	byRel := map[string]media.Item{}
	for _, it := range got {
		byRel[it.RelPath] = it
	}

	ep1 := byRel["tv/Show S01E01.mkv"].Subtitles
	if len(ep1) != 2 || ep1[0].RelPath != "tv/Show S01E01.en.srt" || ep1[0].Label != "English" || ep1[1].Lang != "fr" {
		t.Errorf("episode 1 subtitles = %+v", ep1)
	}
	if want := filepath.Join(root, "tv", "Show S01E01.en.srt"); ep1[0].Path != want {
		t.Errorf("subtitle path = %q, want %q", ep1[0].Path, want)
	}
	if ep2 := byRel["tv/Show S01E02.mkv"].Subtitles; len(ep2) != 1 || ep2[0].Label != "Subtitles" {
		t.Errorf("episode 2 subtitles = %+v", ep2)
	}
	if song := byRel["music/song.mp3"].Subtitles; len(song) != 0 {
		t.Errorf("audio got subtitles: %+v", song)
	}
}

func TestScan_SkipsHiddenSubtitleFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.mp4", "v")
	write(t, root, ".a.srt", "hidden")
	write(t, root, "._a.srt", "appledouble")

	got, err := scan(context.Background(), root)
	if err != nil || len(got) != 1 {
		t.Fatalf("Scan() = %v, %v", got, err)
	}
	if len(got[0].Subtitles) != 0 {
		t.Errorf("subtitles = %+v, want none", got[0].Subtitles)
	}
}

func TestScan_TheSameRelativePathInTwoLibrariesIsTwoItems(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write(t, a, "shows/ep1.mkv", "x")
	write(t, b, "shows/ep1.mkv", "y")

	ga, err := scan(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	gb, err := scan(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(ga) != 1 || len(gb) != 1 || ga[0].ID == gb[0].ID {
		t.Fatalf("ids: %v vs %v; want different IDs for the same relative path", ga, gb)
	}
	if ga[0].RelPath != gb[0].RelPath {
		t.Error("the relative paths should be equal; only the library differs")
	}
}
