package updates

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
)

// fakeSource answers Latest from a script and counts how often it was asked: the
// count is how the tests prove that nothing went to the network.
type fakeSource struct {
	mu      sync.Mutex
	release app.Release
	err     error
	calls   int
	block   chan struct{} // when set, Latest waits for it to be closed
}

func (f *fakeSource) Latest(ctx context.Context) (app.Release, error) {
	f.mu.Lock()
	f.calls++
	rel, err, block := f.release, f.err, f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return app.Release{}, ctx.Err()
		}
	}
	return rel, err
}

func (f *fakeSource) set(rel app.Release, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release, f.err = rel, err
}

func (f *fakeSource) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeSettings struct {
	mu sync.Mutex
	on bool
}

func (f *fakeSettings) Get() app.SettingsView {
	f.mu.Lock()
	defer f.mu.Unlock()
	return app.SettingsView{Settings: app.Settings{CheckForUpdates: f.on}}
}

func (f *fakeSettings) set(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.on = on
}

type recorder struct {
	mu  sync.Mutex
	got []app.UpdateStatus
}

func (r *recorder) Publish(ev app.Event) {
	if u, ok := ev.(app.UpdateChanged); ok {
		r.mu.Lock()
		r.got = append(r.got, u.Status)
		r.mu.Unlock()
	}
}

func (r *recorder) all() []app.UpdateStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]app.UpdateStatus(nil), r.got...)
}

func newest(tag string) app.Release {
	return app.Release{
		Tag: tag, Name: "EinCast " + tag, Notes: "- fixes", URL: "https://github.com/x/y/releases/tag/" + tag,
		PublishedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Assets: []app.ReleaseAsset{
			{Name: "eincast-1.1.2-macos-universal.zip", Size: 10, SHA256: "aa"},
			{Name: "eincast-1.1.2-windows-amd64-installer.exe", Size: 20, SHA256: "bb"},
			{Name: "eincast-1.1.2-windows-amd64.exe", Size: 30, SHA256: "cc"},
		},
	}
}

func newTestService(current string, src *fakeSource, set *fakeSettings, opts ...Option) (*Service, *recorder) {
	rec := &recorder{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := NewService(src, set, rec, log, current, Timing{FirstAfter: time.Millisecond, PollEvery: 5 * time.Millisecond, MaxAge: time.Hour}, opts...)
	return s, rec
}

func TestCheck_NewerReleaseIsAvailable(t *testing.T) {
	tests := []struct {
		name, current, tag string
		want               bool
	}{
		{"a newer patch", "1.1.1", "v1.1.2", true},
		{"the same version", "1.1.2", "v1.1.2", false},
		{"running a newer build than the latest release", "1.2.0", "v1.1.2", false},
		{"two digits sort as numbers", "1.9.0", "v1.10.0", true},
		{"a release candidate gets the final release", "1.1.1-rc1", "v1.1.1", true},
		{"the final release is not replaced by its own candidate", "1.1.1", "v1.1.1-rc1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeSource{release: newest(tt.tag)}
			s, _ := newTestService(tt.current, src, &fakeSettings{on: true})

			st := s.Check(context.Background())

			if st.Available != tt.want || st.Error != "" || st.Latest == nil || st.CheckedAt.IsZero() {
				t.Fatalf("status = %+v, want Available=%v and no error", st, tt.want)
			}
			if st.Latest.Version != tt.tag[1:] {
				t.Errorf("Latest.Version = %q, want %q (no leading v)", st.Latest.Version, tt.tag[1:])
			}
		})
	}
}

func TestCheck_ListsOnlyTheFilesForThisComputer(t *testing.T) {
	for goos, want := range map[string][]string{
		"darwin":  {"eincast-1.1.2-macos-universal.zip"},
		"windows": {"eincast-1.1.2-windows-amd64-installer.exe", "eincast-1.1.2-windows-amd64.exe"},
		"linux":   {"eincast-1.1.2-macos-universal.zip", "eincast-1.1.2-windows-amd64-installer.exe", "eincast-1.1.2-windows-amd64.exe"},
	} {
		t.Run(goos, func(t *testing.T) {
			s, _ := newTestService("1.1.1", &fakeSource{release: newest("v1.1.2")}, &fakeSettings{on: true}, WithPlatform(goos))
			st := s.Check(context.Background())
			var got []string
			for _, a := range st.Latest.Assets {
				got = append(got, a.Name)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("assets = %v, want %v", got, want)
			}
		})
	}
}

