package thumbnails

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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

type fakeCache struct {
	mu     sync.Mutex
	data   map[string][]byte
	putErr error
}

func (f *fakeCache) Get(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.data[key]
	return d, ok
}

func (f *fakeCache) Put(key string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return f.putErr
	}
	f.data[key] = data
	return nil
}

func (f *fakeCache) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.data)
}

// fakeGrabber counts calls and can be held open to test overlap.
type fakeGrabber struct {
	available atomic.Bool
	calls     atomic.Int32
	running   atomic.Int32
	peak      atomic.Int32
	err       error
	gate      chan struct{} // when set, Grab waits for it to be closed
	gotReq    atomic.Value  // app.GrabRequest
	gotCtx    atomic.Value  // context.Context
}

func (f *fakeGrabber) Available() bool { return f.available.Load() }

func (f *fakeGrabber) Grab(ctx context.Context, req app.GrabRequest) ([]byte, error) {
	f.calls.Add(1)
	f.gotReq.Store(req)
	f.gotCtx.Store(ctx)
	n := f.running.Add(1)
	defer f.running.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return []byte("jpeg:" + req.Path), nil
}

type rig struct {
	svc   *Service
	items *fakeItems
	cache *fakeCache
	grab  *fakeGrabber
}

var modTime = time.Unix(1_700_000_000, 0)

func newRig(opts Options) *rig {
	r := &rig{
		items: &fakeItems{items: map[media.ID]media.Item{
			"v1": {ID: "v1", Name: "a.mkv", Root: "/lib", Path: "/lib/a.mkv", Kind: media.KindVideo, Size: 10, ModTime: modTime},
			"v2": {ID: "v2", Name: "b.mkv", Root: "/lib", Path: "/lib/b.mkv", Kind: media.KindVideo, Size: 20, ModTime: modTime},
			"v3": {ID: "v3", Name: "c.mkv", Root: "/lib", Path: "/lib/c.mkv", Kind: media.KindVideo, Size: 30, ModTime: modTime},
		}},
		cache: &fakeCache{data: map[string][]byte{}},
		grab:  &fakeGrabber{},
	}
	r.grab.available.Store(true)
	r.svc = NewService(r.items, r.grab, r.cache, slog.New(slog.DiscardHandler), opts)
	return r
}

func TestGet_MakesAThumbnailOnceAndRemembersIt(t *testing.T) {
	r := newRig(DefaultOptions)

	got, err := r.svc.Get(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got.JPEG) != "jpeg:/lib/a.mkv" || !got.ModTime.Equal(modTime) {
		t.Errorf("Get() = %q @ %v", got.JPEG, got.ModTime)
	}
	req := r.grab.gotReq.Load().(app.GrabRequest)
	if req.Root != "/lib" || req.Path != "/lib/a.mkv" || req.Kind != media.KindVideo || req.Width != Width {
		t.Errorf("grab request = %+v", req)
	}
	if r.cache.len() != 1 {
		t.Errorf("cache holds %d, want 1", r.cache.len())
	}

	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatalf("second Get() error = %v", err)
	}
	if n := r.grab.calls.Load(); n != 1 {
		t.Errorf("grabbed %d times, want 1 (the second came from the cache)", n)
	}
}

func TestGet_ACachedThumbnailNeedsNoFFmpeg(t *testing.T) {
	r := newRig(DefaultOptions)
	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatal(err)
	}
	r.grab.available.Store(false)

	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatalf("Get() error = %v, want the remembered thumbnail", err)
	}
}

func TestGet_UnknownIDIsNotFound(t *testing.T) {
	r := newRig(DefaultOptions)
	_, err := r.svc.Get(context.Background(), "nope")
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("error = %v, want media.ErrNotFound", err)
	}
}

func TestGet_WithoutFFmpegSaysSoAndRecoversWhenItAppears(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.available.Store(false)

	_, err := r.svc.Get(context.Background(), "v1")
	if !errors.Is(err, app.ErrThumbnailsUnavailable) {
		t.Fatalf("error = %v, want ErrThumbnailsUnavailable", err)
	}
	if r.svc.Available() {
		t.Error("Available() = true without ffmpeg")
	}

	r.grab.available.Store(true) // installed while the app runs
	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatalf("after installing ffmpeg: %v", err)
	}
}

