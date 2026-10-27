// Package library holds the library use cases: adding and removing the folders
// that make up the media library, scanning them, and reading back what was found.
package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"unicode"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// Errors a caller can match with errors.Is.
var (
	// ErrScanInProgress is returned when a library is asked to scan while it is
	// already being scanned.
	ErrScanInProgress = errors.New("that library is already being scanned")
	// ErrNoSuchLibrary is returned for a library ID that is not (or no longer) in
	// the list.
	ErrNoSuchLibrary = errors.New("no such library")
	// ErrLibraryExists is returned when the folder is already a library.
	ErrLibraryExists = errors.New("that folder is already in the library")
	// ErrLibraryOverlaps is returned when the folder contains, or is inside, an
	// existing library: its files would be listed twice.
	ErrLibraryOverlaps = errors.New("that folder overlaps a folder already in the library")
	// ErrInvalidName is returned for a library name that cannot be used.
	ErrInvalidName = errors.New("a library name must be 1 to 64 characters without control characters")
)

const maxNameLen = 64

// Service owns the list of libraries and what was found in them. It coordinates
// the scanner, the repository, the remembered folders and event publishing.
type Service struct {
	scanner app.Scanner
	repo    app.LibraryRepo
	memory  app.LibraryMemory
	events  app.EventPublisher
	log     *slog.Logger

	mu     sync.RWMutex // guards everything below
	libs   []media.Library
	flags  map[media.LibraryID]*flags
	locks  map[media.LibraryID]*sync.Mutex // held for a whole scan; TryLock rejects overlaps
	wanted map[media.LibraryID]string      // roots that should be watched (see watch.go)

	// Watching the folders is optional (see WithWatcher and Watch).
	watcher app.FolderWatcher
	timing  WatchTiming
	wake    chan struct{} // tells Watch that wanted changed
}

// flags is what is happening to one library.
type flags struct {
	scanning bool
	err      string
	watching bool
}

// Option customises a Service.
type Option func(*Service)

// NewService builds a Service from its ports. The libraries remembered from the
// last run are listed at once (so the UI never mistakes a restart for a first
// run); Restore scans them.
func NewService(scanner app.Scanner, repo app.LibraryRepo, memory app.LibraryMemory, events app.EventPublisher, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		scanner: scanner,
		repo:    repo,
		memory:  memory,
		events:  events,
		log:     log.With(slog.String("source", "library")),
		flags:   map[media.LibraryID]*flags{},
		locks:   map[media.LibraryID]*sync.Mutex{},
		wanted:  map[media.LibraryID]string{},
		timing:  DefaultWatchTiming,
		wake:    make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(s)
	}
	for _, lib := range memory.Libraries() {
		s.adopt(lib)
	}
	return s
}

// adopt puts a library in the list. Must be called with mu held (or before the
// service is shared).
func (s *Service) adopt(lib media.Library) {
	s.libs = append(s.libs, lib)
	s.flags[lib.ID] = &flags{}
	s.locks[lib.ID] = &sync.Mutex{}
}

// scanTrigger says who asked for a scan, which decides how loud it is.
type scanTrigger int

const (
	// byRequest is a person's add, scan or rescan: it shows progress and always
	// reports its result.
	byRequest scanTrigger = iota
	// byWatch is a rescan because the disk changed. It is quiet: no progress, and
	// no result at all when nothing changed.
	byWatch
)

// Add makes a folder a library and scans it. The library appears in the list at
// once, scanning; if the scan fails the folder is not kept, since a bad choice
// should not leave a broken entry behind. Errors: ErrLibraryExists,
// ErrLibraryOverlaps, and the scan's own failure in words for the person (the
// cause is wrapped, and the details are in the log).
func (s *Service) Add(ctx context.Context, root string) (app.LibraryState, error) {
	if strings.TrimSpace(root) == "" {
		return app.LibraryState{}, errors.New("add library: path is empty")
	}
	lib := media.NewLibrary(root)

	s.mu.Lock()
	for _, other := range s.libs {
		switch {
		case other.ID == lib.ID:
			s.mu.Unlock()
			return app.LibraryState{}, ErrLibraryExists
		case media.Overlaps(other.Root, lib.Root):
			s.mu.Unlock()
			return app.LibraryState{}, fmt.Errorf("%w (%s)", ErrLibraryOverlaps, other.Name)
		}
	}
	s.adopt(lib)
	s.flags[lib.ID].scanning = true
	s.remember()
	s.mu.Unlock()
	s.publishStatus()

	if _, err := s.scan(ctx, lib, byRequest); err != nil {
		s.drop(lib.ID)
		return app.LibraryState{}, &scanFailure{msg: friendlyScanError(err), cause: err}
	}
	state, _ := s.stateOf(lib.ID)
	return state, nil
}

