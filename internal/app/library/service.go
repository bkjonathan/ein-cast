// Package library holds the library use cases: scanning a folder and reading
// back what was found.
package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"sync"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// ErrScanInProgress is returned when a scan is requested while one is running.
var ErrScanInProgress = errors.New("a scan is already running")

// ErrNoLibrary is returned when asked to rescan before any folder was chosen.
var ErrNoLibrary = errors.New("no library folder has been chosen yet")

// Service coordinates the scanner, the repository, the remembered folder and
// event publishing.
type Service struct {
	scanner app.Scanner
	repo    app.LibraryRepo
	memory  app.LibraryMemory
	events  app.EventPublisher
	log     *slog.Logger

	scanMu sync.Mutex // held for the whole of a scan; TryLock rejects overlaps

	statusMu sync.RWMutex
	status   app.LibraryStatus

	// Watching the folder is optional (see WithWatcher and Watch).
	watcher app.FolderWatcher
	timing  WatchTiming
	roots   chan string // the folder to watch, announced by each successful scan
}

// Option customises a Service.
type Option func(*Service)

// NewService builds a Service from its ports.
func NewService(scanner app.Scanner, repo app.LibraryRepo, memory app.LibraryMemory, events app.EventPublisher, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		scanner: scanner,
		repo:    repo,
		memory:  memory,
		events:  events,
		log:     log.With(slog.String("source", "library")),
		timing:  DefaultWatchTiming,
		roots:   make(chan string, 1),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// scanTrigger says who asked for a scan, which decides how loud it is.
type scanTrigger int

const (
	// byRequest is a person's scan or rescan: it shows progress and always
	// reports its result.
	byRequest scanTrigger = iota
	// byWatch is a rescan because the disk changed. It is quiet: no progress, and
	// no result at all when nothing changed.
	byWatch
)

// ScanFolder scans path and, only if that succeeds, replaces the current
// library, remembers the folder and publishes app.LibraryScanned. A failed scan
// leaves the previous library untouched, so a bad folder choice cannot wipe a
// working setup; the reason is kept in Status for the UI. Only one scan runs at
// a time: a second request gets ErrScanInProgress.
func (s *Service) ScanFolder(ctx context.Context, path string) (app.LibrarySummary, error) {
	return s.scan(ctx, path, byRequest)
}

func (s *Service) scan(ctx context.Context, path string, trigger scanTrigger) (app.LibrarySummary, error) {
	if path == "" {
		return app.LibrarySummary{}, errors.New("scan folder: path is empty")
	}
	if !s.scanMu.TryLock() {
		return app.LibrarySummary{}, ErrScanInProgress
	}
	defer s.scanMu.Unlock()

	if trigger == byRequest {
		s.setStatus(app.LibraryStatus{Scanning: true, Root: path})
	}

	items, err := s.scanner.Scan(ctx, path)
	if err != nil {
		s.log.Warn("scan failed", slog.String("root", path), slog.String("error", err.Error()))
		s.setStatus(app.LibraryStatus{Root: path, Error: friendlyScanError(err)})
		return app.LibrarySummary{}, fmt.Errorf("scan %q: %w", path, err)
	}

	if trigger == byWatch && s.repo.Root() == path && sameFiles(s.repo.List(), items) {
		// The disk stirred but the library did not (a file touched, a download
		// that finished with the same size). Say nothing; only clear an earlier
		// error, since the folder is evidently readable again.
		s.log.Debug("library unchanged after a change on disk", slog.String("root", path))
		if s.Status().Error != "" {
			s.setStatus(app.LibraryStatus{Root: path})
		}
		return app.LibrarySummary{Root: path, Stats: media.Summarize(items)}, nil
	}

	s.repo.Replace(path, items)
	summary := app.LibrarySummary{Root: path, Stats: media.Summarize(items)}

	if err := s.memory.RememberRoot(path); err != nil {
		// Not worth failing a good scan over; the user just picks again next launch.
		s.log.Warn("could not remember the library folder", slog.String("error", err.Error()))
	}

	s.log.Info("library scanned",
		slog.String("root", path),
		slog.Int("items", summary.Stats.Total),
		slog.Int64("bytes", summary.Stats.TotalSize),
		slog.Bool("watch_triggered", trigger == byWatch),
	)
	// The new library goes out before "scan finished": a UI that sees the
	// status clear first would briefly show an empty state.
	s.events.Publish(app.LibraryScanned{Summary: summary})
	s.setStatus(app.LibraryStatus{Root: path})
	s.announceRoot(path)
	return summary, nil
}

// sameFiles reports whether two scans found the same files, unchanged. Paths
// need no comparing: both scans walked the same root, and the ID is derived
// from the path.
func sameFiles(a, b []media.Item) bool {
	return slices.EqualFunc(a, b, func(x, y media.Item) bool {
		return x.ID == y.ID && x.Size == y.Size && x.ModTime.Equal(y.ModTime)
	})
}

// Rescan scans the current library folder again, picking up added and removed
// files.
func (s *Service) Rescan(ctx context.Context) (app.LibrarySummary, error) {
	root := s.repo.Root()
	if root == "" {
		return app.LibrarySummary{}, ErrNoLibrary
	}
	return s.ScanFolder(ctx, root)
}

// Restore reopens the folder remembered from the last run. With nothing
// remembered it does nothing. A folder that has since vanished is reported
// through Status, leaving the UI on its first-run screen with the reason.
func (s *Service) Restore(ctx context.Context) error {
	root := s.memory.LastRoot()
	if root == "" {
		return nil
	}
	_, err := s.ScanFolder(ctx, root)
	return err
}

// Status says whether a scan is running and why the last one failed.
func (s *Service) Status() app.LibraryStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

func (s *Service) setStatus(st app.LibraryStatus) {
	s.statusMu.Lock()
	// Whether the folder is watched is the watcher's to say, not the scan's.
	st.Watching = s.status.Watching
	s.status = st
	s.statusMu.Unlock()
	s.events.Publish(app.LibraryStatusChanged{Status: st})
}

func (s *Service) setWatching(on bool) {
	s.statusMu.Lock()
	if s.status.Watching == on {
		s.statusMu.Unlock()
		return
	}
	s.status.Watching = on
	st := s.status
	s.statusMu.Unlock()
	s.events.Publish(app.LibraryStatusChanged{Status: st})
}

// friendlyScanError turns a scan failure into one sentence for the UI. The full
// error, with paths and system text, goes to the log instead.
func friendlyScanError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "That folder no longer exists. It may have been moved, renamed or unplugged."
	case errors.Is(err, fs.ErrPermission):
		return "EinCast isn't allowed to read that folder. Check its permissions."
	case errors.Is(err, context.Canceled):
		return "The scan was cancelled."
	default:
		return "That folder couldn't be scanned. The Logs view has the details."
	}
}

// Summary describes the current library. ok is false before the first scan.
func (s *Service) Summary() (summary app.LibrarySummary, ok bool) {
	root := s.repo.Root()
	if root == "" {
		return app.LibrarySummary{}, false
	}
	return app.LibrarySummary{Root: root, Stats: media.Summarize(s.repo.List())}, true
}

// Root is the scanned folder, or "" before the first scan. Unlike Summary it is
// O(1), so request handlers can call it on every request.
func (s *Service) Root() string {
	return s.repo.Root()
}

// List returns every item in the library.
func (s *Service) List() []media.Item {
	return s.repo.List()
}

// Get returns one item, or an error wrapping media.ErrNotFound.
func (s *Service) Get(id media.ID) (media.Item, error) {
	return s.repo.Get(id)
}
