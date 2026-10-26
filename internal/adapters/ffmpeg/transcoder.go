package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"eincast/internal/adapters/safepath"
	"eincast/internal/app"
)

// Transcoder converts videos to HLS with the installed ffmpeg. It implements
// app.Transcoder.
type Transcoder struct {
	finder
	baseDir string // each conversion gets a folder in here
}

var _ app.Transcoder = (*Transcoder)(nil)

// NewTranscoder returns a Transcoder that works in folders under baseDir. Whatever
// an earlier run left there (a crash, a killed process) is deleted now: nothing
// in it can be served again.
func NewTranscoder(baseDir string, log *slog.Logger, opts ...Option) *Transcoder {
	t := &Transcoder{baseDir: baseDir}
	t.init(log.With(slog.String("source", "transcode")), opts)
	t.clearStale()
	return t
}

func (t *Transcoder) clearStale() {
	entries, err := os.ReadDir(t.baseDir)
	if err != nil {
		return // nothing there yet
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(t.baseDir, e.Name()))
	}
}

// Start makes a folder for the output and starts converting in the background.
// The probing and ffmpeg itself run in a goroutine owned by the returned stream;
// Stop ends it.
func (t *Transcoder) Start(ctx context.Context, req app.TranscodeRequest) (app.HLSStream, error) {
	bin, err := t.binary()
	if err != nil {
		return nil, app.ErrTranscodingUnavailable
	}
	// ffmpeg opens whatever it is told, so the guard that keeps the HTTP server
	// inside the library applies here too.
	file, err := safepath.Inside(req.Root, req.Path)
	if err != nil {
		return nil, fmt.Errorf("convert %q: %w", req.Path, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(t.baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create conversion folder: %w", err)
	}
	dir, err := os.MkdirTemp(t.baseDir, "s-")
	if err != nil {
		return nil, fmt.Errorf("create conversion folder: %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	st := &stream{
		log:    t.log,
		dir:    dir,
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	go st.run(runCtx, bin, probeBinary(bin), file)
	return st, nil
}

// probeBinary finds ffprobe: the one installed beside ffmpeg, else on PATH, else
// "" (the video is then converted without looking first).
func probeBinary(ffmpegPath string) string {
	name := "ffprobe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	beside := filepath.Join(filepath.Dir(ffmpegPath), name)
	if info, err := os.Stat(beside); err == nil && !info.IsDir() {
		return beside
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return ""
}

// outputName is every file name a stream serves: the playlist and its segments.
// Nothing else is ever opened, whatever the request says.
var outputName = regexp.MustCompile(`^(index\.m3u8|seg[0-9]{5}\.ts)$`)

// readyPoll is how often the output folder is checked for the first segments.
const readyPoll = 150 * time.Millisecond

// stream is one running conversion. It implements app.HLSStream.
type stream struct {
	log    *slog.Logger
	dir    string
	cancel context.CancelFunc

	ready     chan struct{}
	readyOnce sync.Once
	done      chan struct{}

	mu       sync.Mutex
	err      error // written before done is closed
	stopping bool
	removed  bool
}

func (s *stream) Ready() <-chan struct{} { return s.ready }
func (s *stream) Done() <-chan struct{}  { return s.done }

func (s *stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// run probes, converts and reports. It is the only writer of err and done.
func (s *stream) run(ctx context.Context, ffmpegPath, ffprobePath, file string) {
	defer close(s.done)

	var streams []streamInfo
	if ffprobePath != "" {
		var err error
		if streams, err = probe(ctx, ffprobePath, file); err != nil && ctx.Err() == nil {
			// Converting everything still works; it is only slower.
			s.log.Debug("probe failed; converting everything", slog.String("error", err.Error()))
		}
	}
	p := makePlan(streams)
	s.log.Debug("converting", slog.Bool("copy_video", p.copyVideo), slog.Bool("copy_audio", p.copyAudio))

	// The readiness check ends with the conversion, one way or another, and is
	// waited for so nothing of ours runs after Done is closed.
	pollCtx, stopPoll := context.WithCancel(ctx)
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		s.pollReady(pollCtx)
	}()
	defer func() {
		stopPoll()
		<-pollDone
	}()

	cmd := exec.CommandContext(ctx, ffmpegPath, hlsArgs(file, s.dir, p)...)
	hideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, left: 4 << 10}
	runErr := cmd.Run()

	s.mu.Lock()
	stopped := s.stopping
	s.mu.Unlock()
	if runErr != nil && !stopped {
		s.setErr(fmt.Errorf("ffmpeg: %s", lastLine(stderr.String())))
		return
	}
	if runErr == nil {
		// Finished: playable even if it was shorter than the readiness threshold.
		if _, err := os.Stat(filepath.Join(s.dir, "index.m3u8")); err == nil {
			s.readyOnce.Do(func() { close(s.ready) })
		} else {
			s.setErr(errors.New("ffmpeg finished without writing a playlist"))
		}
	}
}

func (s *stream) setErr(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// pollReady closes ready once the playlist and the second segment exist: enough
// to start without stalling at the first segment boundary.
func (s *stream) pollReady(ctx context.Context) {
	ticker := time.NewTicker(readyPoll)
	defer ticker.Stop()
	for {
		if exists(filepath.Join(s.dir, "index.m3u8")) && exists(filepath.Join(s.dir, "seg00001.ts")) {
			s.readyOnce.Do(func() { close(s.ready) })
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Open opens the playlist or a segment.
func (s *stream) Open(name string) (io.ReadSeekCloser, error) {
	if !outputName.MatchString(name) {
		return nil, fs.ErrNotExist
	}
	s.mu.Lock()
	removed := s.removed
	s.mu.Unlock()
	if removed {
		return nil, fs.ErrNotExist
	}
	return os.Open(filepath.Join(s.dir, name))
}

// Stop ends ffmpeg and deletes its output.
func (s *stream) Stop() {
	s.mu.Lock()
	if s.removed || s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	s.mu.Unlock()

	s.cancel()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		s.log.Warn("ffmpeg did not stop in time", slog.String("dir", s.dir))
	}

	s.mu.Lock()
	s.removed = true
	s.mu.Unlock()
	if err := os.RemoveAll(s.dir); err != nil {
		s.log.Warn("could not delete a conversion's files", slog.String("error", err.Error()))
	}
}

// probe asks ffprobe about the streams of a file.
func probe(ctx context.Context, ffprobePath, file string) ([]streamInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error", "-protocol_whitelist", "file,pipe",
		"-show_entries", "stream=codec_type,codec_name,pix_fmt",
		"-of", "json", "file:"+file)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	return parseProbe(out)
}
