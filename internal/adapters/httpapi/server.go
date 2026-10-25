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
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"eincast/internal/app"
)

const (
	// bindHost is loopback until the Connect phase adds the LAN interface
	// addresses. Never 0.0.0.0: that would also expose every VPN and docker
	// interface.
	bindHost = "127.0.0.1"

	// shutdownGrace bounds the shutdown that follows a cancelled parent context.
	// An explicit Stop uses the caller's own deadline instead.
	shutdownGrace = 5 * time.Second
)

// Server starts HTTP servers for the library. It implements app.WebServer.
type Server struct {
	lib     Library
	version string
	log     *slog.Logger
}

var _ app.WebServer = (*Server)(nil)

// New builds a Server. version appears in the Server header and /api/health.
func New(lib Library, version string, log *slog.Logger) *Server {
	// Every line from this package carries source=http (the access log, panics, lifecycle).
	return &Server{lib: lib, version: version, log: log.With(slog.String("source", "http"))}
}

// Start binds the port and serves in the background.
func (s *Server) Start(ctx context.Context, cfg app.ServeConfig) (app.RunningServer, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(bindHost, strconv.Itoa(cfg.Port)))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d: %w", cfg.Port, err)
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
	r := &running{srv: srv, addr: ln.Addr().String(), cancel: cancel, done: make(chan struct{}), log: s.log}

	// The errgroup owns both goroutines. Each ends when runCtx is cancelled
	// (Stop or parent cancel) or when serving fails.
	g, gctx := errgroup.WithContext(runCtx)
	g.Go(func() error {
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	})
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

	s.log.Info("http server listening", slog.String("addr", r.addr), slog.String("name", cfg.Name))
	return r, nil
}

// newAPI builds the handler state for one run.
func (s *Server) newAPI(cfg app.ServeConfig) *api {
	return &api{lib: s.lib, version: s.version, name: cfg.Name, log: s.log}
}

// running is one live server. It implements app.RunningServer.
type running struct {
	srv    *http.Server
	addr   string
	cancel context.CancelFunc
	log    *slog.Logger

	done chan struct{}
	err  error // written before done is closed, read only after
}

func (r *running) Addr() string          { return r.addr }
func (r *running) Done() <-chan struct{} { return r.done }
func (r *running) Err() error            { return r.err }

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
