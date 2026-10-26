package remote

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/crypto/scrypt"

	"eincast/internal/app"
)

const (
	// pinKey is where the PIN's hash lives in the SecretStore.
	pinKey = "remote-pin"

	// A public URL can be found (certificate transparency logs list every
	// hostname), so the PIN is the only thing between the Internet and the
	// library. Rate limiting slows guessing; length is what stops it.
	minPINLen = 8
	maxPINLen = 64

	// scrypt cost: about 100 ms and 32 MiB per check, which is nothing for one
	// sign-in and a real cost for someone guessing offline.
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1

	saltLen = 16
	hashLen = 32

	// pinFormat versions the stored value so the cost can change later.
	pinFormat = "v1"
)

// pinStore keeps the access PIN as a salted scrypt hash, in the credential
// store and in memory. The PIN itself is never stored or logged.
type pinStore struct {
	secrets app.SecretStore
	// cost is scrypt's N. It is the constant in production; tests lower it so
	// a lockout test that makes dozens of wrong guesses stays fast.
	cost int

	mu   sync.RWMutex
	salt []byte
	hash []byte
}

func newPINStore(secrets app.SecretStore) *pinStore {
	return &pinStore{secrets: secrets, cost: scryptN}
}

// load reads the stored hash. A missing one means no PIN is set. Any other
// error is returned, and leaves no PIN set: public access stays refused.
func (p *pinStore) load() error {
	stored, err := p.secrets.Get(pinKey)
	if errors.Is(err, app.ErrSecretNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read PIN: %w", err)
	}
	salt, hash, err := decodePIN(stored)
	if err != nil {
		return fmt.Errorf("read PIN: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.salt, p.hash = salt, hash
	return nil
}

func (p *pinStore) isSet() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.hash) > 0
}

// set validates, hashes and saves a new PIN. Nothing changes unless saving worked.
func (p *pinStore) set(pin string) error {
	if err := validatePIN(pin); err != nil {
		return err
	}
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt) // never fails: crypto/rand crashes the program instead
	hash, err := p.derive(pin, salt)
	if err != nil {
		return fmt.Errorf("hash PIN: %w", err)
	}
	if err := p.secrets.Set(pinKey, encodePIN(salt, hash)); err != nil {
		return fmt.Errorf("save PIN: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.salt, p.hash = salt, hash
	return nil
}

// clear removes the PIN.
func (p *pinStore) clear() error {
	if err := p.secrets.Delete(pinKey); err != nil {
		return fmt.Errorf("delete PIN: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.salt, p.hash = nil, nil
	return nil
}

// verify reports whether pin is the PIN, in constant time. With no PIN set it
// is always false: an unset PIN must never let anyone in.
func (p *pinStore) verify(pin string) bool {
	p.mu.RLock()
	salt, want := p.salt, p.hash
	p.mu.RUnlock()
	if len(want) == 0 {
		return false
	}
	got, err := p.derive(pin, salt)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func validatePIN(pin string) error {
	n := len([]rune(pin))
	switch {
	case n < minPINLen:
		return fmt.Errorf("%w: use at least %d characters", app.ErrInvalidPIN, minPINLen)
	case n > maxPINLen:
		return fmt.Errorf("%w: use at most %d characters", app.ErrInvalidPIN, maxPINLen)
	case strings.IndexFunc(pin, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: control characters are not allowed", app.ErrInvalidPIN)
	}
	return nil
}

func (p *pinStore) derive(pin string, salt []byte) ([]byte, error) {
	return scrypt.Key([]byte(pin), salt, p.cost, scryptR, scryptP, hashLen)
}

func encodePIN(salt, hash []byte) string {
	enc := base64.RawStdEncoding
	return pinFormat + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(hash)
}

func decodePIN(s string) (salt, hash []byte, err error) {
	parts := strings.Split(s, "$")
	if len(parts) != 3 || parts[0] != pinFormat {
		return nil, nil, errors.New("unrecognised PIN format")
	}
	enc := base64.RawStdEncoding
	if salt, err = enc.DecodeString(parts[1]); err != nil || len(salt) != saltLen {
		return nil, nil, errors.New("corrupt PIN salt")
	}
	if hash, err = enc.DecodeString(parts[2]); err != nil || len(hash) != hashLen {
		return nil, nil, errors.New("corrupt PIN hash")
	}
	return salt, hash, nil
}
