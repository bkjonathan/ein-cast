// Package fswatch is the fsnotify implementation of app.FolderWatcher.
package fswatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	"eincast/internal/domain/media"
)

// Watcher watches a folder tree for files and folders coming and going.
type Watcher struct {
	log *slog.Logger
}

// New returns a Watcher that logs to log.
func New(log *slog.Logger) *Watcher {
	return &Watcher{log: log.With(slog.String("source", "library"))}
}

// Watch watches root and every folder under it, and reports on the returned
// channel when a media file, or a folder that may hold some, appears,
// disappears or changes. What the scanner would skip (hidden entries, system
// folders, symlinks) is skipped here too, so it cannot wake the library.
//
// fsnotify watches folders one by one, so a folder created later is added as it
// appears. On macOS (kqueue) every file in a watched folder also costs an open
// file handle, so a very large library can hit the system limit; Watch then
// returns an error rather than watch half the tree and let the library drift
// silently out of date. The caller falls back to manual rescans.
func (w *Watcher) Watch(ctx context.Context, root string) (<-chan struct{}, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", root, err)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	r := &run{log: w.log, fw: fw, root: abs, dirs: make(map[string]struct{})}
	if err := r.addTree(abs); err != nil {
		_ = fw.Close()
		return nil, fmt.Errorf("watch %q: %w", abs, err)
	}

	out := make(chan struct{}, 1)
	go r.loop(ctx, out)
	return out, nil
}

// run is one live watch. Only its goroutine touches it after Watch returns.
type run struct {
	log  *slog.Logger
	fw   *fsnotify.Watcher
	root string
	dirs map[string]struct{} // folders currently watched
}

// addTree watches dir and the folders below it. A folder that cannot be listed
// is skipped, as the scanner skips it; a folder the system refuses to watch
// (typically its limit was reached) is an error.
func (r *run) addTree(dir string) error {
	walk := func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == dir {
				return walkErr
			}
			// Unreadable or vanished: the scanner skips it too.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != r.root && media.Ignored(d.Name()) {
			return fs.SkipDir
		}
		if err := r.fw.Add(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
				return fs.SkipDir // removed or locked since we listed it
			}
			return err
		}
		r.dirs[path] = struct{}{}
		return nil
	}
	return filepath.WalkDir(dir, walk)
}

func (r *run) loop(ctx context.Context, out chan<- struct{}) {
	defer close(out)
	defer func() { _ = r.fw.Close() }()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-r.fw.Events:
			if !ok {
				return
			}
			if r.relevant(ev) {
				notify(out)
			}
		case err, ok := <-r.fw.Errors:
			if !ok {
				return
			}
			// Typically an overflowed event queue: changes were lost, so ask for
			// a look at the whole folder.
			r.log.Warn("folder watcher error", slog.String("error", err.Error()))
			notify(out)
		}
	}
}

// notify raises the signal unless one is already waiting; the receiver rescans
// either way, so a second one adds nothing.
func notify(out chan<- struct{}) {
	select {
	case out <- struct{}{}:
	default:
	}
}

// relevant decides whether an event can change the library, and keeps the set
// of watched folders current as folders come and go.
func (r *run) relevant(ev fsnotify.Event) bool {
	if ev.Op == fsnotify.Chmod || media.Ignored(filepath.Base(ev.Name)) {
		return false
	}
	switch {
	case ev.Has(fsnotify.Create):
		if info, err := os.Lstat(ev.Name); err == nil && info.IsDir() {
			// Files may have landed in it before the watch was added; the
			// signal makes the library look.
			if err := r.addTree(ev.Name); err != nil {
				r.log.Warn("could not watch a new folder; use Rescan after changes in it",
					slog.String("path", ev.Name), slog.String("error", err.Error()))
			}
			return true
		}
		return isMedia(ev.Name)
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		if r.forget(ev.Name) {
			return true
		}
		return isMedia(ev.Name)
	default: // Write
		return isMedia(ev.Name)
	}
}

// forget drops path and anything below it from the watched set, and reports
// whether path was a watched folder. The system removes the watches itself.
func (r *run) forget(path string) bool {
	if _, ok := r.dirs[path]; !ok {
		return false
	}
	prefix := path + string(filepath.Separator)
	for dir := range r.dirs {
		if dir == path || strings.HasPrefix(dir, prefix) {
			delete(r.dirs, dir)
		}
	}
	return true
}

// isMedia reports whether a file name can change the library: a media file, or
// a subtitle that may belong to a video.
func isMedia(name string) bool {
	if media.IsSubtitleFile(name) {
		return true
	}
	_, err := media.DetectKind(name)
	return err == nil
}
