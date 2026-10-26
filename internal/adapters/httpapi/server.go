// Package httpapi is the HTTP adapter: the server lifecycle, the route table,
// middleware and handlers that devices on the network talk to.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"eincast/internal/adapters/dlna"
	"eincast/internal/app"
)

const (
	// loopbackHost is always bound, in addition to any LAN addresses.
	loopbackHost = "127.0.0.1"

	// shutdownGrace bounds the shutdown that follows a cancelled parent context.
	// An explicit Stop uses the caller's own deadline instead.
	shutdownGrace = 5 * time.Second
)

// Server starts HTTP servers for the library. It implements app.WebServer.
type Server struct {
	lib     Library
	access  Access
	remote  Remote
	thumbs  Thumbnails  // nil: no thumbnails are offered
	hls     Transcoding // nil: no conversion is offered
	dlnaID  string      // the server's UPnP identity; empty: no DLNA pages
	version string
	log     *slog.Logger
}

var _ app.WebServer = (*Server)(nil)

// Option customises a Server.
type Option func(*Server)

// WithThumbnails serves preview images at /thumb/{id}. Without it that route
// answers "not found" and the player page shows no pictures.
func WithThumbnails(t Thumbnails) Option {
	return func(s *Server) { s.thumbs = t }
}

// WithDLNA lets the server answer smart TVs under /dlna/ when a run asks for it
// (ServeConfig.DLNA), identifying itself as uuid on the network.
func WithDLNA(uuid string) Option {
	return func(s *Server) { s.dlnaID = uuid }
}

// WithTranscoding serves converted (HLS) streams at /hls/{id}/…. Without it that
// route answers "not found" and the player page only plays files as they are.
func WithTranscoding(t Transcoding) Option {
	return func(s *Server) { s.hls = t }
}

// New builds a Server. version appears in the Server header and /api/health.
func New(lib Library, access Access, remote Remote, version string, log *slog.Logger, opts ...Option) *Server {
	// Every line from this package carries source=http (the access log, panics, lifecycle).
	s := &Server{lib: lib, access: access, remote: remote, version: version, log: log.With(slog.String("source", "http"))}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start binds the port on loopback plus every address in cfg.Hosts and serves
// in the background.
//
// Binding is explicit, one listener per address. There is no 0.0.0.0 option:
// that would also expose VPN, container and virtual interfaces. Loopback is
// always included because the local UI and, later, tunnels reach us through it.
func (s *Server) Start(ctx context.Context, cfg app.ServeConfig) (app.RunningServer, error) {
	listeners, err := listenAll(ctx, bindAddrs(cfg.Hosts), cfg.Port)
	if err != nil {
		return nil, err
	}

	a := s.newAPI(cfg)
	srv := &http.Server{
		Handler: a.handler(),
		// Slowloris protection. There is deliberately no WriteTimeout: it would
		// cut off long video streams.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}

	runCtx, cancel := context.WithCancel(ctx)
	addrs := make([]netip.AddrPort, len(listeners))
	for i, ln := range listeners {
		addrs[i] = ln.Addr().(*net.TCPAddr).AddrPort()
	}
	r := &running{srv: srv, addrs: addrs, cancel: cancel, done: make(chan struct{}), log: s.log,
		extras: map[netip.Addr]*extraListener{}}
	// Set before any Serve starts, so the hook is never raced.
	srv.ConnState = r.trackConn

	// The errgroup owns every goroutine: one Serve per listener plus the
	// shutdown watcher. All end when runCtx is cancelled (Stop or parent
	// cancel) or when any listener fails.
	g, gctx := errgroup.WithContext(runCtx)
	for _, ln := range listeners {
		g.Go(func() error {
			if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("serve %s: %w", ln.Addr(), err)
			}
			return nil
		})
	}
	g.Go(func() error {
		<-gctx.Done()
		graceCtx, stop := context.WithTimeout(context.Background(), shutdownGrace)
		defer stop()
		if err := srv.Shutdown(graceCtx); err != nil {
			_ = srv.Close()
		}
		return nil
	})
	go func() {
		r.err = g.Wait()
		r.closeExtras() // listeners added by Bind end with the run
		cancel()        // release runCtx when serving failed on its own
		close(r.done)
	}()

	s.log.Info("http server listening",
		slog.String("name", cfg.Name), slog.Any("addrs", addrStrings(addrs)))
	return r, nil
}

// bindAddrs is loopback first, then the requested hosts without duplicates.
func bindAddrs(hosts []netip.Addr) []netip.Addr {
	out := []netip.Addr{netip.MustParseAddr(loopbackHost)}
	for _, h := range hosts {
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

// listenAll opens one listener per address on the same port. With port 0 the
// first listener lets the OS choose and the rest reuse that port, so every
// address ends up on one number. If any bind fails, the ones already opened are
// closed: the server either starts completely or not at all.
func listenAll(ctx context.Context, hosts []netip.Addr, port int) ([]net.Listener, error) {
	var lc net.ListenConfig
	var opened []net.Listener
	closeAll := func() {
		for _, ln := range opened {
			_ = ln.Close()
		}
	}

	for _, h := range hosts {
		if h.IsUnspecified() {
			closeAll()
			return nil, fmt.Errorf("refusing to listen on %s: bind explicit addresses, not every interface", h)
		}
		ln, err := lc.Listen(ctx, "tcp", netip.AddrPortFrom(h, uint16(port)).String())
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("listen on %s port %d: %w", h, port, err)
		}
		opened = append(opened, ln)
		if port == 0 {
			port = ln.Addr().(*net.TCPAddr).Port
		}
	}
	return opened, nil
}

func addrStrings(addrs []netip.AddrPort) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.String()
	}
	return out
}

