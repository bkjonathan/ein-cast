package fswatch

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// settle is how long a test waits to be sure that no signal is coming. The
// real events arrive within milliseconds.
const settle = 300 * time.Millisecond

func newWatch(t *testing.T, root string) (<-chan struct{}, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := New(slog.New(slog.DiscardHandler)).Watch(ctx, root)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	return ch, cancel
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expectSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed while waiting for a signal for %s", what)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no signal for %s", what)
	}
}

func expectQuiet(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("unexpected signal for %s", what)
		}
		t.Fatalf("channel closed unexpectedly during %s", what)
	case <-time.After(settle):
	}
}

// drain discards signals caused by test setup.
func drain(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		case <-time.After(settle):
			return
		}
	}
}

func TestWatch_SignalsWhenAMediaFileAppears(t *testing.T) {
	root := t.TempDir()
	ch, _ := newWatch(t, root)

	write(t, filepath.Join(root, "movie.mkv"), "x")
	expectSignal(t, ch, "a new video")
}

func TestWatch_SignalsWhenAMediaFileIsRemovedOrRenamed(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.mp4"), "x")
	write(t, filepath.Join(root, "b.mp3"), "x")
	ch, _ := newWatch(t, root)

	if err := os.Remove(filepath.Join(root, "a.mp4")); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, ch, "a removed video")
	drain(ch)

	if err := os.Rename(filepath.Join(root, "b.mp3"), filepath.Join(root, "c.mp3")); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, ch, "a renamed song")
}

func TestWatch_SignalsWhenAFileGrows(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "download.mp4")
	write(t, path, "x")
	ch, _ := newWatch(t, root)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("more"); err != nil {
		t.Fatal(err)
	}
	// Windows reports a change to an open file only once the handle is flushed
	// and closed, so a download still in progress is noticed when its writer
	// next closes it, not on every write.
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, ch, "an appended write")
}

func TestWatch_IgnoresWhatTheLibraryIgnores(t *testing.T) {
	root := t.TempDir()
	ch, _ := newWatch(t, root)

	write(t, filepath.Join(root, "notes.txt"), "x")      // not media
	write(t, filepath.Join(root, ".DS_Store"), "x")      // hidden
	write(t, filepath.Join(root, "._movie.mp4"), "x")    // AppleDouble
	write(t, filepath.Join(root, "Thumbs.db"), "x")      // system
	write(t, filepath.Join(root, "movie.mp4.part"), "x") // unfinished download
	expectQuiet(t, ch, "files the library ignores")
}

func TestWatch_IgnoresChangesOnlyToAttributes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a permission change as a write")
	}
	root := t.TempDir()
	path := filepath.Join(root, "a.mp4")
	write(t, path, "x")
	ch, _ := newWatch(t, root)

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	expectQuiet(t, ch, "a chmod")
}

func TestWatch_FollowsNewSubfolders(t *testing.T) {
	root := t.TempDir()
	ch, _ := newWatch(t, root)

	sub := filepath.Join(root, "season 1")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, ch, "a new folder")
	drain(ch)

	write(t, filepath.Join(sub, "e01.mkv"), "x") // inside the folder we only just met
	expectSignal(t, ch, "a file in a new folder")
}

func TestWatch_WatchesFoldersThatAlreadyExist(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	ch, _ := newWatch(t, root)

	write(t, filepath.Join(deep, "x.mp4"), "x")
	expectSignal(t, ch, "a file three folders down")
}

func TestWatch_SignalsWhenAFolderIsRemoved(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "old")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	ch, _ := newWatch(t, root)

	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, ch, "a removed folder")
}

func TestWatch_DoesNotWatchHiddenFolders(t *testing.T) {
	root := t.TempDir()
	hidden := filepath.Join(root, ".cache")
	if err := os.Mkdir(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	ch, _ := newWatch(t, root)

	write(t, filepath.Join(hidden, "x.mp4"), "x")
	expectQuiet(t, ch, "a file in a hidden folder")
}

func TestWatch_ABurstIsOneSignal(t *testing.T) {
	root := t.TempDir()
	ch, _ := newWatch(t, root)

	for i := range 20 {
		write(t, filepath.Join(root, string(rune('a'+i))+".mp4"), "x")
	}
	time.Sleep(settle)
	got := 0
	for {
		select {
		case <-ch:
			got++
			continue
		default:
		}
		break
	}
	if got != 1 {
		t.Fatalf("%d signals pending after a burst, want exactly 1 (they coalesce)", got)
	}
}

func TestWatch_ClosesTheChannelWhenItsContextEnds(t *testing.T) {
	root := t.TempDir()
	ch, cancel := newWatch(t, root)

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("got a signal, want the channel closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed after cancel")
	}
}

func TestWatch_RejectsAMissingFolder(t *testing.T) {
	_, err := New(slog.New(slog.DiscardHandler)).Watch(context.Background(), filepath.Join(t.TempDir(), "gone"))
	if err == nil {
		t.Fatal("Watch() error = nil, want an error for a folder that does not exist")
	}
}

func TestWatch_SignalsWhenASubtitleAppears(t *testing.T) {
	root := t.TempDir()
	ch, _ := newWatch(t, root)

	write(t, filepath.Join(root, "movie.en.srt"), "x")
	expectSignal(t, ch, "a new subtitle file")
}
