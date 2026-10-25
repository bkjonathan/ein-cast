package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

type fakeEvents struct{ got []app.Event }

func (f *fakeEvents) Publish(ev app.Event) { f.got = append(f.got, ev) }

type ctxKey struct{}

func newService(sc *fakeScanner, repo *fakeRepo, ev *fakeEvents) *Service {
	return NewService(sc, repo, ev, slog.New(slog.DiscardHandler))
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
	if len(ev.got) != 1 {
		t.Fatalf("published %d events, want 1", len(ev.got))
	}
	scanned, ok := ev.got[0].(app.LibraryScanned)
	if !ok || scanned.Summary.Root != "/media" || scanned.Summary.Stats.Total != 2 {
		t.Errorf("event = %#v", ev.got[0])
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
			if len(ev.got) != 0 {
				t.Errorf("published %d events on failure, want 0", len(ev.got))
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
