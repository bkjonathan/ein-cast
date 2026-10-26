package fsscan_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"eincast/internal/adapters/fsscan"
	"eincast/internal/adapters/memrepo"
	"eincast/internal/app"
	"eincast/internal/app/library"
	"eincast/internal/domain/media"
)

// The library service with the real scanner, the real repository and real
// folders: what the app does, without the window.

type nopEvents struct{}

func (nopEvents) Publish(app.Event) {}

type memory struct{ libs []media.Library }

func (m *memory) Libraries() []media.Library            { return slices.Clone(m.libs) }
func (m *memory) SaveLibraries(l []media.Library) error { m.libs = slices.Clone(l); return nil }

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newService(mem *memory) (*library.Service, *memrepo.Repo) {
	log := slog.New(slog.DiscardHandler)
	repo := memrepo.New()
	return library.NewService(fsscan.New(log), repo, mem, nopEvents{}, log), repo
}

func TestTwoFoldersWithTheSameFileNamesAreTwoLibraries(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write(t, a, "shows/pilot.mkv", "from a")
	write(t, b, "shows/pilot.mkv", "from b, longer")
	svc, _ := newService(&memory{})

	sa, err := svc.Add(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := svc.Add(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}

	items := svc.List()
	if len(items) != 2 || items[0].ID == items[1].ID {
		t.Fatalf("items = %+v, want two items with different IDs", items)
	}
	for _, it := range items {
		if it.RelPath != "shows/pilot.mkv" {
			t.Errorf("RelPath = %q", it.RelPath)
		}
		want := sa.Library.Root
		if it.LibraryID == sb.Library.ID {
			want = sb.Library.Root
		}
		if it.Root != want {
			t.Errorf("an item of %s claims root %q, want %q", it.LibraryID, it.Root, want)
		}
		// Each item is fetchable by its own ID, and is the right file.
		got, err := svc.Get(it.ID)
		if err != nil || got.Path != it.Path {
			t.Errorf("Get(%s) = %+v, %v", it.ID, got, err)
		}
	}
}

func TestRemovingAndReaddingAFolderBringsItsIDsBack(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.mp4", "x")
	svc, _ := newService(&memory{})

	first, err := svc.Add(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	before := svc.List()[0].ID

	if err := svc.Remove(first.Library.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(before); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("a removed library's item is still served: %v", err)
	}

	if _, err := svc.Add(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if after := svc.List()[0].ID; after != before {
		t.Errorf("item ID after re-adding = %s, want %s: bookmarks and resume positions would be lost", after, before)
	}
}

func TestNestedFoldersAreRefusedForRealPaths(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	svc, _ := newService(&memory{})
	if _, err := svc.Add(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(context.Background(), inner); !errors.Is(err, library.ErrLibraryOverlaps) {
		t.Fatalf("adding a folder inside a library: error = %v", err)
	}
	if _, err := svc.Add(context.Background(), root+string(filepath.Separator)); !errors.Is(err, library.ErrLibraryExists) {
		t.Fatalf("adding the same folder with a trailing slash: error = %v", err)
	}
}

func TestAFolderThatDoesNotExistIsNotAdded(t *testing.T) {
	svc, _ := newService(&memory{})
	_, err := svc.Add(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("Add() = nil error for a missing folder")
	}
	if len(svc.Libraries()) != 0 {
		t.Error("a missing folder was kept as a library")
	}
	t.Logf("message shown to the person: %v", err)
}

func TestARestartKeepsEveryLibraryEvenWhenOneFolderIsGone(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	write(t, a, "a.mp4", "x")
	write(t, b, "b.mp4", "y")
	mem := &memory{}
	svc, _ := newService(mem)
	for _, root := range []string{a, b} {
		if _, err := svc.Add(context.Background(), root); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(b); err != nil { // the drive was unplugged between runs
		t.Fatal(err)
	}

	again, _ := newService(mem)
	if got := again.Libraries(); len(got) != 2 {
		t.Fatalf("after a restart %d libraries are listed, want both", len(got))
	}
	if err := again.Restore(context.Background()); err == nil {
		t.Fatal("Restore() = nil with a folder missing")
	}
	states := again.State()
	if states[0].Error != "" || states[0].Stats.Total != 1 {
		t.Errorf("the intact library = %+v", states[0])
	}
	if states[1].Error == "" {
		t.Errorf("the missing folder's library = %+v, want an error", states[1])
	}
	if len(again.Libraries()) != 2 {
		t.Error("a missing folder was forgotten; it may come back when the drive does")
	}
}
