package httpapi

import (
	"log/slog"
	"net/http"

	"eincast/internal/domain/media"
)

// Library is the slice of the library use cases the HTTP layer reads. Declared
// here, at the consumer, so handlers are tested against a fake.
type Library interface {
	List() []media.Item
	Get(id media.ID) (media.Item, error)
}

// api holds what handlers share. Handlers parse the request, call the library,
// and write the response; there is no business logic here.
type api struct {
	lib     Library
	access  Access
	remote  Remote
	thumbs  Thumbnails
	hls     Transcoding
	dlna    http.Handler // nil unless Smart TV discovery is on for this run
	version string
	name    string
	log     *slog.Logger
}

// routes is the single route table. Go 1.22 patterns give us method matching
// and path parameters without a router library (ADR 0002).
func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /static/{file}", a.handleStatic)
	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/library", a.handleLibrary)
	mux.HandleFunc("GET /api/library/{id}", a.handleLibraryItem)
	mux.HandleFunc("GET /stream/{id}", a.handleStream)
	mux.HandleFunc("GET /thumb/{id}", a.handleThumbnail)
	mux.HandleFunc("GET /hls/{id}/{file}", a.handleHLS)
	mux.HandleFunc("GET /subtitle/{id}/{track}", a.handleSubtitle)
	mux.HandleFunc("GET /playlist.m3u", a.handlePlaylist)
	mux.HandleFunc("POST /login", a.handleLogin)
	if a.dlna != nil {
		// No method in the pattern: UPnP eventing uses SUBSCRIBE and UNSUBSCRIBE.
		mux.Handle("/dlna/", a.dlna)
	}
	mux.HandleFunc("/", a.handleNotFound)
	return mux
}

// handler is the routes wrapped in the middleware chain, outermost first:
// request ID, Server header, security headers, access log, panic recovery,
// Host check, CORS, access control (LAN token or tunnel sign-in). The Host check
// precedes everything that reads data; CORS precedes access control so a
// browser's preflight (which cannot carry credentials) is answered before
// authentication is demanded.
func (a *api) handler() http.Handler {
	return chain(a.routes(),
		requestID,
		serverHeader(a.version),
		securityHeaders,
		accessLog(a.log),
		recoverer(a.log),
		a.hostGuard,
		corsLAN,
		a.requireAccess,
	)
}
