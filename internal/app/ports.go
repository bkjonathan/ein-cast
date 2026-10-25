package app

import (
	"context"
	"log/slog"
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
	// Addr is the address actually bound, e.g. "127.0.0.1:8765".
	Addr() string
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
