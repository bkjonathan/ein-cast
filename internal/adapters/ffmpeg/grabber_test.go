package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

func TestArgs(t *testing.T) {
	t.Run("seeks before the input, and only when asked", func(t *testing.T) {
		with := args("/lib/a.mkv", 320, 8)
		i, j := slices.Index(with, "-ss"), slices.Index(with, "-i")
		if i < 0 || j < 0 || i > j || with[i+1] != "8" {
			t.Errorf("args = %v, want -ss 8 before -i", with)
		}
		if slices.Contains(args("/lib/a.mkv", 320, 0), "-ss") {
			t.Error("-ss present for a seek of 0")
		}
	})

	t.Run("reads the name as a file and limits protocols", func(t *testing.T) {
		got := args("/lib/concat:evil.mkv", 320, 0)
		if !slices.Contains(got, "file:/lib/concat:evil.mkv") {
			t.Errorf("args = %v, want the input prefixed with file:", got)
		}
		k := slices.Index(got, "-protocol_whitelist")
		if k < 0 || got[k+1] != "file,pipe" || k > slices.Index(got, "-i") {
			t.Errorf("args = %v, want -protocol_whitelist file,pipe before -i", got)
		}
	})

	t.Run("a name that looks like an option stays an argument of -i", func(t *testing.T) {
		got := args("/lib/-f", 320, 0)
		if i := slices.Index(got, "-i"); got[i+1] != "file:/lib/-f" {
			t.Errorf("args = %v", got)
		}
	})

	t.Run("shrinks to the width, never enlarges", func(t *testing.T) {
		got := strings.Join(args("/x.mp4", 240, 0), " ")
		if !strings.Contains(got, "scale='min(240,iw)':-2") {
			t.Errorf("args = %s", got)
		}
	})
}

func TestInstallLocations(t *testing.T) {
	if got := installLocations("darwin"); !slices.Contains(got, "/opt/homebrew/bin/ffmpeg") {
		t.Errorf("darwin locations = %v, want Homebrew's (a GUI app has no shell PATH)", got)
	}
	if got := installLocations("plan9"); got != nil {
		t.Errorf("unknown OS locations = %v, want none", got)
	}
}

func TestGrab_WithoutFFmpeg(t *testing.T) {
	g := New(slog.New(slog.DiscardHandler))
	g.find = func() (string, error) { return "", errors.New("ffmpeg was not found") }

	if g.Available() {
		t.Error("Available() = true")
	}
	_, err := g.Grab(context.Background(), app.GrabRequest{Root: "/lib", Path: "/lib/a.mp4", Kind: media.KindVideo, Width: 320})
	if !errors.Is(err, app.ErrThumbnailsUnavailable) {
		t.Fatalf("error = %v, want ErrThumbnailsUnavailable", err)
	}
}

func TestGrab_FindsFFmpegLaterWithoutARestart(t *testing.T) {
	g := New(slog.New(slog.DiscardHandler))
	found := false
	g.find = func() (string, error) {
		if !found {
			return "", errors.New("ffmpeg was not found")
		}
		return "/somewhere/ffmpeg", nil
	}
	if g.Available() {
		t.Fatal("Available() = true before install")
	}
	found = true
	if !g.Available() {
		t.Fatal("Available() = false after install")
	}
}

// The tests below run the real program and are skipped where it is missing.

func realGrabber(t *testing.T) *Grabber {
	t.Helper()
	g := New(slog.New(slog.DiscardHandler))
	if !g.Available() {
		t.Skip("ffmpeg is not installed")
	}
	return g
}

// makeMedia creates a file with ffmpeg's own test generators and returns its path.
func makeMedia(t *testing.T, g *Grabber, name string, args ...string) string {
	t.Helper()
	bin, _ := g.binary()
	path := filepath.Join(t.TempDir(), name)
	full := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
	full = append(full, path)
	if out, err := exec.Command(bin, full...).CombinedOutput(); err != nil {
		t.Skipf("cannot generate %s with this ffmpeg: %v: %s", name, err, out)
	}
	return path
}

func decode(t *testing.T, data []byte) image.Config {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a JPEG: %v", err)
	}
	return cfg
}

func grab(t *testing.T, g *Grabber, path string, kind media.Kind) ([]byte, error) {
	t.Helper()
	return g.Grab(context.Background(), app.GrabRequest{Root: filepath.Dir(path), Path: path, Kind: kind, Width: 320})
}

