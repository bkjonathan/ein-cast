package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/settings"
	"eincast/internal/app/streaming"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

type fakeLibrary struct {
	scanErr    error
	scanned    []string
	summary    app.LibrarySummary
	hasSummary bool

	rescanErr  error
	rescans    int
	restoreErr error
	restores   int
	watching   chan struct{} // closed when Watch is called
	status     app.LibraryStatus
	items      []media.Item
}

func (f *fakeLibrary) Rescan(context.Context) (app.LibrarySummary, error) {
	f.rescans++
	return f.summary, f.rescanErr
}

func (f *fakeLibrary) Restore(context.Context) error {
	f.restores++
	return f.restoreErr
}

func (f *fakeLibrary) Status() app.LibraryStatus { return f.status }

func (f *fakeLibrary) List() []media.Item { return f.items }

func (f *fakeLibrary) Watch(ctx context.Context) {
	if f.watching != nil {
		close(f.watching)
	}
	<-ctx.Done()
}

type fakeSettings struct {
	view      app.SettingsView
	updateErr error
	rotateErr error
	gotInput  settings.Input
	updates   int
	rotations int
}

func (f *fakeSettings) Get() app.SettingsView { return f.view }

func (f *fakeSettings) Update(in settings.Input) (app.SettingsView, error) {
	f.updates++
	f.gotInput = in
	return f.view, f.updateErr
}

func (f *fakeSettings) RotateToken() (app.SettingsView, error) {
	f.rotations++
	return f.view, f.rotateErr
}

func (f *fakeLibrary) ScanFolder(_ context.Context, path string) (app.LibrarySummary, error) {
	f.scanned = append(f.scanned, path)
	return f.summary, f.scanErr
}

func (f *fakeLibrary) Summary() (app.LibrarySummary, bool) { return f.summary, f.hasSummary }

type fakeStreaming struct {
	startErr, stopErr error
	startCtx          context.Context
	startCfg          streaming.Config
	stopCtx           context.Context
	starts, stops     int
	status            app.ServerStatus
	local             app.LocalServer
	localErr          error
}

func (f *fakeStreaming) Local() (app.LocalServer, error) { return f.local, f.localErr }

func (f *fakeStreaming) Start(ctx context.Context, cfg streaming.Config) error {
	f.starts++
	f.startCtx, f.startCfg = ctx, cfg
	return f.startErr
}

func (f *fakeStreaming) Stop(ctx context.Context) error {
	f.stops++
	f.stopCtx = ctx
	return f.stopErr
}

func (f *fakeStreaming) Status() app.ServerStatus { return f.status }

type fakeLogs struct {
	history []app.LogEntry
	live    chan app.LogEntry
	cancels int
}

func newFakeLogs() *fakeLogs { return &fakeLogs{live: make(chan app.LogEntry, 8)} }

func (f *fakeLogs) History() []app.LogEntry { return f.history }
func (f *fakeLogs) Subscribe(int) (<-chan app.LogEntry, func()) {
	return f.live, func() { f.cancels++ }
}

func newTestApp(lib *fakeLibrary) *App {
	a, _ := newTestAppWithStreaming(lib)
	return a
}

func newTestAppWithStreaming(lib *fakeLibrary) (*App, *fakeStreaming) {
	log := slog.New(slog.DiscardHandler)
	st := &fakeStreaming{}
	a := New(Deps{
		Info:      Info{Name: "EinCast", Version: "1.2.3"},
		Library:   lib,
		Settings:  &fakeSettings{view: app.SettingsView{Settings: settings.Default()}},
		Streaming: st,
		Remote:    &fakeRemote{},
		Logs:      newFakeLogs(),
		Emitter:   NewEmitter(log),
		Log:       log,
	})
	a.ctx = context.Background()
	return a, st
}

func TestAppInfo(t *testing.T) {
	got := newTestApp(&fakeLibrary{}).AppInfo()

	want := AppInfoDTO{Name: "EinCast", Version: "1.2.3", Platform: runtime.GOOS}
	if got != want {
		t.Fatalf("AppInfo() = %+v, want %+v", got, want)
	}
}

