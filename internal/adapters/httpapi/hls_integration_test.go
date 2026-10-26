package httpapi

import (
	"bufio"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eincast/internal/adapters/ffmpeg"
	"eincast/internal/app"
	"eincast/internal/app/transcode"
	"eincast/internal/domain/media"
)

// TestHLS_EndToEnd runs the whole path with the real parts: HTTP handler,
// conversion service and the installed ffmpeg. A player's behaviour is
// reproduced by hand: fetch the playlist, then every segment it lists.
func TestHLS_EndToEnd(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "hls")
	tc := ffmpeg.NewTranscoder(cache, slog.New(slog.DiscardHandler))
	if !tc.Available() {
		t.Skip("ffmpeg is not installed")
	}

	root := t.TempDir()
	bin, _ := exec.LookPath("ffmpeg")
	if bin == "" {
		bin = "/opt/homebrew/bin/ffmpeg"
	}
	src := filepath.Join(root, "film.mkv")
	gen := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=25:duration=14",
		"-f", "lavfi", "-i", "sine=duration=14",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test video: %v: %s", err, out)
	}

	it := itemFor(t, root, "film.mkv", media.KindVideo)
	lib := &fakeLibrary{root: root, items: []media.Item{it}}
	svc := transcode.NewService(lib, tc, slog.New(slog.DiscardHandler), transcode.DefaultOptions)
	t.Cleanup(svc.StopAll)

	a := New(lib, &fakeAccess{}, &fakeRemote{}, "1.2.3", discardLog(), WithTranscoding(svc)).
		newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)

	base := "/hls/" + string(it.ID) + "/"
	resp := getFrom(t, srv, base+"index.m3u8")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("playlist status = %d: %s", resp.StatusCode, readBody(t, resp))
	}
	var segments []string
	sc := bufio.NewScanner(strings.NewReader(string(readBody(t, resp))))
	for sc.Scan() {
		if line := sc.Text(); strings.HasSuffix(line, ".ts") {
			segments = append(segments, line)
		}
	}
	if len(segments) == 0 {
		t.Fatal("the playlist lists no segments")
	}

	for _, seg := range segments {
		r := getFrom(t, srv, base+seg)
		body := readBody(t, r)
		if r.StatusCode != http.StatusOK || len(body) < 188 || body[0] != 0x47 {
			t.Fatalf("segment %s: status %d, %d bytes, first byte %#x", seg, r.StatusCode, len(body), firstByte(body))
		}
	}

	// A second playlist request reuses the conversion instead of starting another.
	if r := getFrom(t, srv, base+"index.m3u8"); r.StatusCode != http.StatusOK {
		t.Fatalf("second playlist status = %d", r.StatusCode)
	}
	if n := svc.Running(); n != 1 {
		t.Errorf("%d conversions running, want 1", n)
	}

	// Stopping leaves nothing on disk and nothing to fetch.
	svc.StopAll()
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, _ := os.ReadDir(cache)
		if len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("files left behind: %v", entries)
		}
		time.Sleep(20 * time.Millisecond)
	}
	assertErrorShape(t, getFrom(t, srv, base+segments[0]), http.StatusNotFound, codeNotFound)
}

func firstByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	return b[0]
}
