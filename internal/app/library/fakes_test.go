package library

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// fakeScanner returns canned results per folder. It stamps the items the way the
// real scanner does, and is safe to use while the watcher scans on its own
// goroutine.
type fakeScanner struct {
	mu    sync.Mutex
	items map[string][]media.Item // by root
	errs  map[string]error        // by root
	calls []string                // roots, in order
	hold  map[string]*hold        // by root: block the scan until released
}

// hold keeps a scan open until released.
type hold struct {
	entered chan struct{}
	release chan struct{}
}

func newHold() *hold { return &hold{entered: make(chan struct{}), release: make(chan struct{})} }

func newScanner() *fakeScanner {
	return &fakeScanner{items: map[string][]media.Item{}, errs: map[string]error{}, hold: map[string]*hold{}}
}

func (f *fakeScanner) Scan(ctx context.Context, lib media.Library) ([]media.Item, error) {
	f.mu.Lock()
	f.calls = append(f.calls, lib.Root)
	h := f.hold[lib.Root]
	f.mu.Unlock()
	if h != nil {
		close(h.entered)
		select {
		case <-h.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs[lib.Root]; err != nil {
		return nil, err
	}
	out := slices.Clone(f.items[lib.Root])
	for i := range out {
		out[i].LibraryID, out[i].Root = lib.ID, lib.Root
	}
	return out, nil
}

func (f *fakeScanner) set(root string, items ...media.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[root] = items
	delete(f.errs, root)
}

func (f *fakeScanner) fail(root string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[root] = err
}

func (f *fakeScanner) scans() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeScanner) scansOf(root string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == root {
			n++
		}
	}
	return n
}

func (f *fakeScanner) lastCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// fakeRepo is app.LibraryRepo over plain maps.
type fakeRepo struct {
	mu    sync.Mutex
	order []media.LibraryID
	items map[media.LibraryID][]media.Item
}

func newRepo() *fakeRepo { return &fakeRepo{items: map[media.LibraryID][]media.Item{}} }

func (f *fakeRepo) Replace(id media.LibraryID, items []media.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[id]; !ok {
		f.order = append(f.order, id)
	}
	f.items[id] = slices.Clone(items)
}

func (f *fakeRepo) Remove(id media.LibraryID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, id)
	f.order = slices.DeleteFunc(f.order, func(o media.LibraryID) bool { return o == id })
}

func (f *fakeRepo) Items(id media.LibraryID) []media.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.items[id])
}

func (f *fakeRepo) List() []media.Item {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []media.Item
	for _, id := range f.order {
		out = append(out, f.items[id]...)
	}
	return out
}

func (f *fakeRepo) Get(id media.ID) (media.Item, error) {
	for _, it := range f.List() {
		if it.ID == id {
			return it, nil
		}
	}
	return media.Item{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
}

func (f *fakeRepo) has(id media.LibraryID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.items[id]
	return ok
}

// fakeEvents records everything published.
type fakeEvents struct {
	mu  sync.Mutex
	got []app.Event
}

func (f *fakeEvents) Publish(ev app.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, ev)
}

func (f *fakeEvents) events() []app.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.got)
}

// scanned returns the LibraryScanned events.
func (f *fakeEvents) scanned() []app.LibraryScanned {
	var out []app.LibraryScanned
	for _, e := range f.events() {
		if s, ok := e.(app.LibraryScanned); ok {
			out = append(out, s)
		}
	}
	return out
}

// statuses returns the LibraryStatusChanged events.
func (f *fakeEvents) statuses() []app.LibraryStatusChanged {
	var out []app.LibraryStatusChanged
	for _, e := range f.events() {
		if s, ok := e.(app.LibraryStatusChanged); ok {
			out = append(out, s)
		}
	}
	return out
}

// kinds names the events in order: "status" or "scanned".
func (f *fakeEvents) kinds() []string {
	var out []string
	for _, e := range f.events() {
		switch e.(type) {
		case app.LibraryScanned:
			out = append(out, "scanned")
		case app.LibraryStatusChanged:
			out = append(out, "status")
		}
	}
	return out
}

// fakeMemory remembers the list of libraries.
type fakeMemory struct {
	mu    sync.Mutex
	libs  []media.Library
	err   error
	saves [][]media.Library
}

func (f *fakeMemory) Libraries() []media.Library {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.libs)
}

func (f *fakeMemory) SaveLibraries(libs []media.Library) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves = append(f.saves, slices.Clone(libs))
	if f.err != nil {
		return f.err
	}
	f.libs = slices.Clone(libs)
	return nil
}

func (f *fakeMemory) lastSave() []media.Library {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.saves) == 0 {
		return nil
	}
	return f.saves[len(f.saves)-1]
}

// rig is a Service wired to fakes.
type rig struct {
	svc  *Service
	sc   *fakeScanner
	repo *fakeRepo
	mem  *fakeMemory
	ev   *fakeEvents
}

func newRig(remembered ...media.Library) *rig {
	r := &rig{sc: newScanner(), repo: newRepo(), mem: &fakeMemory{libs: remembered}, ev: &fakeEvents{}}
	r.svc = NewService(r.sc, r.repo, r.mem, r.ev, discardLog())
	return r
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

var modTime = time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)

// file is a video item as a scan would find it (the fake scanner adds the library).
func file(rel string, size int64) media.Item {
	return media.Item{ID: media.ID("id:" + rel), Name: rel, RelPath: rel, Kind: media.KindVideo, Size: size, ModTime: modTime}
}

// stateOf finds one library's state, failing the test if it is not listed.
func (r *rig) stateOf(id media.LibraryID) (app.LibraryState, bool) {
	for _, st := range r.svc.State() {
		if st.Library.ID == id {
			return st, true
		}
	}
	return app.LibraryState{}, false
}

// p turns a slash path into the platform's own form. The service cleans every
// root it is given, which on Windows rewrites "/media" as "\media"; the tests
// key their fakes by the same string, so they must write it the same way.
func p(path string) string { return filepath.FromSlash(path) }