func TestListLibrary(t *testing.T) {
	t.Run("maps items without exposing disk paths", func(t *testing.T) {
		mod := time.UnixMilli(1_700_000_000_123)
		lib := &fakeLibrary{items: []media.Item{{
			ID: "abc", Name: "a.mkv", RelPath: "shows/a.mkv", Path: "/secret/shows/a.mkv",
			Kind: media.KindVideo, Size: 42, ModTime: mod,
		}}}
		got := newTestApp(lib).ListLibrary()
		want := []LibraryItemDTO{{
			ID: "abc", Name: "a.mkv", RelPath: "shows/a.mkv", Kind: "video", Size: 42, ModTime: mod.UnixMilli(),
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ListLibrary() = %+v, want %+v", got, want)
		}
	})

	t.Run("empty library is an empty list, not nil", func(t *testing.T) {
		if got := newTestApp(&fakeLibrary{}).ListLibrary(); got == nil || len(got) != 0 {
			t.Fatalf("ListLibrary() = %#v, want empty non-nil slice", got)
		}
	})
}

type fakeThumbs struct {
	th        thumbnails.Thumbnail
	err       error
	available bool
	gotID     media.ID
}

func (f *fakeThumbs) Get(_ context.Context, id media.ID) (thumbnails.Thumbnail, error) {
	f.gotID = id
	return f.th, f.err
}
func (f *fakeThumbs) Available() bool { return f.available }

func TestGetThumbnail(t *testing.T) {
	t.Run("returns a data URL", func(t *testing.T) {
		a := newTestApp(&fakeLibrary{})
		ft := &fakeThumbs{th: thumbnails.Thumbnail{JPEG: []byte{0xFF, 0xD8, 0x01}}}
		a.thumbs = ft
		got, err := a.GetThumbnail("abc")
		if err != nil {
			t.Fatal(err)
		}
		if want := "data:image/jpeg;base64,/9gB"; got != want {
			t.Errorf("GetThumbnail() = %q, want %q", got, want)
		}
		if ft.gotID != "abc" {
			t.Errorf("asked for %q", ft.gotID)
		}
	})

	t.Run("no picture is an empty string, not an error", func(t *testing.T) {
		for name, err := range map[string]error{
			"no picture": fmt.Errorf("x: %w", app.ErrNoThumbnail),
			"no ffmpeg":  app.ErrThumbnailsUnavailable,
			"unknown id": fmt.Errorf("x: %w", media.ErrNotFound),
		} {
			a := newTestApp(&fakeLibrary{})
			a.thumbs = &fakeThumbs{err: err}
			if got, gotErr := a.GetThumbnail("abc"); got != "" || gotErr != nil {
				t.Errorf("%s: GetThumbnail() = %q, %v; want empty and no error", name, got, gotErr)
			}
		}
	})

	t.Run("an unexpected failure is reported", func(t *testing.T) {
		a := newTestApp(&fakeLibrary{})
		a.thumbs = &fakeThumbs{err: errors.New("boom")}
		if _, err := a.GetThumbnail("abc"); err == nil {
			t.Error("GetThumbnail() error = nil, want the failure")
		}
	})

	t.Run("without a thumbnail service there are none", func(t *testing.T) {
		a := newTestApp(&fakeLibrary{})
		if got, err := a.GetThumbnail("abc"); got != "" || err != nil {
			t.Errorf("GetThumbnail() = %q, %v", got, err)
		}
		if a.ThumbnailsAvailable() {
			t.Error("ThumbnailsAvailable() = true with no service")
		}
	})
}

func TestThumbnailsAvailable(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	a.thumbs = &fakeThumbs{available: true}
	if !a.ThumbnailsAvailable() {
		t.Error("ThumbnailsAvailable() = false, want true")
	}
	a.thumbs = &fakeThumbs{available: false}
	if a.ThumbnailsAvailable() {
		t.Error("ThumbnailsAvailable() = true, want false")
	}
}

func TestSetWindowTheme(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	var applied []string
	a.windowThemes = map[string]func(context.Context){
		"system": func(context.Context) { applied = append(applied, "system") },
		"dark":   func(context.Context) { applied = append(applied, "dark") },
	}

	if err := a.SetWindowTheme("dark"); err != nil {
		t.Fatalf("SetWindowTheme(dark) = %v", err)
	}
	if err := a.SetWindowTheme("system"); err != nil {
		t.Fatalf("SetWindowTheme(system) = %v", err)
	}
	if want := []string{"dark", "system"}; !reflect.DeepEqual(applied, want) {
		t.Fatalf("applied = %v, want %v", applied, want)
	}
	if err := a.SetWindowTheme("sepia"); err == nil {
		t.Fatal("SetWindowTheme(sepia) = nil, want an error for an unknown theme")
	}
	if len(applied) != 2 {
		t.Fatalf("an unknown theme must not apply anything, applied = %v", applied)
	}
}

func TestChooseFolder(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		pickPath    string
		pickErr     error
		scanErr     error
		wantErr     error
		wantScanned []string
	}{
		{name: "picked and scanned", pickPath: "/media", wantScanned: []string{"/media"}},
		{name: "dialog cancelled is not an error", pickPath: ""},
		{name: "dialog fails", pickErr: boom, wantErr: boom},
		{name: "scan fails", pickPath: "/media", scanErr: boom, wantErr: boom, wantScanned: []string{"/media"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &fakeLibrary{scanErr: tt.scanErr}
			a := newTestApp(lib)
			a.pickFolder = func(context.Context) (string, error) { return tt.pickPath, tt.pickErr }

			err := a.ChooseFolder()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ChooseFolder() error = %v, want %v", err, tt.wantErr)
			}
			if len(lib.scanned) != len(tt.wantScanned) || (len(lib.scanned) > 0 && lib.scanned[0] != tt.wantScanned[0]) {
				t.Fatalf("scanned = %v, want %v", lib.scanned, tt.wantScanned)
			}
		})
	}
}

