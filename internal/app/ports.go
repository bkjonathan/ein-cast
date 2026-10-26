package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"eincast/internal/domain/media"
)

// Scanner walks a library's folder and reports the media files in it.
type Scanner interface {
	// Scan returns every servable file under lib.Root, stamped with the library's
	// ID and root. It must honour ctx cancellation, since walking a large library
	// can take a while.
	Scan(ctx context.Context, lib media.Library) ([]media.Item, error)
}

// FolderWatcher tells the app that something under a folder may have changed,
// so the library can follow the disk without the person pressing Rescan.
type FolderWatcher interface {
	// Watch starts watching root and everything under it. Each value on the
	// returned channel means "something may have changed, look again"; it carries
	// no detail, because the answer to "what" is a rescan. At most one signal is
	// pending at a time, so a burst of changes arrives as one. The channel is
	// closed when ctx ends or when watching fails for good. An error means
	// nothing is being watched (for example, the system's limit on watches was
	// reached); the library still works, it just needs manual rescans.
	Watch(ctx context.Context, root string) (<-chan struct{}, error)
}

// ErrNoThumbnail means an item has no preview image: it holds no picture to show
// (a song without cover art), or it could not be read as media.
var ErrNoThumbnail = errors.New("no thumbnail available")

// ErrThumbnailsUnavailable means this computer cannot make thumbnails at all,
// because the tool that does it (ffmpeg) is not installed.
var ErrThumbnailsUnavailable = errors.New("thumbnails need ffmpeg, which was not found")

// GrabRequest asks for one preview image.
type GrabRequest struct {
	// Root is the library folder; Path must resolve inside it.
	Root string
	// Path is the media file on disk.
	Path string
	Kind media.Kind
	// Width is the widest the image may be, in pixels. Smaller sources are not
	// enlarged.
	Width int
}

// FrameGrabber makes a preview image out of a media file: a frame of a video,
// the cover art of a song, a shrunken photo.
type FrameGrabber interface {
	// Available reports whether Grab can work at all on this computer.
	Available() bool
	// Grab returns a JPEG. It returns ErrNoThumbnail (wrapped) when the file has
	// nothing to show, and ErrThumbnailsUnavailable when it cannot run.
	Grab(ctx context.Context, req GrabRequest) ([]byte, error)
}

// ThumbnailCache keeps made thumbnails between runs, so each is made once.
type ThumbnailCache interface {
	// Get returns the thumbnail stored under key, if there is one.
	Get(key string) ([]byte, bool)
	// Put stores a thumbnail. A failure only means it will be made again.
	Put(key string, data []byte) error
}

// ErrTranscodingUnavailable means this computer cannot convert video, because
// the tool that does it (ffmpeg) is not installed.
var ErrTranscodingUnavailable = errors.New("converting video needs ffmpeg, which was not found")

// ErrNotConvertible means ffmpeg could not turn a file into a playable stream.
var ErrNotConvertible = errors.New("this video could not be converted")

// TranscodeRequest asks for one video to be converted for streaming.
type TranscodeRequest struct {
	// Root is the library folder; Path must resolve inside it.
	Root string
	// Path is the video file on disk.
	Path string
}

// HLSStream is one video being converted into an HLS stream (a playlist plus
// short segments) that Safari, iOS and most TVs play natively. The output
// appears as ffmpeg works; the playlist grows until the whole video is done.
type HLSStream interface {
	// Ready is closed once playback can start: the playlist and its first
	// segments exist, or the conversion has finished. It stays open if the
	// conversion fails first.
	Ready() <-chan struct{}
	// Done is closed when ffmpeg is no longer running, whether it finished,
	// failed or was stopped.
	Done() <-chan struct{}
	// Err explains why the conversion ended badly; nil means it finished or was
	// stopped on request. Only valid after Done is closed.
	Err() error
	// Open opens one output file by name: "index.m3u8" or a "seg00000.ts" segment.
	// Any other name is refused. It fails with fs.ErrNotExist when the file is
	// not there (yet, or any more).
	Open(name string) (io.ReadSeekCloser, error)
	// Stop ends the conversion and deletes its files. Safe to call more than once.
	Stop()
}

// Transcoder starts conversions.
type Transcoder interface {
	// Available reports whether conversions can run on this computer.
	Available() bool
	// Start begins converting and returns at once; the work continues in the
	// background until the stream ends or is stopped. ctx governs only this call,
	// not the conversion.
	Start(ctx context.Context, req TranscodeRequest) (HLSStream, error)
}

// LibraryRepo holds the items of every library. It is a snapshot store: a scan
// replaces one library's items wholesale.
type LibraryRepo interface {
	// Replace swaps in a library's items atomically, adding the library if it
	// was not there.
	Replace(id media.LibraryID, items []media.Item)
	// Remove forgets a library and its items. Removing an unknown one is not an
	// error.
	Remove(id media.LibraryID)
	// Items returns one library's items in scan order. The slice is the caller's.
	Items(id media.LibraryID) []media.Item
	// List returns every item: libraries in the order they were first added to
	// the repository, each in scan order. The slice is the caller's to keep.
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
	// DLNA turns on the Smart TV (DLNA/UPnP) pages, served under /dlna/. They
	// are open to the home network by nature, so the caller must not enable them
	// while the access token is required.
	DLNA bool
	// Hosts are extra local addresses to listen on. Loopback is always bound in
	// addition, because tunnels and the local UI connect through it. A server
	// must refuse unspecified addresses (0.0.0.0, ::): listening on every
	// interface would also expose VPN and container networks.
	Hosts []netip.Addr
}

