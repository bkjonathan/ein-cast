package transcode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

type fakeItems struct {
	mu    sync.Mutex
	items map[media.ID]media.Item
}

func (f *fakeItems) Get(id media.ID) (media.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if it, ok := f.items[id]; ok {
		return it, nil
	}
	return media.Item{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
}
func (f *fakeItems) set(it media.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[it.ID] = it
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// fakeStream is a conversion the test drives by hand.
type fakeStream struct {
	path    string
	ready   chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	err     error
	stopped int
	files   map[string]string
}

func newFakeStream(path string) *fakeStream {
	return &fakeStream{path: path, ready: make(chan struct{}), done: make(chan struct{}), files: map[string]string{
		"index.m3u8": "#EXTM3U\n", "seg00000.ts": "ts-bytes",
	}}
}

func (f *fakeStream) Ready() <-chan struct{} { return f.ready }
func (f *fakeStream) Done() <-chan struct{}  { return f.done }
func (f *fakeStream) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}
func (f *fakeStream) Open(name string) (io.ReadSeekCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped > 0 {
		return nil, fs.ErrNotExist
	}
	body, ok := f.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return nopCloser{bytes.NewReader([]byte(body))}, nil
}
func (f *fakeStream) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
}
func (f *fakeStream) stops() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stopped
}
func (f *fakeStream) becomeReady() { close(f.ready) }
func (f *fakeStream) fail(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
	close(f.done)
}

type fakeTranscoder struct {
	mu        sync.Mutex
	available bool
	startErr  error
	streams   []*fakeStream
	reqs      []app.TranscodeRequest
	// autoReady makes every started stream ready immediately.
	autoReady bool
}

func (f *fakeTranscoder) Available() bool { return f.available }
func (f *fakeTranscoder) Start(_ context.Context, req app.TranscodeRequest) (app.HLSStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.reqs = append(f.reqs, req)
	s := newFakeStream(req.Path)
	if f.autoReady {
		s.becomeReady()
	}
	f.streams = append(f.streams, s)
	return s, nil
}
func (f *fakeTranscoder) started() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.streams)
}
func (f *fakeTranscoder) stream(i int) *fakeStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[i]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type rig struct {
	svc   *Service
	items *fakeItems
	tc    *fakeTranscoder
	clk   *clock
}

var epoch = time.Unix(1_700_000_000, 0)

func newRig(opts Options) *rig {
	r := &rig{
		items: &fakeItems{items: map[media.ID]media.Item{}},
		tc:    &fakeTranscoder{available: true, autoReady: true},
		clk:   &clock{t: epoch},
	}
	for _, id := range []media.ID{"a", "b", "c"} {
		r.items.items[id] = media.Item{ID: id, Name: string(id) + ".mkv", Root: "/lib", Path: "/lib/" + string(id) + ".mkv", Kind: media.KindVideo, ModTime: epoch}
	}
	r.svc = NewService(r.items, r.tc, slog.New(slog.DiscardHandler), opts)
	r.svc.now = r.clk.now
	return r
}

var testOpts = Options{MaxStreams: 2, IdleAfter: 2 * time.Minute, ReplaceAfter: 30 * time.Second, ReadyTimeout: time.Second, SweepEvery: time.Hour}

func read(t *testing.T, f io.ReadSeekCloser, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	return string(b)
}

func TestPlaylist_StartsOneConversionAndSharesIt(t *testing.T) {
	r := newRig(testOpts)

	f, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack)
	if got := read(t, f, err); got != "#EXTM3U\n" {
		t.Errorf("playlist = %q", got)
	}
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	if n := r.tc.started(); n != 1 {
		t.Errorf("started %d conversions, want 1", n)
	}
	if got := r.tc.stream(0).path; got != "/lib/a.mkv" {
		t.Errorf("converted %q", got)
	}
}

