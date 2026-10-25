package app

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"time"

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

// ServeConfig is what a web server needs to start. It is the app's own type so
// the use case never touches net/http or its configuration.
type ServeConfig struct {
	// Port to listen on; 0 lets the OS pick a free one.
	Port int
	// Name is the human-readable server name.
	Name string
	// Hosts are extra local addresses to listen on. Loopback is always bound in
	// addition, because tunnels and the local UI connect through it. A server
	// must refuse unspecified addresses (0.0.0.0, ::): listening on every
	// interface would also expose VPN and container networks.
	Hosts []netip.Addr
}

// WebServer starts the HTTP interface that devices connect to.
type WebServer interface {
	// Start binds the socket and serves in the background. It returns once the
	// socket is bound, so "port already in use" is reported here, not later.
	// Cancelling ctx shuts the server down.
	Start(ctx context.Context, cfg ServeConfig) (RunningServer, error)
}

// RunningServer is one live run of a WebServer. A stopped server is not
// restarted; call WebServer.Start again for a fresh one.
type RunningServer interface {
	// Addrs lists every address actually bound, loopback first.
	Addrs() []netip.AddrPort
	// Stop shuts down gracefully, finishing in-flight requests until ctx
	// expires and then dropping the rest.
	Stop(ctx context.Context) error
	// Done is closed once serving has ended, for any reason.
	Done() <-chan struct{}
	// Err explains why serving ended. Only valid after Done is closed; nil
	// means a clean stop.
	Err() error
}

// LogAttr is one key/value pair of a structured log record, already rendered
// as text (the UI shows text, not typed values).
type LogAttr struct {
	Key   string
	Value string
}

// LogEntry is one log record as the UI sees it.
type LogEntry struct {
	// Seq increases by one per record, across the whole app run. Subscribers
	// use it to order lines, drop duplicates and spot gaps.
	Seq     uint64
	Time    time.Time
	Level   slog.Level
	Source  string // the component that logged ("http", "library", ...); "app" if untagged
	Message string
	Attrs   []LogAttr
}

// LogStream gives read access to the app's log: recent history for a late
// subscriber, and a live feed.
type LogStream interface {
	// History returns the most recent entries, oldest first.
	History() []LogEntry
	// Subscribe returns a live feed and a function that ends it. The feed
	// never blocks the logger: if the subscriber falls behind and its buffer
	// fills, entries are dropped for it (visible as a gap in Seq).
	Subscribe(buffer int) (live <-chan LogEntry, cancel func())
}

// NetInfo reports which local network addresses other devices can reach us on.
type NetInfo interface {
	// LANAddrs returns this machine's private IPv4 addresses on real network
	// interfaces (not loopback, VPN, container or virtual adapters), most
	// likely home network first.
	LANAddrs() ([]netip.Addr, error)
}

// Settings are the user's persisted preferences.
type Settings struct {
	// Port the server listens on.
	Port int
	// Name is the server's display name.
	Name string
	// LibraryRoot is the folder chosen last time, rescanned at startup.
	LibraryRoot string
	// RequireToken turns on the access token: devices must present it.
	RequireToken bool
}

// ErrNoSettings means nothing has been saved yet (first run). It is not a failure.
var ErrNoSettings = errors.New("no saved settings")

// SettingsStore persists Settings between runs. Secrets never go through it.
type SettingsStore interface {
	// Load returns ErrNoSettings (possibly wrapped) when nothing is saved. Any
	// other error means the saved data could not be used.
	Load() (Settings, error)
	Save(Settings) error
}

// ErrSecretNotFound means the secret does not exist. It is not a failure.
var ErrSecretNotFound = errors.New("secret not found")

// SecretStore keeps secrets (access tokens, later tunnel tokens) out of the
// settings file, in the operating system's credential store.
type SecretStore interface {
	// Get returns ErrSecretNotFound (possibly wrapped) when the key is absent.
	// Any other error means the store itself is unavailable.
	Get(key string) (string, error)
	Set(key, value string) error
	// Delete removes the key; deleting an absent key is not an error.
	Delete(key string) error
}

// LibraryMemory remembers which folder the user chose, so the next launch can
// open it without asking again.
type LibraryMemory interface {
	RememberRoot(root string) error
	// LastRoot is the remembered folder, or "" if none.
	LastRoot() string
}
