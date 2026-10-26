// Package tunnel is the registry of remote-access providers. Each provider
// (Tailscale, Cloudflare, ngrok) lives in its own subpackage and implements
// app.Tunnel; this package only keeps them by ID, so remote.Service can pick
// one without knowing which it got (the strategy pattern, ADR 0007).
package tunnel

import (
	"fmt"

	"eincast/internal/app"
)

// Registry holds the available providers. It is filled once at startup and
// read-only afterwards, so it needs no lock.
type Registry struct {
	byID  map[app.ProviderID]app.Tunnel
	order []app.ProviderID
}

var _ app.TunnelRegistry = (*Registry)(nil)

// NewRegistry builds a Registry. It fails on an empty or duplicate ID, which
// would be a wiring mistake in main.
func NewRegistry(providers ...app.Tunnel) (*Registry, error) {
	r := &Registry{byID: make(map[app.ProviderID]app.Tunnel, len(providers))}
	for _, p := range providers {
		id := p.ID()
		if id == "" {
			return nil, fmt.Errorf("register tunnel provider: empty ID (%T)", p)
		}
		if _, dup := r.byID[id]; dup {
			return nil, fmt.Errorf("register tunnel provider: duplicate ID %q", id)
		}
		r.byID[id] = p
		r.order = append(r.order, id)
	}
	return r, nil
}

// Lookup returns the provider with the given ID, or an error wrapping
// app.ErrUnknownProvider.
func (r *Registry) Lookup(id app.ProviderID) (app.Tunnel, error) {
	p, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("provider %q: %w", id, app.ErrUnknownProvider)
	}
	return p, nil
}

// List describes the providers in registration order.
func (r *Registry) List() []app.ProviderInfo {
	out := make([]app.ProviderInfo, 0, len(r.order))
	for _, id := range r.order {
		p := r.byID[id]
		info := app.ProviderInfo{ID: id, Public: p.Public()}
		if c, ok := p.(app.Configurable); ok {
			info.Fields = c.Fields()
		}
		out = append(out, info)
	}
	return out
}
