// Package server models the lifecycle of EinCast's HTTP server as an explicit
// state machine, so "Start" and "Stop" can be clicked at any moment and an
// illegal sequence is rejected instead of corrupting state.
package server

import (
	"errors"
	"fmt"
	"sync"
)

// State is where the server is in its lifecycle.
type State int

// The lifecycle states. Valid moves:
//
//	Idle → Starting → Running → Stopping → Idle
//	Starting → Failed          (could not bind)
//	Running  → Failed          (died unexpectedly)
//	Stopping → Failed          (shutdown failed)
//	Failed   → Starting | Idle (retry, or acknowledge)
const (
	Idle State = iota
	Starting
	Running
	Stopping
	Failed
)

// String returns the lowercase name used on the wire ("idle", "running", ...).
func (s State) String() string {
	switch s {
	case Idle:
		return "idle"
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Stopping:
		return "stopping"
	case Failed:
		return "failed"
	default:
		return "unknown"
	}
}

// CanTransitionTo reports whether moving from s to next is a legal step.
func (s State) CanTransitionTo(next State) bool {
	switch s {
	case Idle:
		return next == Starting
	case Starting:
		return next == Running || next == Failed
	case Running:
		return next == Stopping || next == Failed
	case Stopping:
		return next == Idle || next == Failed
	case Failed:
		return next == Starting || next == Idle
	default:
		return false
	}
}

// ErrInvalidTransition is returned (wrapped) for an illegal state change.
var ErrInvalidTransition = errors.New("server: invalid state transition")

// Snapshot is a consistent view of the machine at one instant.
type Snapshot struct {
	State State
	// Err is why the machine is Failed; nil in every other state.
	Err error
}

// Machine holds the current State and enforces the transition table. Checking
// and changing happen under one lock, so two racing callers cannot both win.
type Machine struct {
	mu    sync.Mutex
	state State
	err   error
}

// NewMachine returns a Machine in the Idle state.
func NewMachine() *Machine {
	return &Machine{state: Idle}
}

// Transition moves to the given state, or returns an error wrapping
// ErrInvalidTransition and leaves the state unchanged. Entering any state other
// than Failed clears the recorded failure.
func (m *Machine) Transition(to State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveLocked(to, nil)
}

// Fail moves to Failed and records why. It is subject to the same table as
// Transition (Idle cannot fail; there is nothing to fail).
func (m *Machine) Fail(cause error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moveLocked(Failed, cause)
}

// Snapshot returns the current state and failure cause together.
func (m *Machine) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{State: m.state, Err: m.err}
}

func (m *Machine) moveLocked(to State, cause error) error {
	if !m.state.CanTransitionTo(to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, m.state, to)
	}
	m.state, m.err = to, cause
	return nil
}