func TestGet_AFailureIsNotRetried(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.err = fmt.Errorf("decode: %w", app.ErrNoThumbnail)

	for range 3 {
		_, err := r.svc.Get(context.Background(), "v1")
		if !errors.Is(err, app.ErrNoThumbnail) {
			t.Fatalf("error = %v, want ErrNoThumbnail", err)
		}
	}
	if n := r.grab.calls.Load(); n != 1 {
		t.Errorf("grabbed %d times, want 1: a file ffmpeg cannot read is not worth another process each time", n)
	}
}

func TestGet_AnyGrabFailureBecomesNoThumbnail(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.err = errors.New("start ffmpeg: exec format error")

	_, err := r.svc.Get(context.Background(), "v1")
	if !errors.Is(err, app.ErrNoThumbnail) {
		t.Fatalf("error = %v, want ErrNoThumbnail so callers need only one case", err)
	}
}

func TestGet_AChangedFileGetsANewThumbnail(t *testing.T) {
	r := newRig(DefaultOptions)
	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatal(err)
	}

	it, _ := r.items.Get("v1")
	it.Size++
	it.ModTime = modTime.Add(time.Hour)
	r.items.set(it)

	if _, err := r.svc.Get(context.Background(), "v1"); err != nil {
		t.Fatal(err)
	}
	if n := r.grab.calls.Load(); n != 2 {
		t.Errorf("grabbed %d times, want 2 (the file changed)", n)
	}
}

func TestGet_SimultaneousRequestsShareOneAttempt(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.gate = make(chan struct{})

	const callers = 5
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.svc.Get(context.Background(), "v1")
			errs <- err
		}()
	}
	waitFor(t, "the attempt to start", func() bool { return r.grab.calls.Load() == 1 })
	time.Sleep(50 * time.Millisecond) // time for a wrongly separate second attempt
	close(r.grab.gate)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Get() error = %v", err)
		}
	}
	if n := r.grab.calls.Load(); n != 1 {
		t.Errorf("grabbed %d times for %d simultaneous requests, want 1", n, callers)
	}
}

func TestGet_ACallerWhoGivesUpDoesNotCancelTheAttempt(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.gate = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Get(ctx, "v1")
		done <- err
	}()
	waitFor(t, "the attempt to start", func() bool { return r.grab.calls.Load() == 1 })

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	close(r.grab.gate) // the attempt finishes anyway ...
	waitFor(t, "the thumbnail to be remembered", func() bool { return r.cache.len() == 1 })
	if _, err := r.svc.Get(context.Background(), "v1"); err != nil { // ... and the next caller is served
		t.Fatalf("Get() error = %v", err)
	}
	if n := r.grab.calls.Load(); n != 1 {
		t.Errorf("grabbed %d times, want 1", n)
	}
}

func TestGet_LimitsHowManyAreMadeAtOnce(t *testing.T) {
	r := newRig(Options{Parallel: 2, Timeout: 5 * time.Second})
	r.grab.gate = make(chan struct{})

	var wg sync.WaitGroup
	for _, id := range []media.ID{"v1", "v2", "v3"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.svc.Get(context.Background(), id)
		}()
	}
	waitFor(t, "two attempts to run", func() bool { return r.grab.running.Load() == 2 })
	time.Sleep(50 * time.Millisecond)
	if got := r.grab.running.Load(); got != 2 {
		t.Fatalf("%d running at once, want the limit of 2", got)
	}
	close(r.grab.gate)
	wg.Wait()
	if p := r.grab.peak.Load(); p > 2 {
		t.Errorf("peak concurrency = %d, want at most 2", p)
	}
	if n := r.grab.calls.Load(); n != 3 {
		t.Errorf("grabbed %d times, want all 3 eventually", n)
	}
}

