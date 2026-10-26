package library

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"eincast/internal/app"
)

// WatchTiming decides how long the library waits after a change on disk before
// it scans again.
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

// WithWatcher makes the library follow its folder: Watch then rescans after
// files are added, removed or changed.
func WithWatcher(w app.FolderWatcher, t WatchTiming) Option {
	return func(s *Service) {
		s.watcher = w
		s.timing = t
	}
}

// announceRoot tells Watch which folder the library now shows. Only the latest
// announcement matters, so a pending older one is replaced, and a scan never
// waits for the watcher to listen.
func (s *Service) announceRoot(root string) {
	if s.watcher == nil {
		return
	}
	for {
		select {
		case s.roots <- root:
			return
		default:
		}
		select {
		case <-s.roots: // drop the stale one and try again
		default:
		}
	}
}

// startWatch begins watching root. It returns the change signals and a function
// that stops the watch; the signals are nil when the folder cannot be watched.
func (s *Service) startWatch(ctx context.Context, root string) (<-chan struct{}, func()) {
	wctx, cancel := context.WithCancel(ctx)
	ch, err := s.watcher.Watch(wctx, root)
	if err != nil {
		cancel()
		s.log.Warn("not watching the library folder for changes; use Rescan to refresh it",
			slog.String("root", root), slog.String("error", err.Error()))
		return nil, func() {}
	}
	return ch, cancel
}

// Watch keeps the library in step with its folder until ctx ends: it watches
// whichever folder the latest scan loaded and, once changes settle, scans it
// again quietly. Run it in its own goroutine, owned by whoever owns ctx. It
// returns at once when no watcher was configured.
//
// A folder that cannot be watched is not an error to the person: the library
// works as before and Status().Watching says false, so the UI can suggest
// Rescan. Scanning the same folder again retries the watch.
func (s *Service) Watch(ctx context.Context) {
	if s.watcher == nil {
		return
	}
	var (
		root    string
		stop    = func() {}
		changes <-chan struct{}
		due     <-chan time.Time // nil while the folder is quiet
		began   time.Time        // when the pending burst of changes started
	)
	defer func() {
		stop()
		s.setWatching(false)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case next := <-s.roots:
			if next == root && changes != nil {
				continue // the scan we caused ourselves
			}
			stop()
			changes, due = nil, nil
			s.setWatching(false)
			root = next
			changes, stop = s.startWatch(ctx, root)
			s.setWatching(changes != nil)

		case _, ok := <-changes:
			if !ok {
				s.log.Warn("stopped watching the library folder for changes", slog.String("root", root))
				changes, due = nil, nil
				s.setWatching(false)
				continue
			}
			now := time.Now()
			if due == nil {
				began = now
			}
			wait := min(s.timing.Quiet, max(s.timing.MaxWait-now.Sub(began), 0))
			due = time.After(wait)

		case <-due:
			due = nil
			_, err := s.scan(ctx, root, byWatch)
			if errors.Is(err, ErrScanInProgress) {
				// A person's scan is running and may have missed the change.
				due = time.After(s.timing.Quiet)
				began = time.Now()
			}
		}
	}
}
