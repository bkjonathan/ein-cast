//go:build !darwin && !windows

package loginitem

import (
	"errors"
	"log/slog"

	"eincast/internal/app"
)

// Item is not available on platforms EinCast does not target.
type Item struct{}

var _ app.LoginItem = (*Item)(nil)

// New reports that this platform has no support.
func New(*slog.Logger) (*Item, error) {
	return nil, errors.New("opening at login is not supported on this system")
}

// Set is never reached: New fails first.
func (*Item) Set(bool) error { return errors.New("not supported") }