func TestGet_EachAttemptHasATimeout(t *testing.T) {
	r := newRig(Options{Parallel: 1, Timeout: 50 * time.Millisecond})
	r.grab.gate = make(chan struct{}) // never opened: ffmpeg hangs
	defer close(r.grab.gate)

	_, err := r.svc.Get(context.Background(), "v1")
	if !errors.Is(err, app.ErrNoThumbnail) {
		t.Fatalf("error = %v, want ErrNoThumbnail after the timeout", err)
	}
	ctx := r.grab.gotCtx.Load().(context.Context)
	if _, ok := ctx.Deadline(); !ok {
		t.Error("the grabber's context has no deadline")
	}
}

func TestGet_AFailureToRememberStillServesTheThumbnail(t *testing.T) {
	r := newRig(DefaultOptions)
	r.cache.putErr = errors.New("disk full")

	got, err := r.svc.Get(context.Background(), "v1")
	if err != nil || len(got.JPEG) == 0 {
		t.Fatalf("Get() = %q, %v; want the thumbnail despite the cache failing", got.JPEG, err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestGet_ThePosterIsWhatIsMadeWhenThereIsOne(t *testing.T) {
	r := newRig(DefaultOptions)
	art := media.Artwork{Path: "/lib/poster.jpg", Size: 7, ModTime: modTime}
	it, _ := r.items.Get("v1")
	it.Artwork = art
	r.items.set(it)

	got, err := r.svc.Get(context.Background(), "v1")
	if err != nil || string(got.JPEG) != "jpeg:/lib/poster.jpg" {
		t.Fatalf("Get() = %q, %v; want a picture made from the poster", got.JPEG, err)
	}
	req := r.grab.gotReq.Load().(app.GrabRequest)
	if req.Path != "/lib/poster.jpg" || req.Kind != media.KindImage || req.Root != "/lib" {
		t.Errorf("request = %+v, want the poster, as an image, inside the library", req)
	}
}

func TestGet_AddingAPosterMakesANewPictureInsteadOfReusingTheFrame(t *testing.T) {
	r := newRig(DefaultOptions)
	if got, _ := r.svc.Get(context.Background(), "v1"); string(got.JPEG) != "jpeg:/lib/a.mkv" {
		t.Fatalf("first picture = %q", got.JPEG)
	}

	it, _ := r.items.Get("v1")
	it.Artwork = media.Artwork{Path: "/lib/poster.jpg", Size: 7, ModTime: modTime}
	r.items.set(it)
	if got, _ := r.svc.Get(context.Background(), "v1"); string(got.JPEG) != "jpeg:/lib/poster.jpg" {
		t.Errorf("picture after adding a poster = %q, want it made from the poster", got.JPEG)
	}
}

func TestGet_AReplacedPosterMakesANewPicture(t *testing.T) {
	r := newRig(DefaultOptions)
	it, _ := r.items.Get("v1")
	it.Artwork = media.Artwork{Path: "/lib/poster.jpg", Size: 7, ModTime: modTime}
	r.items.set(it)
	_, _ = r.svc.Get(context.Background(), "v1")

	it.Artwork.Size = 9
	r.items.set(it)
	_, _ = r.svc.Get(context.Background(), "v1")
	if r.grab.calls.Load() != 2 {
		t.Errorf("grabs = %d, want 2: a new poster file needs a new thumbnail", r.grab.calls.Load())
	}
}

func TestGet_APosterStillShowsWhenFFmpegIsMissing(t *testing.T) {
	r := newRig(DefaultOptions)
	r.grab.available.Store(false)
	if _, err := r.svc.Get(context.Background(), "v1"); !errors.Is(err, app.ErrThumbnailsUnavailable) {
		t.Fatalf("no poster, no ffmpeg: error = %v, want unavailable", err)
	}

	it, _ := r.items.Get("v1")
	it.Artwork = media.Artwork{Path: "/lib/poster.jpg", Size: 7, ModTime: modTime}
	r.items.set(it)
	if got, err := r.svc.Get(context.Background(), "v1"); err != nil || len(got.JPEG) == 0 {
		t.Errorf("Get() = %q, %v; the grabber is asked even without ffmpeg, since it can serve a small poster as it is", got.JPEG, err)
	}
}