func TestGetLibrarySummary(t *testing.T) {
	t.Run("nil before first scan", func(t *testing.T) {
		if got := newTestApp(&fakeLibrary{}).GetLibrarySummary(); got != nil {
			t.Fatalf("GetLibrarySummary() = %+v, want nil", got)
		}
	})

	t.Run("maps to DTO", func(t *testing.T) {
		lib := &fakeLibrary{hasSummary: true, summary: app.LibrarySummary{
			Root: "/m",
			Stats: media.Stats{Total: 3, TotalSize: 99, Counts: map[media.Kind]int{
				media.KindVideo: 2, media.KindImage: 1,
			}},
		}}
		got := newTestApp(lib).GetLibrarySummary()
		want := LibrarySummaryDTO{Root: "/m", Total: 3, TotalSize: 99, Counts: KindCountsDTO{Video: 2, Image: 1}}
		if got == nil || *got != want {
			t.Fatalf("GetLibrarySummary() = %+v, want %+v", got, want)
		}
	})
}

type appCtxKey struct{}

func TestStartServer_UsesAppContextAndSavedSettings(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	a.ctx = context.WithValue(context.Background(), appCtxKey{}, "app")
	a.settings = &fakeSettings{view: app.SettingsView{Settings: app.Settings{Port: 9123, Name: "Den"}}}

	if err := a.StartServer(); err != nil {
		t.Fatalf("StartServer() error = %v", err)
	}
	if st.startCtx != a.ctx {
		t.Error("server must live as long as the app context, not a per-call one")
	}
	if want := streaming.NewConfig(streaming.WithPort(9123), streaming.WithName("Den")); st.startCfg != want {
		t.Errorf("config = %+v, want %+v (the saved settings)", st.startCfg, want)
	}
}

func TestStartStopServer_PropagateErrors(t *testing.T) {
	boom := errors.New("boom")
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	st.startErr, st.stopErr = boom, boom

	if err := a.StartServer(); !errors.Is(err, boom) {
		t.Errorf("StartServer() error = %v, want %v", err, boom)
	}
	if err := a.StopServer(); !errors.Is(err, boom) {
		t.Errorf("StopServer() error = %v, want %v", err, boom)
	}
}

func TestGetServerState(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	st.status = app.ServerStatus{State: server.Failed, Name: "Den", Error: "port in use"}

	got := a.GetServerState()
	want := ServerStateDTO{State: "failed", Name: "Den", Error: "port in use", LANURLs: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetServerState() = %#v, want %#v", got, want)
	}
	if got.LANURLs == nil {
		t.Error("LANURLs must be an empty slice so JSON carries [] not null")
	}

	st.status = app.ServerStatus{
		State: server.Running, Name: "Den", Port: 8765,
		LocalURL: "http://127.0.0.1:8765", LANURLs: []string{"http://192.168.1.20:8765"},
	}
	want = ServerStateDTO{
		State: "running", Name: "Den", Port: 8765,
		LocalURL: "http://127.0.0.1:8765", LANURLs: []string{"http://192.168.1.20:8765"},
	}
	if got := a.GetServerState(); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetServerState() = %#v, want %#v", got, want)
	}
}

