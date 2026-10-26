//go:build !windows

package tray

import (
	"context"
	"log/slog"
)

// Run does nothing: macOS keeps a closed window in the Dock, and no other
// platform is targeted (ADR 0030).
func Run(context.Context, *slog.Logger, []byte, string, func(), func()) {}
