// Package tailscale is the tunnel provider for a Tailscale network. Unlike the
// others it runs nothing and relays nothing: Tailscale itself carries the
// traffic. EinCast only asks the local Tailscale node who it is, and has the
// server listen on that tailnet address while this provider is on.
//
// It is private: only the user's own devices can reach the address, so no PIN
// is required. Requests arrive from tailnet addresses, not loopback, so they
// follow the home-network rules (the access token, if the user turned it on).
package tailscale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

// ID is this provider's identifier.
const ID app.ProviderID = "tailscale"

// statusTimeout bounds `tailscale status`, which can hang while the daemon
// starts up.
const statusTimeout = 10 * time.Second

// ErrNotInstalled means no Tailscale command-line tool was found.
var ErrNotInstalled = errors.New("Tailscale was not found. Install it from tailscale.com/download, sign in, and try again")

// runner runs the Tailscale command-line tool. Declared here, at the consumer,
// so the provider is tested without Tailscale installed.
type runner interface {
	// Status returns the output of `tailscale status --json`.
	Status(ctx context.Context) ([]byte, error)
}

// Provider implements app.Tunnel for Tailscale.
type Provider struct {
	run runner
	log *slog.Logger

	mu     sync.Mutex
	status app.TunnelStatus
	unbind func()      // stops listening on the tailnet address; nil when off
	cancel func() bool // stops the lifetime watcher (context.AfterFunc's stop)
}

var _ app.Tunnel = (*Provider)(nil)

// New returns a Provider that uses the Tailscale tool installed on this machine.
func New(log *slog.Logger) *Provider {
	return newWith(cliRunner{locate: locateCLI}, log)
}

func newWith(run runner, log *slog.Logger) *Provider {
	return &Provider{run: run, log: log.With(slog.String("source", "tunnel"), slog.String("provider", string(ID)))}
}

// ID implements app.Tunnel.
func (*Provider) ID() app.ProviderID { return ID }

// Public implements app.Tunnel. A tailnet is private to the user's own devices.
func (*Provider) Public() bool { return false }

// Start asks the local Tailscale node for its address, has the server listen on
// it, and returns the URL other devices on the tailnet can open. ctx is the
// tunnel's lifetime: cancelling it stops listening.
func (p *Provider) Start(ctx context.Context, srv app.LocalServer) (app.PublicURL, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.unbind != nil {
		return "", errors.New("tailscale is already on")
	}
	p.status = app.TunnelStatus{State: server.Starting}

	n, err := p.lookup(ctx)
	if err != nil {
		return "", p.failLocked(err)
	}
	addrs := srv.Addrs()
	if len(addrs) == 0 {
		return "", p.failLocked(errors.New("server has no address"))
	}
	port := addrs[0].Port()

	unbind, err := srv.Bind(n.ip)
	if err != nil {
		return "", p.failLocked(fmt.Errorf("listen on the Tailscale address: %w", err))
	}

	url := app.PublicURL("http://" + netip.AddrPortFrom(n.ip, port).String())
	if n.host != n.ip.String() {
		url = app.PublicURL(fmt.Sprintf("http://%s:%d", n.host, port))
	}
	p.unbind = unbind
	// Ends when ctx is cancelled or Stop calls the returned stop function.
	p.cancel = context.AfterFunc(ctx, p.release)
	p.status = app.TunnelStatus{State: server.Running, URL: url}
	p.log.Info("listening on the tailnet", slog.String("url", string(url)))
	return url, nil
}

// Stop stops listening on the tailnet address and drops connections that came
// in through it. Stopping when off is not an error.
func (p *Provider) Stop(context.Context) error {
	p.release()
	return nil
}

// Status implements app.Tunnel.
func (p *Provider) Status() app.TunnelStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *Provider) release() {
	p.mu.Lock()
	unbind, cancel := p.unbind, p.cancel
	p.unbind, p.cancel = nil, nil
	p.status = app.TunnelStatus{}
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if unbind != nil {
		unbind() // outside the lock: it closes connections
		p.log.Info("stopped listening on the tailnet")
	}
}

func (p *Provider) lookup(ctx context.Context) (node, error) {
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	raw, err := p.run.Status(ctx)
	if err != nil {
		return node{}, err
	}
	return parseStatus(raw)
}

func (p *Provider) failLocked(err error) error {
	p.status = app.TunnelStatus{State: server.Failed, Error: err.Error()}
	return err
}

// cliRunner runs the real `tailscale` tool.
type cliRunner struct {
	locate func() (string, error)
}

func (c cliRunner) Status(ctx context.Context) ([]byte, error) {
	bin, err := c.locate()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, "status", "--json")
	// Without this, a child that inherited the output pipe would keep Output
	// waiting after the context killed the tool itself.
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if msg := strings.TrimSpace(string(exit.Stderr)); msg != "" {
				return nil, fmt.Errorf("Tailscale says: %s", msg)
			}
		}
		return nil, fmt.Errorf("run Tailscale: %w", err)
	}
	return out, nil
}

// locateCLI finds the command-line tool: on PATH first, then where each
// platform's installer puts it. On macOS the app's own binary doubles as the
// command-line tool.
func locateCLI() (string, error) {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p, nil
	}
	for _, p := range installLocations(runtime.GOOS) {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotInstalled
}

func installLocations(goos string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
			"/opt/homebrew/bin/tailscale",
			"/usr/local/bin/tailscale",
		}
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				out = append(out, filepath.Join(dir, "Tailscale", "tailscale.exe"))
			}
		}
		return out
	default:
		return nil
	}
}
