package tracks

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

var epoch = time.Unix(1_700_000_000, 0)

type fakeItems struct {
	mu    sync.Mutex
	items map[media.ID]media.Item
}

func (f *fakeItems) Get(id media.ID) (media.Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it, ok := f.items[id]
	if !ok {
		return media.Item{}, media.ErrNotFound
	}
	return it, nil
}

func (f *fakeItems) set(it media.Item) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[it.ID] = it
}

type fakeProber struct {
	available bool
	streams   app.Streams
	err       error
	calls     atomic.Int32
}

func (f *fakeProber) Available() bool { return f.available }
func (f *fakeProber) Probe(context.Context, app.StreamProbe) (app.Streams, error) {
	f.calls.Add(1)
	return f.streams, f.err
}

type fakeExtractor struct {
	calls   atomic.Int32
	gate    chan struct{} // when set, Extract waits for it to be closed
	err     error
	reqs    []app.SubtitleExtraction
	reqsMu  sync.Mutex
	running atomic.Int32
	peak    atomic.Int32
}

func (f *fakeExtractor) Extract(_ context.Context, req app.SubtitleExtraction) ([]byte, error) {
	f.calls.Add(1)
	n := f.running.Add(1)
	defer f.running.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	f.reqsMu.Lock()
	f.reqs = append(f.reqs, req)
	f.reqsMu.Unlock()
	if f.gate != nil {
		<-f.gate
	}
	if f.err != nil {
		return nil, f.err
	}
	return []byte("WEBVTT\n\n00:01.000 --> 00:02.000\nhi\n"), nil
}

type fakeCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (c *fakeCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.data[key]
	return d, ok
}
func (c *fakeCache) Put(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = data
	return nil
}

type rig struct {
	svc    *Service
	items  *fakeItems
	prober *fakeProber
	ex     *fakeExtractor
	cache  *fakeCache
}

var twoTracks = app.Streams{
	Audio: []app.AudioStream{{Index: 0, Language: "eng"}, {Index: 1, Language: "jpn"}},
	Subtitles: []app.SubtitleStream{
		{Index: 0, Language: "eng", Codec: "subrip", Text: true},
		{Index: 1, Language: "eng", Codec: "hdmv_pgs_subtitle"},
	},
}

func newRig() *rig {
	r := &rig{
		items:  &fakeItems{items: map[media.ID]media.Item{}},
		prober: &fakeProber{available: true, streams: twoTracks},
		ex:     &fakeExtractor{},
		cache:  &fakeCache{data: map[string][]byte{}},
	}
	r.items.set(media.Item{ID: "v", Name: "film.mkv", Root: "/lib", Path: "/lib/film.mkv", Kind: media.KindVideo, Size: 100, ModTime: epoch})
	r.items.set(media.Item{ID: "m", Name: "song.mp3", Root: "/lib", Path: "/lib/song.mp3", Kind: media.KindAudio, Size: 10, ModTime: epoch})
	r.svc = NewService(r.items, r.prober, r.ex, r.cache, slog.New(slog.DiscardHandler), DefaultOptions)
	return r
}

func TestStreams_ListsTheTracksOfAVideoAndRemembersThem(t *testing.T) {
	r := newRig()
	for range 3 {
		got, err := r.svc.Streams(context.Background(), "v")
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Audio) != 2 || len(got.Subtitles) != 2 {
			t.Fatalf("Streams() = %+v", got)
		}
	}
	if n := r.prober.calls.Load(); n != 1 {
		t.Errorf("probed %d times, want once", n)
	}
}

func TestStreams_AChangedFileIsLookedAtAgain(t *testing.T) {
	r := newRig()
	_, _ = r.svc.Streams(context.Background(), "v")
	it, _ := r.items.Get("v")
	it.ModTime = it.ModTime.Add(time.Hour)
	r.items.set(it)
	_, _ = r.svc.Streams(context.Background(), "v")
	if n := r.prober.calls.Load(); n != 2 {
		t.Errorf("probed %d times, want 2", n)
	}
}

