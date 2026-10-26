// Package remote holds the remote-access use cases: turning a tunnel on and
// off, the access PIN, and the sign-ins and signed links that let people use
// the library through it.
//
// The rule that makes this safe lives here, not in the UI: Enable refuses a
// public provider while no PIN is set (ADR 0008).
package remote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

const (
	defaultStartTimeout = 60 * time.Second

	// stopTimeout bounds how long taking a tunnel down may take.
	stopTimeout = 10 * time.Second

	// defaultPollInterval is how often a running tunnel is asked how it is.
	defaultPollInterval = time.Second

	// Sign-in limits. One address gets a burst of loginBurst wrong guesses and
	// then one more per loginRefill. A second bucket covers everyone together,
	// so a botnet with a thousand addresses is slowed as well.
	loginBurst   = 5
	loginRefill  = time.Minute
	globalBurst  = 30
	globalRefill = 10 * time.Second
)

// Service owns remote access. Enable and Disable are serialized, and the
// lifecycle goes through the same state machine as the server, so clicking
// either at any moment is safe.
type Service struct {
	reg    app.TunnelRegistry
	events app.EventPublisher
	log    *slog.Logger

	now          func() time.Time
	startTimeout time.Duration
	pollInterval time.Duration

	pins   *pinStore
	creds  *credentials
	perIP  *limiter
	global *limiter
	cfg    *providerconfig.Store

	// setupMu guards needsSetup, which is read by every Status call and so is
	// cached: working it out reads the credential store.
	setupMu    sync.RWMutex
	needsSetup []app.ProviderID

	machine *server.Machine
	mu      sync.Mutex // serializes Enable, Disable and PIN changes
	gen     uint64     // counts runs, so a watcher can tell it belongs to an old one (guarded by mu)
	// lastLive is the provider status last published (guarded by mu).
	lastLive app.TunnelStatus

	// view is what the HTTP layer reads on every request. It has its own lock so
	// a tunnel that takes a minute to start never holds up a request.
	viewMu sync.RWMutex
	view   view
}

// view is the enabled tunnel, as request-time code needs to see it.
type view struct {
	tunnel   app.Tunnel
	provider app.ProviderID
	// public stays true from the moment a public tunnel begins starting until it
	// is confirmed stopped: failing closed means "might be reachable" counts.
	public bool
	url    app.PublicURL
	cancel context.CancelFunc // ends the tunnel's lifetime context
}

// Option customizes a Service.
type Option func(*Service)

// WithClock replaces the time source, for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Service) { s.now = now }
}

// WithStartTimeout sets how long a tunnel gets to report its address.
func WithStartTimeout(d time.Duration) Option {
	return func(s *Service) { s.startTimeout = d }
}

// WithPollInterval sets how often a running tunnel is checked for changes.
func WithPollInterval(d time.Duration) Option {
	return func(s *Service) { s.pollInterval = d }
}

// NewService builds a Service from its ports; call Load at startup.
func NewService(reg app.TunnelRegistry, secrets app.SecretStore, events app.EventPublisher, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		reg:          reg,
		events:       events,
		log:          log.With(slog.String("source", "remote")),
		now:          time.Now,
		startTimeout: defaultStartTimeout,
		pollInterval: defaultPollInterval,
		pins:         newPINStore(secrets),
		cfg:          providerconfig.New(secrets),
		machine:      server.NewMachine(),
	}
	for _, opt := range opts {
		opt(s)
	}
	s.creds = newCredentials(s.now)
	s.perIP = newLimiter(s.now, loginBurst, loginRefill)
	s.global = newLimiter(s.now, globalBurst, globalRefill)
	return s
}

var _ app.PublicAccess = (*Service)(nil)

// Load reads the saved PIN and works out which providers still need setting up.
// It never fails: if the credential store cannot be read, no PIN is set and
// public access stays refused, and providers that need values read as not set
// up. Both are the safe side.
func (s *Service) Load() {
	if err := s.pins.load(); err != nil {
		s.log.Warn("could not read the access PIN; public access stays off until one is set",
			slog.String("error", err.Error()))
	}
	s.refreshSetup()
}

// Providers lists the tunnel providers that can be enabled.
func (s *Service) Providers() []app.ProviderInfo { return s.reg.List() }

