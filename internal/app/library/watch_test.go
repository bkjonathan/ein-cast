package library

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// lockedScanner is a scanner safe to use while Watch scans on its own goroutine.
type lockedScanner struct {
	mu    sync.Mutex
	items []media.Item
	err   error
	calls int
}

func (f *lockedScanner) Scan(context.Context, string) ([]media.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.items, f.err
}

func (f *lockedScanner) set(items []media.Item, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items, f.err = items, err
}

func (f *lockedScanner) scans() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// lockedRepo is fakeRepo with a lock, for the same reason.
type lockedRepo struct {
	mu sync.Mutex
	fakeRepo
}

func (f *lockedRepo) Replace(root string, items []media.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fakeRepo.Replace(root, items)
}
func (f *lockedRepo) Root() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fakeRepo.Root()
}
func (f *lockedRepo) List() []media.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fakeRepo.List()
}

// fakeWatcher hands out one signal channel per Watch call and records them.
type fakeWatcher struct {
	mu    sync.Mutex
	err   error
	roots []string
	ctxs  []context.Context
	chans []chan struct{}
}

func (f *fakeWatcher) Watch(ctx context.Context, root string) (<-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.roots = append(f.roots, root)
	f.ctxs = append(f.ctxs, ctx)
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan struct{}, 1)
	f.chans = append(f.chans, ch)
	return ch, nil
}

func (f *fakeWatcher) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeWatcher) watched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.roots...)
}

// signal wakes the n-th watch (0-based), as the disk changing would.
func (f *fakeWatcher) signal(n int) {
	f.mu.Lock()
	ch := f.chans[n]
	f.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default: // one is already pending, as with the real thing
	}
}

func (f *fakeWatcher) ctx(n int) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxs[n]
}

func (f *fakeWatcher) closeChan(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	close(f.chans[n])
}

var fastTiming = WatchTiming{Quiet: 30 * time.Millisecond, MaxWait: 300 * time.Millisecond}

type watchRig struct {
	svc  *Service
	sc   *lockedScanner
	repo *lockedRepo
	ev   *fakeEvents
	w    *fakeWatcher
}

// newWatchRig builds a service whose Watch loop is already running, stopped at
// the end of the test. The scanner starts out with one file.
func newWatchRig(t *testing.T, timing WatchTiming) *watchRig {
	t.Helper()
	r := &watchRig{
		sc:   &lockedScanner{items: []media.Item{file("a", 1)}},
		repo: &lockedRepo{},
		ev:   &fakeEvents{},
		w:    &fakeWatcher{},
	}
	r.svc = NewService(r.sc, r.repo, &fakeMemory{}, r.ev, discardLog(), WithWatcher(r.w, timing))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.svc.Watch(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Watch did not return after its context was cancelled")
		}
	})
	return r
}

func file(id string, size int64) media.Item {
	return media.Item{ID: media.ID(id), Kind: media.KindVideo, Size: size}
}

// eventually polls cond until it holds or two seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWatch_WatchesTheFolderTheScanLoaded(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}

	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })
	if got := r.w.watched(); len(got) != 1 || got[0] != "/media" {
		t.Fatalf("watched = %v, want [/media]", got)
	}
	last := r.ev.statuses()[len(r.ev.statuses())-1]
	if !last.Watching {
		t.Errorf("the last published status = %+v, want Watching so the UI hears about it", last)
	}
}

func TestWatch_AChangeRescansOnceItSettles(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })

	r.sc.set([]media.Item{file("a", 1), file("b", 2)}, nil)
	r.w.signal(0)

	eventually(t, "the rescan to publish", func() bool { return len(r.ev.scanned()) == 2 })
	if got := r.ev.scanned()[1].Summary.Stats.Total; got != 2 {
		t.Errorf("rescanned total = %d, want 2", got)
	}
	if got := len(r.repo.List()); got != 2 {
		t.Errorf("repo has %d items, want 2", got)
	}
}

func TestWatch_ABurstIsOneScan(t *testing.T) {
	r := newWatchRig(t, WatchTiming{Quiet: 80 * time.Millisecond, MaxWait: 2 * time.Second})
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })
	before := r.sc.scans()

	r.sc.set([]media.Item{file("a", 1), file("b", 2)}, nil)
	for range 5 {
		r.w.signal(0)
		time.Sleep(10 * time.Millisecond) // well inside the quiet period
	}

	eventually(t, "the rescan", func() bool { return r.sc.scans() > before })
	time.Sleep(200 * time.Millisecond) // long enough for a wrongly separate second scan
	if got := r.sc.scans() - before; got != 1 {
		t.Errorf("%d scans after one burst, want 1", got)
	}
}

func TestWatch_ChangesThatNeverStopStillRefreshWithinMaxWait(t *testing.T) {
	r := newWatchRig(t, WatchTiming{Quiet: 100 * time.Millisecond, MaxWait: 250 * time.Millisecond})
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })
	before := r.sc.scans()

	stop := make(chan struct{})
	defer close(stop)
	go func() { // a download that keeps writing: never quiet for 100ms
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				r.w.signal(0)
			}
		}
	}()

	eventually(t, "a rescan despite constant change", func() bool { return r.sc.scans() > before })
}

