// Package thumbcache is a folder-backed cache of small generated files. It is
// an app.ThumbnailCache, and with WithFiles it also keeps extracted subtitles
// (an app.SubtitleCache): the two have the same needs, a file per key that is
// made once and forgotten after a while.
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

// Cache stores each item as a file named after its key.
type Cache struct {
	dir  string
	ext  string // with the dot
	noun string // for messages
	log  *slog.Logger
}

// Option customizes a Cache.
type Option func(*Cache)

// WithFiles says what the cache holds: what to call one in messages, and the
// extension of its files. Without it a Cache holds thumbnails (".jpg").
func WithFiles(noun, ext string) Option {
	return func(c *Cache) { c.noun, c.ext = noun, ext }
}

// New returns a Cache that keeps files in dir. The folder is created on first
// use; nothing is touched until then.
func New(dir string, log *slog.Logger, opts ...Option) *Cache {
	c := &Cache{dir: dir, ext: ".jpg", noun: "thumbnail"}
	for _, opt := range opts {
		opt(c)
	}
	c.log = log.With(slog.String("source", c.noun+"s"))
	return c
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

func (c *Cache) file(key string) string { return filepath.Join(c.dir, key+c.ext) }

// Get returns the stored file, or false for a miss or an unreadable file.
func (c *Cache) Get(key string) ([]byte, bool) {
	if !validKey(key) {
		return nil, false
	}
	data, err := os.ReadFile(c.file(key))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			c.log.Warn("could not read a cached "+c.noun, slog.String("error", err.Error()))
		}
		return nil, false
	}
	return data, len(data) > 0
}

// Put stores a file. It writes a temporary file and renames it, so a
// crash or a second writer never leaves half a picture under the real name.
func (c *Cache) Put(key string, data []byte) error {
	if !validKey(key) {
		return fmt.Errorf("%s cache: bad key %q", c.noun, key)
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("create %s folder: %w", c.noun, err)
	}
	tmp, err := os.CreateTemp(c.dir, "put-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", c.noun, err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", c.noun, err)
	}
	if err := os.Rename(tmp.Name(), c.file(key)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("store %s: %w", c.noun, err)
	}
	return nil
}

// Prune deletes files not written for maxAge, and returns how many. A
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
		c.log.Info("removed old "+c.noun+"s", slog.Int("count", removed))
	}
	return removed
}