// Enable brings a tunnel up for the running server srv.
//
// A public provider needs an access PIN. This is checked before anything starts
// and cannot be bypassed by a UI that forgot to ask.
func (s *Service) Enable(ctx context.Context, id app.ProviderID, srv app.LocalServer) error {
	t, err := s.reg.Lookup(id)
	if err != nil {
		return fmt.Errorf("enable %q: %w", id, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// What the provider needs from the person comes first: without it nothing
	// else matters, and the message says what to add.
	if fields := fieldsOf(t); len(fields) > 0 {
		if err := s.cfg.Missing(id, fields); err != nil {
			return fmt.Errorf("enable %q: %w", id, err)
		}
	}
	if t.Public() && !s.pins.isSet() {
		return fmt.Errorf("enable %q: %w", id, app.ErrPINRequired)
	}
	if err := s.machine.Transition(server.Starting); err != nil {
		return fmt.Errorf("enable %q: %w", id, app.ErrAlreadyEnabled)
	}

	// New keys for every run: nobody carries a sign-in or a link over from an
	// earlier one.
	s.creds.rotate()
	life, cancel := context.WithCancel(ctx)
	s.setView(view{tunnel: t, provider: id, public: t.Public(), cancel: cancel})
	s.publish()

	// The start deadline cancels the tunnel's lifetime context rather than
	// wrapping Start in a timeout: the same context must outlive Start.
	timer := time.AfterFunc(s.startTimeout, cancel)
	url, err := t.Start(life, srv)
	if timedOut := !timer.Stop(); timedOut {
		err = fmt.Errorf("did not come up within %s", s.startTimeout)
	}
	if err == nil && url.Host() == "" {
		err = fmt.Errorf("provider returned an unusable address %q", url)
	}
	if err != nil {
		s.abort(ctx, t, cancel, err)
		return fmt.Errorf("enable %q: %w", id, err)
	}

	s.viewMu.Lock()
	s.view.url = url
	s.viewMu.Unlock()
	if err := s.machine.Transition(server.Running); err != nil {
		return fmt.Errorf("enable %q: %w", id, err) // unreachable: Starting → Running is legal
	}
	s.log.Info("remote access on",
		slog.String("provider", string(id)), slog.Bool("public", t.Public()), slog.String("url", string(url)))
	s.gen++
	s.lastLive = t.Status()
	// Owned by this run: it ends when life is cancelled, which Disable and a
	// failed stop-out do, and it ignores itself if a newer run has begun.
	go s.watch(life, t, s.gen)
	s.publish()
	return nil
}

// watch follows a running tunnel. A provider can change under us: a supervised
// process restarts, and a quick tunnel then has a new address; or it gives up.
// Neither is announced, so the tunnel is asked how it is.
func (s *Service) watch(life context.Context, t app.Tunnel, gen uint64) {
	tick := time.NewTicker(s.pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-life.Done():
			return
		case <-tick.C:
			s.observe(gen, t, t.Status())
		}
	}
}

// observe reacts to the provider's current status.
func (s *Service) observe(gen uint64, t app.Tunnel, live app.TunnelStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen || s.machine.Snapshot().State != server.Running {
		return // a newer run, or already being turned off
	}

	switch {
	case live.State == server.Failed:
		s.tunnelDied(t, live.Error)
	case live.State == server.Running && live.URL != "" && live.URL != s.currentView().url:
		// The address changed (the tunnel restarted). The Host check must accept
		// the new one, and the old one is gone.
		s.viewMu.Lock()
		s.view.url = live.URL
		s.viewMu.Unlock()
		s.log.Info("remote address changed", slog.String("url", string(live.URL)))
		s.lastLive = live
		s.publish()
	case live != s.lastLive:
		s.lastLive = live // reconnecting, or back
		s.publish()
	}
}

// tunnelDied handles a provider that gave up. It is told to stop, to be sure
// nothing is left running; only when that is confirmed does the library stop
// counting as public. Otherwise the view stays, so tunnel traffic is still held
// to the public rules, and the person can retry turning it off.
func (s *Service) tunnelDied(t app.Tunnel, reason string) {
	if reason == "" {
		reason = "the connection ended"
	}
	cause := errors.New(reason)

	stopCtx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	v := s.currentView()
	if err := t.Stop(stopCtx); err != nil {
		s.log.Warn("could not confirm the tunnel stopped", slog.String("error", err.Error()))
		s.fail(cause) // the view stays: Disable retries the stop
		return
	}
	if v.cancel != nil {
		v.cancel()
	}
	s.setView(view{})
	s.creds.rotate()
	s.fail(cause)
}

// abort cleans up after a tunnel that failed to start. The provider may have
// got halfway (a process spawned), so it is told to stop.
func (s *Service) abort(ctx context.Context, t app.Tunnel, cancel context.CancelFunc, cause error) {
	cancel()
	stopCtx, done := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer done()
	if err := t.Stop(stopCtx); err != nil {
		s.log.Warn("cleanup after failed start", slog.String("error", err.Error()))
	}
	s.setView(view{})
	s.creds.rotate()
	s.fail(cause)
}

// Disable takes the tunnel down and signs everyone out. Disabling when nothing
// is on is not an error, so callers can use it as "make sure it is off".
func (s *Service) Disable(ctx context.Context) error {
	// Interrupt a start in progress first, or the lock below waits for it.
	s.cancelLife()

	s.mu.Lock()
	defer s.mu.Unlock()

	was := s.machine.Snapshot().State
	if was == server.Idle {
		return nil
	}
	if was != server.Failed {
		if err := s.transition(server.Stopping); err != nil {
			return fmt.Errorf("disable: %w", err)
		}
	}

	if v := s.currentView(); v.tunnel != nil {
		stopCtx, cancel := context.WithTimeout(ctx, stopTimeout)
		defer cancel()
		v.cancel()
		if err := v.tunnel.Stop(stopCtx); err != nil {
			// Not confirmed stopped: leave the view as it is, so the server keeps
			// treating tunnel traffic as public, and let the user retry.
			if was != server.Failed {
				s.fail(err)
			}
			return fmt.Errorf("disable: stop %q: %w", v.provider, err)
		}
	}

	s.setView(view{})
	s.creds.rotate()
	s.log.Info("remote access off")
	return s.transition(server.Idle)
}

// Status returns remote access as it is now. It never waits on a start or stop.
func (s *Service) Status() app.RemoteStatus {
	snap := s.machine.Snapshot()
	v := s.currentView()
	st := app.RemoteStatus{
		State:    snap.State,
		Provider: v.provider,
		Public:   v.public,
		URL:      v.url,
		PINSet:   s.pins.isSet(),
		Sessions: s.creds.count(),

		NeedsSetup: s.setupNeeded(),
	}
	if snap.Err != nil {
		st.Error = snap.Err.Error()
	}
	if v.tunnel != nil && snap.State == server.Running {
		st.Live = v.tunnel.Status()
	}
	return st
}

// ProviderConfig returns a provider's settings and whether each is saved.
// Secrets are reported as set or not, never with their value.
func (s *Service) ProviderConfig(id app.ProviderID) ([]app.FieldState, error) {
	t, err := s.reg.Lookup(id)
	if err != nil {
		return nil, err
	}
	return s.cfg.States(id, fieldsOf(t))
}

// SetProviderField saves one setting of a provider. It is refused while that
// provider is on: the running tunnel would not pick the change up, and the
// screen would show something that is not in effect.
func (s *Service) SetProviderField(id app.ProviderID, key, raw string) error {
	return s.changeField(id, key, func(f app.ConfigField) error { return s.cfg.Set(id, f, raw) }, "saved")
}

// ClearProviderField removes one setting of a provider.
func (s *Service) ClearProviderField(id app.ProviderID, key string) error {
	return s.changeField(id, key, func(f app.ConfigField) error { return s.cfg.Clear(id, f) }, "removed")
}

func (s *Service) changeField(id app.ProviderID, key string, change func(app.ConfigField) error, what string) error {
	t, err := s.reg.Lookup(id)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(fieldsOf(t), func(f app.ConfigField) bool { return f.Key == key })
	if i < 0 {
		return fmt.Errorf("%s has no setting %q: %w", id, key, app.ErrUnknownField)
	}
	field := fieldsOf(t)[i]

	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.currentView(); v.provider == id && s.machine.Snapshot().State != server.Idle {
		return fmt.Errorf("%s: %w", field.Label, app.ErrProviderInUse)
	}
	if err := change(field); err != nil {
		return err
	}
	// Only the name of the setting is logged, never its value.
	s.log.Info("provider setting "+what, slog.String("provider", string(id)), slog.String("setting", key))
	s.refreshSetup()
	s.publish()
	return nil
}

// refreshSetup recomputes which providers cannot be turned on yet.
func (s *Service) refreshSetup() {
	var need []app.ProviderID
	for _, p := range s.reg.List() {
		if len(p.Fields) == 0 {
			continue
		}
		if err := s.cfg.Missing(p.ID, p.Fields); err != nil {
			if !errors.Is(err, app.ErrNotConfigured) {
				s.log.Warn("could not read a provider's settings",
					slog.String("provider", string(p.ID)), slog.String("error", err.Error()))
			}
			need = append(need, p.ID)
		}
	}
	s.setupMu.Lock()
	s.needsSetup = need
	s.setupMu.Unlock()
}

func (s *Service) setupNeeded() []app.ProviderID {
	s.setupMu.RLock()
	defer s.setupMu.RUnlock()
	return slices.Clone(s.needsSetup)
}

// fieldsOf is what a provider needs from the person, if it needs anything.
func fieldsOf(t app.Tunnel) []app.ConfigField {
	if c, ok := t.(app.Configurable); ok {
		return c.Fields()
	}
	return nil
}

// SetPIN sets or changes the access PIN. Changing it signs everyone out.
func (s *Service) SetPIN(pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.pins.set(pin); err != nil {
		return err
	}
	s.creds.rotate()
	s.log.Info("access PIN set")
	s.publish()
	return nil
}

// ClearPIN removes the PIN. It is refused while a public tunnel is on, since
// that would leave the library open to the Internet.
func (s *Service) ClearPIN() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentView().public {
		return fmt.Errorf("remove PIN: %w", app.ErrPublicActive)
	}
	if err := s.pins.clear(); err != nil {
		return err
	}
	s.log.Info("access PIN removed")
	s.publish()
	return nil
}

