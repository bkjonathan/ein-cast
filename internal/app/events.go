package app

import "eincast/internal/domain/media"

// Event is something that happened in the app. The unexported method seals the
// set: only this package defines events, so a type switch over them is
// exhaustive by review.
type Event interface {
	event()
}

// LibrarySummary describes the library currently loaded.
type LibrarySummary struct {
	Root  string
	Stats media.Stats
}

// LibraryScanned is published after every successful scan.
type LibraryScanned struct {
	Summary LibrarySummary
}

func (LibraryScanned) event() {}
