//go:build !darwin && !windows

package powerguard

import (
	"errors"
	"log/slog"

	"eincast/internal/app"
)

// Guard does nothing on platforms EinCast does not target.
type Guard struct{}

var _ app.PowerGuard = (*Guard)(nil)

// New returns a Guard that always reports it cannot hold.
func New(*slog.Logger) *Guard { return &Guard{} }

// Hold reports that this platform has no support.
func (*Guard) Hold(string) (func(), error) {
	return nil, errors.New("keeping the computer awake is not supported on this system")
}
