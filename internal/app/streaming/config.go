// Package streaming holds the server lifecycle use cases: start, stop, status.
package streaming

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Defaults for a freshly built Config.
const (
	DefaultPort = 8765
	DefaultName = "EinCast"

	maxNameLen = 64
)

// Config is the validated input to Service.Start.
type Config struct {
	Port int
	Name string
	// DLNA turns on Smart TV discovery and the pages TVs browse.
	DLNA bool
	// KeepAwake holds the computer awake while the server runs.
	KeepAwake bool
}

// Option customizes a Config.
type Option func(*Config)

// WithPort sets the listen port. 0 asks the OS for a free port (used in tests).
func WithPort(port int) Option {
	return func(c *Config) { c.Port = port }
}

// WithName sets the server's display name.
func WithName(name string) Option {
	return func(c *Config) { c.Name = strings.TrimSpace(name) }
}

// WithDLNA turns Smart TV (DLNA) discovery on or off.
func WithDLNA(on bool) Option {
	return func(c *Config) { c.DLNA = on }
}

// WithKeepAwake keeps the computer from sleeping while the server runs. It only
// has an effect when the Service was given a PowerGuard.
func WithKeepAwake(on bool) Option {
	return func(c *Config) { c.KeepAwake = on }
}

// NewConfig returns the defaults with opts applied. It does not validate:
// Service.Start does, so there is a single place that enforces the rules.
func NewConfig(opts ...Option) Config {
	c := Config{Port: DefaultPort, Name: DefaultName}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// Validate reports whether the config can be used to start a server. The name
// ends up in headers, JSON and playlists, so control characters are rejected.
func (c Config) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port %d out of range 0-65535", c.Port)
	}
	if c.Name == "" {
		return errors.New("name is empty")
	}
	if len([]rune(c.Name)) > maxNameLen {
		return fmt.Errorf("name longer than %d characters", maxNameLen)
	}
	if strings.IndexFunc(c.Name, unicode.IsControl) >= 0 {
		return errors.New("name contains control characters")
	}
	return nil
}