func TestPlaylist_WaitsUntilPlaybackCanStart(t *testing.T) {
	r := newRig(testOpts)
	r.tc.autoReady = false

	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("returned before ready: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	r.tc.stream(0).becomeReady()
	if err := <-done; err != nil {
		t.Fatalf("Playlist() error = %v", err)
	}
}

func TestPlaylist_ErrorCases(t *testing.T) {
	t.Run("unknown video", func(t *testing.T) {
		r := newRig(testOpts)
		if _, err := r.svc.Playlist(context.Background(), "nope", app.DefaultAudioTrack); !errors.Is(err, media.ErrNotFound) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("no ffmpeg", func(t *testing.T) {
		r := newRig(testOpts)
		r.tc.available = false
		if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); !errors.Is(err, app.ErrTranscodingUnavailable) {
			t.Fatalf("error = %v", err)
		}
		if r.svc.Available() {
			t.Error("Available() = true")
		}
	})
	t.Run("start fails", func(t *testing.T) {
		r := newRig(testOpts)
		r.tc.startErr = errors.New("cannot make a folder")
		if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err == nil || r.svc.Running() != 0 {
			t.Fatalf("error = %v running = %d, want an error and no session", err, r.svc.Running())
		}
	})
	t.Run("caller gives up", func(t *testing.T) {
		r := newRig(testOpts)
		r.tc.autoReady = false
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := r.svc.Playlist(ctx, "a", app.DefaultAudioTrack); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
		if r.svc.Running() != 1 {
			t.Error("the conversion must keep going for the next request")
		}
	})
	t.Run("too slow to start", func(t *testing.T) {
		opts := testOpts
		opts.ReadyTimeout = 20 * time.Millisecond
		r := newRig(opts)
		r.tc.autoReady = false
		if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); !errors.Is(err, app.ErrNotConvertible) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPlaylist_AFailedConversionIsReportedAndNotRetriedAtOnce(t *testing.T) {
	r := newRig(testOpts)
	r.tc.autoReady = false
	go func() {
		for r.tc.started() == 0 {
			time.Sleep(time.Millisecond)
		}
		r.tc.stream(0).fail(errors.New("Invalid data found when processing input"))
	}()

	_, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack)
	if !errors.Is(err, app.ErrNotConvertible) || !strings.Contains(err.Error(), "Invalid data") {
		t.Fatalf("error = %v, want ErrNotConvertible with ffmpeg's reason", err)
	}
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); !errors.Is(err, app.ErrNotConvertible) {
		t.Fatalf("second error = %v", err)
	}
	if n := r.tc.started(); n != 1 {
		t.Errorf("started %d conversions, want 1: a file ffmpeg cannot read is not retried per request", n)
	}

	// Cleaned up after sitting idle, it may be tried again.
	r.clk.advance(3 * time.Minute)
	r.svc.sweep()
	r.tc.autoReady = true
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatalf("after cleanup: %v", err)
	}
}

func TestPlaylist_AChangedFileIsConvertedAgain(t *testing.T) {
	r := newRig(testOpts)
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	it, _ := r.items.Get("a")
	it.ModTime = epoch.Add(time.Hour)
	r.items.set(it)

	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	if r.tc.started() != 2 || r.tc.stream(0).stops() != 1 || r.svc.Running() != 1 {
		t.Errorf("started=%d stops=%d running=%d, want the old one stopped and replaced",
			r.tc.started(), r.tc.stream(0).stops(), r.svc.Running())
	}
}

func TestFile_ServesSegmentsOfARunningConversionOnly(t *testing.T) {
	r := newRig(testOpts)
	if _, err := r.svc.File("a", app.DefaultAudioTrack, "seg00000.ts"); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("before any conversion: error = %v, want ErrNotStarted", err)
	}
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	f, err := r.svc.File("a", app.DefaultAudioTrack, "seg00000.ts")
	if got := read(t, f, err); got != "ts-bytes" {
		t.Errorf("segment = %q", got)
	}
	if _, err := r.svc.File("a", app.DefaultAudioTrack, "seg00099.ts"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing segment: error = %v, want fs.ErrNotExist", err)
	}
	if r.tc.started() != 1 {
		t.Error("asking for a segment must never start a conversion")
	}
}

func TestSlots_AWatchedVideoIsNotCutOffForANewOne(t *testing.T) {
	r := newRig(testOpts) // two slots
	for _, id := range []media.ID{"a", "b"} {
		if _, err := r.svc.Playlist(context.Background(), id, app.DefaultAudioTrack); err != nil {
			t.Fatal(err)
		}
	}
	r.clk.advance(10 * time.Second) // both recently used

	if _, err := r.svc.Playlist(context.Background(), "c", app.DefaultAudioTrack); !errors.Is(err, ErrBusy) {
		t.Fatalf("error = %v, want ErrBusy", err)
	}
	if r.tc.stream(0).stops()+r.tc.stream(1).stops() != 0 {
		t.Error("a conversion in use was stopped")
	}
}

func TestSlots_TheLongestIdleVideoIsReplaced(t *testing.T) {
	r := newRig(testOpts)
	for _, id := range []media.ID{"a", "b"} {
		if _, err := r.svc.Playlist(context.Background(), id, app.DefaultAudioTrack); err != nil {
			t.Fatal(err)
		}
		r.clk.advance(20 * time.Second)
	}
	// a idle 40 s, b idle 20 s: a goes.
	if _, err := r.svc.Playlist(context.Background(), "c", app.DefaultAudioTrack); err != nil {
		t.Fatalf("error = %v", err)
	}
	if r.tc.stream(0).stops() != 1 || r.tc.stream(1).stops() != 0 {
		t.Errorf("stops: a=%d b=%d, want only a stopped", r.tc.stream(0).stops(), r.tc.stream(1).stops())
	}
	if r.svc.Running() != 2 {
		t.Errorf("running = %d", r.svc.Running())
	}
}

