package streaming

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

// stopTimeout bounds how long Stop waits for in-flight requests (a long video
// download would otherwise keep the app open forever).
const stopTimeout = 5 * time.Second

// Service owns the server lifecycle. Start and Stop are serialized by mu, and
// every change goes through the domain state machine, so clicking either at any
// time is safe: the loser of a race gets an invalid-transition error.
type Service struct {
	web    app.WebServer
	net    app.NetInfo
	events app.EventPublisher
	log    *slog.Logger
	public app.PublicAccess // nil when remote access is not wired in

	machine *server.Machine

	mu      sync.Mutex // serializes Start, Stop and the crash watcher
	current app.RunningServer
	gen     uint64 // identifies a run, so a stale watcher can recognise itself

	infoMu sync.RWMutex // guards info so Status never waits on mu
	info   endpoints
}

// endpoints is what the UI needs to show how to connect.
type endpoints struct {
	name     string
	port     int
	localURL string
	lanURLs  []string
}

// ServiceOption customizes a Service.
type ServiceOption func(*Service)

// WithPublicAccess makes the server withdraw public access before it stops or
// after it dies, so a tunnel never outlives the server it points at.
func WithPublicAccess(p app.PublicAccess) ServiceOption {
	return func(s *Service) { s.public = p }
}

// NewService builds a Service from its ports.
func NewService(web app.WebServer, net app.NetInfo, events app.EventPublisher, log *slog.Logger, opts ...ServiceOption) *Service {
	s := &Service{web: web, net: net, events: events, log: log.With(slog.String("source", "server")), machine: server.NewMachine()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start launches the server. ctx bounds the server's whole lifetime: cancelling
// it (app shutdown) stops the server, so pass a long-lived context, not a
// per-request one.
func (s *Service) Start(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("start: invalid config: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.transition(server.Starting); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	s.setInfo(endpoints{name: cfg.Name})

	run, err := s.web.Start(ctx, app.ServeConfig{Port: cfg.Port, Name: cfg.Name, Hosts: s.lanHosts()})
	if err != nil {
		s.fail(err)
		return fmt.Errorf("start server: %w", err)
	}

	s.current = run
	s.gen++
	ep := describe(cfg.Name, run.Addrs())
	s.setInfo(ep)
	if err := s.transition(server.Running); err != nil {
		return fmt.Errorf("start: %w", err) // unreachable: Starting → Running is legal
	}

	s.log.Info("server started",
		slog.String("name", cfg.Name),
		slog.Int("port", ep.port),
		slog.Any("urls", append([]string{ep.localURL}, ep.lanURLs...)),
	)
	go s.watch(run, s.gen)
	return nil
}

// Stop shuts the server down gracefully. Stopping a Failed server just clears
// the failure; stopping an Idle one is an invalid transition.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.machine.Snapshot().State == server.Failed {
		return s.transitionIdle()
	}
	if err := s.transition(server.Stopping); err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	run := s.current
	s.current = nil // from here a finishing run is expected, not a crash

	stopCtx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()
	s.withdrawPublicAccess(stopCtx) // the tunnel goes first, then the server behind it
	if run != nil {
		if err := run.Stop(stopCtx); err != nil {
			s.clearEndpoints()
			s.fail(err)
			return fmt.Errorf("stop server: %w", err)
		}
	}

	s.log.Info("server stopped")
	return s.transitionIdle()
}

// Local hands out the running server so a tunnel can attach to it. It fails
// with app.ErrServerNotRunning in every state but Running.
func (s *Service) Local() (app.LocalServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.machine.Snapshot().State != server.Running || s.current == nil {
		return nil, app.ErrServerNotRunning
	}
	return s.current, nil
}

// Status returns the current lifecycle state. It never blocks on a running
// Start or Stop.
func (s *Service) Status() app.ServerStatus {
	snap := s.machine.Snapshot()

	s.infoMu.RLock()
	status := app.ServerStatus{
		State:    snap.State,
		Name:     s.info.name,
		Port:     s.info.port,
		LocalURL: s.info.localURL,
		LANURLs:  slices.Clone(s.info.lanURLs),
	}
	s.infoMu.RUnlock()

	if snap.Err != nil {
		status.Error = snap.Err.Error()
	}
	return status
}

// watch is the single goroutine per run. It exits when the run ends, and turns
// an unexpected end (crash) into Failed. After a normal Stop, current is nil or
// belongs to a newer run, so it does nothing.
func (s *Service) watch(run app.RunningServer, gen uint64) {
	<-run.Done()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen || s.current == nil {
		return
	}
	s.current = nil

	cause := run.Err()
	if cause == nil {
		cause = errors.New("server stopped unexpectedly")
	}
	s.log.Error("server died", slog.String("error", cause.Error()))
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	s.withdrawPublicAccess(ctx)
	s.clearEndpoints()
	s.fail(cause)
}

// withdrawPublicAccess takes any tunnel down. A failure is logged, not returned:
// the server must still stop, and a stopped server leaves a tunnel pointing at
// nothing.
func (s *Service) withdrawPublicAccess(ctx context.Context) {
	if s.public == nil {
		return
	}
	if err := s.public.Disable(ctx); err != nil {
		s.log.Error("could not turn off remote access", slog.String("error", err.Error()))
	}
}

func (s *Service) transition(to server.State) error {
	if err := s.machine.Transition(to); err != nil {
		return err
	}
	s.publish()
	return nil
}

func (s *Service) transitionIdle() error {
	s.clearEndpoints()
	if err := s.transition(server.Idle); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}

// fail records a failure. Fail only errors for an illegal source state, which
// would be a bug in this file, so it is logged rather than returned.
func (s *Service) fail(cause error) {
	if err := s.machine.Fail(cause); err != nil {
		s.log.Error("cannot record failure", slog.String("error", err.Error()))
		return
	}
	s.publish()
}

func (s *Service) publish() {
	s.events.Publish(app.ServerStateChanged{Status: s.Status()})
}

func (s *Service) setInfo(ep endpoints) {
	s.infoMu.Lock()
	defer s.infoMu.Unlock()
	s.info = ep
}

// clearEndpoints forgets how to connect but remembers the server's name.
func (s *Service) clearEndpoints() {
	s.infoMu.Lock()
	defer s.infoMu.Unlock()
	s.info = endpoints{name: s.info.name}
}

// lanHosts asks the network for this machine's LAN addresses. Failing to find
// any is not fatal: the server still starts on loopback, and the UI says that
// no network was found.
func (s *Service) lanHosts() []netip.Addr {
	addrs, err := s.net.LANAddrs()
	if err != nil {
		s.log.Warn("could not detect LAN addresses; serving on this computer only",
			slog.String("error", err.Error()))
		return nil
	}
	return addrs
}

// describe turns the addresses a run actually bound into connection URLs:
// loopback becomes the local URL, everything else a LAN URL, in bind order.
func describe(name string, addrs []netip.AddrPort) endpoints {
	ep := endpoints{name: name}
	for _, ap := range addrs {
		if ep.port == 0 {
			ep.port = int(ap.Port())
		}
		url := "http://" + ap.String()
		switch {
		case ap.Addr().IsLoopback():
			if ep.localURL == "" {
				ep.localURL = url
			}
		default:
			ep.lanURLs = append(ep.lanURLs, url)
		}
	}
	return ep
}
