package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"eincast/internal/app"
)

// These tests use go-keyring's in-memory mock, so they never touch (or depend
// on) the real Keychain / Credential Manager of the machine running them.

func TestKeyring_RoundTrip(t *testing.T) {
	keyring.MockInit()
	k := New()

	if err := k.Set("access-token", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get("access-token"); err != nil || got != "s3cret" {
		t.Fatalf("Get() = %q, %v", got, err)
	}

	if err := k.Set("access-token", "rotated"); err != nil { // replaces
		t.Fatal(err)
	}
	if got, _ := k.Get("access-token"); got != "rotated" {
		t.Errorf("Get() after replace = %q", got)
	}
}

func TestKeyring_MissingSecretMapsToTheAppSentinel(t *testing.T) {
	keyring.MockInit()
	_, err := New().Get("nope")
	if !errors.Is(err, app.ErrSecretNotFound) {
		t.Fatalf("Get() error = %v, want app.ErrSecretNotFound", err)
	}
}

func TestKeyring_DeleteIsIdempotent(t *testing.T) {
	keyring.MockInit()
	k := New()
	_ = k.Set("a", "1")

	if err := k.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get("a"); !errors.Is(err, app.ErrSecretNotFound) {
		t.Errorf("secret still present after Delete: %v", err)
	}
	if err := k.Delete("a"); err != nil {
		t.Errorf("deleting an absent secret: error = %v, want nil", err)
	}
}

func TestKeyring_UnavailableStoreIsAnErrorNotNotFound(t *testing.T) {
	boom := errors.New("keychain is locked")
	keyring.MockInitWithError(boom)

	_, err := New().Get("a")
	if err == nil || errors.Is(err, app.ErrSecretNotFound) {
		t.Fatalf("Get() error = %v; a locked store must not look like 'no secret' (that would trigger overwriting)", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("error %v does not wrap the cause", err)
	}
	if err := New().Set("a", "b"); !errors.Is(err, boom) {
		t.Errorf("Set() error = %v", err)
	}
}