func TestSweep_StopsIdleConversionsAndKeepsBusyOnes(t *testing.T) {
	r := newRig(testOpts)
	for _, id := range []media.ID{"a", "b"} {
		if _, err := r.svc.Playlist(context.Background(), id, app.DefaultAudioTrack); err != nil {
			t.Fatal(err)
		}
	}
	r.clk.advance(90 * time.Second)
	if _, err := r.svc.File("b", app.DefaultAudioTrack, "seg00000.ts"); err != nil { // b is being watched
		t.Fatal(err)
	}
	r.clk.advance(60 * time.Second) // a idle 150 s, b idle 60 s

	r.svc.sweep()
	if r.tc.stream(0).stops() != 1 || r.tc.stream(1).stops() != 0 || r.svc.Running() != 1 {
		t.Errorf("a stops=%d b stops=%d running=%d", r.tc.stream(0).stops(), r.tc.stream(1).stops(), r.svc.Running())
	}
	if _, err := r.svc.File("a", app.DefaultAudioTrack, "seg00000.ts"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("a's segment after cleanup: error = %v, want ErrNotStarted", err)
	}
}

func TestRun_SweepsOnATimerAndStopsEverythingWhenItEnds(t *testing.T) {
	opts := testOpts
	opts.SweepEvery = 10 * time.Millisecond
	r := newRig(opts)
	if _, err := r.svc.Playlist(context.Background(), "a", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Playlist(context.Background(), "b", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		r.svc.Run(ctx)
	}()

	r.clk.advance(3 * time.Minute) // both idle; the next tick removes them
	deadline := time.Now().Add(2 * time.Second)
	for r.svc.Running() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("idle conversions were not swept")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, err := r.svc.Playlist(context.Background(), "c", app.DefaultAudioTrack); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
	if r.svc.Running() != 0 || r.tc.stream(2).stops() != 1 {
		t.Errorf("running = %d stops = %d, want everything stopped on exit", r.svc.Running(), r.tc.stream(2).stops())
	}
}

func TestAudioTracks_EachIsItsOwnConversionAndIsPassedOn(t *testing.T) {
	r := newRig(testOpts)
	ctx := context.Background()
	for _, track := range []int{app.DefaultAudioTrack, 1, 1, app.DefaultAudioTrack} {
		if _, err := r.svc.Playlist(ctx, "a", track); err != nil {
			t.Fatal(err)
		}
	}
	if r.tc.started() != 2 {
		t.Fatalf("started = %d, want one per track (default and 1)", r.tc.started())
	}
	if got := []int{r.tc.reqs[0].AudioTrack, r.tc.reqs[1].AudioTrack}; got[0] != app.DefaultAudioTrack || got[1] != 1 {
		t.Errorf("tracks asked for = %v", got)
	}

	// Segments are found under the track they belong to.
	if _, err := r.svc.File("a", 1, "seg00000.ts"); err != nil {
		t.Errorf("File(track 1) error = %v", err)
	}
	if _, err := r.svc.File("a", 2, "seg00000.ts"); !errors.Is(err, ErrNotStarted) {
		t.Errorf("File(track 2) error = %v, want ErrNotStarted", err)
	}
}

func TestSlots_SwitchingLanguageReplacesTheTrackJustLeft(t *testing.T) {
	r := newRig(testOpts) // two slots
	ctx := context.Background()
	for _, track := range []int{app.DefaultAudioTrack, 1} {
		if _, err := r.svc.Playlist(ctx, "a", track); err != nil {
			t.Fatal(err)
		}
	}
	r.clk.advance(2 * time.Second) // both in use a moment ago: nothing else would give way

	if _, err := r.svc.Playlist(ctx, "a", 2); err != nil {
		t.Fatalf("a third language: error = %v", err)
	}
	if got := r.tc.stream(0).stops() + r.tc.stream(1).stops(); got != 1 {
		t.Errorf("stopped %d of the video's other tracks, want 1", got)
	}
	if r.svc.Running() != 2 {
		t.Errorf("running = %d", r.svc.Running())
	}
}

func TestSlots_AnotherVideoStillCannotCutOffAWatchedOne(t *testing.T) {
	r := newRig(testOpts)
	ctx := context.Background()
	_, _ = r.svc.Playlist(ctx, "a", 1)
	_, _ = r.svc.Playlist(ctx, "a", 2)
	r.clk.advance(2 * time.Second)
	if _, err := r.svc.Playlist(ctx, "b", app.DefaultAudioTrack); !errors.Is(err, ErrBusy) {
		t.Fatalf("error = %v, want ErrBusy", err)
	}
}

func TestPlaylist_ADiskThatIsTooFullIsReportedAndAskedAgain(t *testing.T) {
	r := newRig(testOpts)
	ctx := context.Background()
	r.tc.startErr = app.ErrNotEnoughDiskSpace
	for range 2 {
		if _, err := r.svc.Playlist(ctx, "a", app.DefaultAudioTrack); !errors.Is(err, app.ErrNotEnoughDiskSpace) {
			t.Fatalf("error = %v, want ErrNotEnoughDiskSpace", err)
		}
	}
	if r.svc.Running() != 0 {
		t.Errorf("running = %d: a refused conversion took a slot", r.svc.Running())
	}

	// Space was freed: the same request now works, nothing was remembered.
	r.tc.startErr = nil
	if _, err := r.svc.Playlist(ctx, "a", app.DefaultAudioTrack); err != nil {
		t.Fatalf("after freeing space: error = %v", err)
	}
}
