package memrepo

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"eincast/internal/domain/media"
)

func ids(items []media.Item) []media.ID {
	out := make([]media.ID, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestRepo_EmptyAndReplace(t *testing.T) {
	r := New()
	if len(r.List()) != 0 || len(r.Items("m")) != 0 {
		t.Fatalf("new repo not empty: %d items", len(r.List()))
	}

	r.Replace("m", []media.Item{{ID: "a", Name: "a.mp4"}, {ID: "b", Name: "b.mp3"}})
	if got := r.List(); !slices.Equal(ids(got), []media.ID{"a", "b"}) {
		t.Errorf("List() = %v, want a then b (scan order)", ids(got))
	}

	r.Replace("m", []media.Item{{ID: "c"}})
	if _, err := r.Get("a"); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Get(a) after replace error = %v, want ErrNotFound", err)
	}
	if got := r.Items("m"); !slices.Equal(ids(got), []media.ID{"c"}) {
		t.Errorf("Items(m) = %v", ids(got))
	}
}

func TestRepo_LibrariesAreIndependent(t *testing.T) {
	r := New()
	r.Replace("films", []media.Item{{ID: "f1"}, {ID: "f2"}})
	r.Replace("music", []media.Item{{ID: "m1"}})

	if got := r.List(); !slices.Equal(ids(got), []media.ID{"f1", "f2", "m1"}) {
		t.Errorf("List() = %v, want libraries in the order they were added", ids(got))
	}

	// Rescanning one library leaves the other alone, and keeps its place in line.
	r.Replace("films", []media.Item{{ID: "f3"}})
	if got := r.List(); !slices.Equal(ids(got), []media.ID{"f3", "m1"}) {
		t.Errorf("List() after replacing films = %v", ids(got))
	}
	if _, err := r.Get("f1"); !errors.Is(err, media.ErrNotFound) {
		t.Error("an item the rescan dropped can still be fetched")
	}
	if _, err := r.Get("m1"); err != nil {
		t.Errorf("music lost an item when films was rescanned: %v", err)
	}
	if got := r.Items("music"); !slices.Equal(ids(got), []media.ID{"m1"}) {
		t.Errorf("Items(music) = %v", ids(got))
	}
}

func TestRepo_Remove(t *testing.T) {
	r := New()
	r.Replace("films", []media.Item{{ID: "f1"}})
	r.Replace("music", []media.Item{{ID: "m1"}})

	r.Remove("films")
	if got := r.List(); !slices.Equal(ids(got), []media.ID{"m1"}) {
		t.Errorf("List() = %v", ids(got))
	}
	if _, err := r.Get("f1"); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Get(f1) after remove = %v, want ErrNotFound", err)
	}
	r.Remove("films") // removing it twice, or one never added, is fine
	r.Remove("nope")

	// Added again later, it goes to the end.
	r.Replace("films", []media.Item{{ID: "f1"}})
	if got := r.List(); !slices.Equal(ids(got), []media.ID{"m1", "f1"}) {
		t.Errorf("List() = %v", ids(got))
	}
}

func TestRepo_Get(t *testing.T) {
	r := New()
	r.Replace("m", []media.Item{{ID: "a", Name: "a.mp4"}})

	tests := []struct {
		name    string
		id      media.ID
		wantErr error
	}{
		{"found", "a", nil},
		{"missing", "nope", media.ErrNotFound},
		{"path-like id is just a missing key", "../../etc/passwd", media.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Get(tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Get(%q) error = %v, want %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestRepo_DefensiveCopies(t *testing.T) {
	in := []media.Item{{ID: "a", Name: "orig"}}
	r := New()
	r.Replace("m", in)

	in[0].Name = "mutated-input"
	r.List()[0].Name = "mutated-output"
	r.Items("m")[0].Name = "mutated-items"

	got, _ := r.Get("a")
	if got.Name != "orig" || r.List()[0].Name != "orig" || r.Items("m")[0].Name != "orig" {
		t.Fatalf("internal state was mutated through a shared slice: %+v", got)
	}
}

// Run with -race: readers must never observe a torn library while it is replaced.
func TestRepo_ConcurrentReplaceRemoveAndRead(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := range 4 {
		lib := media.LibraryID(rune('a' + i))
		wg.Add(3)
		go func() {
			defer wg.Done()
			for range 200 {
				r.Replace(lib, []media.Item{{ID: media.ID(rune('a' + i))}})
			}
		}()
		go func() {
			defer wg.Done()
			for range 100 {
				r.Remove(lib)
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				_ = r.List()
				_, _ = r.Get("a")
				_ = r.Items(lib)
			}
		}()
	}
	wg.Wait()
}