func TestShutdown_StopsServerEvenIfContextIsCancelled(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a.shutdown(ctx)

	if st.stops != 1 {
		t.Fatalf("Stop called %d times, want 1", st.stops)
	}
	if st.stopCtx.Err() != nil {
		t.Error("Stop received a cancelled context; graceful shutdown would be skipped")
	}
}

func TestShutdown_IdleServerIsNotAnError(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	st.stopErr = fmt.Errorf("stop: %w", server.ErrInvalidTransition)
	a.shutdown(context.Background()) // must not panic or log an error; nothing to assert beyond that
}

func TestGetLogHistory_MapsEntries(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	logs := newFakeLogs()
	logs.history = []app.LogEntry{
		{Seq: 4, Time: time.UnixMilli(1_700_000_000_123), Level: slog.LevelWarn, Source: "http",
			Message: "slow", Attrs: []app.LogAttr{{Key: "ms", Value: "900"}}},
		{Seq: 5, Level: slog.LevelInfo, Source: "app", Message: "no attrs"},
	}
	a.logs = logs

	got := a.GetLogHistory()
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	want := LogLineDTO{Seq: 4, Time: 1_700_000_000_123, Level: "warn", Source: "http",
		Message: "slow", Attrs: []LogAttrDTO{{Key: "ms", Value: "900"}}}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("line 0 = %+v, want %+v", got[0], want)
	}
	if got[1].Attrs == nil {
		t.Error("Attrs is nil; it must be an empty slice so JSON carries [] not null")
	}
}

func TestGetLogHistory_EmptyIsNotNil(t *testing.T) {
	if got := newTestApp(&fakeLibrary{}).GetLogHistory(); got == nil || len(got) != 0 {
		t.Fatalf("GetLogHistory() = %#v, want an empty non-nil slice", got)
	}
}

func TestLevelName(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug - 4, "debug"},
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo - 1, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelWarn - 1, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError - 1, "warn"},
		{slog.LevelError, "error"},
		{slog.LevelError + 8, "error"},
	}
	for _, tt := range tests {
		if got := levelName(tt.level); got != tt.want {
			t.Errorf("levelName(%d) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestForwardLogs_EmitsUntilContextIsCancelled(t *testing.T) {
	var mu sync.Mutex
	var got []emitted
	a := newTestApp(&fakeLibrary{})
	logs := newFakeLogs()
	a.logs = logs
	a.emitter.emit = func(_ context.Context, name string, data ...any) {
		mu.Lock()
		got = append(got, emitted{name, data})
		mu.Unlock()
	}
	a.emitter.bind(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() { a.forwardLogs(ctx); close(finished) }()

	logs.live <- app.LogEntry{Seq: 1, Level: slog.LevelError, Message: "boom"}
	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("log line was not forwarded within 1s")
		}
		time.Sleep(2 * time.Millisecond)
	}

	mu.Lock()
	first := got[0]
	mu.Unlock()
	dto, ok := first.data[0].(LogLineDTO)
	if first.name != "log:line" || !ok || dto.Seq != 1 || dto.Level != "error" || dto.Message != "boom" {
		t.Fatalf("emitted = %+v", first)
	}

	cancel()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("forwardLogs did not return after its context was cancelled (goroutine leak)")
	}
	if logs.cancels != 1 {
		t.Errorf("unsubscribed %d times, want exactly 1", logs.cancels)
	}
}

func TestForwardLogs_ReturnsWhenFeedCloses(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	logs := newFakeLogs()
	a.logs = logs
	finished := make(chan struct{})
	go func() { a.forwardLogs(context.Background()); close(finished) }()

	close(logs.live)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("forwardLogs kept running after the feed closed")
	}
}

func TestRescanLibrary(t *testing.T) {
	boom := errors.New("a scan is already running")
	lib := &fakeLibrary{}
	a := newTestApp(lib)
	if err := a.RescanLibrary(); err != nil || lib.rescans != 1 {
		t.Fatalf("RescanLibrary() = %v after %d rescans", err, lib.rescans)
	}
	lib.rescanErr = boom
	if err := a.RescanLibrary(); !errors.Is(err, boom) {
		t.Errorf("error = %v, want %v", err, boom)
	}
}

