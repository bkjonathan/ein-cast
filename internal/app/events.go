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

// LibraryStatus says what the library scanner is doing, including why the last
// attempt failed, so the UI can show an error that survives a reload.
type LibraryStatus struct {
	Scanning bool
	// Root is the folder being scanned, or the last one attempted.
	Root string
	// Error is a short, user-facing reason the last scan failed; empty if it
	// did not.
	Error string
}

// LibraryStatusChanged is published when a scan starts, succeeds or fails.
type LibraryStatusChanged struct {
	Status LibraryStatus
}

func (LibraryStatusChanged) event() {}

// SettingsView is Settings as shown to the user, plus the access token.
type SettingsView struct {
	Settings
	// Token is the current access token; empty unless RequireToken is on.
	Token string
	// TokenPersisted is false when the token could not be saved to the system
	// credential store and will change on the next launch.
	TokenPersisted bool
}

// SettingsChanged is published after a settings change or a token rotation.
type SettingsChanged struct {
	View SettingsView
}

func (SettingsChanged) event() {}
