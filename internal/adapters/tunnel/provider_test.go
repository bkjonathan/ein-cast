package tunnel

import (
	"context"
	"errors"
	"testing"

	"eincast/internal/app"
)

type stubTunnel struct {
	id     app.ProviderID
	public bool
}

func (s stubTunnel) ID() app.ProviderID { return s.id }
func (s stubTunnel) Public() bool       { return s.public }
func (stubTunnel) Start(context.Context, app.LocalServer) (app.PublicURL, error) {
	return "", errors.New("not used")
}
func (stubTunnel) Stop(context.Context) error { return nil }
func (stubTunnel) Status() app.TunnelStatus   { return app.TunnelStatus{} }

func TestRegistry_LookupAndList(t *testing.T) {
	ts := stubTunnel{id: "tailscale"}
	cf := stubTunnel{id: "cloudflare-quick", public: true}
	r, err := NewRegistry(ts, cf)
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.Lookup("cloudflare-quick")
	if err != nil || got.ID() != "cloudflare-quick" {
		t.Fatalf("Lookup = %v, %v", got, err)
	}

	list := r.List()
	if len(list) != 2 || list[0].ID != "tailscale" || list[0].Public || list[1].ID != "cloudflare-quick" || !list[1].Public {
		t.Errorf("List = %+v, want registration order, with visibility", list)
	}
	if len(list[0].Fields) != 0 || len(list[1].Fields) != 0 {
		t.Errorf("providers that need nothing listed fields: %+v", list)
	}
}

// configurableTunnel is a stub that needs a value from the person.
type configurableTunnel struct{ stubTunnel }

func (configurableTunnel) Fields() []app.ConfigField {
	return []app.ConfigField{{Key: "token", Label: "Token", Kind: app.FieldSecret, Required: true}}
}

func TestRegistry_ListCarriesWhatAProviderNeeds(t *testing.T) {
	r, err := NewRegistry(stubTunnel{id: "plain"}, configurableTunnel{stubTunnel{id: "needs-token", public: true}})
	if err != nil {
		t.Fatal(err)
	}

	list := r.List()

	if len(list[0].Fields) != 0 {
		t.Errorf("plain: %+v", list[0])
	}
	if len(list[1].Fields) != 1 || list[1].Fields[0].Key != "token" || list[1].Fields[0].Kind != app.FieldSecret || !list[1].Fields[0].Required {
		t.Errorf("needs-token: %+v", list[1])
	}
}

func TestRegistry_UnknownProvider(t *testing.T) {
	r, _ := NewRegistry(stubTunnel{id: "tailscale"})
	for _, id := range []app.ProviderID{"", "nope", "Tailscale"} {
		if _, err := r.Lookup(id); !errors.Is(err, app.ErrUnknownProvider) {
			t.Errorf("Lookup(%q) error = %v, want ErrUnknownProvider", id, err)
		}
	}
}

func TestRegistry_RejectsWiringMistakes(t *testing.T) {
	if _, err := NewRegistry(stubTunnel{id: "a"}, stubTunnel{id: "a"}); err == nil {
		t.Error("duplicate IDs accepted")
	}
	if _, err := NewRegistry(stubTunnel{}); err == nil {
		t.Error("empty ID accepted")
	}
}

func TestRegistry_CanBeEmpty(t *testing.T) {
	r, err := NewRegistry()
	if err != nil || len(r.List()) != 0 {
		t.Fatalf("empty registry: %v, %v", r, err)
	}
}
