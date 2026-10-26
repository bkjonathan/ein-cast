// Package portproc finds the programs listening on a TCP port and ends them,
// using the tools each system already has: lsof on macOS and Linux, netstat and
// tasklist on Windows. It implements app.PortInspector.
package portproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"eincast/internal/app"
)

// lookupTimeout bounds each call to a system tool; lsof can be slow on a busy
// machine but should never hang the Settings screen.
const lookupTimeout = 10 * time.Second

// runFunc runs a program and returns its standard output. A field so tests
// can replace the operating system.
type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Inspector implements app.PortInspector.
type Inspector struct {
	goos string
	run  runFunc
}

var _ app.PortInspector = (*Inspector)(nil)

// New returns an Inspector for the operating system it runs on.
func New() *Inspector { return &Inspector{goos: runtime.GOOS, run: runCommand} }

// Holders returns the programs listening on port. port is an int, so nothing a
// person typed can reach the command line as anything but digits.
func (i *Inspector) Holders(ctx context.Context, port int) ([]app.PortHolder, error) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	if i.goos == "windows" {
		return i.holdersWindows(ctx, port)
	}
	return i.holdersUnix(ctx, port)
}

func (i *Inspector) holdersUnix(ctx context.Context, port int) ([]app.PortHolder, error) {
	// -F p c: machine-readable "p<pid>" and "c<command>" lines. +c 0 stops lsof
	// cutting command names to nine characters.
	out, err := i.run(ctx, "lsof", "-nP", "+c", "0", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpc")
	if err != nil {
		// lsof exits 1 when it finds nothing; that is an answer, not a failure.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && len(bytes.TrimSpace(out)) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof: %w", err)
	}
	return parseLsof(out), nil
}

func (i *Inspector) holdersWindows(ctx context.Context, port int) ([]app.PortHolder, error) {
	out, err := i.run(ctx, "netstat", "-ano", "-p", "tcp")
	if err != nil {
		return nil, fmt.Errorf("netstat: %w", err)
	}
	holders := parseNetstat(out, port)
	for n := range holders {
		holders[n].Name = i.windowsName(ctx, holders[n].PID)
	}
	return holders, nil
}

// windowsName is best effort: a program without a readable name is still shown
// by its process id.
func (i *Inspector) windowsName(ctx context.Context, pid int) string {
	out, err := i.run(ctx, "tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	if err != nil {
		return ""
	}
	return parseTasklist(out)
}

// Terminate asks a program to quit. Windows has no such request for programs
// without a window, so it ends them there.
func (i *Inspector) Terminate(pid int) error {
	if i.goos == "windows" {
		return i.Kill(pid)
	}
	return signal(pid, syscall.SIGTERM)
}

// Kill ends a program at once.
func (i *Inspector) Kill(pid int) error {
	return signal(pid, os.Kill)
}

func signal(pid int, sig os.Signal) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to signal pid %d", pid) // never the system's own processes
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return wrap(pid, err)
	}
	defer proc.Release() //nolint:errcheck // nothing to do if releasing the handle fails
	return wrap(pid, proc.Signal(sig))
}

// wrap turns the system's refusal into the app's error, and treats "already
// gone" as success: the program is no longer there, which is what was wanted.
func wrap(pid int, err error) error {
	switch {
	case err == nil, errors.Is(err, os.ErrProcessDone), errors.Is(err, syscall.ESRCH):
		return nil
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("pid %d: %w", pid, app.ErrNotPermitted)
	default:
		return fmt.Errorf("pid %d: %w", pid, err)
	}
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd.Output()
}