// scanFailure is a failed scan in words for the person; Unwrap gives the cause.
type scanFailure struct {
	msg   string
	cause error
}

func (e *scanFailure) Error() string { return e.msg }
func (e *scanFailure) Unwrap() error { return e.cause }

// Remove takes a library out of the list and forgets its files. The folder on
// disk is untouched.
func (s *Service) Remove(id media.LibraryID) error {
	if !s.drop(id) {
		return ErrNoSuchLibrary
	}
	return nil
}

// drop removes a library and reports whether it was there.
func (s *Service) drop(id media.LibraryID) bool {
	s.mu.Lock()
	i := slices.IndexFunc(s.libs, func(l media.Library) bool { return l.ID == id })
	if i < 0 {
		s.mu.Unlock()
		return false
	}
	s.libs = slices.Delete(s.libs, i, i+1)
	delete(s.flags, id)
	delete(s.locks, id)
	delete(s.wanted, id)
	s.remember()
	s.mu.Unlock()

	s.repo.Remove(id)
	s.kickWatch()
	s.log.Info("library removed", slog.String("id", string(id)))
	s.publishScanned()
	return true
}

// Rename changes the name a library is shown as.
func (s *Service) Rename(id media.LibraryID, name string) error {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n == 0 || n > maxNameLen || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ErrInvalidName
	}
	s.mu.Lock()
	i := slices.IndexFunc(s.libs, func(l media.Library) bool { return l.ID == id })
	if i < 0 {
		s.mu.Unlock()
		return ErrNoSuchLibrary
	}
	s.libs[i].Name = name
	s.remember()
	s.mu.Unlock()
	s.publishStatus()
	return nil
}

// SetHiddenFromTVs keeps a library off smart TVs, or shows it to them again. A TV
// cannot say who is watching, so this is the only choice it gets (ADR 0027).
func (s *Service) SetHiddenFromTVs(id media.LibraryID, hidden bool) error {
	s.mu.Lock()
	i := slices.IndexFunc(s.libs, func(l media.Library) bool { return l.ID == id })
	if i < 0 {
		s.mu.Unlock()
		return ErrNoSuchLibrary
	}
	s.libs[i].HiddenFromTVs = hidden
	s.remember()
	s.mu.Unlock()
	s.publishStatus()
	return nil
}

// remember saves the list. A failure is logged and not returned: the library
// works for this run, and the person just adds the folder again next launch.
// Must be called with mu held.
func (s *Service) remember() {
	if err := s.memory.SaveLibraries(slices.Clone(s.libs)); err != nil {
		s.log.Warn("could not remember the library folders", slog.String("error", err.Error()))
	}
}

// Rescan scans one library again, picking up added and removed files.
func (s *Service) Rescan(ctx context.Context, id media.LibraryID) error {
	lib, ok := s.library(id)
	if !ok {
		return ErrNoSuchLibrary
	}
	_, err := s.scan(ctx, lib, byRequest)
	return err
}

