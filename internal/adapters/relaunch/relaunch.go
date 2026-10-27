// Package relaunch opens EinCast again after it quits, for "Reset EinCast". It
// implements app.Relauncher (ADR 0034).
//
// A new copy cannot simply be started: the running one still holds the
// single-instance lock, so the new copy would hand over to it and exit. So the
// new copy is told the process id of the old one and waits for it to be gone
// before it does anything else (WaitIfRelaunched).
package relaunch

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"eincast/internal/app"
)

// AfterFlag is the command-line argument of a relaunched copy, as
// "--relaunch-after=<pid>": wait for that process to exit, then start normally.
const AfterFlag = "--relaunch-after"

// waitLimit is the longest a new copy waits for the old one. Past it the copy
// carries on; if the lock is still held it hands over and exits, which is no worse
// than not having relaunched.
const waitLimit = 15 * time.Second

const pollEvery = 100 * time.Millisecond

// Launcher starts the program that is running now.
type Launcher struct {
	exe    string
	exeErr error // why the program could not be found; Relaunch then reports it
	pid    int
	log    *slog.Logger
}

var _ app.Relauncher = (*Launcher)(nil)

// New returns a Launcher for the running program. If the program cannot be found
// the failure is kept and reported by Relaunch, when it matters.
func New(log *slog.Logger) *Launcher {
	exe, err := os.Executable()
	return &Launcher{exe: exe, exeErr: err, pid: os.Getpid(), log: log.With(slog.String("source", "relaunch"))}
}

// Relaunch starts a detached copy that waits for this process to exit. It returns
// at once; the caller must then quit.
func (l *Launcher) Relaunch() error {
	if l.exeErr != nil {
		return fmt.Errorf("find the running program: %w", l.exeErr)
	}
	name, args := command(runtime.GOOS, l.exe, l.pid)
	cmd := exec.Command(name, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Not waited for: it must outlive this process.
	if err := cmd.Process.Release(); err != nil {
		l.log.Warn("could not release the new process", slog.String("error", err.Error()))
	}
	l.log.Info("a new copy will open when this one has quit", slog.String("program", name))
	return nil
}

// command is what to run to open EinCast again. Inside a macOS app bundle it asks
// the system to open the bundle, as the Finder does (Dock icon, menu bar); -n is
// needed because the old copy is still running. Anywhere else, including a build
// made from source, it runs the program itself.
func command(goos, exe string, pid int) (name string, args []string) {
	flag := fmt.Sprintf("%s=%d", AfterFlag, pid)
	if bundle, ok := bundleOf(exe); ok && goos == "darwin" {
		return "/usr/bin/open", []string{"-n", bundle, "--args", flag}
	}
	return exe, []string{flag}
}

// bundleOf returns the .app folder a program path sits in.
func bundleOf(exe string) (string, bool) {
	const marker = ".app/Contents/MacOS/"
	i := strings.Index(exe, marker)
	if i < 0 {
		return "", false
	}
	return exe[:i+len(".app")], true
}

// WaitIfRelaunched blocks while the process named by AfterFlag is still running.
// main calls it first, before anything is read or written, so the old copy has
// finished with the files and the lock by the time this one starts. It does
// nothing for a normal launch.
func WaitIfRelaunched(args []string, log *slog.Logger) {
	pid, ok := parseAfter(args)
	if !ok {
		return
	}
	waitForExit(pid, waitLimit, pollEvery, alive, log)
}

// waitForExit polls until running(pid) is false or limit has passed. The probe is
// a parameter so a test can use a fake that does not need a real process.
func waitForExit(pid int, limit, every time.Duration, running func(pid int) bool, log *slog.Logger) {
	deadline := time.Now().Add(limit)
	for running(pid) {
		if time.Now().After(deadline) {
			log.Warn("the previous copy is still running; carrying on", slog.Int("pid", pid))
			return
		}
		time.Sleep(every)
	}
}

// parseAfter reads "--relaunch-after=<pid>" from the arguments.
func parseAfter(args []string) (int, bool) {
	for _, a := range args {
		v, found := strings.CutPrefix(a, AfterFlag+"=")
		if !found {
			continue
		}
		pid, err := strconv.Atoi(v)
		if err != nil || pid <= 0 {
			return 0, false
		}
		return pid, true
	}
	return 0, false
}