// newAPI builds the handler state for one run.
func (s *Server) newAPI(cfg app.ServeConfig) *api {
	a := &api{lib: s.lib, access: s.access, remote: s.remote, thumbs: s.thumbs, hls: s.hls, version: s.version, name: cfg.Name, log: s.log}
	if cfg.DLNA && s.dlnaID != "" {
		a.dlna = dlna.NewHandler(dlna.Config{Library: s.lib, Name: cfg.Name, UUID: s.dlnaID, Version: s.version})
	}
	return a
}

// running is one live server. It implements app.RunningServer.
type running struct {
	srv    *http.Server
	addrs  []netip.AddrPort
	cancel context.CancelFunc
	log    *slog.Logger

	done chan struct{}
	err  error // written before done is closed, read only after

	// mu guards the listeners added after start by Bind. They are kept apart
	// from the ones Start opened because they come and go while the server runs.
	mu     sync.Mutex
	closed bool
	extras map[netip.Addr]*extraListener
	wg     sync.WaitGroup // one Serve goroutine per extra listener
}

// extraListener is a listener added by Bind and the connections accepted on it.
type extraListener struct {
	ln    net.Listener
	conns map[net.Conn]struct{}
}

var _ app.RunningServer = (*running)(nil)

// Bind listens on addr as well, on the server's port, until the returned
// function is called or the server ends. This is how a tunnel that is reached
// directly (Tailscale) gets its address, without the server listening on it the
// rest of the time. Like Start, it refuses unspecified addresses.
func (r *running) Bind(addr netip.Addr) (func(), error) {
	addr = addr.Unmap()
	switch {
	case !addr.IsValid():
		return nil, errors.New("bind: invalid address")
	case addr.IsUnspecified():
		return nil, fmt.Errorf("refusing to listen on %s: bind explicit addresses, not every interface", addr)
	case addr.IsLoopback() || r.listensOn(addr):
		return nil, fmt.Errorf("bind %s: already listening there", addr)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("bind: the server is stopping")
	}
	if _, dup := r.extras[addr]; dup {
		return nil, fmt.Errorf("bind %s: already listening there", addr)
	}
	ap := netip.AddrPortFrom(addr, r.addrs[0].Port())
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", ap.String())
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", ap, err)
	}
	r.extras[addr] = &extraListener{ln: ln, conns: map[net.Conn]struct{}{}}

	// Owned by r.wg; ends when the listener is closed, by unbind or by the
	// server shutting down.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := r.srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			r.log.Warn("extra listener failed", slog.String("addr", ap.String()), errAttr(err))
		}
	}()
	r.log.Info("listening on an extra address", slog.String("addr", ap.String()))

	var once sync.Once
	return func() { once.Do(func() { r.unbind(addr) }) }, nil
}

func (r *running) listensOn(addr netip.Addr) bool {
	return slices.ContainsFunc(r.addrs, func(ap netip.AddrPort) bool { return ap.Addr() == addr })
}

// unbind stops listening on addr and drops what came in through it: turning a
// tunnel off must also end the streams already open over it.
func (r *running) unbind(addr netip.Addr) {
	r.mu.Lock()
	e := r.extras[addr]
	delete(r.extras, addr)
	r.mu.Unlock()
	if e == nil {
		return
	}
	r.log.Info("stopped listening on an extra address", slog.String("addr", addr.String()))
	r.closeListener(e)
}

// closeListener closes outside r.mu: closing a connection triggers ConnState,
// which takes the lock.
func (r *running) closeListener(e *extraListener) {
	_ = e.ln.Close()
	r.mu.Lock()
	conns := slices.Collect(func(yield func(net.Conn) bool) {
		for c := range e.conns {
			if !yield(c) {
				return
			}
		}
	})
	r.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// closeExtras ends every extra listener and waits for their Serve goroutines.
// After it, Bind refuses.
func (r *running) closeExtras() {
	r.mu.Lock()
	r.closed = true
	extras := slices.Collect(func(yield func(*extraListener) bool) {
		for _, e := range r.extras {
			if !yield(e) {
				return
			}
		}
	})
	clear(r.extras)
	r.mu.Unlock()

	for _, e := range extras {
		r.closeListener(e)
	}
	r.wg.Wait()
}

// trackConn remembers which connections arrived on an extra listener, by the
// local address they were accepted on, so unbind can drop them. Connections on
// the addresses Start opened are not tracked.
//
// Tracking uses http.Server.ConnState rather than wrapping the listener's
// connections: a wrapper would hide the TCP connection's ReadFrom and cost
// large file downloads their zero-copy sendfile path.
func (r *running) trackConn(c net.Conn, state http.ConnState) {
	tcp, ok := c.LocalAddr().(*net.TCPAddr)
	if !ok {
		return
	}
	local, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.extras[local.Unmap()]
	if e == nil {
		return
	}
	switch state {
	case http.StateNew:
		e.conns[c] = struct{}{}
	case http.StateClosed, http.StateHijacked:
		delete(e.conns, c)
	}
}

func (r *running) Addrs() []netip.AddrPort { return slices.Clone(r.addrs) }
func (r *running) Done() <-chan struct{}   { return r.done }
func (r *running) Err() error              { return r.err }

// Stop shuts down gracefully until ctx expires, then drops remaining
// connections. Dropping them is the intended outcome of the deadline, not a
// failure; only a failure to close at all is returned.
func (r *running) Stop(ctx context.Context) error {
	err := r.srv.Shutdown(ctx)
	if err != nil {
		r.log.Warn("graceful shutdown incomplete; closing remaining connections",
			slog.String("reason", err.Error()))
		err = r.srv.Close()
	}
	r.cancel()
	<-r.done
	if err != nil {
		return fmt.Errorf("close server: %w", err)
	}
	return nil
}
