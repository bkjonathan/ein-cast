package powerguard

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"sync"

	"eincast/internal/app"
)

// Guard holds a power assertion by running the system's own caffeinate tool.
// Using the tool instead of calling IOKit directly keeps cgo out of the build,
// and `-w` ties its life to ours: if EinCast is killed, caffeinate exits too
// and the Mac is free to sleep again.
type Guard struct {
	log *slog.Logger
	// command builds the process to run; a field so tests can use a stand-in.
	command func(pid int) *exec.Cmd
}

var _ app.PowerGuard = (*Guard)(nil)

// New returns a Guard that logs to log.
func New(log *slog.Logger) *Guard {
	return &Guard{
		log: log.With(slog.String("source", "power")),
		// -i: do not let the system sleep because it is idle. The display may
		// still sleep, which is what people expect from a media server.
		command: func(pid int) *exec.Cmd { return exec.Command("caffeinate", "-i", "-w", strconv.Itoa(pid)) },
	}
}

// Hold starts caffeinate. The returned release stops it and may be called any
// number of times.
func (g *Guard) Hold(reason string) (func(), error) {
	cmd := g.command(os.Getpid())
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start caffeinate: %w", err)
	}
	g.log.Debug("holding the Mac awake", slog.String("reason", reason), slog.Int("pid", cmd.Process.Pid))

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait() // reap it, so no zombie is left behind
		})
	}, nil
}