func TestCheck_CarriesTheChecksums(t *testing.T) {
	s, _ := newTestService("1.1.1", &fakeSource{release: newest("v1.1.2")}, &fakeSettings{on: true}, WithPlatform("darwin"))
	st := s.Check(context.Background())
	if a := st.Latest.Assets[0]; a.SHA256 != "aa" || a.Size != 10 {
		t.Fatalf("asset = %+v, want the checksum and size kept", a)
	}
}

func TestPageURL(t *testing.T) {
	tests := []struct {
		name, current, url, want string
	}{
		{"newer release with a normal address", "1.1.1", "https://github.com/x/y/releases/tag/v1.1.2", "https://github.com/x/y/releases/tag/v1.1.2"},
		{"nothing newer", "1.1.2", "https://github.com/x/y/releases/tag/v1.1.2", ""},
		{"an address that is not https is never offered", "1.1.1", "http://example.com/x", ""},
		{"a file address is never offered", "1.1.1", "file:///etc/passwd", ""},
		{"an address with no host is never offered", "1.1.1", "https:///x", ""},
		{"nonsense is never offered", "1.1.1", "::not a url", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel := newest("v1.1.2")
			rel.URL = tt.url
			s, _ := newTestService(tt.current, &fakeSource{release: rel}, &fakeSettings{on: true})
			if got := s.PageURL(); got != "" {
				t.Fatalf("PageURL() before any check = %q, want empty", got)
			}
			s.Check(context.Background())
			if got := s.PageURL(); got != tt.want {
				t.Fatalf("PageURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheck_AFailureIsAStateAndKeepsTheNotice(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, rec := newTestService("1.1.1", src, &fakeSettings{on: true})
	s.Check(context.Background())

	src.set(app.Release{}, errors.New("connection refused"))
	st := s.Check(context.Background())

	if st.Error == "" {
		t.Fatal("a failed check must say so")
	}
	if !st.Available || st.Latest == nil {
		t.Errorf("status = %+v: the notice from the last good check must survive a failed one", st)
	}
	if st.Checking {
		t.Error("Checking stayed true after a failure")
	}
	if last := rec.all()[len(rec.all())-1]; last.Error != st.Error {
		t.Errorf("the failure was not published: %+v", last)
	}

	src.set(newest("v1.1.2"), nil)
	if st = s.Check(context.Background()); st.Error != "" {
		t.Errorf("a good check must clear the error, got %q", st.Error)
	}
}

func TestCheck_FailureMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{app.ErrNoRelease, "No release has been published yet."},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "GitHub did not answer in time. Check your internet connection and try again."},
		{errors.New("HTTP 503"), "Could not check for updates: HTTP 503"},
	}
	for _, tt := range tests {
		s, _ := newTestService("1.1.1", &fakeSource{err: tt.err}, &fakeSettings{on: true})
		if got := s.Check(context.Background()).Error; got != tt.want {
			t.Errorf("Check() error = %q, want %q", got, tt.want)
		}
	}
}

func TestCheck_ATagThatIsNotAVersionIsAnError(t *testing.T) {
	s, _ := newTestService("1.1.1", &fakeSource{release: newest("nightly")}, &fakeSettings{on: true})
	st := s.Check(context.Background())
	if st.Error == "" || st.Available || st.Latest != nil {
		t.Fatalf("status = %+v, want an error and no notice", st)
	}
}

func TestCheck_ADevelopmentBuildAsksNothing(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, _ := newTestService("dev", src, &fakeSettings{on: true})

	st := s.Check(context.Background())

	if src.count() != 0 {
		t.Fatalf("a development build made %d request(s); it has nothing to compare, so it must make none", src.count())
	}
	if st.Error != devBuildMessage || st.Available {
		t.Errorf("status = %+v", st)
	}
}

func TestCheck_PublishesStartAndEnd(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, rec := newTestService("1.1.1", src, &fakeSettings{on: true})
	s.Check(context.Background())

	got := rec.all()
	if len(got) != 2 || !got[0].Checking || got[1].Checking || !got[1].Available {
		t.Fatalf("events = %+v, want checking then done", got)
	}
}

func TestCheck_WhileCheckingDoesNotAskAgain(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2"), block: make(chan struct{})}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: true})

	done := make(chan struct{})
	go func() { s.Check(context.Background()); close(done) }()
	waitFor(t, func() bool { return s.Status().Checking })

	if st := s.Check(context.Background()); !st.Checking {
		t.Errorf("second check = %+v, want the running one's status", st)
	}
	close(src.block)
	<-done
	if src.count() != 1 {
		t.Fatalf("source asked %d times, want 1", src.count())
	}
}

func TestCheckIfDue_SettingOffMakesNoRequest(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	set := &fakeSettings{on: false}
	s, _ := newTestService("1.1.1", src, set)

	s.checkIfDue(context.Background())
	if src.count() != 0 {
		t.Fatalf("with the setting off, %d request(s) were made; want 0", src.count())
	}

	set.set(true)
	s.checkIfDue(context.Background())
	if src.count() != 1 {
		t.Fatalf("with the setting on, %d request(s) were made; want 1", src.count())
	}
}

func TestCheck_AButtonPressWorksWithTheSettingOff(t *testing.T) {
	// The setting turns off what EinCast does by itself. Pressing "Check now" is the
	// person asking, so it is answered (ADR 0029).
	src := &fakeSource{release: newest("v1.1.2")}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: false})

	if st := s.Check(context.Background()); !st.Available || src.count() != 1 {
		t.Fatalf("status = %+v, requests = %d", st, src.count())
	}
}