func TestWatch_NothingChangedSaysNothing(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })
	before := r.sc.scans()
	statusesBefore := len(r.ev.statuses())

	r.w.signal(0) // the scanner still returns the same one file
	eventually(t, "the rescan", func() bool { return r.sc.scans() > before })
	time.Sleep(50 * time.Millisecond)

	if got := len(r.ev.scanned()); got != 1 {
		t.Errorf("%d LibraryScanned events, want only the first", got)
	}
	if got := len(r.ev.statuses()); got != statusesBefore {
		t.Errorf("%d new status events, want none (no progress flicker for a quiet rescan)", got-statusesBefore)
	}
}

func TestWatch_AFailedQuietRescanIsReportedAndKeepsTheLibrary(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })

	r.sc.set(nil, errors.New("walk: no such file or directory")) // the drive was unplugged
	r.w.signal(0)

	eventually(t, "the failure to show", func() bool { return r.svc.Status().Error != "" })
	if got := len(r.repo.List()); got != 1 {
		t.Errorf("repo has %d items, want the previous library kept", got)
	}
	if !r.svc.Status().Watching {
		t.Error("a failed rescan must not turn Watching off; the watch itself is fine")
	}

	r.sc.set([]media.Item{file("a", 1)}, nil) // plugged back in, nothing else changed
	r.w.signal(0)
	eventually(t, "the error to clear", func() bool { return r.svc.Status().Error == "" })
}

func TestWatch_UnwatchableFolderLeavesTheLibraryWorking(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	r.w.setErr(errors.New("too many open files"))

	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch attempt", func() bool { return len(r.w.watched()) == 1 })
	if r.svc.Status().Watching {
		t.Error("Watching = true although the watch failed")
	}

	// Scanning the same folder again retries.
	r.w.setErr(nil)
	if _, err := r.svc.Rescan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the retry to succeed", func() bool { return r.svc.Status().Watching })
	if got := len(r.w.watched()); got != 2 {
		t.Errorf("Watch called %d times, want 2 (the failed try and the retry)", got)
	}
}

func TestWatch_AnotherFolderMovesTheWatch(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/one"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the first watch", func() bool { return len(r.w.watched()) == 1 })

	if _, err := r.svc.ScanFolder(context.Background(), "/two"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the second watch", func() bool { return len(r.w.watched()) == 2 })

	if got := r.w.watched(); got[1] != "/two" {
		t.Errorf("watched = %v, want /two second", got)
	}
	eventually(t, "the first watch to be stopped", func() bool { return r.w.ctx(0).Err() != nil })
	if r.w.ctx(1).Err() != nil {
		t.Error("the new watch was cancelled along with the old one")
	}
}

func TestWatch_SameFolderIsNotWatchedTwice(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	for range 3 {
		if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, "the watch", func() bool { return r.svc.Status().Watching })
	time.Sleep(50 * time.Millisecond)
	if got := len(r.w.watched()); got != 1 {
		t.Errorf("Watch called %d times for one folder, want 1", got)
	}
}

func TestWatch_AWatcherThatEndsTurnsWatchingOff(t *testing.T) {
	r := newWatchRig(t, fastTiming)
	if _, err := r.svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return r.svc.Status().Watching })

	r.w.closeChan(0)
	eventually(t, "Watching to turn off", func() bool { return !r.svc.Status().Watching })
}

func TestWatch_EndsWithItsContext(t *testing.T) {
	w := &fakeWatcher{}
	svc := NewService(&lockedScanner{}, &lockedRepo{}, &fakeMemory{}, &fakeEvents{}, discardLog(), WithWatcher(w, fastTiming))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Watch(ctx)
	}()

	if _, err := svc.ScanFolder(ctx, "/media"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { return svc.Status().Watching })

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return")
	}
	if w.ctx(0).Err() == nil {
		t.Error("the folder watch outlived Watch")
	}
	if svc.Status().Watching {
		t.Error("Watching = true after Watch ended")
	}
}

func TestWatch_WithoutAWatcherReturnsAtOnce(t *testing.T) {
	svc := NewService(&lockedScanner{}, &lockedRepo{}, &fakeMemory{}, &fakeEvents{}, discardLog())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.Watch(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch blocked although nothing was configured")
	}
	// And scanning never waits on a watcher that is not there.
	for range 3 {
		if _, err := svc.ScanFolder(context.Background(), "/media"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan_NeverWaitsForTheWatcher(t *testing.T) {
	// Nobody runs Watch: announcements must not block or pile up.
	svc := NewService(&lockedScanner{}, &lockedRepo{}, &fakeMemory{}, &fakeEvents{}, discardLog(),
		WithWatcher(&fakeWatcher{}, fastTiming))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 5 {
			_, _ = svc.ScanFolder(context.Background(), []string{"/a", "/b", "/c", "/d", "/e"}[i])
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ScanFolder blocked with no one listening for the watch announcement")
	}
}

var _ app.FolderWatcher = (*fakeWatcher)(nil)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }
