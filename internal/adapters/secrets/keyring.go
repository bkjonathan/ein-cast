// Package secrets keeps secrets in the operating system's credential store:
// the macOS Keychain or the Windows Credential Manager. It implements
// app.SecretStore.
package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"

	"eincast/internal/app"
)

// Service is the credential-store service name under which EinCast's secrets
// are filed, so they are easy to find (and delete) in Keychain Access.
const Service = "com.bkthomas.eincast"

// Keyring is an app.SecretStore backed by the system credential store.
type Keyring struct{}

var _ app.SecretStore = Keyring{}

// New returns the system-backed store.
func New() Keyring { return Keyring{} }

// Get returns the secret, or an error wrapping app.ErrSecretNotFound.
func (Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("get %q: %w", key, app.ErrSecretNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("get %q from the system credential store: %w", key, err)
	}
	return v, nil
}

// Set stores or replaces the secret.
func (Keyring) Set(key, value string) error {
	if err := keyring.Set(Service, key, value); err != nil {
		return fmt.Errorf("save %q to the system credential store: %w", key, err)
	}
	return nil
}

// Delete removes the secret. Deleting one that does not exist is not an error.
func (Keyring) Delete(key string) error {
	err := keyring.Delete(Service, key)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("delete %q from the system credential store: %w", key, err)
}
