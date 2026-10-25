package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

type fakeScanner struct {
	items   []media.Item
	err     error
	gotRoot string
	gotCtx  context.Context
}

func (f *fakeScanner) Scan(ctx context.Context, root string) ([]media.Item, error) {
	f.gotCtx, f.gotRoot = ctx, root
	return f.items, f.err
}

type fakeRepo struct {
	root  string
	items []media.Item
}

func (f *fakeRepo) Replace(root string, items []media.Item) { f.root, f.items = root, items }
func (f *fakeRepo) Root() string                            { return f.root }
func (f *fakeRepo) List() []media.Item                      { return f.items }
func (f *fakeRepo) Get(id media.ID) (media.Item, error) {
	for _, it := range f.items {
		if it.ID == id {
			return it, nil
		}
	}
	return media.Item{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
}

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
	return append([]app.Event(nil), f.got...)
}

// scanned filters the events down to LibraryScanned.
func (f *fakeEvents) scanned() []app.LibraryScanned {
	var out []app.LibraryScanned
	for _, e := range f.events() {
		if s, ok := e.(app.LibraryScanned); ok {
			out = append(out, s)
		}
	}
	return out
}

// statuses filters the events down to LibraryStatusChanged.
func (f *fakeEvents) statuses() []app.LibraryStatus {
	var out []app.LibraryStatus
	for _, e := range f.events() {
		if s, ok := e.(app.LibraryStatusChanged); ok {
			out = append(out, s.Status)
		}
	}
	return out
}

type fakeMemory struct {
	root    string
	err     error
	remembs []string
}

func (f *fakeMemory) RememberRoot(r string) error {
	if f.err != nil {
		return f.err
	}
	f.root = r
	f.remembs = append(f.remembs, r)
	return nil
}
func (f *fakeMemory) LastRoot() string { return f.root }

type ctxKey struct{}

func newService(sc *fakeScanner, repo *fakeRepo, ev *fakeEvents) *Service {
	return newServiceWithMemory(sc, repo, &fakeMemory{}, ev)
}

func newServiceWithMemory(sc *fakeScanner, repo *fakeRepo, mem *fakeMemory, ev *fakeEvents) *Service {
	return NewService(sc, repo, mem, ev, slog.New(slog.DiscardHandler))
}

func TestScanFolder_Success(t *testing.T) {
	items := []media.Item{
		{ID: "a", Kind: media.KindVideo, Size: 10},
		{ID: "b", Kind: media.KindAudio, Size: 5},
	}
	sc, repo, ev := &fakeScanner{items: items}, &fakeRepo{}, &fakeEvents{}
	svc := newService(sc, repo, ev)

	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	got, err := svc.ScanFolder(ctx, "/media")
	if err != nil {
		t.Fatalf("ScanFolder() error = %v", err)
	}

	if sc.gotRoot != "/media" || sc.gotCtx != ctx {
		t.Errorf("scanner got root=%q ctx=%v, want /media and the caller's ctx", sc.gotRoot, sc.gotCtx)
	}
	if repo.root != "/media" || len(repo.items) != 2 {
		t.Errorf("repo = %q/%d items, want /media/2", repo.root, len(repo.items))
	}
	if got.Root != "/media" || got.Stats.Total != 2 || got.Stats.TotalSize != 15 {
		t.Errorf("summary = %+v", got)
	}
	scanned := ev.scanned()
	if len(scanned) != 1 || scanned[0].Summary.Root != "/media" || scanned[0].Summary.Stats.Total != 2 {
		t.Errorf("LibraryScanned events = %#v", scanned)
	}
}

