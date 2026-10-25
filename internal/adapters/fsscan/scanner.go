// Package fsscan is the filesystem implementation of app.Scanner.
package fsscan

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"eincast/internal/domain/media"
)

// Scanner walks a directory tree and lists the media files in it.
type Scanner struct {
	log *slog.Logger
}

// New returns a Scanner that logs skipped entries to log.
func New(log *slog.Logger) *Scanner {
	return &Scanner{log: log}
}

// Scan returns every supported media file under root in walk order (lexical
// within each directory).
//
// It deliberately skips:
//   - hidden entries (name starts with "."), which also covers macOS "._*"
//     AppleDouble files and ".DS_Store";
//   - well-known OS/NAS metadata folders (see isSystemName);
//   - symlinks, so a link inside the library cannot expose files outside it;
//   - files with an unsupported extension.
//
// An unreadable subdirectory is logged and skipped rather than failing the
// whole scan; an unusable root is an error.
func (s *Scanner) Scan(ctx context.Context, root string) ([]media.Item, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", absRoot)
	}

	var items []media.Item
	walk := func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if path == absRoot {
				return walkErr
			}
			s.log.Warn("skipping unreadable entry",
				slog.String("path", path), slog.String("error", walkErr.Error()))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == absRoot {
			return nil
		}
		if skipName(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // directories to descend into, symlinks, devices, sockets
		}

		item, ok := s.itemFor(absRoot, path, d)
		if ok {
			items = append(items, item)
		}
		return nil
	}

	if err := filepath.WalkDir(absRoot, walk); err != nil {
		return nil, fmt.Errorf("walk %q: %w", absRoot, err)
	}
	return items, nil
}

// itemFor builds the media.Item for a regular file, or reports false if the
// file is not servable.
func (s *Scanner) itemFor(root, path string, d fs.DirEntry) (media.Item, bool) {
	kind, err := media.DetectKind(d.Name())
	if err != nil {
		return media.Item{}, false // unsupported type: expected, not worth a log line
	}
	info, err := d.Info()
	if err != nil {
		s.log.Warn("skipping file without stat info",
			slog.String("path", path), slog.String("error", err.Error()))
		return media.Item{}, false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return media.Item{}, false // cannot happen for a path found under root
	}
	// IDs hash the slash form so they match across macOS and Windows.
	rel = filepath.ToSlash(rel)

	return media.Item{
		ID:      media.NewID(rel),
		Name:    d.Name(),
		RelPath: rel,
		Path:    path,
		Kind:    kind,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, true
}

func skipName(name string) bool {
	return strings.HasPrefix(name, ".") || isSystemName(name)
}

// isSystemName matches metadata that is not dot-prefixed: Windows, macOS
// archive and Synology NAS leftovers. Case-insensitive because Windows is.
func isSystemName(name string) bool {
	switch strings.ToLower(name) {
	case "thumbs.db", "desktop.ini", "$recycle.bin", "system volume information",
		"__macosx", "@eadir":
		return true
	default:
		return false
	}
}
