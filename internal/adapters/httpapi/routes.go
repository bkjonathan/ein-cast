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
	// Root is the library folder, used to confirm served files stay inside it.
	Root() string
}

// api holds what handlers share. Handlers parse the request, call the library,
// and write the response; there is no business logic here.
type api struct {
	lib     Library
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
	mux.HandleFunc("GET /playlist.m3u", a.handlePlaylist)
	mux.HandleFunc("/", a.handleNotFound)
	return mux
}

// handler is the routes wrapped in the middleware chain, outermost first:
// request ID, Server header, security headers, access log, panic recovery, CORS.
func (a *api) handler() http.Handler {
	return chain(a.routes(),
		requestID,
		serverHeader(a.version),
		securityHeaders,
		accessLog(a.log),
		recoverer(a.log),
		corsLAN,
	)
}
