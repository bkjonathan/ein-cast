package app

import (
	"time"

	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

// Event is something that happened in the app. The unexported method seals the
// set: only this package defines events, so a type switch over them is
// exhaustive by review.
type Event interface {
	event()
}

// LibraryState is one library as the UI shows it: what it is, what is in it, and
// what is happening to it.
type LibraryState struct {
	Library media.Library
	// Stats counts the library's items; zero until the first scan finishes.
	Stats media.Stats
	// Scanning is true while the library is being scanned for a person's request.
	// Quiet rescans after a change on disk do not set it.
	Scanning bool
	// Error is a short, user-facing reason the last scan failed; empty if it did
	// not. The previous contents are kept meanwhile.
	Error string
	// Watching is true while the folder is watched for changes, so the library
	// follows the disk without a manual rescan.
	Watching bool
}

// LibraryScanned is published when what is in the libraries changed: a scan
// finished with a different result, or a library was added or removed. It
// carries every library's state, so a listener never has to piece the answer
// together from several events.
type LibraryScanned struct {
	Libraries []LibraryState
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
	// NameURLs are friendlier addresses announced on the network, such as
	// http://eincast.local:8765. Empty when nothing announced one.
	NameURLs []string
	// Discovery lists the kinds of discovery that are active ("mdns", "dlna").
	Discovery []string
	// Error is the failure reason while Failed; empty otherwise.
	Error string
}

// ServerStateChanged is published on every server lifecycle transition.
type ServerStateChanged struct {
	Status ServerStatus
}

func (ServerStateChanged) event() {}

// LibraryStatusChanged is published when a scan starts, succeeds or fails, when
// watching a folder starts or stops, and when a library is renamed. Like
// LibraryScanned it carries every library's state.
type LibraryStatusChanged struct {
	Libraries []LibraryState
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

// RemoteStatusChanged is published when remote access turns on or off, fails,
// or its PIN or sessions change.
type RemoteStatusChanged struct {
	Status RemoteStatus
}

func (RemoteStatusChanged) event() {}

// ProfilesChanged is published after a profile is added, changed or removed, or
// its PIN is set or cleared. It carries no PINs or hashes.
type ProfilesChanged struct {
	Profiles []ProfileEntry
}

func (ProfilesChanged) event() {}

// UpdateStatus describes the version situation at one instant.
type UpdateStatus struct {
	// Current is the version running now, as it was built ("dev" for a build from
	// source, which has nothing to compare).
	Current string
	// Checking is true while a check is running.
	Checking bool
	// Available is true when the newest release seen is newer than Current.
	Available bool
	// Latest is the newest release the last successful check saw, newer or not.
	// It is kept when a later check fails, so a notice does not vanish with the
	// Wi-Fi. Nil until a check succeeds.
	Latest *UpdateRelease
	// CheckedAt is when the last check succeeded; zero until then.
	CheckedAt time.Time
	// Error is why the last check failed, in words for the person; empty if it
	// did not.
	Error string
}

// UpdateRelease is a release as the UI shows it: its version without the "v",
// and only the files for this computer.
type UpdateRelease struct {
	Version     string
	Name        string
	Notes       string
	URL         string
	PublishedAt time.Time
	Assets      []ReleaseAsset
}

// UpdateChanged is published when a check starts or ends.
type UpdateChanged struct {
	Status UpdateStatus
}

func (UpdateChanged) event() {}
