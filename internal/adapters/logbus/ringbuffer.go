// Package logbus fans log records out to the UI. A slog.Handler decorator turns
// every record into an app.LogEntry, a Bus delivers it to live subscribers
// without ever blocking the logger, and a ring buffer keeps recent history for
// a UI that opens late. See ADR 0004.
package logbus

import "eincast/internal/app"

// ring is a fixed-capacity buffer that keeps the newest entries. It has no lock
// of its own: Bus guards it, so history and sequence numbers stay consistent.
type ring struct {
	buf   []app.LogEntry
	start int // index of the oldest entry
	n     int // number of valid entries
}

func newRing(capacity int) *ring {
	return &ring{buf: make([]app.LogEntry, max(capacity, 1))}
}

// push appends e, overwriting the oldest entry when full.
func (r *ring) push(e app.LogEntry) {
	if r.n < len(r.buf) {
		r.buf[(r.start+r.n)%len(r.buf)] = e
		r.n++
		return
	}
	r.buf[r.start] = e
	r.start = (r.start + 1) % len(r.buf)
}

// snapshot copies the entries out, oldest first.
func (r *ring) snapshot() []app.LogEntry {
	out := make([]app.LogEntry, r.n)
	for i := range out {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}
