package fsscan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"eincast/internal/adapters/memrepo"
	"eincast/internal/domain/media"
)

// TestScale measures what a very large library costs. It is a measurement, not
// a regression test, so it only runs when EINCAST_SCALE_FILES is set:
//
//	EINCAST_SCALE_FILES=50000 go test ./internal/adapters/fsscan -run TestScale -v
//
// The numbers it prints go in docs/ROADMAP.md (Phase 13.7).
func TestScale(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("EINCAST_SCALE_FILES"))
	if n <= 0 {
		t.Skip("set EINCAST_SCALE_FILES to run")
	}

	root := t.TempDir()
	const perDir = 100
	start := time.Now()
	for i := 0; i < n; i++ {
		dir := filepath.Join(root, fmt.Sprintf("show-%04d", i/perDir))
		if i%perDir == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		// Every tenth file is a subtitle, so the sidecar matching is exercised.
		name := fmt.Sprintf("episode-%05d.mp4", i)
		if i%10 == 9 {
			name = fmt.Sprintf("episode-%05d.en.srt", i-1)
		}
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	t.Logf("generated %d files in %v", n, time.Since(start).Round(time.Millisecond))

	lib := media.NewLibrary(root)
	start = time.Now()
	items, err := newScanner().Scan(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scan:    %d items in %v", len(items), time.Since(start).Round(time.Millisecond))

	repo := memrepo.New()
	start = time.Now()
	repo.Replace(lib.ID, items)
	t.Logf("replace: %v", time.Since(start).Round(time.Microsecond))

	// library.Service.State() does this once per library on every event.
	start = time.Now()
	const rounds = 20
	for i := 0; i < rounds; i++ {
		media.Summarize(repo.Items(lib.ID))
	}
	t.Logf("state:   %v per call (Items copy + Summarize)", (time.Since(start) / rounds).Round(time.Microsecond))
}
