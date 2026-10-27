package resumefile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

func sample() map[profile.ID]map[media.ID]app.ResumePosition {
	return map[profile.ID]map[media.ID]app.ResumePosition{
		"everyone": {"abc": {T: 754, D: 3600, M: 1_700_000_000_123, S: 1_700_000_100_000}},
		"kids":     {"def": {T: 30, D: 600, M: 5, S: 6}, "ghi": {T: 1, D: 2, M: 3, S: 4}},
	}
}

func TestSaveThenLoad(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "EinCast"))
	if err := s.Save(sample()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, sample()) {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_NothingSavedIsEmptyNotAnError(t *testing.T) {
	got, err := New(t.TempDir()).Load()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_ACorruptFileIsMovedAside(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, fileName), []byte("{broken"), 0o600)
	if _, err := New(dir).Load(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName+".bad")); err != nil {
		t.Errorf("not kept: %v", err)
	}
}