func TestGetLibraryStatus(t *testing.T) {
	lib := &fakeLibrary{status: app.LibraryStatus{Scanning: false, Root: "/gone", Error: "That folder no longer exists.", Watching: true}}
	got := newTestApp(lib).GetLibraryStatus()
	want := LibraryStatusDTO{Root: "/gone", Error: "That folder no longer exists.", Watching: true}
	if got != want {
		t.Fatalf("GetLibraryStatus() = %+v, want %+v", got, want)
	}
}

func TestStartup_StartsWatchingTheLibraryUntilShutdown(t *testing.T) {
	lib := &fakeLibrary{watching: make(chan struct{})}
	a := newTestApp(lib)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.startup(ctx)

	select {
	case <-lib.watching:
	case <-time.After(2 * time.Second):
		t.Fatal("startup did not start the library watch")
	}
}

func TestRestoreLibrary(t *testing.T) {
	t.Run("asks the library to reopen the remembered folder", func(t *testing.T) {
		lib := &fakeLibrary{}
		newTestApp(lib).restoreLibrary(context.Background())
		if lib.restores != 1 {
			t.Errorf("Restore called %d times, want 1", lib.restores)
		}
	})
	t.Run("a failure is logged, not fatal", func(t *testing.T) {
		lib := &fakeLibrary{restoreErr: errors.New("folder gone")}
		newTestApp(lib).restoreLibrary(context.Background()) // must return normally
	})
}

func TestGetSettings_ExposesTheTokenOnlyWhenOn(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	fs := &fakeSettings{view: app.SettingsView{
		Settings: app.Settings{Port: 9000, Name: "Den", RequireToken: true},
		Token:    "tok", TokenPersisted: true,
	}}
	a.settings = fs

	want := SettingsDTO{Port: 9000, Name: "Den", RequireToken: true, AccessToken: "tok", TokenPersisted: true}
	if got := a.GetSettings(); got != want {
		t.Fatalf("GetSettings() = %+v, want %+v", got, want)
	}

	// The DTO has no field for the library folder: it is not a setting the form edits.
	fs.view = app.SettingsView{Settings: app.Settings{Port: 9000, Name: "Den", LibraryRoot: "/private/path"}}
	if got := a.GetSettings(); got.AccessToken != "" || got.RequireToken {
		t.Errorf("GetSettings() = %+v, want no token when it is off", got)
	}
}

func TestUpdateSettings_PassesTheFormValuesAndReturnsValidationErrors(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	fs := &fakeSettings{}
	a.settings = fs

	if err := a.UpdateSettings(SettingsInputDTO{Port: 9100, Name: "Den", RequireToken: true}); err != nil {
		t.Fatal(err)
	}
	if fs.gotInput != (settings.Input{Port: 9100, Name: "Den", RequireToken: true}) {
		t.Errorf("settings received %+v", fs.gotInput)
	}

	fs.updateErr = errors.New("port must be between 1024 and 65535")
	if err := a.UpdateSettings(SettingsInputDTO{Port: 80}); err == nil || err.Error() != fs.updateErr.Error() {
		t.Errorf("error = %v, want the validation message passed through unchanged", err)
	}
}

func TestRotateAccessToken(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	fs := &fakeSettings{}
	a.settings = fs
	if err := a.RotateAccessToken(); err != nil || fs.rotations != 1 {
		t.Fatalf("RotateAccessToken() = %v after %d rotations", err, fs.rotations)
	}
	fs.rotateErr = settings.ErrTokenOff
	if err := a.RotateAccessToken(); !errors.Is(err, settings.ErrTokenOff) {
		t.Errorf("error = %v", err)
	}
}

// fakeRemote is a controllable Remote that remembers how it was called.
type fakeRemote struct {
	status                          app.RemoteStatus
	providers                       []app.ProviderInfo
	enableErr                       error
	disableErr, setPINErr, clearErr error

	states        []app.FieldState
	configErr     error
	setFields     [][3]string // provider, key, value
	clearedFields [][2]string // provider, key

	enabledID  app.ProviderID
	enabledSrv app.LocalServer
	enableCtx  context.Context
	enables    int
	disables   int
	pins       []string
	clears     int
}

func (f *fakeRemote) Providers() []app.ProviderInfo { return f.providers }
func (f *fakeRemote) Status() app.RemoteStatus      { return f.status }

func (f *fakeRemote) Enable(ctx context.Context, id app.ProviderID, srv app.LocalServer) error {
	f.enables++
	f.enableCtx, f.enabledID, f.enabledSrv = ctx, id, srv
	return f.enableErr
}

func (f *fakeRemote) Disable(context.Context) error { f.disables++; return f.disableErr }

