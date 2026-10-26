package library

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

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
	return slices.Clone(f.roots)
}

// indexOf is the n-th Watch call for root, or -1.
func (f *fakeWatcher) indexOf(root string, nth int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := 0
	for i, r := range f.roots {
		if r == root {
			if seen == nth {
				return i
			}
			seen++
		}
	}
	return -1
}

// signal wakes watch number i, as the disk changing would.
func (f *fakeWatcher) signal(i int) {
	f.mu.Lock()
	ch := f.chans[i]
	f.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default: // one is already pending, as with the real thing
	}
}

func (f *fakeWatcher) ctx(i int) context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxs[i]
}

func (f *fakeWatcher) closeChan(i int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	close(f.chans[i])
}

var fastTiming = WatchTiming{Quiet: 30 * time.Millisecond, MaxWait: 300 * time.Millisecond}

type watchRig struct {
	*rig
	w *fakeWatcher
}

// newWatchRig builds a service whose Watch loop is already running, stopped at
// the end of the test.
func newWatchRig(t *testing.T, timing WatchTiming) *watchRig {
	t.Helper()
	r := &rig{sc: newScanner(), repo: newRepo(), mem: &fakeMemory{}, ev: &fakeEvents{}}
	w := &fakeWatcher{}
	r.svc = NewService(r.sc, r.repo, r.mem, r.ev, discardLog(), WithWatcher(w, timing))

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
	return &watchRig{rig: r, w: w}
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

func (g *watchRig) watching(id media.LibraryID) bool {
	st, ok := g.stateOf(id)
	return ok && st.Watching
}

// add adds a library holding one file and waits for it to be watched.
func (g *watchRig) add(t *testing.T, root string) media.Library {
	t.Helper()
	g.sc.set(root, file("a.mkv", 1))
	st, err := g.svc.Add(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, root+" to be watched", func() bool { return g.watching(st.Library.ID) })
	return st.Library
}

func TestWatch_WatchesTheFolderAScanLoaded(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")

	if got := g.w.watched(); len(got) != 1 || got[0] != "/media" {
		t.Fatalf("watched = %v, want [/media]", got)
	}
	last := g.ev.statuses()[len(g.ev.statuses())-1].Libraries
	if len(last) != 1 || last[0].Library != lib || !last[0].Watching {
		t.Errorf("the last published status = %+v, want Watching so the UI hears about it", last)
	}
}

func TestWatch_AChangeRescansOnceItSettles(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")

	g.sc.set("/media", file("a.mkv", 1), file("b.mkv", 2))
	g.w.signal(g.w.indexOf("/media", 0))

	eventually(t, "the rescan to publish", func() bool {
		st, _ := g.stateOf(lib.ID)
		return st.Stats.Total == 2
	})
	if got := len(g.repo.Items(lib.ID)); got != 2 {
		t.Errorf("repo has %d items, want 2", got)
	}
}

func TestWatch_ABurstIsOneScan(t *testing.T) {
	g := newWatchRig(t, WatchTiming{Quiet: 80 * time.Millisecond, MaxWait: 2 * time.Second})
	g.add(t, "/media")
	before := g.sc.scans()

	g.sc.set("/media", file("a.mkv", 1), file("b.mkv", 2))
	for range 5 {
		g.w.signal(0)
		time.Sleep(10 * time.Millisecond) // well inside the quiet period
	}

	eventually(t, "the rescan", func() bool { return g.sc.scans() > before })
	time.Sleep(200 * time.Millisecond) // long enough for a wrongly separate second scan
	if got := g.sc.scans() - before; got != 1 {
		t.Errorf("%d scans after one burst, want 1", got)
	}
}

func TestWatch_ChangesThatNeverStopStillRefreshWithinMaxWait(t *testing.T) {
	g := newWatchRig(t, WatchTiming{Quiet: 100 * time.Millisecond, MaxWait: 250 * time.Millisecond})
	g.add(t, "/media")
	before := g.sc.scans()

	stop := make(chan struct{})
	defer close(stop)
	go func() { // a download that keeps writing: never quiet for 100ms
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				g.w.signal(0)
			}
		}
	}()

	eventually(t, "a rescan despite constant change", func() bool { return g.sc.scans() > before })
}

func TestWatch_NothingChangedSaysNothing(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	g.add(t, "/media")
	before := g.sc.scans()
	eventsBefore := len(g.ev.events())

	g.w.signal(0) // the scanner still returns the same one file
	eventually(t, "the rescan", func() bool { return g.sc.scans() > before })
	time.Sleep(50 * time.Millisecond)

	if got := len(g.ev.events()) - eventsBefore; got != 0 {
		t.Errorf("%d new events (%v), want none: no flicker for a quiet rescan", got, g.ev.kinds()[eventsBefore:])
	}
}

func TestWatch_AFailedQuietRescanIsReportedAndKeepsTheLibrary(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")

	g.sc.fail("/media", errors.New("walk: no such file or directory")) // the drive was unplugged
	g.w.signal(0)

	eventually(t, "the failure to show", func() bool {
		st, _ := g.stateOf(lib.ID)
		return st.Error != ""
	})
	if got := len(g.repo.Items(lib.ID)); got != 1 {
		t.Errorf("repo has %d items, want the previous library kept", got)
	}
	if !g.watching(lib.ID) {
		t.Error("a failed rescan must not turn Watching off; the watch itself is fine")
	}

	g.sc.set("/media", file("a.mkv", 1)) // plugged back in, nothing else changed
	g.w.signal(0)
	eventually(t, "the error to clear", func() bool {
		st, _ := g.stateOf(lib.ID)
		return st.Error == ""
	})
}

