package loginitem

import (
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/sys/windows/registry"

	"eincast/internal/app"
)

const (
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "EinCast"
)

// Item is a value under the current user's Run key: no privileges needed, and
// it shows in Task Manager → Startup apps where the person can switch it off.
type Item struct {
	log *slog.Logger
	exe string
}

var _ app.LoginItem = (*Item)(nil)

// New returns an Item for the running program.
func New(log *slog.Logger) (*Item, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find the running program: %w", err)
	}
	return &Item{log: log.With(slog.String("source", "login")), exe: exe}, nil
}

// Set adds or removes the Run value.
func (i *Item) Set(enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the startup list: %w", err)
	}
	defer k.Close()

	if !enabled {
		if err := k.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove login entry: %w", err)
		}
		return nil
	}
	// Quoted: the path may hold spaces ("C:\Program Files\...").
	if err := k.SetStringValue(valueName, `"`+i.exe+`" `+BackgroundFlag); err != nil {
		return fmt.Errorf("write login entry: %w", err)
	}
	return nil
}