func TestStreams_EdgeCases(t *testing.T) {
	t.Run("unknown id", func(t *testing.T) {
		if _, err := newRig().svc.Streams(context.Background(), "nope"); !errors.Is(err, media.ErrNotFound) {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("a song has none and is not probed", func(t *testing.T) {
		r := newRig()
		got, err := r.svc.Streams(context.Background(), "m")
		if err != nil || len(got.Audio) != 0 || r.prober.calls.Load() != 0 {
			t.Errorf("Streams() = %+v, %v (probes: %d)", got, err, r.prober.calls.Load())
		}
	})
	t.Run("without ffprobe a video has none and that is not an error", func(t *testing.T) {
		r := newRig()
		r.prober.available = false
		got, err := r.svc.Streams(context.Background(), "v")
		if err != nil || len(got.Audio) != 0 || len(got.Subtitles) != 0 {
			t.Errorf("Streams() = %+v, %v", got, err)
		}
		if r.svc.Available() {
			t.Error("Available() = true")
		}
	})
	t.Run("a file that cannot be read is an error", func(t *testing.T) {
		r := newRig()
		r.prober.err = errors.New("ffprobe: exit status 1")
		if _, err := r.svc.Streams(context.Background(), "v"); err == nil {
			t.Error("error = nil")
		}
	})
}

func TestSubtitle_ExtractsOnceAndKeepsIt(t *testing.T) {
	r := newRig()
	for range 3 {
		got, err := r.svc.Subtitle(context.Background(), "v", 0)
		if err != nil {
			t.Fatal(err)
		}
		if string(got[:6]) != "WEBVTT" {
			t.Fatalf("Subtitle() = %q", got)
		}
	}
	if n := r.ex.calls.Load(); n != 1 {
		t.Errorf("extracted %d times, want once", n)
	}
	if req := r.ex.reqs[0]; req.Root != "/lib" || req.Path != "/lib/film.mkv" || req.Index != 0 {
		t.Errorf("asked ffmpeg for %+v", req)
	}
}

func TestSubtitle_AChangedFileIsExtractedAgain(t *testing.T) {
	r := newRig()
	_, _ = r.svc.Subtitle(context.Background(), "v", 0)
	it, _ := r.items.Get("v")
	it.Size++
	r.items.set(it)
	_, _ = r.svc.Subtitle(context.Background(), "v", 0)
	if n := r.ex.calls.Load(); n != 2 {
		t.Errorf("extracted %d times, want 2", n)
	}
}

func TestSubtitle_OnlyRealTextTracksAreExtracted(t *testing.T) {
	r := newRig()
	for name, index := range map[string]int{"a picture track": 1, "past the end": 2, "negative": -1} {
		if _, err := r.svc.Subtitle(context.Background(), "v", index); !errors.Is(err, app.ErrNoSubtitleTrack) {
			t.Errorf("%s: error = %v, want ErrNoSubtitleTrack", name, err)
		}
	}
	if _, err := r.svc.Subtitle(context.Background(), "nope", 0); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("unknown id: error = %v", err)
	}
	if n := r.ex.calls.Load(); n != 0 {
		t.Errorf("ffmpeg was run %d times for tracks that cannot be shown", n)
	}
}

func TestSubtitle_WithoutFFprobeIsUnavailable(t *testing.T) {
	r := newRig()
	r.prober.available = false
	if _, err := r.svc.Subtitle(context.Background(), "v", 0); !errors.Is(err, app.ErrSubtitlesUnavailable) {
		t.Errorf("error = %v", err)
	}
}

func TestSubtitle_ARequestForTheSameTrackSharesOneExtraction(t *testing.T) {
	r := newRig()
	r.ex.gate = make(chan struct{})

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.svc.Subtitle(context.Background(), "v", 0)
			errs <- err
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for r.ex.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let the others queue up behind it
	close(r.ex.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if n := r.ex.calls.Load(); n != 1 {
		t.Errorf("extracted %d times, want once", n)
	}
}

func TestSubtitle_AnImpatientCallerDoesNotStopTheExtraction(t *testing.T) {
	r := newRig()
	r.ex.gate = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Subtitle(ctx, "v", 0)
		done <- err
	}()
	for r.ex.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	close(r.ex.gate)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := r.cache.Get(cacheKey(media.Item{ID: "v", Size: 100, ModTime: epoch}, 0)); ok {
			return // the work was kept for the next person
		}
		if time.Now().After(deadline) {
			t.Fatal("the extraction was lost when its first caller left")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestSubtitle_FailuresAreNotRemembered(t *testing.T) {
	r := newRig()
	r.ex.err = errors.New("ffmpeg: boom")
	if _, err := r.svc.Subtitle(context.Background(), "v", 0); err == nil {
		t.Fatal("error = nil")
	}
	r.ex.err = nil
	if _, err := r.svc.Subtitle(context.Background(), "v", 0); err != nil {
		t.Fatalf("after the problem went away: error = %v", err)
	}
}

func TestSubtitle_OnlyAFewAreExtractedAtOnce(t *testing.T) {
	r := newRig()
	r.svc = NewService(r.items, &fakeProber{available: true, streams: app.Streams{
		Subtitles: []app.SubtitleStream{{Index: 0, Text: true}, {Index: 1, Text: true}, {Index: 2, Text: true}, {Index: 3, Text: true}},
	}}, r.ex, r.cache, slog.New(slog.DiscardHandler), Options{Parallel: 2})
	r.ex.gate = make(chan struct{})

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.svc.Subtitle(context.Background(), "v", i)
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(r.ex.gate)
	wg.Wait()
	if peak := r.ex.peak.Load(); peak != 2 {
		t.Errorf("peak concurrency = %d, want 2", peak)
	}
}
