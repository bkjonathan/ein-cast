package fswatch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestScale measures what watching a very large library costs. It is a
// measurement, not a regression test, so it only runs when EINCAST_SCALE_FILES
// is set:
//
//	EINCAST_SCALE_FILES=50000 go test ./internal/adapters/fswatch -run TestScale -v
//
// An error from Watch is a valid outcome (the system refused more handles);
// the library then falls back to manual rescans.
func TestScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("EINCAST_SCALE_FILES"))
	if n <= 0 {
		t.Skip("set EINCAST_SCALE_FILES to run")
	}

	root := t.TempDir()
	const perDir = 100
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, fmt.Sprintf("show-%04d", i/perDir))
		if i%perDir == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("episode-%05d.mp4", i)))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	changes, err := New(slog.New(slog.DiscardHandler)).Watch(ctx, root)
	if err != nil {
		t.Logf("watch refused after %v: %v", time.Since(start).Round(time.Millisecond), err)
		return
	}
	t.Logf("watch:   set up in %v", time.Since(start).Round(time.Millisecond))

	// A new file must still be noticed with everything else watched.
	start = time.Now()
	f, err := os.Create(filepath.Join(root, "show-0000", "new-episode.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	select {
	case <-changes:
		t.Logf("notice:  new file seen after %v", time.Since(start).Round(time.Millisecond))
	case <-time.After(10 * time.Second):
		t.Error("new file was not noticed within 10s")
	}
}
