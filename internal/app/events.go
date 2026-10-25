package app

import (
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

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

// ServerStatus describes the HTTP server at one instant.
type ServerStatus struct {
	State server.State
	// Name is the configured server name; empty until the first start.
	Name string
	// Port is the port in use while Running; 0 otherwise.
	Port int
	// LocalURL reaches the server from this computer, e.g. "http://127.0.0.1:8765".
	LocalURL string
	// LANURLs reach it from other devices on the network, most likely first.
	// Empty when the machine has no usable network.
	LANURLs []string
	// Error is the failure reason while Failed; empty otherwise.
	Error string
}

// ServerStateChanged is published on every server lifecycle transition.
type ServerStateChanged struct {
	Status ServerStatus
}

func (ServerStateChanged) event() {}
