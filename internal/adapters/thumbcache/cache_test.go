package thumbcache

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newCache(t *testing.T) (*Cache, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "thumbs") // does not exist yet
	return New(dir, slog.New(slog.DiscardHandler)), dir
}

func TestCache_RoundTrip(t *testing.T) {
	c, _ := newCache(t)
	if _, ok := c.Get("abc-1-2-v1"); ok {
		t.Fatal("Get() hit on an empty cache")
	}
	if err := c.Put("abc-1-2-v1", []byte("jpeg")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, ok := c.Get("abc-1-2-v1")
	if !ok || string(got) != "jpeg" {
		t.Fatalf("Get() = %q, %v", got, ok)
	}
}

func TestCache_NothingIsCreatedUntilThereIsSomethingToStore(t *testing.T) {
	c, dir := newCache(t)
	c.Get("abc")
	c.Prune(time.Hour)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("folder exists after only reads (stat error = %v)", err)
	}
}

func TestCache_RejectsKeysThatCouldEscape(t *testing.T) {
	c, dir := newCache(t)
	for _, key := range []string{"", "../x", "a/b", `a\b`, "a b", "x.jpg", "é", string(make([]byte, 300))} {
		if err := c.Put(key, []byte("x")); err == nil {
			t.Errorf("Put(%q) = nil, want an error", key)
		}
		if _, ok := c.Get(key); ok {
			t.Errorf("Get(%q) hit", key)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("rejected keys left %d files behind", len(entries))
	}
}

func TestCache_PutLeavesNoTemporaryFiles(t *testing.T) {
	c, dir := newCache(t)
	if err := c.Put("k1", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("k1", []byte("y")); err != nil { // replacing is fine
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "k1.jpg" {
		t.Fatalf("folder holds %v, want only k1.jpg", entries)
	}
	if got, _ := c.Get("k1"); string(got) != "y" {
		t.Errorf("Get() = %q, want the replacement", got)
	}
}

func TestCache_AnEmptyFileIsAMiss(t *testing.T) {
	c, dir := newCache(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "k.jpg"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); ok {
		t.Error("Get() hit on an empty file, which would serve a blank image")
	}
}

func TestCache_PruneRemovesOnlyOldFiles(t *testing.T) {
	c, dir := newCache(t)
	for _, k := range []string{"old", "new"} {
		if err := c.Put(k, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-100 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old.jpg"), old, old); err != nil {
		t.Fatal(err)
	}

	if n := c.Prune(90 * 24 * time.Hour); n != 1 {
		t.Fatalf("Prune() = %d, want 1", n)
	}
	if _, ok := c.Get("old"); ok {
		t.Error("old thumbnail survived")
	}
	if _, ok := c.Get("new"); !ok {
		t.Error("new thumbnail was removed")
	}
}

func TestCache_WithFilesKeepsAnotherKindUnderItsOwnExtension(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, slog.New(slog.DiscardHandler), WithFiles("subtitle", ".vtt"))
	if err := c.Put("abc-1", []byte("WEBVTT\n")); err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Get("abc-1"); !ok || string(got) != "WEBVTT\n" {
		t.Fatalf("Get() = %q, %v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, "abc-1.vtt")); err != nil {
		t.Errorf("file not stored as .vtt: %v", err)
	}
	// A thumbnail cache on the same folder does not see it.
	if _, ok := New(dir, slog.New(slog.DiscardHandler)).Get("abc-1"); ok {
		t.Error("a thumbnail cache read a subtitle")
	}
}
