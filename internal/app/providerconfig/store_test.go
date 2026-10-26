package providerconfig

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"eincast/internal/app"
)

type memSecrets struct {
	mu     sync.Mutex
	m      map[string]string
	getErr error
	setErr error
}

func newSecrets() *memSecrets { return &memSecrets{m: map[string]string{}} }

func (s *memSecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return "", s.getErr
	}
	v, ok := s.m[k]
	if !ok {
		return "", fmt.Errorf("get %q: %w", k, app.ErrSecretNotFound)
	}
	return v, nil
}

func (s *memSecrets) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.m[k] = v
	return nil
}

func (s *memSecrets) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

var (
	token    = app.ConfigField{Key: "token", Label: "Tunnel token", Kind: app.FieldSecret, Required: true}
	hostname = app.ConfigField{Key: "hostname", Label: "Hostname", Kind: app.FieldText, Required: true,
		Clean: func(raw string) (string, error) { return strings.ToLower(raw), nil }}
	binary = app.ConfigField{Key: "binary", Label: "cloudflared", Kind: app.FieldFile, Shared: "cloudflared"}
)

func TestConfigKey_SharedFieldsAreStoredOnce(t *testing.T) {
	a := app.ConfigKey("cloudflare-named", binary)
	b := app.ConfigKey("cloudflare-quick", binary)
	if a != b || a != "provider/cloudflared/binary" {
		t.Errorf("keys = %q, %q: a shared field must be the same value for both providers", a, b)
	}
	if app.ConfigKey("ngrok", token) == app.ConfigKey("cloudflare-named", token) {
		t.Error("two providers' tokens share a key")
	}
}

func TestSetAndGet_RoundTripWithCleaning(t *testing.T) {
	st := New(newSecrets())
	if err := st.Set("cf", hostname, "  Media.Example.COM \n"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("cf", hostname)
	if err != nil || got != "media.example.com" {
		t.Errorf("Get = %q, %v: want it trimmed and cleaned by the field", got, err)
	}
}

func TestGet_NothingSavedIsEmptyNotAnError(t *testing.T) {
	if v, err := New(newSecrets()).Get("cf", token); v != "" || err != nil {
		t.Errorf("Get = %q, %v", v, err)
	}
}

func TestGet_AnUnavailableStoreIsAnError(t *testing.T) {
	s := newSecrets()
	s.getErr = errors.New("keychain locked")
	if _, err := New(s).Get("cf", token); err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Errorf("error = %v: a locked store must not read as 'not set'", err)
	}
}

func TestSet_RejectsBadValuesAndStoresNothing(t *testing.T) {
	cases := map[string]struct {
		f   app.ConfigField
		raw string
	}{
		"empty":                     {token, ""},
		"only spaces":               {token, "   \n"},
		"two things pasted":         {token, "eyJhIjoi abc def"},
		"line break inside":         {token, "abc\ndef"},
		"control character":         {hostname, "host\x07name"},
		"secret too long":           {token, strings.Repeat("a", maxSecretLen+1)},
		"text too long":             {hostname, strings.Repeat("a", maxTextLen+1)},
		"the field's own rule says": {app.ConfigField{Key: "k", Label: "K", Clean: func(string) (string, error) { return "", errors.New("not a host") }}, "x"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			secrets := newSecrets()
			err := New(secrets).Set("cf", c.f, c.raw)
			if !errors.Is(err, app.ErrInvalidValue) {
				t.Fatalf("error = %v, want ErrInvalidValue", err)
			}
			if len(secrets.m) != 0 {
				t.Errorf("stored %v despite the error", secrets.m)
			}
		})
	}
}

func TestSet_AcceptsTheLongestSecretAndSpacesInText(t *testing.T) {
	st := New(newSecrets())
	if err := st.Set("cf", token, strings.Repeat("a", maxSecretLen)); err != nil {
		t.Errorf("longest secret: %v", err)
	}
	if err := st.Set("cf", app.ConfigField{Key: "name", Label: "Name", Kind: app.FieldText}, "my media server"); err != nil {
		t.Errorf("text with spaces: %v", err)
	}
}

func TestSet_FailureToSaveIsReportedAndKeepsTheOldValue(t *testing.T) {
	s := newSecrets()
	st := New(s)
	if err := st.Set("cf", token, "old"); err != nil {
		t.Fatal(err)
	}
	s.setErr = errors.New("keychain locked")

	if err := st.Set("cf", token, "new"); err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("error = %v", err)
	}
	if v, _ := st.Get("cf", token); v != "old" {
		t.Errorf("value = %q, want the old one", v)
	}
}

func TestClear(t *testing.T) {
	st := New(newSecrets())
	_ = st.Set("cf", token, "abc")
	if err := st.Clear("cf", token); err != nil {
		t.Fatal(err)
	}
	if v, _ := st.Get("cf", token); v != "" {
		t.Errorf("value = %q after Clear", v)
	}
	if err := st.Clear("cf", token); err != nil {
		t.Errorf("clearing what is not set: %v", err)
	}
}

func TestStates_NeverReturnASecretsValue(t *testing.T) {
	st := New(newSecrets())
	_ = st.Set("cf", token, "SUPER-SECRET-TOKEN")
	_ = st.Set("cf", hostname, "media.example.com")

	states, err := st.States("cf", []app.ConfigField{token, hostname, binary})
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 {
		t.Fatalf("states = %+v", states)
	}
	if !states[0].Set || states[0].Value != "" {
		t.Errorf("secret state = %+v: must be reported as set, without its value", states[0])
	}
	if !states[1].Set || states[1].Value != "media.example.com" {
		t.Errorf("text state = %+v", states[1])
	}
	if states[2].Set || states[2].Value != "" {
		t.Errorf("unset state = %+v", states[2])
	}
}

func TestMissing_NamesWhatIsRequiredAndUnset(t *testing.T) {
	st := New(newSecrets())
	fields := []app.ConfigField{token, hostname, binary} // binary is optional

	err := st.Missing("cf", fields)
	var nc *app.NotConfiguredError
	if !errors.Is(err, app.ErrNotConfigured) || !errors.As(err, &nc) {
		t.Fatalf("error = %v", err)
	}
	if strings.Join(nc.Missing, ",") != "Tunnel token,Hostname" {
		t.Errorf("missing = %v, want the required fields only, in order", nc.Missing)
	}

	_ = st.Set("cf", token, "abc")
	if err := st.Missing("cf", fields); err == nil || !strings.Contains(err.Error(), "Hostname") || strings.Contains(err.Error(), "Tunnel token") {
		t.Errorf("error = %v, want only Hostname left", err)
	}
	_ = st.Set("cf", hostname, "a.example.com")
	if err := st.Missing("cf", fields); err != nil {
		t.Errorf("everything required is set, yet: %v", err)
	}
}

func TestMissing_AnUnreadableStoreIsNotReportedAsMissing(t *testing.T) {
	s := newSecrets()
	s.getErr = errors.New("keychain locked")
	err := New(s).Missing("cf", []app.ConfigField{token})
	if err == nil || errors.Is(err, app.ErrNotConfigured) {
		t.Errorf("error = %v: a locked store is a different problem from an unset value", err)
	}
}

func TestMissing_ProvidersWithNoFieldsAreAlwaysReady(t *testing.T) {
	if err := New(newSecrets()).Missing("tailscale", nil); err != nil {
		t.Error(err)
	}
}
