package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"eincast/internal/domain/server"
)

// ProviderID names a tunnel provider: "tailscale", "cloudflare-quick", ...
type ProviderID string

// PublicURL is the address a tunnel makes the library reachable at.
type PublicURL string

// Host is the URL's host and port as a client's Host header spells it, lower
// case, or "" if the URL cannot be parsed.
func (u PublicURL) Host() string {
	parsed, err := url.Parse(string(u))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(parsed.Host), ".")
}

// TunnelStatus is a tunnel's own view of itself. The lifecycle reuses the
// server state machine's states: Idle is "off", Starting is "connecting" and
// Running is "online".
type TunnelStatus struct {
	State server.State
	// URL is where the library is reachable while Running.
	URL PublicURL
	// Error is why the tunnel failed while Failed; empty otherwise.
	Error string
}

// Tunnel exposes the local web server at a remote address. Providers (Tailscale,
// Cloudflare, ngrok) implement it and remote.Service picks one by ID, so the
// service never knows which one is running.
type Tunnel interface {
	ID() ProviderID
	// Public reports whether anyone on the Internet who learns the URL can reach
	// it. A private provider (a tailnet) is only reachable by the user's own
	// devices. Public providers are refused unless a PIN is set. There is no
	// default: an implementation must decide.
	Public() bool
	// Start brings the tunnel up and returns once the remote address is known.
	// ctx bounds the tunnel's whole lifetime: cancelling it takes the tunnel
	// down, as with WebServer.Start. srv is the running server: process-based
	// providers forward to its loopback address (see LoopbackAddr), and a
	// provider that is reached directly asks it to Bind another address.
	Start(ctx context.Context, srv LocalServer) (PublicURL, error)
	// Stop takes the tunnel down gracefully. Stopping a tunnel that is not
	// running is not an error.
	Stop(ctx context.Context) error
	// Status is the provider's live state, which can differ from what the
	// service asked for (a supervised process that is reconnecting).
	Status() TunnelStatus
}

// LoopbackAddr is the "host:port" a tunnel process should forward to.
func LoopbackAddr(srv LocalServer) (string, error) {
	for _, a := range srv.Addrs() {
		if a.Addr().IsLoopback() {
			return a.String(), nil
		}
	}
	return "", errors.New("server has no loopback address")
}

// FieldKind says how the UI asks for one value a provider needs.
type FieldKind string

// The kinds of field.
const (
	// FieldText is an ordinary value, shown back to the person.
	FieldText FieldKind = "text"
	// FieldSecret is a credential. It is write-only: once saved it is never sent
	// back to the UI, which only learns that it is set.
	FieldSecret FieldKind = "secret"
	// FieldFile is the path of a file, chosen with a file dialog or typed.
	FieldFile FieldKind = "file"
)

// ConfigField describes one value a provider needs from the person: a tunnel
// token, a host name, where a program is installed.
type ConfigField struct {
	// Key identifies the field within its provider: "token", "hostname".
	Key   string
	Label string
	// Help tells the person where to find the value.
	Help        string
	Placeholder string
	Kind        FieldKind
	// Required fields must be set before the provider can be turned on.
	Required bool
	// Shared stores the value under this name instead of the provider's, so two
	// providers that need the same thing (both Cloudflare tunnels need the
	// cloudflared program) ask once.
	Shared string
	// Clean checks and tidies what the person typed (trims it, rejects nonsense)
	// and returns what to store. Nil applies the defaults for the Kind.
	Clean func(raw string) (string, error)
}

// Configurable is implemented by providers that need something from the person
// before they can run.
type Configurable interface {
	Fields() []ConfigField
}

// FieldState is a field and what is saved for it.
type FieldState struct {
	Field ConfigField
	Set   bool
	// Value is the saved value for text and file fields. It is always empty for
	// secrets: they are not sent back out.
	Value string
}

// ProviderInfo describes a provider for a picker, without exposing the Tunnel.
type ProviderInfo struct {
	ID     ProviderID
	Public bool
	// Fields are the values the provider needs; empty for one that needs none.
	Fields []ConfigField
}

