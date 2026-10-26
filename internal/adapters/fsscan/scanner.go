// Package fsscan is the filesystem implementation of app.Scanner.
package fsscan

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"eincast/internal/domain/media"
)

// Scanner walks a directory tree and lists the media files in it.
type Scanner struct {
	log *slog.Logger
}

// New returns a Scanner that logs skipped entries to log.
func New(log *slog.Logger) *Scanner {
	return &Scanner{log: log.With(slog.String("source", "library"))}
}

// Scan returns every supported media file under root in walk order (lexical
// within each directory).
//
// It deliberately skips:
//   - hidden entries and well-known OS/NAS metadata (see media.Ignored);
//   - symlinks, so a link inside the library cannot expose files outside it;
//   - files with an unsupported extension.
//
// Subtitle files (.srt, .vtt) are not items. They are attached to the video
// they belong to (see media.MatchSubtitles).
//
// An unreadable subdirectory is logged and skipped rather than failing the
// whole scan; an unusable root is an error.
func (s *Scanner) Scan(ctx context.Context, lib media.Library) ([]media.Item, error) {
	absRoot, err := filepath.Abs(lib.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", lib.Root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %q is not a directory", absRoot)
	}

	var items []media.Item
	subtitles := make(map[string][]string) // folder → subtitle file names in it
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
		if media.Ignored(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // directories to descend into, symlinks, devices, sockets
		}

		if media.IsSubtitleFile(d.Name()) {
			dir := filepath.Dir(path)
			subtitles[dir] = append(subtitles[dir], d.Name())
			return nil
		}

		item, ok := s.itemFor(lib.ID, absRoot, path, d)
		if ok {
			items = append(items, item)
		}
		return nil
	}

	if err := filepath.WalkDir(absRoot, walk); err != nil {
		return nil, fmt.Errorf("walk %q: %w", absRoot, err)
	}
	for i := range items {
		if items[i].Kind == media.KindVideo {
			items[i].Subtitles = attachSubtitles(absRoot, items[i], subtitles[filepath.Dir(items[i].Path)])
		}
	}
	return items, nil
}

// attachSubtitles returns the sidecar subtitles of a video, given the subtitle
// files found in its folder.
func attachSubtitles(root string, video media.Item, folder []string) []media.Subtitle {
	matches := media.MatchSubtitles(video.Name, folder)
	if len(matches) == 0 {
		return nil
	}
	dir := filepath.Dir(video.Path)
	out := make([]media.Subtitle, len(matches))
	for i, m := range matches {
		path := filepath.Join(dir, m.Name)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = m.Name // cannot happen for a path found under root
		}
		out[i] = media.Subtitle{Lang: m.Lang, Label: m.Label, RelPath: filepath.ToSlash(rel), Path: path}
	}
	return out
}

// itemFor builds the media.Item for a regular file, or reports false if the
// file is not servable.
func (s *Scanner) itemFor(libID media.LibraryID, root, path string, d fs.DirEntry) (media.Item, bool) {
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
		ID:        media.NewID(libID, rel),
		LibraryID: libID,
		Root:      root,
		Name:      d.Name(),
		RelPath:   rel,
		Path:      path,
		Kind:      kind,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
	}, true
}
