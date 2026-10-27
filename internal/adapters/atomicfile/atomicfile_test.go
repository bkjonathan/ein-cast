package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWrite_ReplacesAndLeavesNoTemporaryFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "dir") // does not exist yet
	for _, content := range []string{"first", "second"} {
		if err := Write(dir, "f.json", []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "f.json"))
		if err != nil || string(got) != content {
			t.Fatalf("read %q, %v", got, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the file", len(entries))
	}
}

func TestWrite_IsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are not modelled the same way on Windows")
	}
	dir := t.TempDir()
	if err := Write(dir, "f", []byte("x")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(dir, "f"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestWrite_FailsCleanlyWhenTheDirectoryCannotBeMade(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	if err := Write(filepath.Join(blocker, "sub"), "f", []byte("x")); err == nil {
		t.Fatal("Write() error = nil")
	}
}