func TestScanFolder_Errors(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name    string
		path    string
		scanErr error
		wantErr error
	}{
		{"empty path", "", nil, nil},
		{"scanner fails", "/media", boom, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := []media.Item{{ID: "keep"}}
			sc := &fakeScanner{err: tt.scanErr}
			repo := &fakeRepo{root: "/old", items: previous}
			ev := &fakeEvents{}

			_, err := newService(sc, repo, ev).ScanFolder(context.Background(), tt.path)
			if err == nil {
				t.Fatal("ScanFolder() error = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error %v does not wrap %v", err, tt.wantErr)
			}
			if repo.root != "/old" || len(repo.items) != 1 {
				t.Errorf("previous library was modified: %q / %v", repo.root, repo.items)
			}
			if n := len(ev.scanned()); n != 0 {
				t.Errorf("published %d LibraryScanned events on failure, want 0", n)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	t.Run("before any scan", func(t *testing.T) {
		svc := newService(&fakeScanner{}, &fakeRepo{}, &fakeEvents{})
		if _, ok := svc.Summary(); ok {
			t.Fatal("Summary() ok = true before any scan")
		}
	})

	t.Run("after a scan", func(t *testing.T) {
		repo := &fakeRepo{root: "/media", items: []media.Item{{Kind: media.KindImage, Size: 3}}}
		got, ok := newService(&fakeScanner{}, repo, &fakeEvents{}).Summary()
		if !ok || got.Root != "/media" || got.Stats.Counts[media.KindImage] != 1 {
			t.Fatalf("Summary() = %+v, %v", got, ok)
		}
	})

	t.Run("empty folder still counts as scanned", func(t *testing.T) {
		repo := &fakeRepo{root: "/empty"}
		got, ok := newService(&fakeScanner{}, repo, &fakeEvents{}).Summary()
		if !ok || got.Stats.Total != 0 {
			t.Fatalf("Summary() = %+v, %v", got, ok)
		}
	})
}

func TestGet(t *testing.T) {
	repo := &fakeRepo{root: "/m", items: []media.Item{{ID: "a", Name: "a.mp4"}}}
	svc := newService(&fakeScanner{}, repo, &fakeEvents{})

	t.Run("found", func(t *testing.T) {
		it, err := svc.Get("a")
		if err != nil || it.Name != "a.mp4" {
			t.Fatalf("Get(a) = %+v, %v", it, err)
		}
	})
	t.Run("missing wraps ErrNotFound", func(t *testing.T) {
		if _, err := svc.Get("zzz"); !errors.Is(err, media.ErrNotFound) {
			t.Fatalf("Get(zzz) error = %v, want ErrNotFound", err)
		}
	})
}

func TestScanFolder_StatusTransitions(t *testing.T) {
	ev := &fakeEvents{}
	svc := newService(&fakeScanner{items: []media.Item{{ID: "a"}}}, &fakeRepo{}, ev)

	if _, err := svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatal(err)
	}

	got := ev.statuses()
	want := []app.LibraryStatus{{Scanning: true, Root: "/media"}, {Root: "/media"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("statuses = %+v, want %+v", got, want)
	}

	// "scanned" must come before "scan finished" so the UI never shows an empty
	// library between the two.
	order := []string{}
	for _, e := range ev.events() {
		switch e.(type) {
		case app.LibraryScanned:
			order = append(order, "scanned")
		case app.LibraryStatusChanged:
			order = append(order, "status")
		}
	}
	if strings.Join(order, ",") != "status,scanned,status" {
		t.Errorf("event order = %v, want status,scanned,status", order)
	}
	if svc.Status() != (app.LibraryStatus{Root: "/media"}) {
		t.Errorf("Status() = %+v", svc.Status())
	}
}

func TestScanFolder_FailureIsExplainedInStatus(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		contain string
	}{
		{"folder vanished", fmt.Errorf("stat root: %w", fs.ErrNotExist), "no longer exists"},
		{"no permission", fmt.Errorf("open: %w", fs.ErrPermission), "isn't allowed"},
		{"cancelled", context.Canceled, "cancelled"},
		{"anything else", errors.New("disk exploded at /secret/path"), "couldn't be scanned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &fakeEvents{}
			svc := newService(&fakeScanner{err: tt.err}, &fakeRepo{}, ev)

			if _, err := svc.ScanFolder(context.Background(), "/media"); !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want it to wrap %v", err, tt.err)
			}

			st := svc.Status()
			if st.Scanning || st.Root != "/media" || !strings.Contains(st.Error, tt.contain) {
				t.Errorf("Status() = %+v, want an error containing %q", st, tt.contain)
			}
			if strings.Contains(st.Error, "/secret") || strings.Contains(st.Error, "exploded") {
				t.Errorf("Status().Error leaks system detail: %q", st.Error)
			}
			last := ev.statuses()[len(ev.statuses())-1]
			if last != st {
				t.Errorf("last published status %+v differs from Status() %+v", last, st)
			}
		})
	}
}

func TestScanFolder_SuccessClearsAnEarlierError(t *testing.T) {
	sc := &fakeScanner{err: errors.New("boom")}
	svc := newService(sc, &fakeRepo{}, &fakeEvents{})
	_, _ = svc.ScanFolder(context.Background(), "/bad")
	if svc.Status().Error == "" {
		t.Fatal("setup: expected an error status")
	}

	sc.err = nil
	if _, err := svc.ScanFolder(context.Background(), "/good"); err != nil {
		t.Fatal(err)
	}
	if st := svc.Status(); st.Error != "" || st.Root != "/good" {
		t.Errorf("Status() = %+v, want it cleared", st)
	}
}

func TestScanFolder_RemembersTheFolderOnlyOnSuccess(t *testing.T) {
	mem := &fakeMemory{}
	sc := &fakeScanner{err: errors.New("boom")}
	svc := newServiceWithMemory(sc, &fakeRepo{}, mem, &fakeEvents{})

	_, _ = svc.ScanFolder(context.Background(), "/bad")
	if len(mem.remembs) != 0 {
		t.Fatalf("remembered %v after a failed scan", mem.remembs)
	}

	sc.err = nil
	_, _ = svc.ScanFolder(context.Background(), "/good")
	if len(mem.remembs) != 1 || mem.remembs[0] != "/good" {
		t.Errorf("remembered = %v, want [/good]", mem.remembs)
	}
}

