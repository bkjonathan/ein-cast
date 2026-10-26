package ffmpeg

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeProgram(t *testing.T) string {
	t.Helper()
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestFinder(found string, chosen func() string) *finder {
	f := &finder{}
	f.init(slog.New(slog.DiscardHandler), nil)
	f.find = func() (string, error) {
		if found == "" {
			return "", errors.New("ffmpeg was not found")
		}
		return found, nil
	}
	f.chosen = chosen
	return f
}

func TestBinary_AChosenLocationWinsOverSearching(t *testing.T) {
	chosen := fakeProgram(t)
	f := newTestFinder("/usr/bin/ffmpeg", func() string { return chosen })
	if got, err := f.binary(); err != nil || got != chosen {
		t.Fatalf("binary() = %q, %v; want %q", got, err, chosen)
	}
}

func TestBinary_ABrokenChoiceIsAnErrorNotASilentSwitch(t *testing.T) {
	f := newTestFinder("/usr/bin/ffmpeg", func() string { return filepath.Join(t.TempDir(), "gone") })
	_, err := f.binary()
	if err == nil || !strings.Contains(err.Error(), "the ffmpeg you chose") {
		t.Fatalf("binary() error = %v, want it to say the chosen ffmpeg is the problem", err)
	}
	if f.Available() || f.Path() != "" {
		t.Error("a broken choice must not look available")
	}
}

func TestBinary_NoChoiceSearchesAndFollowsLaterChanges(t *testing.T) {
	choice := ""
	f := newTestFinder("/usr/bin/ffmpeg", func() string { return choice })
	if f.Path() != "/usr/bin/ffmpeg" {
		t.Fatalf("Path() = %q, want the searched location", f.Path())
	}
	choice = fakeProgram(t)
	if f.Path() != choice {
		t.Errorf("Path() = %q, want the new choice without a restart", f.Path())
	}
	choice = ""
	if f.Path() != "/usr/bin/ffmpeg" {
		t.Errorf("Path() = %q after clearing the choice, want the searched location again", f.Path())
	}
}

func TestBinary_NothingFound(t *testing.T) {
	f := newTestFinder("", nil)
	if f.Available() || f.Path() != "" {
		t.Error("nothing found must report unavailable")
	}
}

func TestCheckExecutable(t *testing.T) {
	dir := t.TempDir()
	if err := CheckExecutable(filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "no file") {
		t.Errorf("missing file: %v", err)
	}
	if err := CheckExecutable(dir); err == nil || !strings.Contains(err.Error(), "folder") {
		t.Errorf("folder: %v", err)
	}
	if err := CheckExecutable(fakeProgram(t)); err != nil {
		t.Errorf("an executable file: %v", err)
	}
}