func (f *fakeRemote) SetPIN(pin string) error {
	f.pins = append(f.pins, pin)
	return f.setPINErr
}

func (f *fakeRemote) ClearPIN() error { f.clears++; return f.clearErr }

func (f *fakeRemote) ProviderConfig(app.ProviderID) ([]app.FieldState, error) {
	return f.states, f.configErr
}

func (f *fakeRemote) SetProviderField(id app.ProviderID, key, value string) error {
	f.setFields = append(f.setFields, [3]string{string(id), key, value})
	return f.configErr
}

func (f *fakeRemote) ClearProviderField(id app.ProviderID, key string) error {
	f.clearedFields = append(f.clearedFields, [2]string{string(id), key})
	return f.configErr
}

type stubLocal struct{}

func (stubLocal) Addrs() []netip.AddrPort         { return nil }
func (stubLocal) Bind(netip.Addr) (func(), error) { return func() {}, nil }

func newRemoteTestApp() (*App, *fakeStreaming, *fakeRemote) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	rem := &fakeRemote{}
	a.remote = rem
	return a, st, rem
}

func TestEnableRemote_HandsTheRunningServerToTheService(t *testing.T) {
	a, st, rem := newRemoteTestApp()
	st.local = stubLocal{}

	if err := a.EnableRemote("tailscale"); err != nil {
		t.Fatalf("EnableRemote: %v", err)
	}

	if rem.enables != 1 || rem.enabledID != "tailscale" || rem.enabledSrv != st.local {
		t.Errorf("Enable got id=%q srv=%v (calls %d)", rem.enabledID, rem.enabledSrv, rem.enables)
	}
	if rem.enableCtx != a.ctx {
		t.Error("the tunnel must live as long as the app, not as long as one call")
	}
}

func TestEnableRemote_RefusedWhileTheServerIsNotRunning(t *testing.T) {
	a, st, rem := newRemoteTestApp()
	st.localErr = fmt.Errorf("local: %w", app.ErrServerNotRunning)

	err := a.EnableRemote("tailscale")

	if err == nil || err.Error() != "Start the server first." {
		t.Fatalf("error = %v, want advice to start the server", err)
	}
	if rem.enables != 0 {
		t.Error("a tunnel was requested although there is no server to attach it to")
	}
}

func TestRemoteErrors_BecomeAdviceAndOthersPassThrough(t *testing.T) {
	boom := errors.New("Tailscale is turned off. Turn it on in the Tailscale app, then try again")
	cases := map[string]struct {
		in   error
		want string
	}{
		"no PIN":            {fmt.Errorf("enable: %w", app.ErrPINRequired), "Set an access PIN before turning on public access."},
		"PIN while public":  {fmt.Errorf("remove PIN: %w", app.ErrPublicActive), "Turn off public access before removing the PIN."},
		"already on":        {fmt.Errorf("enable: %w", app.ErrAlreadyEnabled), "Remote access is already on."},
		"unknown provider":  {fmt.Errorf("x: %w", app.ErrUnknownProvider), "That way of connecting is not available."},
		"provider's reason": {fmt.Errorf("enable %q: %w", "tailscale", boom), `enable "tailscale": ` + boom.Error()},
		"weak PIN":          {fmt.Errorf("%w: use at least 8 characters", app.ErrInvalidPIN), "invalid PIN: use at least 8 characters"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a, st, rem := newRemoteTestApp()
			st.local = stubLocal{}
			rem.enableErr, rem.setPINErr, rem.clearErr, rem.disableErr = c.in, c.in, c.in, c.in
			for call, fn := range map[string]func() error{
				"enable":  func() error { return a.EnableRemote("x") },
				"disable": a.DisableRemote,
				"setPIN":  func() error { return a.SetRemotePIN("whatever pin") },
				"clear":   a.ClearRemotePIN,
			} {
				if err := fn(); err == nil || err.Error() != c.want {
					t.Errorf("%s: error = %v, want %q", call, err, c.want)
				}
			}
		})
	}
}

func TestRemoteCommands_ReachTheService(t *testing.T) {
	a, _, rem := newRemoteTestApp()

	if err := a.SetRemotePIN("correct horse"); err != nil || len(rem.pins) != 1 || rem.pins[0] != "correct horse" {
		t.Errorf("SetRemotePIN: err=%v pins=%v", err, rem.pins)
	}
	if err := a.ClearRemotePIN(); err != nil || rem.clears != 1 {
		t.Errorf("ClearRemotePIN: err=%v clears=%d", err, rem.clears)
	}
	if err := a.DisableRemote(); err != nil || rem.disables != 1 {
		t.Errorf("DisableRemote: err=%v disables=%d", err, rem.disables)
	}
}

