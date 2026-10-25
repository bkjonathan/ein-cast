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
	"time"

	"golang.org/x/sync/errgroup"

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
	version string
	log     *slog.Logger
}

var _ app.WebServer = (*Server)(nil)

// New builds a Server. version appears in the Server header and /api/health.
func New(lib Library, access Access, version string, log *slog.Logger) *Server {
	// Every line from this package carries source=http (the access log, panics, lifecycle).
	return &Server{lib: lib, access: access, version: version, log: log.With(slog.String("source", "http"))}
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
	r := &running{srv: srv, addrs: addrs, cancel: cancel, done: make(chan struct{}), log: s.log}

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
		cancel() // release runCtx when serving failed on its own
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
	return &api{lib: s.lib, access: s.access, version: s.version, name: cfg.Name, log: s.log}
}

// running is one live server. It implements app.RunningServer.
type running struct {
	srv    *http.Server
	addrs  []netip.AddrPort
	cancel context.CancelFunc
	log    *slog.Logger

	done chan struct{}
	err  error // written before done is closed, read only after
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
