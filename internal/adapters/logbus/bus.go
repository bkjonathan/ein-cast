package logbus

import (
	"sync"

	"eincast/internal/app"
)

// Bus is a publish/subscribe hub for log entries with bounded memory: the ring
// holds the last N entries and every subscriber has a bounded channel.
//
// Publish never blocks. A subscriber that cannot keep up loses entries instead
// of slowing the HTTP server down (a log line must not stall a video stream).
type Bus struct {
	mu      sync.Mutex
	ring    *ring
	seq     uint64
	subs    map[chan app.LogEntry]struct{}
	dropped uint64
}

var _ app.LogStream = (*Bus)(nil)

// NewBus returns a Bus that remembers the last capacity entries.
func NewBus(capacity int) *Bus {
	return &Bus{ring: newRing(capacity), subs: make(map[chan app.LogEntry]struct{})}
}

// Publish stamps e with the next sequence number, stores it, and offers it to
// every subscriber without waiting.
func (b *Bus) Publish(e app.LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.seq++
	e.Seq = b.seq
	b.ring.push(e)

	for ch := range b.subs {
		select {
		case ch <- e:
		default:
			b.dropped++
		}
	}
}

// History returns the buffered entries, oldest first.
func (b *Bus) History() []app.LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ring.snapshot()
}

// Subscribe registers a live feed with the given buffer size. The returned
// cancel function unregisters and closes the feed; it is safe to call twice.
// Closing happens under the same lock as Publish, so a send on a closed channel
// cannot occur.
func (b *Bus) Subscribe(buffer int) (<-chan app.LogEntry, func()) {
	ch := make(chan app.LogEntry, max(buffer, 1))

	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return ch, cancel
}

// Dropped is how many (entry, subscriber) deliveries were skipped because a
// subscriber's buffer was full. Useful in tests and diagnostics.
func (b *Bus) Dropped() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}
