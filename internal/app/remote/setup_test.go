package remote

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

var (
	tokenField    = app.ConfigField{Key: "token", Label: "Tunnel token", Kind: app.FieldSecret, Required: true}
	hostnameField = app.ConfigField{Key: "hostname", Label: "Hostname", Kind: app.FieldText, Required: true}
	optionalField = app.ConfigField{Key: "binary", Label: "Program", Kind: app.FieldFile, Shared: "cloudflared"}
)

// namedTunnel is a public provider that needs a token and a host name.
func namedTunnel() *fakeTunnel {
	t := publicTunnel()
	t.id = "fake-named"
	t.fields = []app.ConfigField{tokenField, hostnameField, optionalField}
	return t
}

func (e *env) fill(t *testing.T, id app.ProviderID) {
	t.Helper()
	if err := e.svc.SetProviderField(id, "token", "TOKEN-VALUE"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetProviderField(id, "hostname", "media.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestSetup_AProviderThatNeedsValuesCannotBeTurnedOnWithoutThem(t *testing.T) {
	tn := namedTunnel()
	e := withPIN(t, tn)

	err := e.svc.Enable(context.Background(), tn.id, localServer())

	var nc *app.NotConfiguredError
	if !errors.Is(err, app.ErrNotConfigured) || !errors.As(err, &nc) {
		t.Fatalf("error = %v, want ErrNotConfigured naming what is missing", err)
	}
	if strings.Join(nc.Missing, ",") != "Tunnel token,Hostname" {
		t.Errorf("missing = %v: the optional field must not be demanded", nc.Missing)
	}
	if started, _ := tn.counts(); started != 0 {
		t.Error("the provider was started without its settings")
	}
	if got := e.svc.Status().NeedsSetup; !slices.Equal(got, []app.ProviderID{tn.id}) {
		t.Errorf("NeedsSetup = %v", got)
	}
}

func TestSetup_ItIsAskedBeforeThePIN(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn) // no PIN either
	is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrNotConfigured)

	e.fill(t, tn.id)
	is(t, e.svc.Enable(context.Background(), tn.id, localServer()), app.ErrPINRequired)
}

func TestSetup_OnceFilledInItTurnsOn(t *testing.T) {
	tn := namedTunnel()
	e := withPIN(t, tn)
	e.fill(t, tn.id)

	e.enable(t, tn)

	if got := e.svc.Status(); len(got.NeedsSetup) != 0 || got.State != server.Running {
		t.Errorf("status = %+v", got)
	}
}

func TestSetup_ProvidersThatNeedNothingAreNeverListed(t *testing.T) {
	e := newEnv(t, publicTunnel(), privateTunnel(), namedTunnel())
	if got := e.svc.Status().NeedsSetup; !slices.Equal(got, []app.ProviderID{"fake-named"}) {
		t.Errorf("NeedsSetup = %v", got)
	}
}

func TestSetup_SecretsAreWriteOnly(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn)
	e.fill(t, tn.id)

	states, err := e.svc.ProviderConfig(tn.id)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]app.FieldState{}
	for _, st := range states {
		byKey[st.Field.Key] = st
	}
	if st := byKey["token"]; !st.Set || st.Value != "" {
		t.Errorf("token = %+v: a secret is reported as set, without its value", st)
	}
	if st := byKey["hostname"]; !st.Set || st.Value != "media.example.com" {
		t.Errorf("hostname = %+v", st)
	}
	if st := byKey["binary"]; st.Set {
		t.Errorf("binary = %+v: nothing was saved for it", st)
	}
}

func TestSetup_ValuesAreStoredInTheCredentialStoreAndNeverLogged(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn)
	if err := e.svc.SetProviderField(tn.id, "token", "SECRET-TOKEN-XYZ"); err != nil {
		t.Fatal(err)
	}

	if got := e.secrets.stored(app.ConfigKey(tn.id, tokenField)); got != "SECRET-TOKEN-XYZ" {
		t.Errorf("stored = %q", got)
	}
	if strings.Contains(e.logs.String(), "SECRET-TOKEN-XYZ") {
		t.Errorf("the log contains the token:\n%s", e.logs.String())
	}
	if !strings.Contains(e.logs.String(), "setting=token") {
		t.Errorf("the change should be logged by name:\n%s", e.logs.String())
	}
}