// TunnelRegistry finds providers by ID.
type TunnelRegistry interface {
	// Lookup returns an error wrapping ErrUnknownProvider for an unknown ID.
	Lookup(id ProviderID) (Tunnel, error)
	List() []ProviderInfo
}

// PublicAccess is what the server lifecycle needs from remote access: a way to
// make sure nothing is exposed once the server is going away.
type PublicAccess interface {
	// Disable takes any tunnel down. Disabling when nothing is on is not an error.
	Disable(ctx context.Context) error
}

// RemoteStatus describes remote access at one instant.
type RemoteStatus struct {
	// State is the lifecycle of the enabled tunnel (Idle when none is on).
	State server.State
	// Provider is the provider in use, or "" when none is.
	Provider ProviderID
	// Public is true while a public provider is on.
	Public bool
	// URL is the remote address while State is Running.
	URL PublicURL
	// Error is why remote access failed while State is Failed.
	Error string
	// Live is the provider's own view, e.g. "reconnecting" while State is Running.
	Live TunnelStatus
	// PINSet says whether an access PIN exists (needed for public providers).
	PINSet bool
	// NeedsSetup lists providers that cannot be turned on until the person has
	// supplied the values they need.
	NeedsSetup []ProviderID
	// Sessions counts signed-in remote browsers.
	Sessions int
}

// RemoteSession is a signed-in remote browser.
type RemoteSession struct {
	// Token is the cookie value that proves the sign-in. It is a credential.
	Token   string
	Expires time.Time
}

// SignedLink authorises one stream URL until Expires, for players that cannot
// hold cookies (VLC, Infuse). It is a credential.
type SignedLink struct {
	Sig     string
	Expires time.Time
}

// Errors of remote access. Callers match them with errors.Is / errors.As.
var (
	// ErrUnknownProvider means no provider has the requested ID.
	ErrUnknownProvider = errors.New("unknown remote access provider")
	// ErrPINRequired means a public provider was requested without an access PIN.
	ErrPINRequired = errors.New("public access needs an access PIN first")
	// ErrAlreadyEnabled means remote access is already on; turn it off to switch.
	ErrAlreadyEnabled = errors.New("remote access is already on")
	// ErrPublicActive means the action is not allowed while a public tunnel is on.
	ErrPublicActive = errors.New("turn off public access first")
	// ErrInvalidPIN means the proposed PIN breaks the rules.
	ErrInvalidPIN = errors.New("invalid PIN")
	// ErrWrongPIN means the PIN offered at sign-in did not match.
	ErrWrongPIN = errors.New("wrong PIN")
	// ErrNotPublic means a sign-in arrived while no public tunnel is on.
	ErrNotPublic = errors.New("public access is off")
	// ErrNotConfigured means a provider is missing values the person must supply.
	ErrNotConfigured = errors.New("this option needs to be set up first")
	// ErrProviderInUse means the provider is on, so its settings cannot change.
	ErrProviderInUse = errors.New("turn this option off before changing its settings")
	// ErrUnknownField means the provider has no such setting.
	ErrUnknownField = errors.New("unknown setting")
	// ErrInvalidValue means a value the person entered cannot be used.
	ErrInvalidValue = errors.New("invalid value")
)

// NotConfiguredError names what is missing. It matches ErrNotConfigured.
type NotConfiguredError struct {
	// Missing are the labels of the required fields that are not set.
	Missing []string
}

func (e *NotConfiguredError) Error() string {
	return "set this up first: " + strings.Join(e.Missing, ", ")
}

// Is lets errors.Is(err, ErrNotConfigured) match.
func (e *NotConfiguredError) Is(target error) bool { return target == ErrNotConfigured }

// ConfigKey is where a provider's saved value lives in the SecretStore.
func ConfigKey(provider ProviderID, f ConfigField) string {
	ns := string(provider)
	if f.Shared != "" {
		ns = f.Shared
	}
	return "provider/" + ns + "/" + f.Key
}

// LockedError means sign-in attempts are being refused for a while. It is the
// same whether the PIN would have been right or not.
type LockedError struct {
	RetryAfter time.Duration
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("too many sign-in attempts; try again in %s", e.RetryAfter.Round(time.Second))
}
