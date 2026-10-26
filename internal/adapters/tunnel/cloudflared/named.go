package cloudflared

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
)

// NamedID identifies the Cloudflare named tunnel.
const NamedID app.ProviderID = "cloudflare-named"

// registeredLine is what cloudflared logs, at Info level, when a connection to
// Cloudflare's network is up. (The Debug line "Registering tunnel connection" is
// the attempt, and is deliberately not matched.)
const registeredLine = "Registered tunnel connection"

// Named is a Cloudflare tunnel the person created in their own Cloudflare
// account: a stable address on their own domain. It runs `cloudflared tunnel
// run` with the tunnel's token.
//
// The token alone starts the tunnel, but it does not say what address the tunnel
// serves or what it forwards to; both are set in Cloudflare's dashboard. So the
// person tells us the host name (to show and to accept in the Host check), and
// must point the tunnel at this server's port in the dashboard.
type Named struct {
	*engine
}

var (
	_ app.Tunnel       = (*Named)(nil)
	_ app.Configurable = (*Named)(nil)
)

// NewNamed returns the named-tunnel provider. cfg holds the tunnel token (in the
// credential store, never in a file), the host name, and the optional location
// of cloudflared.
func NewNamed(cfg *providerconfig.Store, log *slog.Logger) *Named {
	return &Named{newEngine(NamedID, locator(cfg, NamedID), func(string) (launch, error) { return namedLaunch(cfg) }, log)}
}

// Fields implements app.Configurable.
func (*Named) Fields() []app.ConfigField {
	return []app.ConfigField{tokenField(), hostnameField(), binaryField()}
}

func tokenField() app.ConfigField {
	return app.ConfigField{
		Key:         "token",
		Label:       "Tunnel token",
		Help:        "In Cloudflare, open Zero Trust → Networks → Tunnels, choose your tunnel and copy the long token from its install command (the text after --token). It is kept in your computer's keychain.",
		Placeholder: "eyJhIjoi…",
		Kind:        app.FieldSecret,
		Required:    true,
	}
}

func hostnameField() app.ConfigField {
	return app.ConfigField{
		Key:         "hostname",
		Label:       "Hostname",
		Help:        "The public hostname you added to the tunnel in Cloudflare, for example media.example.com. In Cloudflare, point it at this server: http://localhost and the port shown on the Dashboard.",
		Placeholder: "media.example.com",
		Kind:        app.FieldText,
		Required:    true,
		Clean:       cleanHostname,
	}
}

// namedLaunch builds the command from the saved settings.
func namedLaunch(cfg *providerconfig.Store) (launch, error) {
	token, err := cfg.Get(NamedID, tokenField())
	if err != nil {
		return launch{}, err
	}
	host, err := cfg.Get(NamedID, hostnameField())
	if err != nil {
		return launch{}, err
	}
	if token == "" || host == "" {
		// Enable checks this first; this is the backstop for a value removed in between.
		return launch{}, &app.NotConfiguredError{Missing: []string{"Tunnel token", "Hostname"}}
	}

	url := app.PublicURL("https://" + host)
	return launch{
		args: []string{"tunnel", "--no-autoupdate", "run"},
		// The token goes in the environment (cloudflared reads TUNNEL_TOKEN), so it
		// is not on the command line for other users of the machine to read.
		env:     []string{"TUNNEL_TOKEN=" + token},
		secrets: []string{token},
		ready: func(line string) (app.PublicURL, bool) {
			return url, strings.Contains(line, registeredLine)
		},
	}, nil
}

// cleanHostname accepts a host name, tolerating what people paste: a full
// address, a trailing slash, capitals.
func cleanHostname(raw string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(raw))
	for _, scheme := range []string{"https://", "http://"} {
		h = strings.TrimPrefix(h, scheme)
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")

	switch {
	case h == "":
		return "", errors.New("enter the hostname")
	case strings.ContainsAny(h, ":@"):
		return "", errors.New("enter the hostname only, without a port or a user name")
	case len(h) > 253:
		return "", errors.New("that hostname is too long")
	case !strings.Contains(h, "."):
		return "", errors.New("enter the full hostname, such as media.example.com")
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if !validLabel(l) {
			return "", fmt.Errorf("%q is not a valid hostname", h)
		}
	}
	// An address like 203.0.113.7 is made of valid labels; it is not a hostname
	// Cloudflare can serve.
	if last := labels[len(labels)-1]; strings.Trim(last, "0123456789") == "" {
		return "", errors.New("enter a hostname, not an IP address")
	}
	return h, nil
}

// validLabel reports whether l is one part of a host name: 1 to 63 letters,
// digits and hyphens, not starting or ending with a hyphen.
func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, c := range l {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