// RescanAll scans every library, one after another. A library that fails does
// not stop the others; the failures are returned together.
func (s *Service) RescanAll(ctx context.Context) error {
	var errs []error
	for _, lib := range s.Libraries() {
		if _, err := s.scan(ctx, lib, byRequest); err != nil && !errors.Is(err, ErrScanInProgress) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Restore scans the libraries remembered from the last run. A folder that has
// since vanished is reported through its state, leaving the others usable.
func (s *Service) Restore(ctx context.Context) error {
	var errs []error
	for _, lib := range s.Libraries() {
		if _, err := s.scan(ctx, lib, byRequest); err != nil && !errors.Is(err, ErrScanInProgress) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// scan scans one library and, only if that succeeds, replaces its contents. A
// failed scan leaves the previous contents untouched, so a folder that is briefly
// unreadable cannot wipe a working library; the reason is kept in its state for
// the UI. A library is scanned by one caller at a time.
func (s *Service) scan(ctx context.Context, lib media.Library, trigger scanTrigger) (app.LibraryState, error) {
	s.mu.RLock()
	lock := s.locks[lib.ID]
	s.mu.RUnlock()
	if lock == nil {
		return app.LibraryState{}, ErrNoSuchLibrary
	}
	if !lock.TryLock() {
		return app.LibraryState{}, ErrScanInProgress
	}
	defer lock.Unlock()

	if trigger == byRequest {
		s.setFlags(lib.ID, func(f *flags) { f.scanning, f.err = true, "" })
	}

	items, err := s.scanner.Scan(ctx, lib)
	if err != nil {
		s.log.Warn("scan failed", slog.String("library", lib.Name), slog.String("root", lib.Root), slog.String("error", err.Error()))
		s.setFlags(lib.ID, func(f *flags) { f.scanning, f.err = false, friendlyScanError(err) })
		return app.LibraryState{}, fmt.Errorf("scan %q: %w", lib.Root, err)
	}
	if _, ok := s.library(lib.ID); !ok {
		return app.LibraryState{}, ErrNoSuchLibrary // removed while it was being scanned
	}

	if trigger == byWatch && sameFiles(s.repo.Items(lib.ID), items) {
		// The disk stirred but the library did not (a file touched, a download
		// that finished with the same size). Say nothing; only clear an earlier
		// error, since the folder is evidently readable again.
		s.log.Debug("library unchanged after a change on disk", slog.String("library", lib.Name))
		if st, _ := s.stateOf(lib.ID); st.Error != "" {
			s.setFlags(lib.ID, func(f *flags) { f.err = "" })
		}
		st, _ := s.stateOf(lib.ID)
		return st, nil
	}

	s.repo.Replace(lib.ID, items)
	stats := media.Summarize(items)
	s.log.Info("library scanned",
		slog.String("library", lib.Name),
		slog.String("root", lib.Root),
		slog.Int("items", stats.Total),
		slog.Int64("bytes", stats.TotalSize),
		slog.Bool("watch_triggered", trigger == byWatch),
	)
	// The new contents go out before "scan finished": a UI that sees the status
	// clear first would briefly show an empty library.
	s.mu.Lock()
	if f := s.flags[lib.ID]; f != nil {
		f.err = ""
	}
	s.mu.Unlock()
	s.publishScanned()
	s.setFlags(lib.ID, func(f *flags) { f.scanning = false })
	s.want(lib)
	st, _ := s.stateOf(lib.ID)
	return st, nil
}

// sameFiles reports whether two scans found the same files, unchanged. Subtitles,
// the title and series read from the name or a .nfo, and the poster all count:
// changing one changes what a player shows. Paths need no
// comparing: both scans walked the same root, and the ID is derived from the path.
func sameFiles(a, b []media.Item) bool {
	return slices.EqualFunc(a, b, func(x, y media.Item) bool {
		return x.ID == y.ID && x.Size == y.Size && x.ModTime.Equal(y.ModTime) &&
			slices.Equal(subtitlePaths(x), subtitlePaths(y)) &&
			x.Meta == y.Meta && x.Artwork == y.Artwork
	})
}

func subtitlePaths(it media.Item) []string {
	out := make([]string, len(it.Subtitles))
	for i, sub := range it.Subtitles {
		out[i] = sub.RelPath
	}
	return out
}

// Libraries returns the configured libraries in the order they were added.
func (s *Service) Libraries() []media.Library {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.libs)
}

func (s *Service) library(id media.LibraryID) (media.Library, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.libs {
		if l.ID == id {
			return l, true
		}
	}
	return media.Library{}, false
}

// State describes every library: what it is, what is in it, and what is happening
// to it.
func (s *Service) State() []app.LibraryState {
	s.mu.RLock()
	libs := slices.Clone(s.libs)
	snapshot := make(map[media.LibraryID]flags, len(libs))
	for id, f := range s.flags {
		snapshot[id] = *f
	}
	s.mu.RUnlock()

	out := make([]app.LibraryState, len(libs))
	for i, lib := range libs {
		f := snapshot[lib.ID]
		out[i] = app.LibraryState{
			Library:  lib,
			Stats:    media.Summarize(s.repo.Items(lib.ID)),
			Scanning: f.scanning,
			Error:    f.err,
			Watching: f.watching,
		}
	}
	return out
}

func (s *Service) stateOf(id media.LibraryID) (app.LibraryState, bool) {
	for _, st := range s.State() {
		if st.Library.ID == id {
			return st, true
		}
	}
	return app.LibraryState{}, false
}

// setFlags changes one library's flags and tells the UI.
func (s *Service) setFlags(id media.LibraryID, change func(*flags)) {
	s.mu.Lock()
	f := s.flags[id]
	if f == nil {
		s.mu.Unlock()
		return // removed meanwhile
	}
	before := *f
	change(f)
	changed := *f != before
	s.mu.Unlock()
	if changed {
		s.publishStatus()
	}
}

func (s *Service) publishStatus() {
	s.events.Publish(app.LibraryStatusChanged{Libraries: s.State()})
}

func (s *Service) publishScanned() {
	s.events.Publish(app.LibraryScanned{Libraries: s.State()})
}

// List returns every item of every library.
func (s *Service) List() []media.Item {
	return s.repo.List()
}

// Get returns one item, or an error wrapping media.ErrNotFound.
func (s *Service) Get(id media.ID) (media.Item, error) {
	return s.repo.Get(id)
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
