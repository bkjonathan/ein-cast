package memrepo

import (
	"errors"
	"sync"
	"testing"

	"eincast/internal/domain/media"
)

func TestRepo_EmptyAndReplace(t *testing.T) {
	r := New()
	if r.Root() != "" || len(r.List()) != 0 {
		t.Fatalf("new repo not empty: root=%q items=%d", r.Root(), len(r.List()))
	}

	r.Replace("/m", []media.Item{{ID: "a", Name: "a.mp4"}, {ID: "b", Name: "b.mp3"}})
	if r.Root() != "/m" {
		t.Errorf("Root() = %q, want /m", r.Root())
	}
	if got := r.List(); len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("List() = %+v, want a then b (scan order)", got)
	}

	r.Replace("/n", []media.Item{{ID: "c"}})
	if _, err := r.Get("a"); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("Get(a) after replace error = %v, want ErrNotFound", err)
	}
}

func TestRepo_Get(t *testing.T) {
	r := New()
	r.Replace("/m", []media.Item{{ID: "a", Name: "a.mp4"}})

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
	r.Replace("/m", in)

	in[0].Name = "mutated-input"
	r.List()[0].Name = "mutated-output"

	got, _ := r.Get("a")
	if got.Name != "orig" || r.List()[0].Name != "orig" {
		t.Fatalf("internal state was mutated through a shared slice: %+v", got)
	}
}

// Run with -race: readers must never observe a torn library while it is replaced.
func TestRepo_ConcurrentReplaceAndRead(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				r.Replace("/m", []media.Item{{ID: media.ID(rune('a' + i))}})
			}
		}()
		go func() {
			defer wg.Done()
			for range 200 {
				_ = r.List()
				_, _ = r.Get("a")
				_ = r.Root()
			}
		}()
	}
	wg.Wait()
}
