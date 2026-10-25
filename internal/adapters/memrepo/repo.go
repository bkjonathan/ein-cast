// Package memrepo is an in-memory app.LibraryRepo. The library is rebuilt by
// a scan on every launch, so persistence would add nothing.
package memrepo

import (
	"fmt"
	"slices"
	"sync"

	"eincast/internal/domain/media"
)

// Repo is safe for concurrent use: HTTP handlers read while a rescan replaces
// the contents.
type Repo struct {
	mu    sync.RWMutex
	root  string
	items []media.Item
	byID  map[media.ID]media.Item
}

// New returns an empty repository.
func New() *Repo {
	return &Repo{byID: make(map[media.ID]media.Item)}
}

// Replace swaps the whole library under one lock, so readers see either the old
// or the new library, never a mix.
func (r *Repo) Replace(root string, items []media.Item) {
	byID := make(map[media.ID]media.Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	// Copy so later changes to the caller's slice cannot mutate our state.
	owned := slices.Clone(items)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.root, r.items, r.byID = root, owned, byID
}

// Root returns the scanned folder, or "" before the first Replace.
func (r *Repo) Root() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.root
}

// List returns a copy of the items in scan order.
func (r *Repo) List() []media.Item {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.items)
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
