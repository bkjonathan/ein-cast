// Package portfree holds the use case for a port that something else is using:
// see what holds it, and, only when the person says so, end that program.
package portfree

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"eincast/internal/app"
)

// Same range the server port setting accepts. Ports below 1024 belong to
// system services, and this feature is never the way to stop those.
const minPort, maxPort = 1024, 65535

var (
	// ErrInvalidPort is returned for a port outside the range above.
	ErrInvalidPort = errors.New("port must be between 1024 and 65535")
	// ErrOwnProcess is returned when asked to end EinCast itself. If the server
	// is what holds the port, the answer is to stop the server, not the app.
	ErrOwnProcess = errors.New("that is EinCast itself; stop the server instead")
	// ErrNotHolding is returned when the process is not listening on the port
	// (any more). It guards against a stale screen and against a process id that
	// the system has since given to something else.
	ErrNotHolding = errors.New("that program is no longer using this port")
	// ErrStillHeld is returned when the program kept the port even after being
	// ended.
	ErrStillHeld = errors.New("the port is still in use")
)

// Holder is a program using a port, as the UI shows it.
type Holder struct {
	app.PortHolder
	// Self marks EinCast's own process, which Free refuses to end.
	Self bool
}

// Timing is how long to wait for a program to let go of its port.
type Timing struct {
	// Grace is how long a polite request gets before the program is ended by force.
	Grace time.Duration
	// Force is how long to wait for the port to free after a forced end.
	Force time.Duration
	// Poll is how often to look while waiting.
	Poll time.Duration
}

// DefaultTiming suits servers that shut down in a moment or two.
var DefaultTiming = Timing{Grace: 3 * time.Second, Force: 2 * time.Second, Poll: 100 * time.Millisecond}

// Service finds and frees ports.
type Service struct {
	ports  app.PortInspector
	self   int
	timing Timing
	log    *slog.Logger
}

// NewService returns a Service. self is EinCast's own process id, which the
// caller supplies because this layer does not touch the operating system.
func NewService(ports app.PortInspector, self int, timing Timing, log *slog.Logger) *Service {
	return &Service{ports: ports, self: self, timing: timing, log: log.With(slog.String("source", "portfree"))}
}

// Check lists the programs using port, none if it is free.
func (s *Service) Check(ctx context.Context, port int) ([]Holder, error) {
	if err := validate(port); err != nil {
		return nil, err
	}
	found, err := s.ports.Holders(ctx, port)
	if err != nil {
		return nil, fmt.Errorf("check port %d: %w", port, err)
	}
	out := make([]Holder, len(found))
	for i, h := range found {
		out[i] = Holder{PortHolder: h, Self: h.PID == s.self}
	}
	slices.SortFunc(out, func(a, b Holder) int { return a.PID - b.PID })
	return out, nil
}

// Free ends the program with process id pid so that port is free again. It
// looks again first and acts only if pid is listening on port right now: the
// caller names a process, but never gets to end an arbitrary one. The program
// is asked to quit, and ended by force only if it ignores that.
func (s *Service) Free(ctx context.Context, port, pid int) error {
	holders, err := s.Check(ctx, port)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(holders, func(h Holder) bool { return h.PID == pid })
	switch {
	case i < 0:
		return ErrNotHolding
	case holders[i].Self:
		return ErrOwnProcess
	}
	target := holders[i]
	log := s.log.With(slog.Int("port", port), slog.Int("pid", pid), slog.String("name", target.Name))

	if err := s.ports.Terminate(pid); err != nil {
		return fmt.Errorf("end %s (pid %d): %w", target.Name, pid, err)
	}
	if s.waitReleased(ctx, port, pid, s.timing.Grace) {
		log.Info("freed port")
		return nil
	}
	// A cancelled request must not escalate to a forced end.
	if err := ctx.Err(); err != nil {
		return err
	}

	log.Warn("program ignored the request to quit; ending it by force")
	if err := s.ports.Kill(pid); err != nil {
		return fmt.Errorf("force-end %s (pid %d): %w", target.Name, pid, err)
	}
	if s.waitReleased(ctx, port, pid, s.timing.Force) {
		log.Info("freed port by force")
		return nil
	}
	return ErrStillHeld
}

// waitReleased polls until pid no longer listens on port, or the wait runs out
// or ctx ends. A lookup that fails counts as "not yet": the next poll may work.
func (s *Service) waitReleased(ctx context.Context, port, pid int, wait time.Duration) bool {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(s.timing.Poll)
	defer tick.Stop()
	for {
		if !s.holds(ctx, port, pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return !s.holds(ctx, port, pid)
		case <-tick.C:
		}
	}
}

func (s *Service) holds(ctx context.Context, port, pid int) bool {
	holders, err := s.ports.Holders(ctx, port)
	if err != nil {
		return true
	}
	return slices.ContainsFunc(holders, func(h app.PortHolder) bool { return h.PID == pid })
}

func validate(port int) error {
	if port < minPort || port > maxPort {
		return ErrInvalidPort
	}
	return nil
}
