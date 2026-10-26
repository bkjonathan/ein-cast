// Package procsup supervises one long-running child process, such as the
// cloudflared tunnel client: it starts it, hands its output to a parser and to
// the log, restarts it with exponential backoff when it dies, and makes sure it
// is gone when the owner's context ends.
//
// One Supervisor owns one process at a time and one goroutine. The goroutine
// ends when the context is cancelled, Stop is called, or the process has failed
// too many times; Done reports it.
package procsup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Stream says which of the process's output streams a line came from.
type Stream int

// The two output streams.
const (
	Stdout Stream = iota
	Stderr
)

func (s Stream) String() string {
	if s == Stderr {
		return "stderr"
	}
	return "stdout"
}

// Defaults for the zero values of Spec's tuning fields.
const (
	DefaultMaxRestarts    = 5
	DefaultInitialBackoff = time.Second
	DefaultMaxBackoff     = 30 * time.Second
	DefaultStableAfter    = 30 * time.Second
	DefaultGracePeriod    = 5 * time.Second
)

// Spec describes the process to run and how to treat it.
//
// The callbacks run on the supervisor's goroutines, OnLine possibly from two at
// once (one per stream), so they must be safe for concurrent use and must not
// block: a slow OnLine stalls the child on a full pipe.
type Spec struct {
	// Name labels log lines and errors. It must not contain secrets.
	Name string
	// Path and Args are the command. Args are never logged: they may carry a token.
	Path string
	Args []string
	// Env adds variables to the parent's environment. Prefer it to Args for
	// secrets, since a command line is visible to every user of the machine.
	Env []string
	// Secrets are values that must never appear in the log or reach OnLine. If
	// the process echoes one (a tool that prints its configuration), it is
	// replaced by a placeholder first. A secret split across the 16 KiB cut that
	// very long lines are broken at would not be caught; real output has no such lines.
	Secrets []string

	// OnStart runs after each successful start of the process.
	OnStart func()
	// OnLine receives each line of output.
	OnLine func(stream Stream, line string)
	// OnExit runs after each time the process ends. restarting says whether the
	// supervisor will start it again. err is nil when the end was requested.
	OnExit func(err error, restarting bool)
	// Level picks the log level for a line; the default is Info.
	Level func(stream Stream, line string) slog.Level

	// MaxRestarts is how many times a process that keeps failing is started
	// again before the supervisor gives up. Zero means DefaultMaxRestarts;
	// negative means never restart.
	MaxRestarts int
	// InitialBackoff and MaxBackoff bound the wait before a restart, which
	// doubles after each failure.
	InitialBackoff, MaxBackoff time.Duration
	// StableAfter is how long a process must run to count as healthy: after
	// that, the failure count and the backoff start over.
	StableAfter time.Duration
	// GracePeriod is how long a process gets to exit after being asked to,
	// before it is killed.
	GracePeriod time.Duration
}

func (s Spec) withDefaults() Spec {
	if s.MaxRestarts == 0 {
		s.MaxRestarts = DefaultMaxRestarts
	}
	if s.InitialBackoff <= 0 {
		s.InitialBackoff = DefaultInitialBackoff
	}
	if s.MaxBackoff <= 0 {
		s.MaxBackoff = DefaultMaxBackoff
	}
	if s.StableAfter <= 0 {
		s.StableAfter = DefaultStableAfter
	}
	if s.GracePeriod <= 0 {
		s.GracePeriod = DefaultGracePeriod
	}
	return s
}

// Supervisor runs a Spec. Create one per run; a stopped Supervisor is not reused.
type Supervisor struct {
	spec Spec
	log  *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	err    error // written before done is closed, read only after
}

// New returns a Supervisor. log should already carry the caller's attributes.
func New(spec Spec, log *slog.Logger) *Supervisor {
	return &Supervisor{spec: spec.withDefaults(), log: log}
}

// Start launches the process and supervises it in the background. It returns
// once the first process has been spawned, so "no such file" or "permission
// denied" is reported here. ctx bounds the whole run: when it ends, the process
// is stopped and the Supervisor finishes.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		return errors.New("procsup: already started")
	}

	runCtx, cancel := context.WithCancel(ctx)
	p, err := s.spawn(runCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("start %s: %w", s.spec.Name, err)
	}
	s.cancel = cancel
	s.done = make(chan struct{})

	// The one goroutine. It exits when the loop returns, which it does on
	// cancellation or when the process has failed too often.
	go func() {
		defer close(s.done)
		defer cancel() // release runCtx when the loop ended on its own
		s.err = s.supervise(runCtx, p)
	}()
	return nil
}

// Stop asks the process to exit, kills it after the grace period, and waits for
// the Supervisor to finish or ctx to expire. Stopping one that was never started,
// or has finished, is not an error.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for %s to exit: %w", s.spec.Name, ctx.Err())
	}
}

