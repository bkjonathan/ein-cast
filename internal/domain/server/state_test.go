package server

import (
	"errors"
	"sync"
	"testing"
)

var allStates = []State{Idle, Starting, Running, Stopping, Failed}

// valid lists every legal edge. The test below checks all 25 (from, to) pairs
// against it, so adding an edge to the table without listing it here fails.
var valid = map[[2]State]bool{
	{Idle, Starting}:    true,
	{Starting, Running}: true,
	{Starting, Failed}:  true,
	{Running, Stopping}: true,
	{Running, Failed}:   true,
	{Stopping, Idle}:    true,
	{Stopping, Failed}:  true,
	{Failed, Starting}:  true,
	{Failed, Idle}:      true,
}

func TestCanTransitionTo_AllPairs(t *testing.T) {
	for _, from := range allStates {
		for _, to := range allStates {
			want := valid[[2]State{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s → %s: CanTransitionTo = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestCanTransitionTo_UnknownState(t *testing.T) {
	bogus := State(99)
	for _, s := range allStates {
		if bogus.CanTransitionTo(s) || s.CanTransitionTo(bogus) {
			t.Errorf("unknown state must neither be left nor entered (%s)", s)
		}
	}
}

func TestState_String(t *testing.T) {
	tests := map[State]string{
		Idle: "idle", Starting: "starting", Running: "running",
		Stopping: "stopping", Failed: "failed", State(99): "unknown",
	}
	for s, want := range tests {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}

func TestMachine_HappyPath(t *testing.T) {
	m := NewMachine()
	if m.Snapshot().State != Idle {
		t.Fatalf("new machine = %s, want idle", m.Snapshot().State)
	}
	for _, to := range []State{Starting, Running, Stopping, Idle} {
		if err := m.Transition(to); err != nil {
			t.Fatalf("Transition(%s) error = %v", to, err)
		}
		if got := m.Snapshot().State; got != to {
			t.Fatalf("state = %s, want %s", got, to)
		}
	}
}

func TestMachine_InvalidTransitionLeavesStateUnchanged(t *testing.T) {
	m := NewMachine()
	err := m.Transition(Running) // Idle → Running skips Starting
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}
	if got := m.Snapshot().State; got != Idle {
		t.Fatalf("state after rejected move = %s, want idle", got)
	}
}

func TestMachine_FailRecordsCauseAndLaterMovesClearIt(t *testing.T) {
	cause := errors.New("port in use")
	m := NewMachine()
	_ = m.Transition(Starting)

	if err := m.Fail(cause); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if snap := m.Snapshot(); snap.State != Failed || !errors.Is(snap.Err, cause) {
		t.Fatalf("snapshot = %+v, want Failed with cause", snap)
	}

	if err := m.Transition(Starting); err != nil { // retry
		t.Fatal(err)
	}
	if snap := m.Snapshot(); snap.State != Starting || snap.Err != nil {
		t.Fatalf("snapshot after retry = %+v, want Starting with no error", snap)
	}
}

func TestMachine_FailFromIdleIsRejectedAndRecordsNothing(t *testing.T) {
	m := NewMachine()
	if err := m.Fail(errors.New("x")); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Fail() from idle error = %v, want ErrInvalidTransition", err)
	}
	if snap := m.Snapshot(); snap.State != Idle || snap.Err != nil {
		t.Fatalf("snapshot = %+v, want untouched idle", snap)
	}
}

// Many goroutines race to start; exactly one may win. Run with -race.
func TestMachine_ConcurrentStartHasOneWinner(t *testing.T) {
	m := NewMachine()
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if m.Transition(Starting) == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d goroutines won the Idle → Starting transition, want exactly 1", wins)
	}
}
