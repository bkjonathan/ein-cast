package app

import (
	"context"

	"eincast/internal/domain/media"
)

// Scanner walks a folder and reports the media files in it.
type Scanner interface {
	// Scan returns every servable file under root. It must honour ctx
	// cancellation, since walking a large library can take a while.
	Scan(ctx context.Context, root string) ([]media.Item, error)
}

// LibraryRepo holds the current library. It is a snapshot store: a scan
// replaces the contents wholesale.
type LibraryRepo interface {
	// Replace swaps in a new library atomically.
	Replace(root string, items []media.Item)
	// Root is the folder the current library was scanned from, or "" if none.
	Root() string
	// List returns the items in scan order. The slice is the caller's to keep.
	List() []media.Item
	// Get returns media.ErrNotFound (wrapped) for an unknown ID.
	Get(id media.ID) (media.Item, error)
}

// EventPublisher announces what happened to anyone who cares (today the UI).
// Publishing must never block or fail the caller: the backend does not know,
// and must not depend on, who is listening.
type EventPublisher interface {
	Publish(ev Event)
}
