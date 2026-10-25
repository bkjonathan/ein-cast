// Package library holds the library use cases: scanning a folder and reading
// back what was found.
package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// Service coordinates the scanner, the repository and event publishing.
type Service struct {
	scanner app.Scanner
	repo    app.LibraryRepo
	events  app.EventPublisher
	log     *slog.Logger
}

// NewService builds a Service from its ports.
func NewService(scanner app.Scanner, repo app.LibraryRepo, events app.EventPublisher, log *slog.Logger) *Service {
	return &Service{scanner: scanner, repo: repo, events: events, log: log.With(slog.String("source", "library"))}
}

// ScanFolder scans path and, only if that succeeds, replaces the current
// library and publishes app.LibraryScanned. A failed scan leaves the previous
// library untouched, so a bad folder choice cannot wipe a working setup.
func (s *Service) ScanFolder(ctx context.Context, path string) (app.LibrarySummary, error) {
	if path == "" {
		return app.LibrarySummary{}, errors.New("scan folder: path is empty")
	}

	items, err := s.scanner.Scan(ctx, path)
	if err != nil {
		return app.LibrarySummary{}, fmt.Errorf("scan %q: %w", path, err)
	}

	s.repo.Replace(path, items)
	summary := app.LibrarySummary{Root: path, Stats: media.Summarize(items)}

	s.log.Info("library scanned",
		slog.String("root", path),
		slog.Int("items", summary.Stats.Total),
		slog.Int64("bytes", summary.Stats.TotalSize),
	)
	s.events.Publish(app.LibraryScanned{Summary: summary})
	return summary, nil
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