// The methods below are what the HTTP layer asks on every request. None of
// them block on a tunnel starting or stopping.

// PublicActive reports whether a public tunnel is, or might be, carrying
// traffic. The HTTP layer then requires sign-in for anything arriving through
// the loopback connection a tunnel uses.
func (s *Service) PublicActive() bool {
	s.viewMu.RLock()
	defer s.viewMu.RUnlock()
	return s.view.public
}

// HostAllowed reports whether host is the address of the online tunnel, so the
// Host-header check (which blocks DNS rebinding) lets its traffic through.
func (s *Service) HostAllowed(host string) bool {
	v := s.currentView()
	if v.url == "" {
		return false
	}
	return strings.TrimSuffix(strings.ToLower(host), ".") == v.url.Host()
}

// Login exchanges the PIN for a session. client identifies who is asking, for
// rate limiting. Wrong guesses cost the client attempts; a right one does not.
func (s *Service) Login(client, pin string) (app.RemoteSession, error) {
	if !s.PublicActive() {
		return app.RemoteSession{}, app.ErrNotPublic
	}
	if ok, wait := s.perIP.take(client); !ok {
		return app.RemoteSession{}, &app.LockedError{RetryAfter: wait}
	}
	if ok, wait := s.global.take(""); !ok {
		s.perIP.give(client)
		return app.RemoteSession{}, &app.LockedError{RetryAfter: wait}
	}

	if !s.pins.verify(pin) {
		return app.RemoteSession{}, app.ErrWrongPIN
	}

	s.perIP.reset(client)
	s.global.give("")
	sess := s.creds.issue(client)
	s.publish()
	return sess, nil
}

