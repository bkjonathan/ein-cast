package logbus

import (
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
)

func recv(t *testing.T, ch <-chan app.LogEntry) app.LogEntry {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatal("channel closed, want an entry")
		}
		return e
	case <-time.After(time.Second):
		t.Fatal("no entry within 1s")
	}
	return app.LogEntry{}
}

func TestBus_AssignsIncreasingSeq(t *testing.T) {
	b := NewBus(10)
	live, cancel := b.Subscribe(10)
	defer cancel()

	for range 3 {
		b.Publish(app.LogEntry{Message: "x", Seq: 777}) // caller-supplied Seq is overwritten
	}
	for want := uint64(1); want <= 3; want++ {
		if got := recv(t, live).Seq; got != want {
			t.Fatalf("Seq = %d, want %d", got, want)
		}
	}
}

func TestBus_HistoryServesLateSubscribers(t *testing.T) {
	b := NewBus(3)
	for i := 1; i <= 5; i++ {
		b.Publish(app.LogEntry{Message: "m"})
	}
	if got := seqs(b.History()); !equal(got, []uint64{3, 4, 5}) {
		t.Fatalf("History() seqs = %v, want the last 3", got)
	}

	live, cancel := b.Subscribe(4)
	defer cancel()
	b.Publish(app.LogEntry{Message: "later"})
	if got := recv(t, live).Seq; got != 6 {
		t.Fatalf("live Seq = %d, want 6 (history and live line up by Seq)", got)
	}
}

func TestBus_FansOutToEverySubscriber(t *testing.T) {
	b := NewBus(5)
	a, cancelA := b.Subscribe(5)
	c, cancelC := b.Subscribe(5)
	defer cancelA()
	defer cancelC()

	b.Publish(app.LogEntry{Message: "hello"})
	if recv(t, a).Message != "hello" || recv(t, c).Message != "hello" {
		t.Fatal("not every subscriber received the entry")
	}
}

// The property the whole design exists for: a subscriber that never reads must
// not slow down, or block, the code that logs.
func TestBus_SlowSubscriberNeverBlocksPublishers(t *testing.T) {
	b := NewBus(10)
	_, cancelStuck := b.Subscribe(1) // never drained
	defer cancelStuck()
	healthy, cancelHealthy := b.Subscribe(1000)
	defer cancelHealthy()

	const n = 500
	done := make(chan struct{})
	go func() {
		for range n {
			b.Publish(app.LogEntry{Message: "spam"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a subscriber that is not reading")
	}

	if got := b.Dropped(); got != n-1 { // the stuck subscriber's buffer holds exactly one
		t.Errorf("Dropped() = %d, want %d", got, n-1)
	}
	if len(healthy) != n {
		t.Errorf("healthy subscriber got %d entries, want %d (a stuck peer must not cost it any)", len(healthy), n)
	}
}

func TestBus_CancelClosesAndIsIdempotent(t *testing.T) {
	b := NewBus(5)
	live, cancel := b.Subscribe(2)

	cancel()
	cancel() // must not panic with "close of closed channel"

	if _, ok := <-live; ok {
		t.Fatal("channel still open after cancel")
	}
	b.Publish(app.LogEntry{Message: "after cancel"}) // must not panic with "send on closed channel"
}

// Run with -race: publishers, subscribers coming and going, and History readers.
func TestBus_ConcurrentUse(t *testing.T) {
	b := NewBus(50)
	var wg sync.WaitGroup

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				b.Publish(app.LogEntry{Message: "p"})
			}
		}()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				live, cancel := b.Subscribe(4)
				select {
				case <-live:
				default:
				}
				_ = b.History()
				cancel()
			}
		}()
	}
	wg.Wait()

	if got := len(b.History()); got != 50 {
		t.Errorf("History() has %d entries, want 50 (the ring is full)", got)
	}
}
