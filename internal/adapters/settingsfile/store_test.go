package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"eincast/internal/app"
)

func TestLoad_MissingFileIsNotAFailure(t *testing.T) {
	_, err := New(t.TempDir()).Load()
	if !errors.Is(err, app.ErrNoSettings) {
		t.Fatalf("Load() error = %v, want ErrNoSettings", err)
	}
}

func TestLoad_MissingDirectoryIsNotAFailure(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "does", "not", "exist")).Load()
	if !errors.Is(err, app.ErrNoSettings) {
		t.Fatalf("Load() error = %v, want ErrNoSettings", err)
	}
}

func TestSaveThenLoad(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "EinCast")) // the directory does not exist yet
	want := app.Settings{Port: 9100, Name: "Living Room", LibraryRoot: "/media/films", RequireToken: true}

	if err := s.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load()
	if err != nil || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, err, want)
	}
}

func TestSave_ReplacesAndLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	for i := range 5 {
		if err := s.Save(app.Settings{Port: 9000 + i, Name: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.Load()
	if got.Port != 9004 {
		t.Errorf("Port = %d, want the last save", got.Port)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory has %v, want only settings.json", names)
	}
}

func TestSave_FileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	dir := filepath.Join(t.TempDir(), "EinCast")
	if err := New(dir).Save(app.Settings{Port: 9000, Name: "x"}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(dir, "settings.json"): 0o600,
		dir:                                 0o700,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", filepath.Base(path), got, want)
		}
	}
}

func TestLoad_CorruptFileIsMovedAsideNotDestroyed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"port": 9000, "name": `), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := New(dir).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load() error = %v, want ErrCorrupt", err)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Error("the corrupt file is still in place; a later Save would overwrite the evidence")
	}
	bad, readErr := os.ReadFile(path + ".bad")
	if readErr != nil || !strings.Contains(string(bad), `"port": 9000`) {
		t.Errorf("settings.json.bad = %q, %v; want the original bytes preserved", bad, readErr)
	}

	// After falling back to defaults the next Save works and the next Load is clean.
	if err := New(dir).Save(app.Settings{Port: 9001, Name: "x"}); err != nil {
		t.Fatal(err)
	}
	if got, err := New(dir).Load(); err != nil || got.Port != 9001 {
		t.Errorf("Load() after recovery = %+v, %v", got, err)
	}
}

func TestLoad_EmptyFileIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), nil, 0o600)
	if _, err := New(dir).Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load() error = %v, want ErrCorrupt", err)
	}
}

func TestLoad_ToleratesUnknownAndMissingFields(t *testing.T) {
	dir := t.TempDir()
	// Written by some future version: extra fields, and no libraryRoot.
	data := `{"version": 2, "port": 9200, "name": "Den", "requireToken": true, "theme": "dark", "nested": {"a": 1}}`
	_ = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0o600)

	got, err := New(dir).Load()
	want := app.Settings{Port: 9200, Name: "Den", RequireToken: true}
	if err != nil || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, err, want)
	}
}

func TestSave_NeverWritesSecrets(t *testing.T) {
	dir := t.TempDir()
	_ = New(dir).Save(app.Settings{Port: 9000, Name: "x", RequireToken: true})
	data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	// The on-disk shape has no field that could carry a token.
	if strings.Count(strings.ToLower(string(data)), "token") != 1 {
		t.Errorf("only the requireToken flag may mention tokens:\n%s", data)
	}
}

func TestSave_FailsCleanlyWhenTheDirectoryCannotBeCreated(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)

	err := New(filepath.Join(blocker, "sub")).Save(app.Settings{Port: 9000, Name: "x"})
	if err == nil {
		t.Fatal("Save() under a regular file succeeded")
	}
}