func TestCheckIfDue_ChecksOnceAndThenWaitsForTheDataToAge(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: true})
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	s.checkIfDue(context.Background())
	now = now.Add(30 * time.Minute)
	s.checkIfDue(context.Background())
	if src.count() != 1 {
		t.Fatalf("requests = %d after 30 minutes, want 1: the answer is still fresh", src.count())
	}

	now = now.Add(time.Hour)
	s.checkIfDue(context.Background())
	if src.count() != 2 {
		t.Fatalf("requests = %d after 90 minutes, want 2: the answer is older than MaxAge", src.count())
	}
}

func TestCheckIfDue_AFailedCheckIsRetriedAtTheNextWake(t *testing.T) {
	src := &fakeSource{err: errors.New("offline")}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: true})

	s.checkIfDue(context.Background())
	s.checkIfDue(context.Background())
	if src.count() != 2 {
		t.Fatalf("requests = %d, want 2: nothing was learned, so ask again", src.count())
	}
}

func TestRun_ChecksSoonAndStopsWithItsContext(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: true})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	waitFor(t, func() bool { return s.Status().Available })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context ended: a leaked goroutine")
	}
}

func TestRun_WithTheSettingOffNeverAsks(t *testing.T) {
	src := &fakeSource{release: newest("v1.1.2")}
	s, _ := newTestService("1.1.1", src, &fakeSettings{on: false})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond) // many wakes at these timings
	cancel()
	<-done

	if src.count() != 0 {
		t.Fatalf("requests = %d with the setting off, want 0", src.count())
	}
}

func TestRun_ADevelopmentBuildReturnsAtOnce(t *testing.T) {
	src := &fakeSource{}
	s, _ := newTestService("dev", src, &fakeSettings{on: true})

	done := make(chan struct{})
	go func() { s.Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run on a development build should return, not wait")
	}
	if src.count() != 0 {
		t.Fatalf("requests = %d, want 0", src.count())
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(time.Millisecond)
	}
}
