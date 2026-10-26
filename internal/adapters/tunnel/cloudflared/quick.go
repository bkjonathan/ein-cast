// Package cloudflared holds the Cloudflare tunnel providers. They run the
// cloudflared command-line tool (not bundled: it is found on the machine) under
// a procsup.Supervisor and read from its output when the tunnel is up.
package cloudflared

import (
	"log/slog"

	"eincast/internal/app"
	"eincast/internal/app/providerconfig"
)

// QuickID identifies the account-less quick tunnel.
const QuickID app.ProviderID = "cloudflare-quick"

// Quick is the Cloudflare quick tunnel: `cloudflared tunnel --url …` gives a
// random https://<words>.trycloudflare.com address, no account needed. The
// address changes every time cloudflared starts, including when it is restarted
// after a crash, so the provider reports the current one through Status.
type Quick struct {
	*engine
}

var (
	_ app.Tunnel       = (*Quick)(nil)
	_ app.Configurable = (*Quick)(nil)
)

// NewQuick returns the quick-tunnel provider. cfg holds the optional saved
// location of cloudflared.
func NewQuick(cfg *providerconfig.Store, log *slog.Logger) *Quick {
	return newQuick(locator(cfg, QuickID), log)
}

// NewQuickAt returns the provider using the cloudflared at path, skipping the
// search. For tests that stand a script in for the real program.
func NewQuickAt(path string, log *slog.Logger) *Quick {
	return newQuick(func() (string, error) { return path, nil }, log)
}

func newQuick(locate func() (string, error), log *slog.Logger) *Quick {
	return &Quick{newEngine(QuickID, locate, quickLaunch, log)}
}

// Fields implements app.Configurable. A quick tunnel needs nothing but the program.
func (*Quick) Fields() []app.ConfigField { return []app.ConfigField{binaryField()} }

// quickLaunch forwards to the server and reads the address from the banner.
func quickLaunch(local string) (launch, error) {
	return launch{
		// --no-autoupdate: the tool must not replace itself while we supervise it.
		args:  []string{"tunnel", "--no-autoupdate", "--url", "http://" + local},
		ready: findQuickURL,
	}, nil
}