// ValidSession reports whether token is a live sign-in.
func (s *Service) ValidSession(token string) bool { return s.creds.validSession(token) }

// SignStream returns a link signature for one media item.
func (s *Service) SignStream(id media.ID) app.SignedLink { return s.creds.signLink(id) }

// VerifyStream reports whether sig authorises id until the unix time exp.
func (s *Service) VerifyStream(id media.ID, sig string, exp int64) bool {
	return s.creds.verifyLink(id, sig, exp)
}

func (s *Service) currentView() view {
	s.viewMu.RLock()
	defer s.viewMu.RUnlock()
	return s.view
}

func (s *Service) setView(v view) {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	s.view = v
}

// cancelLife ends the lifetime context of the tunnel being started or run.
func (s *Service) cancelLife() {
	if v := s.currentView(); v.cancel != nil {
		v.cancel()
	}
}

func (s *Service) transition(to server.State) error {
	if err := s.machine.Transition(to); err != nil {
		return err
	}
	s.publish()
	return nil
}

// fail records a failure. Fail only errors for an illegal source state, which
// would be a bug in this file, so it is logged rather than returned.
func (s *Service) fail(cause error) {
	if err := s.machine.Fail(cause); err != nil {
		s.log.Error("cannot record failure", slog.String("error", err.Error()))
		return
	}
	s.log.Error("remote access failed", slog.String("error", cause.Error()))
	s.publish()
}

func (s *Service) publish() {
	s.events.Publish(app.RemoteStatusChanged{Status: s.Status()})
}