func TestRemoteQueries_AreMappedToDTOs(t *testing.T) {
	a, _, rem := newRemoteTestApp()
	rem.providers = []app.ProviderInfo{{ID: "tailscale"}, {ID: "cloudflare-quick", Public: true}}
	rem.status = app.RemoteStatus{
		State: server.Running, Provider: "cloudflare-quick", Public: true,
		URL: "https://abc.trycloudflare.com", PINSet: true, Sessions: 2,
		Live: app.TunnelStatus{State: server.Starting},
	}

	wantProviders := []RemoteProviderDTO{{ID: "tailscale", Fields: []RemoteFieldDTO{}}, {ID: "cloudflare-quick", Public: true, Fields: []RemoteFieldDTO{}}}
	if got := a.ListRemoteProviders(); !reflect.DeepEqual(got, wantProviders) {
		t.Errorf("providers = %+v, want %+v", got, wantProviders)
	}
	want := RemoteStatusDTO{
		State: "running", Provider: "cloudflare-quick", Public: true,
		URL: "https://abc.trycloudflare.com", Live: "starting", PinSet: true, Sessions: 2,
		NeedsSetup: []string{},
	}
	if got := a.GetRemoteStatus(); !reflect.DeepEqual(got, want) {
		t.Errorf("status = %+v, want %+v", got, want)
	}

	rem.providers = nil
	if got := a.ListRemoteProviders(); got == nil {
		t.Error("no providers must be an empty list, not null: JSON null breaks the UI's .map")
	}
}

func TestShutdown_TurnsRemoteAccessOffBeforeStoppingTheServer(t *testing.T) {
	a, st, rem := newRemoteTestApp()
	var order []string
	rem.disableErr = nil
	a.remote = &orderRemote{fakeRemote: rem, order: &order}
	a.streaming = &orderStreaming{fakeStreaming: st, order: &order}

	a.shutdown(context.Background())

	if !reflect.DeepEqual(order, []string{"disable remote", "stop server"}) {
		t.Errorf("order = %v", order)
	}
}

type orderRemote struct {
	*fakeRemote
	order *[]string
}

func (o *orderRemote) Disable(ctx context.Context) error {
	*o.order = append(*o.order, "disable remote")
	return o.fakeRemote.Disable(ctx)
}

type orderStreaming struct {
	*fakeStreaming
	order *[]string
}

func (o *orderStreaming) Stop(ctx context.Context) error {
	*o.order = append(*o.order, "stop server")
	return o.fakeStreaming.Stop(ctx)
}

func TestShutdown_StillStopsTheServerWhenRemoteWillNotGoDown(t *testing.T) {
	a, st, rem := newRemoteTestApp()
	rem.disableErr = errors.New("process stuck")

	a.shutdown(context.Background())

	if st.stops != 1 {
		t.Errorf("server stops = %d, want 1: a stuck tunnel must not keep the app open", st.stops)
	}
}

func TestProviderFieldsAndSetupReachTheUI(t *testing.T) {
	a, _, rem := newRemoteTestApp()
	rem.providers = []app.ProviderInfo{{
		ID: "cloudflare-named", Public: true,
		Fields: []app.ConfigField{
			{Key: "token", Label: "Tunnel token", Help: "h", Placeholder: "p", Kind: app.FieldSecret, Required: true},
			{Key: "binary", Label: "Location", Kind: app.FieldFile},
		},
	}}
	rem.status = app.RemoteStatus{NeedsSetup: []app.ProviderID{"cloudflare-named"}}

	got := a.ListRemoteProviders()[0]
	if len(got.Fields) != 2 || got.Fields[0] != (RemoteFieldDTO{Key: "token", Label: "Tunnel token", Help: "h", Placeholder: "p", Kind: "secret", Required: true}) {
		t.Errorf("fields = %+v", got.Fields)
	}
	if got.Fields[1].Kind != "file" || got.Fields[1].Required {
		t.Errorf("optional file field = %+v", got.Fields[1])
	}
	if s := a.GetRemoteStatus(); len(s.NeedsSetup) != 1 || s.NeedsSetup[0] != "cloudflare-named" {
		t.Errorf("needsSetup = %v", s.NeedsSetup)
	}
}

