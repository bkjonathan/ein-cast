// Package memrepo is an in-memory app.LibraryRepo. The libraries are rebuilt by
// a scan on every launch, so persistence would add nothing.
package memrepo

import (
	"fmt"
	"slices"
	"sync"

	"eincast/internal/domain/media"
)

// Repo is safe for concurrent use: HTTP handlers read while a rescan replaces
// one library's contents.
type Repo struct {
	mu    sync.RWMutex
	order []media.LibraryID // libraries in the order they first appeared
	items map[media.LibraryID][]media.Item
	byID  map[media.ID]media.Item
}

// New returns an empty repository.
func New() *Repo {
	return &Repo{items: map[media.LibraryID][]media.Item{}, byID: map[media.ID]media.Item{}}
}

// Replace swaps one library's items under one lock, so readers see either the
// old or the new contents of it, never a mix.
func (r *Repo) Replace(id media.LibraryID, items []media.Item) {
	// Copy so later changes to the caller's slice cannot mutate our state.
	owned := slices.Clone(items)

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.items[id] {
		delete(r.byID, old.ID)
	}
	if _, known := r.items[id]; !known {
		r.order = append(r.order, id)
	}
	r.items[id] = owned
	for _, it := range owned {
		r.byID[it.ID] = it
	}
}

// Remove forgets a library and its items.
func (r *Repo) Remove(id media.LibraryID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.items[id] {
		delete(r.byID, old.ID)
	}
	delete(r.items, id)
	r.order = slices.DeleteFunc(r.order, func(o media.LibraryID) bool { return o == id })
}

// Items returns a copy of one library's items in scan order.
func (r *Repo) Items(id media.LibraryID) []media.Item {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.items[id])
}

// List returns a copy of every item: libraries in the order they first appeared,
// each in scan order.
func (r *Repo) List() []media.Item {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []media.Item
	for _, id := range r.order {
		out = append(out, r.items[id]...)
	}
	return out
}

// Get returns the item with the given ID, or an error wrapping media.ErrNotFound.
func (r *Repo) Get(id media.ID) (media.Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	it, ok := r.byID[id]
	if !ok {
		return media.Item{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
	}
	return it, nil
}
