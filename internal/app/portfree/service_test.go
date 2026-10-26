package portfree

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
)

const selfPID = 100

var fastTiming = Timing{Grace: 30 * time.Millisecond, Force: 30 * time.Millisecond, Poll: time.Millisecond}

// fakeInspector holds a set of listeners and lets a test say which request
// makes a program let go of its port.
type fakeInspector struct {
	mu       sync.Mutex
	holders  []app.PortHolder
	listErr  error
	termErr  error
	killErr  error
	releases string // "terminate", "kill" or "" for never
	calls    []string
}

func (f *fakeInspector) Holders(context.Context, int) ([]app.PortHolder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]app.PortHolder(nil), f.holders...), f.listErr
}

func (f *fakeInspector) Terminate(pid int) error { return f.end("terminate", pid, f.termErr) }
func (f *fakeInspector) Kill(pid int) error      { return f.end("kill", pid, f.killErr) }

func (f *fakeInspector) end(how string, pid int, err error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, how)
	if err != nil {
		return err
	}
	if f.releases == how {
		for i, h := range f.holders {
			if h.PID == pid {
				f.holders = append(f.holders[:i], f.holders[i+1:]...)
				break
			}
		}
	}
	return nil
}

func newService(f *fakeInspector) *Service {
	return NewService(f, selfPID, fastTiming, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestCheck(t *testing.T) {
	f := &fakeInspector{holders: []app.PortHolder{{PID: 300, Name: "node"}, {PID: selfPID, Name: "eincast"}}}
	got, err := newService(f).Check(context.Background(), 8765)
	if err != nil {
		t.Fatal(err)
	}
	want := []Holder{
		{PortHolder: app.PortHolder{PID: selfPID, Name: "eincast"}, Self: true},
		{PortHolder: app.PortHolder{PID: 300, Name: "node"}},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Check = %+v, want %+v (sorted by pid, own process marked)", got, want)
	}
}

func TestCheckRejectsPortsOutsideTheRange(t *testing.T) {
	for _, port := range []int{-1, 0, 80, 1023, 65536} {
		f := &fakeInspector{}
		if _, err := newService(f).Check(context.Background(), port); !errors.Is(err, ErrInvalidPort) {
			t.Errorf("Check(%d) error = %v, want ErrInvalidPort", port, err)
		}
	}
}

func TestCheckReportsLookupFailure(t *testing.T) {
	boom := errors.New("lsof missing")
	f := &fakeInspector{listErr: boom}
	if _, err := newService(f).Check(context.Background(), 8765); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
}

func TestFree(t *testing.T) {
	tests := []struct {
		name      string
		holders   []app.PortHolder
		pid       int
		releases  string
		termErr   error
		killErr   error
		wantErr   error
		wantCalls []string
	}{
		{
			name:      "a program that quits when asked is not forced",
			holders:   []app.PortHolder{{PID: 300, Name: "node"}},
			pid:       300,
			releases:  "terminate",
			wantCalls: []string{"terminate"},
		},
		{
			name:      "a program that ignores the request is ended by force",
			holders:   []app.PortHolder{{PID: 300, Name: "node"}},
			pid:       300,
			releases:  "kill",
			wantCalls: []string{"terminate", "kill"},
		},
		{
			name:      "a program that survives everything is reported",
			holders:   []app.PortHolder{{PID: 300, Name: "node"}},
			pid:       300,
			wantErr:   ErrStillHeld,
			wantCalls: []string{"terminate", "kill"},
		},
		{
			name:    "EinCast never ends itself",
			holders: []app.PortHolder{{PID: selfPID, Name: "eincast"}},
			pid:     selfPID,
			wantErr: ErrOwnProcess,
		},
		{
			name:    "a process that is not on the port is left alone",
			holders: []app.PortHolder{{PID: 300, Name: "node"}},
			pid:     301,
			wantErr: ErrNotHolding,
		},
		{
			name:    "a free port has nothing to end",
			pid:     300,
			wantErr: ErrNotHolding,
		},
		{
			name:      "a refusal by the system is passed on",
			holders:   []app.PortHolder{{PID: 300, Name: "postgres"}},
			pid:       300,
			termErr:   app.ErrNotPermitted,
			wantErr:   app.ErrNotPermitted,
			wantCalls: []string{"terminate"},
		},
		{
			name:      "a refusal of the forced end is passed on",
			holders:   []app.PortHolder{{PID: 300, Name: "postgres"}},
			pid:       300,
			killErr:   app.ErrNotPermitted,
			wantErr:   app.ErrNotPermitted,
			wantCalls: []string{"terminate", "kill"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeInspector{holders: tt.holders, releases: tt.releases, termErr: tt.termErr, killErr: tt.killErr}
			err := newService(f).Free(context.Background(), 8765, tt.pid)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if len(f.calls) != len(tt.wantCalls) {
				t.Fatalf("calls = %v, want %v", f.calls, tt.wantCalls)
			}
			for i := range f.calls {
				if f.calls[i] != tt.wantCalls[i] {
					t.Fatalf("calls = %v, want %v", f.calls, tt.wantCalls)
				}
			}
		})
	}
}

func TestFreeStopsWaitingWhenCancelled(t *testing.T) {
	f := &fakeInspector{holders: []app.PortHolder{{PID: 300, Name: "node"}}}
	s := NewService(f, selfPID, Timing{Grace: time.Minute, Force: time.Minute, Poll: time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.Free(ctx, 8765, 300) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Free succeeded although the program never let go")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Free kept waiting after its context ended")
	}
}
