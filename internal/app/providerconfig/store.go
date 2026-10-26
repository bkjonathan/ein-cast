// Package providerconfig keeps the values tunnel providers need from the person:
// a tunnel token, a host name, the location of a program.
//
// All of it lives in the operating system's credential store (ADR 0013), secret
// or not, so the settings file never holds anything about a tunnel and there is
// one place to look. Secrets are write-only: nothing here hands one back to the
// UI, and nothing here logs a value.
package providerconfig

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"eincast/internal/app"
)

// Limits on what is stored. A tunnel token is a few hundred characters; these
// are generous, and exist so a pasted file cannot fill the credential store.
const (
	maxSecretLen = 2048
	maxTextLen   = 512
)

// Store reads and writes provider values. It holds no state of its own, so any
// number of Stores over the same SecretStore agree.
type Store struct {
	secrets app.SecretStore
}

// New returns a Store over the credential store.
func New(secrets app.SecretStore) *Store { return &Store{secrets: secrets} }

// Get returns the saved value, or "" if none is saved. An error means the
// credential store itself could not be read.
func (s *Store) Get(provider app.ProviderID, f app.ConfigField) (string, error) {
	v, err := s.secrets.Get(app.ConfigKey(provider, f))
	if errors.Is(err, app.ErrSecretNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", f.Label, err)
	}
	return v, nil
}

// Set tidies, checks and saves a value. Nothing is stored unless it passes.
func (s *Store) Set(provider app.ProviderID, f app.ConfigField, raw string) error {
	clean, err := clean(f, raw)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Label, err)
	}
	if err := s.secrets.Set(app.ConfigKey(provider, f), clean); err != nil {
		return fmt.Errorf("save %s: %w", f.Label, err)
	}
	return nil
}

// Clear removes a saved value. Clearing one that is not set is not an error.
func (s *Store) Clear(provider app.ProviderID, f app.ConfigField) error {
	if err := s.secrets.Delete(app.ConfigKey(provider, f)); err != nil {
		return fmt.Errorf("remove %s: %w", f.Label, err)
	}
	return nil
}

// States reports each field and whether it is set. Secrets come back without
// their value.
func (s *Store) States(provider app.ProviderID, fields []app.ConfigField) ([]app.FieldState, error) {
	out := make([]app.FieldState, 0, len(fields))
	for _, f := range fields {
		v, err := s.Get(provider, f)
		if err != nil {
			return nil, err
		}
		st := app.FieldState{Field: f, Set: v != ""}
		if f.Kind != app.FieldSecret {
			st.Value = v
		}
		out = append(out, st)
	}
	return out, nil
}

// Missing returns the required fields that are not set, as an error naming them
// (matching app.ErrNotConfigured), or nil if there are none.
func (s *Store) Missing(provider app.ProviderID, fields []app.ConfigField) error {
	var missing []string
	for _, f := range fields {
		if !f.Required {
			continue
		}
		v, err := s.Get(provider, f)
		if err != nil {
			return err
		}
		if v == "" {
			missing = append(missing, f.Label)
		}
	}
	if len(missing) > 0 {
		return &app.NotConfiguredError{Missing: missing}
	}
	return nil
}

// clean applies the field's own Clean, or the default for its kind, to a value.
func clean(f app.ConfigField, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("%w: enter a value, or remove the saved one", app.ErrInvalidValue)
	}
	if f.Clean != nil {
		out, err := f.Clean(v)
		if err != nil {
			return "", fmt.Errorf("%w: %s", app.ErrInvalidValue, err)
		}
		v = out
	}
	return v, check(f.Kind, v)
}

// check applies the rules every stored value must meet, whatever the field.
func check(kind app.FieldKind, v string) error {
	limit := maxTextLen
	if kind == app.FieldSecret {
		limit = maxSecretLen
	}
	if len(v) > limit {
		return fmt.Errorf("%w: too long", app.ErrInvalidValue)
	}
	if strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: control characters are not allowed", app.ErrInvalidValue)
	}
	// A pasted credential with a space or line break inside is a mistake (two
	// things pasted together), and would be passed to a program as one value.
	if kind == app.FieldSecret && strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%w: it contains spaces; paste only the token", app.ErrInvalidValue)
	}
	return nil
}
