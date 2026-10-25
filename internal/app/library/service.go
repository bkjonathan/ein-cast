// Package library holds the library use cases: scanning a folder and reading
// back what was found.
package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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
}

// NewService builds a Service from its ports.
func NewService(scanner app.Scanner, repo app.LibraryRepo, memory app.LibraryMemory, events app.EventPublisher, log *slog.Logger) *Service {
	return &Service{
		scanner: scanner,
		repo:    repo,
		memory:  memory,
		events:  events,
		log:     log.With(slog.String("source", "library")),
	}
}

// ScanFolder scans path and, only if that succeeds, replaces the current
// library, remembers the folder and publishes app.LibraryScanned. A failed scan
// leaves the previous library untouched, so a bad folder choice cannot wipe a
// working setup; the reason is kept in Status for the UI. Only one scan runs at
// a time: a second request gets ErrScanInProgress.
func (s *Service) ScanFolder(ctx context.Context, path string) (app.LibrarySummary, error) {
	if path == "" {
		return app.LibrarySummary{}, errors.New("scan folder: path is empty")
	}
	if !s.scanMu.TryLock() {
		return app.LibrarySummary{}, ErrScanInProgress
	}
	defer s.scanMu.Unlock()

	s.setStatus(app.LibraryStatus{Scanning: true, Root: path})

	items, err := s.scanner.Scan(ctx, path)
	if err != nil {
		s.log.Warn("scan failed", slog.String("root", path), slog.String("error", err.Error()))
		s.setStatus(app.LibraryStatus{Root: path, Error: friendlyScanError(err)})
		return app.LibrarySummary{}, fmt.Errorf("scan %q: %w", path, err)
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
	)
	// The new library goes out before "scan finished": a UI that sees the
	// status clear first would briefly show an empty state.
	s.events.Publish(app.LibraryScanned{Summary: summary})
	s.setStatus(app.LibraryStatus{Root: path})
	return summary, nil
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
	s.status = st
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