func TestGrab_VideoFrameIsShrunkToTheWidth(t *testing.T) {
	g := realGrabber(t)
	path := makeMedia(t, g, "long.mp4", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=10:duration=12", "-pix_fmt", "yuv420p")

	data, err := grab(t, g, path, media.KindVideo)
	if err != nil {
		t.Fatalf("Grab() error = %v", err)
	}
	if cfg := decode(t, data); cfg.Width != 320 || cfg.Height != 180 {
		t.Errorf("thumbnail is %dx%d, want 320x180 (width limit, proportions kept)", cfg.Width, cfg.Height)
	}
}

func TestGrab_VideoShorterThanTheSeekFallsBackToTheStart(t *testing.T) {
	g := realGrabber(t)
	path := makeMedia(t, g, "short.mp4", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=10:duration=2", "-pix_fmt", "yuv420p")

	data, err := grab(t, g, path, media.KindVideo)
	if err != nil {
		t.Fatalf("Grab() error = %v, want a frame from the start of a 2 second video", err)
	}
	decode(t, data)
}

func TestGrab_SmallPicturesAreNotEnlarged(t *testing.T) {
	g := realGrabber(t)
	path := makeMedia(t, g, "tiny.png", "-f", "lavfi", "-i", "testsrc=size=100x50:rate=1:duration=1", "-frames:v", "1")

	data, err := grab(t, g, path, media.KindImage)
	if err != nil {
		t.Fatalf("Grab() error = %v", err)
	}
	if cfg := decode(t, data); cfg.Width != 100 || cfg.Height != 50 {
		t.Errorf("thumbnail is %dx%d, want the original 100x50", cfg.Width, cfg.Height)
	}
}

func TestGrab_SongWithoutCoverArtHasNoThumbnail(t *testing.T) {
	g := realGrabber(t)
	path := makeMedia(t, g, "plain.wav", "-f", "lavfi", "-i", "sine=duration=1")

	_, err := grab(t, g, path, media.KindAudio)
	if !errors.Is(err, app.ErrNoThumbnail) {
		t.Fatalf("error = %v, want ErrNoThumbnail", err)
	}
}

func TestGrab_SongWithCoverArtShowsTheCover(t *testing.T) {
	g := realGrabber(t)
	cover := makeMedia(t, g, "cover.png", "-f", "lavfi", "-i", "testsrc=size=500x500:rate=1:duration=1", "-frames:v", "1")
	path := makeMedia(t, g, "song.mp3",
		"-f", "lavfi", "-i", "sine=duration=1", "-i", cover,
		"-map", "0:a", "-map", "1:v", "-c:a", "libmp3lame", "-c:v", "copy", "-id3v2_version", "3",
		"-metadata:s:v", "title=Album cover", "-metadata:s:v", "comment=Cover (front)")

	data, err := grab(t, g, path, media.KindAudio)
	if err != nil {
		t.Fatalf("Grab() error = %v", err)
	}
	if cfg := decode(t, data); cfg.Width != 320 {
		t.Errorf("cover is %d wide, want 320", cfg.Width)
	}
}

func TestGrab_AFileThatIsNotMediaHasNoThumbnail(t *testing.T) {
	g := realGrabber(t)
	path := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(path, []byte("this is not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := grab(t, g, path, media.KindVideo)
	if !errors.Is(err, app.ErrNoThumbnail) {
		t.Fatalf("error = %v, want ErrNoThumbnail", err)
	}
}

func TestGrab_RefusesAFileOutsideTheLibrary(t *testing.T) {
	g := realGrabber(t)
	outside := makeMedia(t, g, "outside.png", "-f", "lavfi", "-i", "testsrc=size=64x64:rate=1:duration=1", "-frames:v", "1")

	_, err := g.Grab(context.Background(), app.GrabRequest{
		Root: t.TempDir(), Path: outside, Kind: media.KindImage, Width: 320,
	})
	if err == nil || errors.Is(err, app.ErrThumbnailsUnavailable) {
		t.Fatalf("error = %v, want a refusal for a file outside the library", err)
	}
}

func TestGrab_StopsWhenItsContextIsCancelled(t *testing.T) {
	g := realGrabber(t)
	path := makeMedia(t, g, "v.mp4", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=2", "-pix_fmt", "yuv420p")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := g.Grab(ctx, app.GrabRequest{Root: filepath.Dir(path), Path: path, Kind: media.KindVideo, Width: 320})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
