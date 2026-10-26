// Package ngrok is the tunnel provider for ngrok. It uses ngrok's Go SDK inside
// this process, so there is no external program to install or supervise; the SDK
// keeps its own connection alive and reconnects.
//
// ngrok's free plan has a small bandwidth allowance and a warning page for
// browsers, so this is for trying remote access, not for streaming films.
package ngrok

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
	"eincast/internal/domain/server"
)

// ID is this provider's identifier.
const ID app.ProviderID = "ngrok"

// Provider implements app.Tunnel for ngrok.
type Provider struct {
	cfg  *providerconfig.Store
	conn connector
	log  *slog.Logger

	mu      sync.Mutex
	ep      endpoint // the published endpoint; nil when off
	closing bool     // we are closing ep, so its ending is not news
	status  app.TunnelStatus
	quit    chan struct{} // closed to end the watcher
	cancel  func() bool   // stops the lifetime watcher (context.AfterFunc's stop)
}

var (
	_ app.Tunnel       = (*Provider)(nil)
	_ app.Configurable = (*Provider)(nil)
)

// New returns the ngrok provider. cfg holds the authtoken, in the credential
// store, never in a file.
func New(cfg *providerconfig.Store, log *slog.Logger) *Provider {
	return newWith(cfg, sdkConnector{}, log)
}

func newWith(cfg *providerconfig.Store, conn connector, log *slog.Logger) *Provider {
	return &Provider{cfg: cfg, conn: conn, log: log.With(slog.String("source", "tunnel"), slog.String("provider", string(ID)))}
}

// ID implements app.Tunnel.
func (*Provider) ID() app.ProviderID { return ID }

// Public implements app.Tunnel. Anyone who learns the address can reach it.
func (*Provider) Public() bool { return true }

// Fields implements app.Configurable.
func (*Provider) Fields() []app.ConfigField { return []app.ConfigField{authtokenField()} }

func authtokenField() app.ConfigField {
	return app.ConfigField{
		Key:         "authtoken",
		Label:       "ngrok authtoken",
		Help:        "Sign in at dashboard.ngrok.com and copy your authtoken from “Your Authtoken”. It is kept in your computer's keychain.",
		Placeholder: "2abc…",
		Kind:        app.FieldSecret,
		Required:    true,
	}
}

// Start signs in to ngrok and publishes an endpoint that forwards to the
// server's loopback address. ctx is the tunnel's lifetime.
func (p *Provider) Start(ctx context.Context, srv app.LocalServer) (app.PublicURL, error) {
	p.mu.Lock()
	if p.ep != nil {
		p.mu.Unlock()
		return "", errors.New("ngrok is already on")
	}
	p.status = app.TunnelStatus{State: server.Starting}
	p.mu.Unlock()

	token, err := p.cfg.Get(ID, authtokenField())
	if err == nil && token == "" {
		err = &app.NotConfiguredError{Missing: []string{authtokenField().Label}}
	}
	if err != nil {
		return "", p.fail(err)
	}
	local, err := app.LoopbackAddr(srv)
	if err != nil {
		return "", p.fail(err)
	}

	ep, err := p.conn.Forward(ctx, token, "http://"+local, p.onEvent)
	if err != nil {
		return "", p.fail(explain(err, token))
	}

	url := app.PublicURL(ep.URL())
	quit := make(chan struct{})
	p.mu.Lock()
	p.ep, p.quit = ep, quit
	p.status = app.TunnelStatus{State: server.Running, URL: url}
	// Ends when ctx is cancelled or release calls the returned stop function.
	p.cancel = context.AfterFunc(ctx, func() { _ = p.release(context.Background()) })
	p.mu.Unlock()

	go p.watch(ep, quit)
	p.log.Info("ngrok endpoint is up", slog.String("url", string(url)))
	return url, nil
}

// Stop ends the endpoint and the connection to ngrok. Stopping when off is not
// an error.
func (p *Provider) Stop(ctx context.Context) error { return p.release(ctx) }

// Status implements app.Tunnel.
func (p *Provider) Status() app.TunnelStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// release ends the endpoint. If closing fails, the endpoint is kept so Stop can
// be retried, and the failure is returned.
func (p *Provider) release(ctx context.Context) error {
	p.mu.Lock()
	ep, quit, cancel := p.ep, p.quit, p.cancel
	// Marked before closing: the endpoint's Done fires as part of Close, and the
	// watcher must not mistake our own close for ngrok ending it.
	p.closing = ep != nil
	p.mu.Unlock()
	if ep == nil {
		return nil
	}
	if err := ep.Close(ctx); err != nil {
		p.mu.Lock()
		p.closing = false // still open: an ending from now on is news again
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	if p.ep == ep { // not already released by a concurrent call
		p.ep, p.quit, p.cancel, p.closing = nil, nil, nil, false
		p.status = app.TunnelStatus{}
		close(quit)
		if cancel != nil {
			cancel()
		}
	}
	p.mu.Unlock()
	p.log.Info("ngrok endpoint closed")
	return nil
}

// watch notices when ngrok ends the endpoint on its side (the account was
// closed, the plan's limit was hit). It ends with the endpoint or when release
// closes quit, whichever is first.
func (p *Provider) watch(ep endpoint, quit <-chan struct{}) {
	select {
	case <-quit:
	case <-ep.Done():
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.ep != ep || p.closing {
			return // we closed it ourselves
		}
		p.status = app.TunnelStatus{State: server.Failed, Error: "ngrok closed the tunnel. Check your ngrok account and plan limits, then turn it on again."}
		p.log.Warn("ngrok ended the endpoint")
	}
}

// onEvent follows the SDK's connection to ngrok, which reconnects by itself.
func (p *Provider) onEvent(e event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ep == nil || p.status.State == server.Failed {
		return // starting, stopped, or over
	}
	if e.connected {
		p.status = app.TunnelStatus{State: server.Running, URL: app.PublicURL(p.ep.URL())}
		return
	}
	p.status = app.TunnelStatus{State: server.Starting}
	p.log.Warn("lost the connection to ngrok; the SDK is reconnecting")
}

func (p *Provider) fail(err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = app.TunnelStatus{State: server.Failed, Error: err.Error()}
	return err
}

// explain turns what the SDK says into advice, and makes sure the token cannot
// be in it.
func explain(err error, token string) error {
	msg := err.Error()
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "authentication failed"), strings.Contains(msg, "ERR_NGROK_105"),
		strings.Contains(lower, "authtoken"):
		return errors.New("ngrok did not accept the authtoken. Copy it again from dashboard.ngrok.com. (" + msg + ")")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	}
	return errors.New("could not start ngrok: " + msg)
}