func TestSetup_RejectsWhatItCannotUse(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn)

	is(t, e.svc.SetProviderField(tn.id, "token", "two words"), app.ErrInvalidValue)
	is(t, e.svc.SetProviderField(tn.id, "token", "  "), app.ErrInvalidValue)
	is(t, e.svc.SetProviderField(tn.id, "nope", "x"), app.ErrUnknownField)
	is(t, e.svc.ClearProviderField(tn.id, "nope"), app.ErrUnknownField)
	is(t, e.svc.SetProviderField("no-such-provider", "token", "x"), app.ErrUnknownProvider)
	if _, err := e.svc.ProviderConfig("no-such-provider"); !errors.Is(err, app.ErrUnknownProvider) {
		t.Errorf("ProviderConfig of an unknown provider: %v", err)
	}
	if got := e.svc.Status().NeedsSetup; len(got) != 1 {
		t.Errorf("a rejected value changed the setup state: %v", got)
	}
}

func TestSetup_SettingsCannotChangeWhileTheProviderIsOn(t *testing.T) {
	tn := namedTunnel()
	other := privateTunnel()
	other.fields = []app.ConfigField{{Key: "k", Label: "K", Kind: app.FieldText}}
	e := withPIN(t, tn, other)
	e.fill(t, tn.id)
	e.enable(t, tn)

	is(t, e.svc.SetProviderField(tn.id, "hostname", "other.example.com"), app.ErrProviderInUse)
	is(t, e.svc.ClearProviderField(tn.id, "token"), app.ErrProviderInUse)
	// Another provider's settings are its own business.
	if err := e.svc.SetProviderField(other.id, "k", "v"); err != nil {
		t.Errorf("an unrelated provider's setting was refused: %v", err)
	}

	if err := e.svc.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetProviderField(tn.id, "hostname", "other.example.com"); err != nil {
		t.Errorf("after turning it off: %v", err)
	}
}

func TestSetup_ClearingARequiredValueMakesItNeedSetupAgain(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn)
	e.fill(t, tn.id)
	if len(e.svc.Status().NeedsSetup) != 0 {
		t.Fatal("still needs setup after filling it in")
	}

	if err := e.svc.ClearProviderField(tn.id, "token"); err != nil {
		t.Fatal(err)
	}

	if got := e.svc.Status().NeedsSetup; !slices.Equal(got, []app.ProviderID{tn.id}) {
		t.Errorf("NeedsSetup = %v", got)
	}
}

func TestSetup_ChangesAreAnnounced(t *testing.T) {
	tn := namedTunnel()
	e := newEnv(t, tn)
	before := len(e.events.states())

	e.fill(t, tn.id)

	e.events.mu.Lock()
	defer e.events.mu.Unlock()
	if len(e.events.got) < before+2 {
		t.Fatalf("%d events for two changes", len(e.events.got)-before)
	}
	if last := e.events.got[len(e.events.got)-1]; len(last.NeedsSetup) != 0 {
		t.Errorf("the last event still says setup is needed: %+v", last.NeedsSetup)
	}
}

func TestSetup_SurvivesARestart(t *testing.T) {
	tn := namedTunnel()
	secrets := newSecrets()
	first := newEnvWith(t, secrets, []*fakeTunnel{tn})
	first.fill(t, tn.id)

	again := newEnvWith(t, secrets, []*fakeTunnel{tn})

	if len(again.svc.Status().NeedsSetup) != 0 {
		t.Error("saved settings were not noticed after a restart")
	}
}

func TestSetup_AnUnreadableCredentialStoreReadsAsNotSetUp(t *testing.T) {
	tn := namedTunnel()
	locked := newSecrets()
	locked.getErr = errors.New("keychain locked")

	e := newEnvWith(t, locked, []*fakeTunnel{tn})

	if got := e.svc.Status().NeedsSetup; !slices.Equal(got, []app.ProviderID{tn.id}) {
		t.Errorf("NeedsSetup = %v: a store that cannot be read must not look configured", got)
	}
	if !strings.Contains(e.logs.String(), "keychain locked") {
		t.Errorf("the reason was not logged:\n%s", e.logs.String())
	}
	err := e.svc.Enable(context.Background(), tn.id, localServer())
	if err == nil || errors.Is(err, app.ErrNotConfigured) {
		t.Errorf("error = %v: the real problem (the store) should be reported, not 'not set up'", err)
	}
}