func TestScanFolder_MemoryFailureDoesNotFailTheScan(t *testing.T) {
	mem := &fakeMemory{err: errors.New("read-only disk")}
	repo := &fakeRepo{}
	svc := newServiceWithMemory(&fakeScanner{items: []media.Item{{ID: "a"}}}, repo, mem, &fakeEvents{})

	if _, err := svc.ScanFolder(context.Background(), "/media"); err != nil {
		t.Fatalf("ScanFolder() error = %v; a settings problem must not lose the scan", err)
	}
	if repo.root != "/media" {
		t.Error("the library was not replaced")
	}
}

// blockingScanner holds a scan open until released.
type blockingScanner struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingScanner) Scan(context.Context, string) ([]media.Item, error) {
	close(b.entered)
	<-b.release
	return nil, nil
}

func TestScanFolder_RejectsAnOverlappingScan(t *testing.T) {
	bs := &blockingScanner{entered: make(chan struct{}), release: make(chan struct{})}
	svc := NewService(bs, &fakeRepo{}, &fakeMemory{}, &fakeEvents{}, slog.New(slog.DiscardHandler))

	done := make(chan error, 1)
	go func() { _, err := svc.ScanFolder(context.Background(), "/a"); done <- err }()
	<-bs.entered

	if !svc.Status().Scanning {
		t.Error("Status().Scanning = false during a scan")
	}
	if _, err := svc.ScanFolder(context.Background(), "/b"); !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("second scan error = %v, want ErrScanInProgress", err)
	}

	close(bs.release)
	if err := <-done; err != nil {
		t.Fatalf("first scan error = %v", err)
	}
	if svc.Status().Scanning {
		t.Error("Scanning stuck at true after the scan finished")
	}
}

func TestRescan(t *testing.T) {
	t.Run("scans the current folder again", func(t *testing.T) {
		sc := &fakeScanner{items: []media.Item{{ID: "a", Size: 1}, {ID: "b", Size: 2}}}
		repo := &fakeRepo{root: "/media", items: []media.Item{{ID: "a", Size: 1}}}
		svc := newService(sc, repo, &fakeEvents{})

		sum, err := svc.Rescan(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if sc.gotRoot != "/media" || sum.Stats.Total != 2 || len(repo.items) != 2 {
			t.Errorf("scanner root=%q summary=%+v repo=%d items", sc.gotRoot, sum, len(repo.items))
		}
	})

	t.Run("before any folder was chosen", func(t *testing.T) {
		svc := newService(&fakeScanner{}, &fakeRepo{}, &fakeEvents{})
		if _, err := svc.Rescan(context.Background()); !errors.Is(err, ErrNoLibrary) {
			t.Fatalf("error = %v, want ErrNoLibrary", err)
		}
	})

	t.Run("a failed rescan keeps the old library", func(t *testing.T) {
		repo := &fakeRepo{root: "/media", items: []media.Item{{ID: "keep"}}}
		svc := newService(&fakeScanner{err: fs.ErrNotExist}, repo, &fakeEvents{})
		_, _ = svc.Rescan(context.Background())
		if len(repo.items) != 1 || repo.items[0].ID != "keep" {
			t.Errorf("library changed after a failed rescan: %+v", repo.items)
		}
	})
}

func TestRestore(t *testing.T) {
	t.Run("nothing remembered does nothing", func(t *testing.T) {
		sc := &fakeScanner{}
		svc := newServiceWithMemory(sc, &fakeRepo{}, &fakeMemory{}, &fakeEvents{})
		if err := svc.Restore(context.Background()); err != nil || sc.gotRoot != "" {
			t.Fatalf("err=%v scannedRoot=%q, want a quiet no-op", err, sc.gotRoot)
		}
	})

	t.Run("reopens the remembered folder", func(t *testing.T) {
		sc := &fakeScanner{items: []media.Item{{ID: "a"}}}
		repo := &fakeRepo{}
		svc := newServiceWithMemory(sc, repo, &fakeMemory{root: "/media"}, &fakeEvents{})
		if err := svc.Restore(context.Background()); err != nil {
			t.Fatal(err)
		}
		if sc.gotRoot != "/media" || repo.root != "/media" {
			t.Errorf("scanned %q, repo root %q", sc.gotRoot, repo.root)
		}
	})

	t.Run("a vanished folder is reported through Status", func(t *testing.T) {
		svc := newServiceWithMemory(&fakeScanner{err: fs.ErrNotExist}, &fakeRepo{}, &fakeMemory{root: "/gone"}, &fakeEvents{})
		if err := svc.Restore(context.Background()); err == nil {
			t.Fatal("Restore() error = nil")
		}
		if st := svc.Status(); !strings.Contains(st.Error, "no longer exists") || st.Root != "/gone" {
			t.Errorf("Status() = %+v", st)
		}
	})
}
