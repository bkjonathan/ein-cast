// Package ffmpeg is the ffmpeg implementation of app.FrameGrabber. It runs the
// program installed on the computer; EinCast does not ship one.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"eincast/internal/adapters/safepath"
	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// maxOutput bounds what is read from ffmpeg. A 320-pixel JPEG is tens of
// kilobytes; anything near this is a runaway.
const maxOutput = 4 << 20

// seekSeconds is where in a video the picture is taken from: past the studio
// logos and black frames most videos open with. A video shorter than this has
// no frame there, so the grab is repeated from the start.
const seekSeconds = 8

// Grabber runs ffmpeg to make preview images.
type Grabber struct {
	finder
}

// New returns a Grabber that looks for ffmpeg on PATH and where installers put
// it, unless an option names a place.
func New(log *slog.Logger, opts ...Option) *Grabber {
	g := &Grabber{}
	g.init(log.With(slog.String("source", "thumbnails")), opts)
	return g
}

// Grab makes a JPEG of the first picture ffmpeg can find in the file: a video
// frame, a song's cover art, or the photo itself, shrunk to fit req.Width.
//
// Without ffmpeg a picture that is already small enough is returned as it is
// (see smallJPEG), so a folder's posters still show.
func (g *Grabber) Grab(ctx context.Context, req app.GrabRequest) ([]byte, error) {
	bin, binErr := g.binary()
	if binErr != nil && req.Kind != media.KindImage {
		return nil, app.ErrThumbnailsUnavailable
	}
	// ffmpeg opens whatever it is told, so the guard that keeps the HTTP server
	// inside the library applies here too.
	file, err := safepath.Inside(req.Root, req.Path)
	if err != nil {
		return nil, fmt.Errorf("thumbnail %q: %w", req.Path, err)
	}
	if binErr != nil {
		return smallJPEG(file, req.Kind)
	}

	seeks := []int{0}
	if req.Kind == media.KindVideo {
		seeks = []int{seekSeconds, 0}
	}
	var lastErr error
	for _, seek := range seeks {
		out, err := g.run(ctx, bin, args(file, req.Width, seek))
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		lastErr = err
	}
	return nil, lastErr
}

// maxUnshrunk is the largest picture served without being shrunk by ffmpeg: a
// poster of this size is already a fair thumbnail, and anything bigger would
// make a page of them slow.
const maxUnshrunk = 512 << 10

// smallJPEG is what a thumbnail is when ffmpeg is missing: a photo or poster that
// is a JPEG and small is its own thumbnail. Anything else needs ffmpeg.
func smallJPEG(file string, kind media.Kind) ([]byte, error) {
	if kind != media.KindImage {
		return nil, app.ErrThumbnailsUnavailable
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", app.ErrNoThumbnail, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxUnshrunk+1))
	if err != nil || len(data) > maxUnshrunk || !isJPEG(data) {
		return nil, app.ErrThumbnailsUnavailable
	}
	return data, nil
}

// args builds the command line. The input is given as "file:<path>" so that a
// name which looks like a URL ("http:…", "concat:…") is still read as a file,
// and only the file and pipe protocols are allowed at all.
func args(file string, width, seek int) []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "file,pipe"}
	if seek > 0 {
		a = append(a, "-ss", strconv.Itoa(seek))
	}
	return append(a,
		"-i", "file:"+file,
		"-an", "-sn",
		"-frames:v", "1",
		// Shrink to the width, keep the proportions, never enlarge.
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", width),
		"-c:v", "mjpeg", "-q:v", "5",
		"-f", "image2pipe", "pipe:1",
	)
}

func (g *Grabber) run(ctx context.Context, bin string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	hideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, left: maxOutput}
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4 << 10}

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// ffmpeg ran and refused the file: nothing to show.
			return nil, fmt.Errorf("%w: ffmpeg: %s", app.ErrNoThumbnail, lastLine(stderr.String()))
		}
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	if !isJPEG(stdout.Bytes()) {
		// Exit 0 with no picture: a video shorter than the seek, a song without
		// cover art.
		return nil, fmt.Errorf("%w: ffmpeg produced no picture", app.ErrNoThumbnail)
	}
	return stdout.Bytes(), nil
}

func isJPEG(b []byte) bool {
	return len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8
}

// lastLine is the most useful line of ffmpeg's complaint: the last one.
func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if s == "" {
		return "failed without a message"
	}
	return s
}

// limitedWriter keeps the first `left` bytes and discards the rest, so a
// misbehaving program cannot fill memory.
type limitedWriter struct {
	w    *bytes.Buffer
	left int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if l.left <= 0 {
		return n, nil
	}
	if len(p) > l.left {
		p = p[:l.left]
	}
	l.left -= len(p)
	l.w.Write(p)
	return n, nil
}

// locate finds ffmpeg: on PATH first, then where installers put it. A program
// started from the Finder or the Start menu does not get the shell's PATH, so
// on macOS Homebrew's folder would otherwise never be searched.
func locate() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	for _, p := range installLocations(runtime.GOOS) {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("ffmpeg was not found")
}

func installLocations(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/opt/local/bin/ffmpeg"}
	case "windows":
		var out []string
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			out = append(out, filepath.Join(dir, "Microsoft", "WinGet", "Links", "ffmpeg.exe"))
		}
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if dir := os.Getenv(env); dir != "" {
				out = append(out, filepath.Join(dir, "ffmpeg", "bin", "ffmpeg.exe"))
			}
		}
		return append(out, `C:\ffmpeg\bin\ffmpeg.exe`)
	default:
		return nil
	}
}
