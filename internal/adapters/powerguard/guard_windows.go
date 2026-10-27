package powerguard

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"

	"eincast/internal/app"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var setThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

// Guard holds the system awake with SetThreadExecutionState.
//
// That call belongs to a thread, not the process: the request lasts only as long
// as the thread that made it. So each hold gets its own goroutine, pinned to one
// operating-system thread, which asks, waits to be released, and takes the
// request back before it ends.
type Guard struct {
	log *slog.Logger
}

var _ app.PowerGuard = (*Guard)(nil)

// New returns a Guard that logs to log.
func New(log *slog.Logger) *Guard {
	return &Guard{log: log.With(slog.String("source", "power"))}
}

// Hold asks Windows not to sleep until release is called.
func (g *Guard) Hold(reason string) (func(), error) {
	ready := make(chan error, 1)
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if r, _, err := setThreadExecutionState.Call(esContinuous | esSystemRequired); r == 0 {
			ready <- fmt.Errorf("SetThreadExecutionState: %w", err)
			return
		}
		ready <- nil
		<-stop
		setThreadExecutionState.Call(esContinuous) // back to normal
	}()

	if err := <-ready; err != nil {
		<-done
		return nil, err
	}
	g.log.Debug("holding Windows awake", slog.String("reason", reason))

	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}, nil
}
