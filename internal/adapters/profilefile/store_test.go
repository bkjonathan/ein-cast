package profilefile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

func sample() []app.ProfileRecord {
	return []app.ProfileRecord{
		{Profile: profile.Everyone(), PINHash: []byte{1, 2, 3, 250}, PINSalt: []byte{9, 8, 7}, Version: 4},
		{Profile: profile.Profile{
			ID: "k1", Name: "Kids", Color: "green",
			Libraries: []media.LibraryID{"cartoons", "music"}, Kinds: []media.Kind{media.KindVideo},
			HiddenFolders: []string{"18*"},
		}, Version: 1},
	}
}

func TestSaveThenLoad(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "EinCast"))
	if err := s.Save(sample()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, sample()) {
		t.Fatalf("Load() = %+v, %v\nwant %+v", got, err, sample())
	}
}

func TestLoad_NothingSavedIsNotAFailure(t *testing.T) {
	if _, err := New(t.TempDir()).Load(); !errors.Is(err, app.ErrNoProfiles) {
		t.Fatalf("Load() error = %v, want ErrNoProfiles", err)
	}
}

func TestLoad_ACorruptFileIsMovedAsideNotDestroyed(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o600)
	if _, err := New(dir).Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load() error = %v, want ErrCorrupt", err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName+".bad")); err != nil {
		t.Errorf("the damaged file was not kept: %v", err)
	}
	if _, err := New(dir).Load(); !errors.Is(err, app.ErrNoProfiles) {
		t.Errorf("after moving it aside the next load should find nothing: %v", err)
	}
}

func TestLoad_ToleratesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	data := `{"version":9,"future":true,"profiles":[{"id":"a","name":"A","color":"blue","allLibraries":true,"pinVersion":2,"avatar":"x"}]}`
	_ = os.WriteFile(filepath.Join(dir, fileName), []byte(data), 0o600)
	got, err := New(dir).Load()
	if err != nil || len(got) != 1 || got[0].Profile.Name != "A" || got[0].Version != 2 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestSave_StoresTheHashNotAPIN(t *testing.T) {
	dir := t.TempDir()
	_ = New(dir).Save(sample())
	data, _ := os.ReadFile(filepath.Join(dir, fileName))
	if strings.Contains(string(data), `"pin"`) {
		t.Errorf("the file has a plain pin field:\n%s", data)
	}
}

func TestSave_IsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are not modelled the same way on Windows")
	}
	dir := t.TempDir()
	_ = New(dir).Save(sample())
	info, _ := os.Stat(filepath.Join(dir, fileName))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600: the file holds PIN hashes", info.Mode().Perm())
	}
}
