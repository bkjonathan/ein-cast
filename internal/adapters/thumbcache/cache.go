// Package thumbcache is a folder-backed app.ThumbnailCache.
package thumbcache

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// Cache stores each thumbnail as a file named after its key.
type Cache struct {
	dir string
	log *slog.Logger
}

// New returns a Cache that keeps files in dir. The folder is created on first
// use; nothing is touched until then.
func New(dir string, log *slog.Logger) *Cache {
	return &Cache{dir: dir, log: log.With(slog.String("source", "thumbnails"))}
}

// validKey allows only what is safe in a file name on every platform. Keys come
// from the app itself, so this is a guard against a bug, not against a person.
func validKey(key string) bool {
	if key == "" || len(key) > 200 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func (c *Cache) file(key string) string { return filepath.Join(c.dir, key+".jpg") }

// Get returns the stored thumbnail, or false for a miss or an unreadable file.
func (c *Cache) Get(key string) ([]byte, bool) {
	if !validKey(key) {
		return nil, false
	}
	data, err := os.ReadFile(c.file(key))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			c.log.Warn("could not read a cached thumbnail", slog.String("error", err.Error()))
		}
		return nil, false
	}
	return data, len(data) > 0
}

// Put stores a thumbnail. It writes a temporary file and renames it, so a
// crash or a second writer never leaves half a picture under the real name.
func (c *Cache) Put(key string, data []byte) error {
	if !validKey(key) {
		return fmt.Errorf("thumbnail cache: bad key %q", key)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("create thumbnail folder: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, "put-*.tmp")
	if err != nil {
		return fmt.Errorf("write thumbnail: %w", err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write thumbnail: %w", err)
	}
	if err := os.Rename(tmp.Name(), c.file(key)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("store thumbnail: %w", err)
	}
	return nil
}

// Prune deletes thumbnails not written for maxAge, and returns how many. A
// changed file gets a new key, so the old picture would otherwise stay forever;
// one still in use is simply made again.
func (c *Cache) Prune(maxAge time.Duration) int {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0 // nothing stored yet
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(c.dir, e.Name())) == nil {
			removed++
		}
	}
	if removed > 0 {
		c.log.Info("removed old thumbnails", slog.Int("count", removed))
	}
	return removed
}
