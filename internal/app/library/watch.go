package library

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// WatchTiming decides how long a library waits after a change on disk before it
// scans again.
type WatchTiming struct {
	// Quiet is how long the folder must be still before a scan starts. Copying a
	// season of episodes is one burst of events, and one scan should follow it.
	Quiet time.Duration
	// MaxWait caps the delay when changes never stop, such as a download that
	// takes ten minutes: the library still refreshes now and then meanwhile.
	MaxWait time.Duration
}

// DefaultWatchTiming suits copying files in a home setting.
var DefaultWatchTiming = WatchTiming{Quiet: 2 * time.Second, MaxWait: 30 * time.Second}

// WithWatcher makes the libraries follow their folders: Watch then rescans a
// library after files are added, removed or changed.
func WithWatcher(w app.FolderWatcher, t WatchTiming) Option {
	return func(s *Service) {
		s.watcher = w
		s.timing = t
	}
}

// want records that a library's folder should be watched. Every successful scan
// says so again, which is what retries a watch that had failed.
func (s *Service) want(lib media.Library) {
	if s.watcher == nil {
		return
	}
	s.mu.Lock()
	if _, ok := s.flags[lib.ID]; ok { // not removed meanwhile
		s.wanted[lib.ID] = lib.Root
	}
	s.mu.Unlock()
	s.kickWatch()
}

// kickWatch tells Watch to look at what should be watched. One pending nudge is
// enough: Watch reads the whole picture each time, so a scan never waits for it.
func (s *Service) kickWatch() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// watchChild is the goroutine watching one library.
type watchChild struct {
	root   string
	cancel context.CancelFunc
	done   chan struct{}
}

func (k *watchChild) exited() bool {
	select {
	case <-k.done:
		return true
	default:
		return false
	}
}

// stop ends the child and waits for it, so its last word (Watching = false) is
// said before anything else is decided.
func (k *watchChild) stop() {
	k.cancel()
	<-k.done
}

// Watch keeps the libraries in step with their folders until ctx ends: it
// watches every library that has been scanned and, once changes settle, scans
// that library again quietly. Run it in its own goroutine, owned by whoever owns
// ctx. It returns at once when no watcher was configured.
//
// A folder that cannot be watched is not an error to the person: the library
// works as before and its state says Watching = false, so the UI can suggest
// Rescan. Scanning the library again retries the watch.
func (s *Service) Watch(ctx context.Context) {
	if s.watcher == nil {
		return
	}
	kids := map[media.LibraryID]*watchChild{}
	defer func() {
		for _, k := range kids {
			k.stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		}

		want := s.wantedLibraries()
		for id, k := range kids {
			lib, ok := want[id]
			if !ok || lib.Root != k.root || k.exited() {
				k.stop()
				delete(kids, id)
			}
		}
		for id, lib := range want {
			if _, running := kids[id]; running {
				continue
			}
			cctx, cancel := context.WithCancel(ctx)
			k := &watchChild{root: lib.Root, cancel: cancel, done: make(chan struct{})}
			kids[id] = k
			go func() {
				defer close(k.done)
				s.watchLibrary(cctx, lib)
			}()
		}
	}
}

// wantedLibraries returns the libraries that should be watched, as configured now.
func (s *Service) wantedLibraries() map[media.LibraryID]media.Library {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[media.LibraryID]media.Library{}
	for _, lib := range s.libs {
		if root, ok := s.wanted[lib.ID]; ok {
			lib.Root = root
			out[lib.ID] = lib
		}
	}
	return out
}

// watchLibrary watches one library's folder and rescans it after changes settle.
// It returns when ctx ends, when the folder cannot be watched, or when the watch
// ends on its own; Watch starts it again after the next successful scan.
func (s *Service) watchLibrary(ctx context.Context, lib media.Library) {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes, err := s.watcher.Watch(wctx, lib.Root)
	if err != nil {
		s.log.Warn("not watching the library folder for changes; use Rescan to refresh it",
			slog.String("library", lib.Name), slog.String("root", lib.Root), slog.String("error", err.Error()))
		return
	}
	s.setFlags(lib.ID, func(f *flags) { f.watching = true })
	defer s.setFlags(lib.ID, func(f *flags) { f.watching = false })

	var (
		due   <-chan time.Time // nil while the folder is quiet
		began time.Time        // when the pending burst of changes started
	)
	for {
		select {
		case <-ctx.Done():
			return

		case _, ok := <-changes:
			if !ok {
				s.log.Warn("stopped watching the library folder for changes", slog.String("library", lib.Name))
				return
			}
			now := time.Now()
			if due == nil {
				began = now
			}
			wait := min(s.timing.Quiet, max(s.timing.MaxWait-now.Sub(began), 0))
			due = time.After(wait)

		case <-due:
			due = nil
			_, err := s.scan(ctx, lib, byWatch)
			switch {
			case errors.Is(err, ErrScanInProgress):
				// A person's scan is running and may have missed the change.
				due = time.After(s.timing.Quiet)
				began = time.Now()
			case errors.Is(err, ErrNoSuchLibrary):
				return
			}
		}
	}
}