func TestProviderConfig_NeverSendsASecretOut(t *testing.T) {
	a, _, rem := newRemoteTestApp()
	// Even if a bug upstream handed over a secret's value, it must stop here.
	rem.states = []app.FieldState{
		{Field: app.ConfigField{Key: "token", Kind: app.FieldSecret}, Set: true, Value: "LEAKED-TOKEN"},
		{Field: app.ConfigField{Key: "hostname", Kind: app.FieldText}, Set: true, Value: "media.example.com"},
		{Field: app.ConfigField{Key: "binary", Kind: app.FieldFile}},
	}

	got, err := a.GetRemoteProviderConfig("cloudflare-named")

	if err != nil {
		t.Fatal(err)
	}
	want := []RemoteFieldStateDTO{{Key: "token", Set: true}, {Key: "hostname", Set: true, Value: "media.example.com"}, {Key: "binary"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestProviderSettings_ReachTheServiceAndErrorsBecomeAdvice(t *testing.T) {
	a, _, rem := newRemoteTestApp()
	if err := a.SetRemoteProviderField("cloudflare-named", "hostname", "media.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := a.ClearRemoteProviderField("cloudflare-named", "token"); err != nil {
		t.Fatal(err)
	}
	if len(rem.setFields) != 1 || rem.setFields[0] != [3]string{"cloudflare-named", "hostname", "media.example.com"} ||
		len(rem.clearedFields) != 1 || rem.clearedFields[0] != [2]string{"cloudflare-named", "token"} {
		t.Errorf("set=%v cleared=%v", rem.setFields, rem.clearedFields)
	}

	cases := map[string]struct {
		in   error
		want string
	}{
		"in use":        {fmt.Errorf("Hostname: %w", app.ErrProviderInUse), "Turn this option off before changing its settings."},
		"unknown":       {fmt.Errorf("x: %w", app.ErrUnknownField), "That setting does not exist."},
		"invalid value": {fmt.Errorf("Hostname: %w: enter the hostname only, without a port", app.ErrInvalidValue), "Hostname: enter the hostname only, without a port"},
		"not set up":    {fmt.Errorf("enable: %w", &app.NotConfiguredError{Missing: []string{"Tunnel token", "Hostname"}}), "Finish setting this up first: Tunnel token, Hostname."},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rem.configErr = c.in
			for call, err := range map[string]error{
				"set":   a.SetRemoteProviderField("p", "k", "v"),
				"clear": a.ClearRemoteProviderField("p", "k"),
			} {
				if err == nil || err.Error() != c.want {
					t.Errorf("%s: error = %v, want %q", call, err, c.want)
				}
			}
			if _, err := a.GetRemoteProviderConfig("p"); err == nil || err.Error() != c.want {
				t.Errorf("get: error = %v, want %q", err, c.want)
			}
		})
	}
}

func TestEnableRemote_ASetupProblemIsAdvice(t *testing.T) {
	a, st, rem := newRemoteTestApp()
	st.local = stubLocal{}
	rem.enableErr = fmt.Errorf("enable %q: %w", "cloudflare-named", &app.NotConfiguredError{Missing: []string{"Tunnel token"}})

	err := a.EnableRemote("cloudflare-named")

	if err == nil || err.Error() != "Finish setting this up first: Tunnel token." {
		t.Errorf("error = %v", err)
	}
}

func TestChooseFile(t *testing.T) {
	a, _, _ := newRemoteTestApp()
	var gotTitle string
	a.pickFile = func(_ context.Context, title string) (string, error) {
		gotTitle = title
		return "/opt/bin/cloudflared", nil
	}

	if path, err := a.ChooseFile("Choose cloudflared"); err != nil || path != "/opt/bin/cloudflared" || gotTitle != "Choose cloudflared" {
		t.Errorf("path=%q err=%v title=%q", path, err, gotTitle)
	}

	a.pickFile = func(context.Context, string) (string, error) { return "", nil } // cancelled
	if path, err := a.ChooseFile("x"); err != nil || path != "" {
		t.Errorf("cancelled: %q, %v; cancelling is not an error", path, err)
	}

	boom := errors.New("no window")
	a.pickFile = func(context.Context, string) (string, error) { return "", boom }
	if _, err := a.ChooseFile("x"); !errors.Is(err, boom) {
		t.Errorf("error = %v", err)
	}
}
