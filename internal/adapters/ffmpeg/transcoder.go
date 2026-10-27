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

	// freeBytes says how much room the disk holding baseDir has; a field so tests
	// can make it small.
	freeBytes func(dir string) (uint64, error)
	// goos picks which chip encoders to look for; a field for tests.
	goos string

	mu        sync.Mutex
	hardware  []encoder // chip encoders this ffmpeg has, once looked for
	hwChecked string    // the ffmpeg they were looked for in
}

var _ app.Transcoder = (*Transcoder)(nil)

// NewTranscoder returns a Transcoder that works in folders under baseDir. Whatever
// an earlier run left there (a crash, a killed process) is deleted now: nothing
// in it can be served again.
func NewTranscoder(baseDir string, log *slog.Logger, opts ...Option) *Transcoder {
	t := &Transcoder{baseDir: baseDir, freeBytes: freeBytes, goos: runtime.GOOS}
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
	if err := t.checkDiskSpace(file); err != nil {
		return nil, err
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
	go st.run(runCtx, job{
		ffmpeg: bin, ffprobe: probeBinary(bin), file: file, audio: req.AudioTrack,
		encoders: func(ctx context.Context) []encoder { return t.encoderChain(ctx, bin) },
	})
	return st, nil
}

// Disk use of a conversion is bounded by what it is told to leave free, not by
// its own size: the output is about as big as the source when copied, and
// usually smaller when encoded, but not by a margin worth trusting.
const (
	// diskHeadroom is what a conversion must leave free for everything else the
	// computer does.
	diskHeadroom = 1 << 30
	// diskEstimateCap caps the guess for one output, so a 60 GB 4K remux, which
	// becomes a 1080p stream a fraction of its size, is not refused on a disk
	// that has room for the result.
	diskEstimateCap = 8 << 30
)

// checkDiskSpace refuses to start when the disk holding the conversion folder
// would probably fill up. A failed check lets the conversion go ahead: not being
// able to ask is no reason to refuse a film, and ffmpeg still fails cleanly if
// the disk does fill.
func (t *Transcoder) checkDiskSpace(file string) error {
	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("convert %q: %w", file, err)
	}
	free, err := t.freeBytes(t.baseDir)
	if err != nil {
		t.log.Warn("could not check the free disk space", slog.String("error", err.Error()))
		return nil
	}
	need := min(uint64(info.Size()), diskEstimateCap) + diskHeadroom
	if free < need {
		t.log.Warn("not converting: the disk is too full",
			slog.Uint64("free_bytes", free), slog.Uint64("needed_bytes", need))
		return app.ErrNotEnoughDiskSpace
	}
	return nil
}

// encoderChain returns the encoders to try for a conversion, best first. The
// chip encoders ffmpeg has are looked up once per ffmpeg program.
func (t *Transcoder) encoderChain(ctx context.Context, bin string) []encoder {
	if t.hardwareOn == nil || !t.hardwareOn() {
		return encoderChain(nil, false)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hwChecked != bin {
		t.hardware = detectEncoders(ctx, bin, hardwareCandidates(t.goos))
		t.hwChecked = bin
		t.log.Debug("chip encoders in this ffmpeg", slog.Int("count", len(t.hardware)))
	}
	return encoderChain(t.hardware, true)
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

// job is what one conversion works from.
type job struct {
	ffmpeg, ffprobe string // the programs; ffprobe may be ""
	file            string
	audio           int
	// encoders gives the video encoders to try, best first. It runs in the
	// conversion's own goroutine because looking them up asks ffmpeg.
	encoders func(ctx context.Context) []encoder
}

// run probes, converts and reports. It is the only writer of err and done.
func (s *stream) run(ctx context.Context, j job) {
	defer close(s.done)

	var streams []streamInfo
	if j.ffprobe != "" {
		var err error
		if streams, err = probe(ctx, j.ffprobe, j.file); err != nil && ctx.Err() == nil {
			// Converting everything still works; it is only slower.
			s.log.Debug("probe failed; converting everything", slog.String("error", err.Error()))
		}
	}
	p, err := makePlan(streams, j.audio)
	if err != nil {
		s.setErr(err)
		return
	}
	s.log.Debug("converting", slog.Bool("copy_video", p.copyVideo), slog.Bool("copy_audio", p.copyAudio), slog.Int("audio_track", p.audio))

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

	// A copied picture needs no encoder. Otherwise the chip comes first and the
	// processor is the way out when the chip does not work: whether it does is
	// only found out by trying.
	chain := []encoder{encoderX264}
	if !p.copyVideo {
		chain = j.encoders(ctx)
	}
	var runErr error
	var stderr bytes.Buffer
	for i, enc := range chain {
		p.encoder = enc
		stderr.Reset()
		cmd := exec.CommandContext(ctx, j.ffmpeg, hlsArgs(j.file, s.dir, p)...)
		hideWindow(cmd)
		cmd.Stderr = &limitedWriter{w: &stderr, left: 4 << 10}
		runErr = cmd.Run()
		if runErr == nil || s.wasStopped() || ctx.Err() != nil || s.isReady() || i == len(chain)-1 {
			break
		}
		// Failed before anything could be played: start again with the next one.
		s.log.Info("video encoder failed; trying another",
			slog.String("encoder", string(enc)), slog.String("reason", lastLine(stderr.String())))
		s.clearOutput()
	}

	if runErr != nil && !s.wasStopped() {
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

func (s *stream) wasStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

func (s *stream) isReady() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

// clearOutput deletes what a failed attempt left, so the next one starts clean.
func (s *stream) clearOutput() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(s.dir, e.Name()))
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
