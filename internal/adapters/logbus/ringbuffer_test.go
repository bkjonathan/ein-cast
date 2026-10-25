package logbus

import (
	"testing"

	"eincast/internal/app"
)

func seqs(es []app.LogEntry) []uint64 {
	out := make([]uint64, len(es))
	for i, e := range es {
		out[i] = e.Seq
	}
	return out
}

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRing(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		pushed   int
		want     []uint64
	}{
		{"empty", 3, 0, []uint64{}},
		{"partly filled", 3, 2, []uint64{1, 2}},
		{"exactly full", 3, 3, []uint64{1, 2, 3}},
		{"wrapped once", 3, 4, []uint64{2, 3, 4}},
		{"wrapped many times", 3, 10, []uint64{8, 9, 10}},
		{"capacity one keeps the newest", 1, 5, []uint64{5}},
		{"zero capacity is raised to one", 0, 3, []uint64{3}},
		{"negative capacity is raised to one", -4, 2, []uint64{2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRing(tt.capacity)
			for i := 1; i <= tt.pushed; i++ {
				r.push(app.LogEntry{Seq: uint64(i)})
			}
			if got := seqs(r.snapshot()); !equal(got, tt.want) {
				t.Fatalf("snapshot = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRing_SnapshotIsACopy(t *testing.T) {
	r := newRing(2)
	r.push(app.LogEntry{Seq: 1})
	snap := r.snapshot()
	snap[0].Seq = 99
	if got := r.snapshot()[0].Seq; got != 1 {
		t.Fatalf("mutating a snapshot changed the ring (Seq = %d)", got)
	}
}
