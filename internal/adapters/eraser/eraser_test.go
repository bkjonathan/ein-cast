package eraser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestErase_RemovesTheFoldersAndEverythingInThem(t *testing.T) {
	base := t.TempDir()
	config, cache := filepath.Join(base, "config", "EinCast"), filepath.Join(base, "cache", "EinCast")
	for _, f := range []string{
		filepath.Join(config, "settings.json"),
		filepath.Join(config, "profiles.json.bad"),
		filepath.Join(cache, "thumbs", "ab", "x.jpg"),
	} {
		mustWrite(t, f)
	}
	media := filepath.Join(base, "Movies", "film.mkv")
	mustWrite(t, media)

	if err := New(config, cache).Erase(); err != nil {
		t.Fatalf("Erase() = %v", err)
	}

	for _, dir := range []string{config, cache} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s still exists (stat error: %v)", dir, err)
		}
	}
	if _, err := os.Stat(media); err != nil {
		t.Errorf("a file outside the given folders was touched: %v", err)
	}
}

func TestErase_NothingToRemoveIsFine(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "never-created")
	if err := New(missing).Erase(); err != nil {
		t.Fatalf("Erase() = %v, want nil", err)
	}
}

func TestErase_RefusesPathsThatWouldDoDamage(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder on this machine")
	}
	root := filepath.VolumeName(home) + string(filepath.Separator)
	for name, dir := range map[string]string{"empty": "", "relative": "EinCast", "disk root": root, "home folder": home} {
		t.Run(name, func(t *testing.T) {
			if err := New(dir).Erase(); err == nil {
				t.Fatalf("Erase(%q) = nil, want a refusal", dir)
			}
		})
	}
}

func TestErase_ARefusalDoesNotStopTheOtherFolders(t *testing.T) {
	good := filepath.Join(t.TempDir(), "EinCast")
	mustWrite(t, filepath.Join(good, "settings.json"))

	if err := New("", good).Erase(); err == nil {
		t.Fatal("Erase() = nil, want the refusal reported")
	}
	if _, err := os.Stat(good); !os.IsNotExist(err) {
		t.Errorf("the good folder was not erased (stat error: %v)", err)
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