// Announcement is what a running server tells the local network about itself.
type Announcement struct {
	// Name is the display name ("Living room").
	Name string
	Port int
	// Addrs are the LAN addresses the server listens on, never loopback.
	Addrs []netip.Addr
	// DLNA says whether Smart TV discovery is wanted.
	DLNA bool
}

// ErrNotAnnounced is returned by Announce when this kind of discovery is not
// wanted for this run. It is a choice, not a failure, and is not logged as one.
var ErrNotAnnounced = errors.New("this kind of discovery is not wanted")

// Discovery makes a running server findable on the local network without
// anyone typing its address: by name (mDNS, "eincast.local") or by TVs (SSDP).
// One implementation per protocol.
type Discovery interface {
	// Name is a short label for logs and the UI ("mdns", "dlna").
	Name() string
	// Announce makes the server findable and returns once it is. stop withdraws
	// it again and is safe to call more than once. urls are friendlier addresses
	// the announcement gives the server, such as http://eincast.local:8765, or
	// none. A failure here never stops the server: the address and QR code still
	// work.
	Announce(ctx context.Context, a Announcement) (stop func(), urls []string, err error)
}

// WebServer starts the HTTP interface that devices connect to.
type WebServer interface {
	// Start binds the socket and serves in the background. It returns once the
	// socket is bound, so "port already in use" is reported here, not later.
	// Cancelling ctx shuts the server down.
	Start(ctx context.Context, cfg ServeConfig) (RunningServer, error)
}

// LocalServer is a running web server as a tunnel provider sees it: where it
// listens, and a way to make it listen on one more address for as long as the
// provider needs.
type LocalServer interface {
	// Addrs lists every address actually bound, loopback first. All share one port.
	Addrs() []netip.AddrPort
	// Bind starts listening on addr too, on the server's port, and returns a
	// function that stops again and drops the connections that came in through
	// it. The function is safe to call more than once, and after the server has
	// stopped. A server must refuse unspecified addresses, as it does for Hosts.
	Bind(addr netip.Addr) (unbind func(), err error)
}

// ErrServerNotRunning means a tunnel was requested while the server is not up.
var ErrServerNotRunning = errors.New("the server is not running")

// RunningServer is one live run of a WebServer. A stopped server is not
// restarted; call WebServer.Start again for a fresh one.
type RunningServer interface {
	LocalServer
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
	// Libraries are the folders the person added, rescanned at startup.
	Libraries []media.Library
	// RequireToken turns on the access token: devices must present it.
	RequireToken bool
	// DLNA turns on Smart TV discovery. It is off by default and cannot be on
	// together with RequireToken: a TV cannot present a token, so the server
	// would either lock it out or show the library to the whole network.
	DLNA bool
	// AutoStart starts the server when EinCast opens, and right after the first
	// folder is added, so sharing needs no extra click.
	AutoStart bool
	// KeepAwake stops the computer sleeping while the server runs: a sleeping
	// laptop is a server nobody can reach.
	KeepAwake bool
	// OpenAtLogin starts EinCast when the person signs in to the computer.
	OpenAtLogin bool
	// HideOnClose keeps EinCast sharing when its window is closed. The window
	// comes back from the Dock or the tray, and Quit really stops it.
	HideOnClose bool
	// FFmpegPath is where the person said ffmpeg is. Empty means "look for it".
	FFmpegPath string
}

// PowerGuard keeps the computer from sleeping on its own while something needs
// it awake.
type PowerGuard interface {
	// Hold asks the system to stay awake. reason is shown to the person by the
	// system where it lists such requests. The returned release gives the request
	// back; it is safe to call more than once.
	Hold(reason string) (release func(), err error)
}

// LoginItem starts the app when the person signs in to the computer.
type LoginItem interface {
	// Set turns the login entry on or off. Turning off what is not on is not an
	// error.
	Set(enabled bool) error
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

// LibraryMemory remembers which folders the person added, so the next launch
// can open them without asking again.
type LibraryMemory interface {
	// Libraries returns the remembered libraries in the order they were added.
	Libraries() []media.Library
	// SaveLibraries replaces the remembered list.
	SaveLibraries([]media.Library) error
}

// PortHolder is a process that is listening on a TCP port.
type PortHolder struct {
	PID int
	// Name is the program's name as the operating system reports it
	// ("node", "Python"); empty when it could not be read.
	Name string
}

// ErrNotPermitted means the operating system refused to end a process, usually
// because it belongs to another user or to the system.
var ErrNotPermitted = errors.New("not allowed to end that process")

// PortInspector finds what is holding a TCP port and can end it. It is the one
// place that talks to the operating system's process tools (lsof, netstat).
type PortInspector interface {
	// Holders returns the processes listening on port, none if it is free. A
	// process that listens on both IPv4 and IPv6 appears once.
	Holders(ctx context.Context, port int) ([]PortHolder, error)
	// Terminate asks a process to quit, giving it the chance to clean up. On
	// Windows, which has no such request for console programs, it ends it.
	// It returns ErrNotPermitted (wrapped) when the system refuses.
	Terminate(pid int) error
	// Kill ends a process at once, without cleanup. Same errors as Terminate.
	Kill(pid int) error
}
