package cloudflared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"eincast/internal/adapters/tunnel/procsup"
	"eincast/internal/app"
	"eincast/internal/domain/server"
)

// tailLines is how many recent lines of output are kept to explain a failure.
const tailLines = 5

// stopWait bounds cleanup after a failed start.
const stopWait = 10 * time.Second

// launch is how one kind of Cloudflare tunnel is run: the command, and how to
// tell from its output that the tunnel is up.
type launch struct {
	args []string
	// env adds variables for the process. A token goes here, not in args: a
	// command line is visible to every user of the machine.
	env []string
	// secrets are scrubbed from the process's output.
	secrets []string
	// ready reads one line of output and reports the tunnel's address once it is
	// up. It is called for lines until it first succeeds in each run.
	ready func(line string) (app.PublicURL, bool)
}

// engine runs cloudflared under a procsup.Supervisor and keeps a Status. Quick
// and Named are this with different commands.
type engine struct {
	id     app.ProviderID
	locate func() (string, error)
	// prepare builds the command for one start. local is the loopback "host:port"
	// of the server.
	prepare func(local string) (launch, error)
	log     *slog.Logger
	// tune adjusts the supervisor's settings; tests use it to shorten the backoff.
	tune func(*procsup.Spec)

	mu     sync.Mutex
	sup    *procsup.Supervisor
	status app.TunnelStatus
	ready  func(string) (app.PublicURL, bool) // this run's detector
	seen   bool                               // this run has reported its address
	tail   []string                           // the last few lines of output
	first  chan app.PublicURL
}

func newEngine(id app.ProviderID, locate func() (string, error), prepare func(local string) (launch, error), log *slog.Logger) *engine {
	return &engine{
		id: id, locate: locate, prepare: prepare,
		log: log.With(slog.String("source", "tunnel"), slog.String("provider", string(id))),
	}
}

// ID implements app.Tunnel.
func (e *engine) ID() app.ProviderID { return e.id }

// Public implements app.Tunnel. Both Cloudflare tunnels are open to the whole
// Internet and so demand a PIN.
func (*engine) Public() bool { return true }

// Start runs cloudflared and returns the address once it is up. ctx is the
// tunnel's lifetime: when it ends, cloudflared is stopped.
func (e *engine) Start(ctx context.Context, srv app.LocalServer) (app.PublicURL, error) {
	e.mu.Lock()
	if e.sup != nil {
		e.mu.Unlock()
		return "", errors.New("the Cloudflare tunnel is already on")
	}
	e.status, e.seen, e.tail = app.TunnelStatus{State: server.Starting}, false, nil
	first := make(chan app.PublicURL, 1)
	e.first = first
	e.mu.Unlock()

	bin, err := e.locate()
	if err != nil {
		return "", e.fail(err)
	}
	local, err := app.LoopbackAddr(srv)
	if err != nil {
		return "", e.fail(err)
	}
	l, err := e.prepare(local)
	if err != nil {
		return "", e.fail(err)
	}
	e.mu.Lock()
	e.ready = l.ready
	e.mu.Unlock()

	spec := procsup.Spec{
		Name: "cloudflared",
		Path: bin,
		Args: l.args,
		Env:  l.env,
		// The process echoing its own token must not put it in the log or the UI.
		Secrets: l.secrets,
		OnStart: e.onStart,
		OnLine:  e.onLine,
		OnExit:  e.onExit,
		Level:   logLevel,
	}
	if e.tune != nil {
		e.tune(&spec)
	}
	sup := procsup.New(spec, e.log)
	if err := sup.Start(ctx); err != nil {
		return "", e.fail(err)
	}
	e.mu.Lock()
	e.sup = sup
	e.mu.Unlock()

	select {
	case url := <-first:
		return url, nil
	case <-sup.Done():
		// cloudflared stopped for good before it ever came up.
		return "", e.abort(ctx, fmt.Errorf("cloudflared stopped before the tunnel came up%s", e.reason()))
	case <-ctx.Done():
		return "", e.abort(ctx, ctx.Err())
	}
}

// Stop stops cloudflared and waits for it to exit. Stopping when off is not an error.
func (e *engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	sup := e.sup
	e.mu.Unlock()
	if sup == nil {
		return nil
	}
	if err := sup.Stop(ctx); err != nil {
		return err // not confirmed gone: keep the supervisor so Stop can be retried
	}
	e.mu.Lock()
	e.sup = nil
	e.status = app.TunnelStatus{}
	e.mu.Unlock()
	return nil
}

// Status implements app.Tunnel. While cloudflared is being restarted it is
// Starting; after it gave up it is Failed with the reason.
func (e *engine) Status() app.TunnelStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// onStart runs each time cloudflared is (re)started. Whatever address the last
// run had is gone.
func (e *engine) onStart() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = false
	e.status = app.TunnelStatus{State: server.Starting}
}

// onLine watches the output. Only the first address in each run counts: after
// the banner, lines may quote requests that came in from the Internet, and
// nothing a stranger can make cloudflared print may change our address.
func (e *engine) onLine(_ procsup.Stream, line string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.tail = append(e.tail, trimLine(line))
	if len(e.tail) > tailLines {
		e.tail = e.tail[len(e.tail)-tailLines:]
	}
	if e.seen || e.ready == nil {
		return
	}
	url, ok := e.ready(line)
	if !ok {
		return
	}
	e.seen = true
	e.status = app.TunnelStatus{State: server.Running, URL: url}
	e.log.Info("tunnel is up", slog.String("url", string(url)))
	select {
	case e.first <- url:
	default: // Start already has an address; later ones are restarts
	}
}

func (e *engine) onExit(err error, restarting bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case restarting:
		e.status = app.TunnelStatus{State: server.Starting}
	case err != nil:
		e.status = app.TunnelStatus{State: server.Failed, Error: "cloudflared stopped and could not be restarted" + e.reasonLocked()}
	default:
		e.status = app.TunnelStatus{}
	}
}

func (e *engine) fail(err error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = app.TunnelStatus{State: server.Failed, Error: err.Error()}
	return err
}

// abort cleans up a start that did not succeed.
func (e *engine) abort(ctx context.Context, cause error) error {
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopWait)
	defer cancel()
	if err := e.Stop(stopCtx); err != nil {
		e.log.Warn("cleanup after failed start", slog.String("error", err.Error()))
	}
	return e.fail(cause)
}

// reason is the last thing cloudflared said, for an error message.
func (e *engine) reason() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.reasonLocked()
}

func (e *engine) reasonLocked() string {
	if len(e.tail) == 0 {
		return ""
	}
	return ": " + e.tail[len(e.tail)-1]
}

// trimLine shortens a line kept for an error message.
func trimLine(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}