func TestWatch_UnwatchableFolderLeavesTheLibraryWorkingAndARescanRetries(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	g.w.setErr(errors.New("too many open files"))
	g.sc.set("/media", file("a.mkv", 1))
	st, err := g.svc.Add(context.Background(), "/media")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch attempt", func() bool { return len(g.w.watched()) == 1 })
	time.Sleep(30 * time.Millisecond)
	if g.watching(st.Library.ID) {
		t.Error("Watching = true although the watch failed")
	}

	g.w.setErr(nil) // scanning the library again retries
	if err := g.svc.Rescan(context.Background(), st.Library.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the retry to succeed", func() bool { return g.watching(st.Library.ID) })
	if got := len(g.w.watched()); got != 2 {
		t.Errorf("Watch called %d times, want 2 (the failed try and the retry)", got)
	}
}

func TestWatch_EachLibraryHasItsOwnWatchAndItsOwnRescans(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	films := g.add(t, "/films")
	music := g.add(t, "/music")
	filmsBefore, musicBefore := g.sc.scansOf("/films"), g.sc.scansOf("/music")

	g.sc.set("/films", file("a.mkv", 1), file("b.mkv", 1))
	g.w.signal(g.w.indexOf("/films", 0))

	eventually(t, "films to be rescanned", func() bool { return g.sc.scansOf("/films") > filmsBefore })
	eventually(t, "films to show the new file", func() bool {
		st, _ := g.stateOf(films.ID)
		return st.Stats.Total == 2
	})
	time.Sleep(100 * time.Millisecond)
	if g.sc.scansOf("/music") != musicBefore {
		t.Error("a change in films made music rescan")
	}
	if !g.watching(music.ID) || !g.watching(films.ID) {
		t.Error("both libraries should still be watched")
	}
}

func TestWatch_RemovingALibraryStopsItsWatchOnly(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	films := g.add(t, "/films")
	music := g.add(t, "/music")

	if err := g.svc.Remove(films.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "films' watch to be stopped", func() bool { return g.w.ctx(g.w.indexOf("/films", 0)).Err() != nil })
	if g.w.ctx(g.w.indexOf("/music", 0)).Err() != nil {
		t.Error("removing films stopped music's watch")
	}
	if !g.watching(music.ID) {
		t.Error("music is no longer watched")
	}
}

func TestWatch_SameFolderIsNotWatchedTwice(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")
	for range 3 {
		if err := g.svc.Rescan(context.Background(), lib.ID); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(60 * time.Millisecond)
	if got := len(g.w.watched()); got != 1 {
		t.Errorf("Watch called %d times for one folder, want 1", got)
	}
}

func TestWatch_AWatcherThatEndsTurnsWatchingOff(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")

	g.w.closeChan(0)
	eventually(t, "Watching to turn off", func() bool { return !g.watching(lib.ID) })
}

func TestWatch_EndsWithItsContext(t *testing.T) {
	r := &rig{sc: newScanner(), repo: newRepo(), mem: &fakeMemory{}, ev: &fakeEvents{}}
	w := &fakeWatcher{}
	r.svc = NewService(r.sc, r.repo, r.mem, r.ev, discardLog(), WithWatcher(w, fastTiming))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.svc.Watch(ctx)
	}()

	r.sc.set("/media", file("a.mkv", 1))
	st, err := r.svc.Add(ctx, "/media")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the watch to start", func() bool { s, _ := r.stateOf(st.Library.ID); return s.Watching })

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return")
	}
	if w.ctx(0).Err() == nil {
		t.Error("the folder watch outlived Watch")
	}
	if s, _ := r.stateOf(st.Library.ID); s.Watching {
		t.Error("Watching = true after Watch ended")
	}
}

func TestWatch_WithoutAWatcherReturnsAtOnce(t *testing.T) {
	r := newRig()
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.svc.Watch(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch blocked although nothing was configured")
	}
	// And scanning never waits on a watcher that is not there.
	r.sc.set("/media", file("a.mkv", 1))
	if _, err := r.svc.Add(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}
}

func TestScan_NeverWaitsForTheWatcher(t *testing.T) {
	// Nobody runs Watch: nudges must not block or pile up.
	r := &rig{sc: newScanner(), repo: newRepo(), mem: &fakeMemory{}, ev: &fakeEvents{}}
	r.svc = NewService(r.sc, r.repo, r.mem, r.ev, discardLog(), WithWatcher(&fakeWatcher{}, fastTiming))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, root := range []string{"/a", "/b", "/c", "/d", "/e"} {
			_, _ = r.svc.Add(context.Background(), root)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Add blocked with no one listening for the watch nudge")
	}
}

func TestWatch_AddingOnlyASubtitleCountsAsAChange(t *testing.T) {
	g := newWatchRig(t, fastTiming)
	lib := g.add(t, "/media")
	ev := len(g.ev.scanned())

	withSub := file("a.mkv", 1)
	withSub.Subtitles = []media.Subtitle{{Lang: "en", Label: "English", RelPath: "a.en.srt"}}
	g.sc.set("/media", withSub) // same file, same size and time; one new track
	g.w.signal(0)

	eventually(t, "the rescan to publish", func() bool { return len(g.ev.scanned()) > ev })
	if got := g.repo.Items(lib.ID)[0].Subtitles; len(got) != 1 {
		t.Errorf("repo subtitles = %+v, want the new track", got)
	}
}

var _ app.FolderWatcher = (*fakeWatcher)(nil)