// Done is closed once the Supervisor has finished and the process is gone. For
// one that was never started it is already closed.
func (s *Supervisor) Done() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done == nil {
		c := make(chan struct{})
		close(c)
		return c
	}
	return s.done
}

// Err explains why the Supervisor finished: nil for a requested stop, otherwise
// the last failure after giving up. Only meaningful after Done is closed.
func (s *Supervisor) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// process is one running instance.
type process struct {
	cmd      *exec.Cmd
	out, err *lineWriter
	started  time.Time
}

func (s *Supervisor) spawn(ctx context.Context) (*process, error) {
	cmd := exec.CommandContext(ctx, s.spec.Path, s.spec.Args...)
	if len(s.spec.Env) > 0 {
		cmd.Env = append(os.Environ(), s.spec.Env...)
	}
	p := &process{
		cmd: cmd,
		out: newLineWriter(func(l string) { s.line(Stdout, l) }),
		err: newLineWriter(func(l string) { s.line(Stderr, l) }),
	}
	cmd.Stdout, cmd.Stderr = p.out, p.err

	// Ask nicely first: a tunnel client closes its connections cleanly on an
	// interrupt. Windows has no such signal, so it is killed outright. Either
	// way, WaitDelay makes the kill certain.
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return cmd.Process.Kill()
		}
		return cmd.Process.Signal(os.Interrupt)
	}
	cmd.WaitDelay = s.spec.GracePeriod

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p.started = time.Now()
	s.log.Info("process started", slog.String("process", s.spec.Name), slog.Int("pid", cmd.Process.Pid))
	if s.spec.OnStart != nil {
		s.spec.OnStart()
	}
	return p, nil
}

// wait blocks until the process has exited and its output has been delivered.
func (p *process) wait() error {
	err := p.cmd.Wait()
	p.out.flush()
	p.err.flush()
	return err
}

// supervise runs p to its end and restarts it as the Spec allows. It returns nil
// when asked to stop, otherwise the error that made it give up.
func (s *Supervisor) supervise(ctx context.Context, p *process) error {
	failures, backoff := 0, s.spec.InitialBackoff

	for {
		if p != nil {
			exitErr := p.wait()
			if ctx.Err() != nil {
				s.exited(nil, false)
				return nil
			}
			if time.Since(p.started) >= s.spec.StableAfter {
				failures, backoff = 0, s.spec.InitialBackoff // it was healthy for a while
			}
			p = nil
			if exitErr == nil {
				exitErr = errors.New("exited")
			}
			if giveUp := s.failed(&failures, exitErr); giveUp != nil {
				return giveUp
			}
		}

		if !sleep(ctx, backoff) {
			s.exited(nil, false)
			return nil
		}
		backoff = min(backoff*2, s.spec.MaxBackoff)

		next, err := s.spawn(ctx)
		if err != nil {
			// A start that fails (the file vanished) counts like a crash.
			if giveUp := s.failed(&failures, err); giveUp != nil {
				return giveUp
			}
			continue
		}
		p = next
	}
}

// failed records one failure. It returns the error to give up with, or nil if
// the process will be started again.
func (s *Supervisor) failed(failures *int, cause error) error {
	*failures++
	restarting := *failures <= s.spec.MaxRestarts
	s.exited(cause, restarting)
	if restarting {
		s.log.Warn("process stopped; will restart",
			slog.String("process", s.spec.Name), slog.String("reason", cause.Error()),
			slog.Int("failures", *failures), slog.Int("max", s.spec.MaxRestarts))
		return nil
	}
	s.log.Error("process keeps stopping; giving up",
		slog.String("process", s.spec.Name), slog.String("reason", cause.Error()))
	return fmt.Errorf("%s stopped %d times in a row, giving up: %w", s.spec.Name, *failures, cause)
}

func (s *Supervisor) exited(err error, restarting bool) {
	if s.spec.OnExit != nil {
		s.spec.OnExit(err, restarting)
	}
}

// redacted is what a secret in the output is replaced with.
const redacted = "[redacted]"

// line logs one line of output and hands it to the parser.
func (s *Supervisor) line(stream Stream, line string) {
	for _, secret := range s.spec.Secrets {
		if secret != "" {
			line = strings.ReplaceAll(line, secret, redacted)
		}
	}
	level := slog.LevelInfo
	if s.spec.Level != nil {
		level = s.spec.Level(stream, line)
	}
	s.log.Log(context.Background(), level, line,
		slog.String("process", s.spec.Name), slog.String("stream", stream.String()))
	if s.spec.OnLine != nil {
		s.spec.OnLine(stream, line)
	}
}

// sleep waits d or until ctx ends, and reports whether the full time passed.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
