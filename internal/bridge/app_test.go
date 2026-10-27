package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/portfree"
	"eincast/internal/app/profiles"
	"eincast/internal/app/settings"
	"eincast/internal/app/streaming"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
	"eincast/internal/domain/server"
)

type fakeLibrary struct {
	state []app.LibraryState
	items []media.Item

	addErr     error
	added      []string
	removeErr  error
	removed    []media.LibraryID
	hidden     []string
	hideErr    error
	renameErr  error
	renamed    [][2]string
	rescanErr  error
	rescanned  []media.LibraryID
	rescanAll  int
	restoreErr error
	restores   int
	watching   chan struct{} // closed when Watch is called
}

func (f *fakeLibrary) Add(_ context.Context, root string) (app.LibraryState, error) {
	f.added = append(f.added, root)
	return app.LibraryState{Library: media.NewLibrary(root)}, f.addErr
}

func (f *fakeLibrary) Remove(id media.LibraryID) error {
	f.removed = append(f.removed, id)
	return f.removeErr
}

func (f *fakeLibrary) SetHiddenFromTVs(id media.LibraryID, hidden bool) error {
	f.hidden = append(f.hidden, fmt.Sprintf("%s=%v", id, hidden))
	return f.hideErr
}

func (f *fakeLibrary) Rename(id media.LibraryID, name string) error {
	f.renamed = append(f.renamed, [2]string{string(id), name})
	return f.renameErr
}

func (f *fakeLibrary) Rescan(_ context.Context, id media.LibraryID) error {
	f.rescanned = append(f.rescanned, id)
	return f.rescanErr
}

func (f *fakeLibrary) RescanAll(context.Context) error {
	f.rescanAll++
	return f.rescanErr
}

func (f *fakeLibrary) Restore(context.Context) error {
	f.restores++
	return f.restoreErr
}

func (f *fakeLibrary) State() []app.LibraryState { return f.state }

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
	ffmpegSet []string
	ffmpegErr error
}

func (f *fakeSettings) SetFFmpegPath(path string) error {
	f.ffmpegSet = append(f.ffmpegSet, path)
	return f.ffmpegErr
}

// fakeFFmpeg answers for the ffmpeg adapter.
type fakeFFmpeg struct {
	path     string
	checkErr error
	checked  []string
}

func (f *fakeFFmpeg) Path() string { return f.path }
func (f *fakeFFmpeg) Check(path string) error {
	f.checked = append(f.checked, path)
	return f.checkErr
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

type fakePorts struct {
	holders  []portfree.Holder
	checkErr error
	freeErr  error
	checked  []int
	freed    [][2]int
}

func (f *fakePorts) Check(_ context.Context, port int) ([]portfree.Holder, error) {
	f.checked = append(f.checked, port)
	return f.holders, f.checkErr
}

func (f *fakePorts) Free(_ context.Context, port, pid int) error {
	f.freed = append(f.freed, [2]int{port, pid})
	return f.freeErr
}

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
		Ports:     &fakePorts{},
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
			ID: "abc", LibraryID: "lib1", Name: "a.mkv", RelPath: "shows/a.mkv", Path: "/secret/shows/a.mkv",
			Kind: media.KindVideo, Size: 42, ModTime: mod,
		}}}
		got := newTestApp(lib).ListLibrary()
		want := []LibraryItemDTO{{
			ID: "abc", LibraryID: "lib1", Name: "a.mkv", RelPath: "shows/a.mkv", Kind: "video", Size: 42, ModTime: mod.UnixMilli(),
			Title: "a.mkv", // no title read from the name: the file name stands in
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ListLibrary() = %+v, want %+v", got, want)
		}
	})

	t.Run("carries titles, years and the series an episode belongs to", func(t *testing.T) {
		lib := &fakeLibrary{items: []media.Item{
			{ID: "m", Name: "Heat.1995.mkv", Kind: media.KindVideo, Meta: media.Meta{Title: "Heat", Year: 1995},
				Artwork: media.Artwork{Path: "/secret/poster.jpg"}},
			{ID: "e", Name: "S01E02.mkv", Kind: media.KindVideo, Meta: media.Meta{Title: "Pilot", Show: "Show", Season: 1, Episode: 2}},
			{ID: "n", Name: "x.mkv", Kind: media.KindVideo, Meta: media.Meta{Title: "x", Episode: 3}}, // numbered but no series
		}}
		got := newTestApp(lib).ListLibrary()
		if got[0].Title != "Heat" || got[0].Year != 1995 || !got[0].Poster || got[0].Show != "" {
			t.Errorf("movie = %+v", got[0])
		}
		if got[1].Show != "Show" || got[1].Season != 1 || got[1].Episode != 2 || got[1].Title != "Pilot" {
			t.Errorf("episode = %+v", got[1])
		}
		if got[2].Show != "" || got[2].Episode != 0 {
			t.Errorf("an item with no series was filed as an episode: %+v", got[2])
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
		name      string
		pickPath  string
		pickErr   error
		addErr    error
		wantErr   error
		wantAdded []string
	}{
		{name: "picked and added", pickPath: "/media", wantAdded: []string{"/media"}},
		{name: "dialog cancelled is not an error", pickPath: ""},
		{name: "dialog fails", pickErr: boom, wantErr: boom},
		{name: "adding fails", pickPath: "/media", addErr: boom, wantErr: boom, wantAdded: []string{"/media"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lib := &fakeLibrary{addErr: tt.addErr}
			a := newTestApp(lib)
			a.pickFolder = func(context.Context) (string, error) { return tt.pickPath, tt.pickErr }

			err := a.ChooseFolder()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ChooseFolder() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(lib.added, tt.wantAdded) {
				t.Fatalf("added = %v, want %v", lib.added, tt.wantAdded)
			}
		})
	}
}

func TestListLibraries(t *testing.T) {
	t.Run("empty is an empty list, not nil", func(t *testing.T) {
		if got := newTestApp(&fakeLibrary{}).ListLibraries(); got == nil || len(got) != 0 {
			t.Fatalf("ListLibraries() = %#v, want an empty non-nil slice", got)
		}
	})

	t.Run("maps every library to a DTO", func(t *testing.T) {
		films := media.Library{ID: "f1", Name: "Films", Root: "/m/films"}
		music := media.Library{ID: "m1", Name: "Music", Root: "/m/music"}
		lib := &fakeLibrary{state: []app.LibraryState{
			{Library: films, Stats: media.Stats{Total: 3, TotalSize: 99, Counts: map[media.Kind]int{media.KindVideo: 2, media.KindImage: 1}}, Watching: true},
			{Library: music, Scanning: true, Error: "That folder no longer exists."},
		}}
		got := newTestApp(lib).ListLibraries()
		want := []LibraryStateDTO{
			{ID: "f1", Name: "Films", Root: "/m/films", Total: 3, TotalSize: 99, Counts: KindCountsDTO{Video: 2, Image: 1}, Watching: true},
			{ID: "m1", Name: "Music", Root: "/m/music", Scanning: true, Error: "That folder no longer exists."},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ListLibraries() = %+v, want %+v", got, want)
		}
	})
}

func TestLibraryCommands_PassTheIDAndReturnTheErrors(t *testing.T) {
	boom := errors.New("boom")
	lib := &fakeLibrary{}
	a := newTestApp(lib)

	if err := a.RemoveLibrary("f1"); err != nil || !reflect.DeepEqual(lib.removed, []media.LibraryID{"f1"}) {
		t.Fatalf("RemoveLibrary: %v %v", err, lib.removed)
	}
	if err := a.RenameLibrary("f1", "Movie night"); err != nil || !reflect.DeepEqual(lib.renamed, [][2]string{{"f1", "Movie night"}}) {
		t.Fatalf("RenameLibrary: %v %v", err, lib.renamed)
	}
	if err := a.RescanLibrary("f1"); err != nil || !reflect.DeepEqual(lib.rescanned, []media.LibraryID{"f1"}) {
		t.Fatalf("RescanLibrary: %v %v", err, lib.rescanned)
	}
	if err := a.RescanAllLibraries(); err != nil || lib.rescanAll != 1 {
		t.Fatalf("RescanAllLibraries: %v %d", err, lib.rescanAll)
	}

	lib.removeErr, lib.renameErr, lib.rescanErr = boom, boom, boom
	for name, err := range map[string]error{
		"remove": a.RemoveLibrary("x"), "rename": a.RenameLibrary("x", "y"),
		"rescan": a.RescanLibrary("x"), "rescan all": a.RescanAllLibraries(),
	} {
		if !errors.Is(err, boom) {
			t.Errorf("%s: error = %v, want %v", name, err, boom)
		}
	}
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

func TestStartServer_SmartTVDiscoveryFollowsTheSettingButNeverWithTheToken(t *testing.T) {
	for name, tc := range map[string]struct {
		in   app.Settings
		want bool
	}{
		"on":                      {app.Settings{Port: 9000, Name: "Den", DLNA: true}, true},
		"off":                     {app.Settings{Port: 9000, Name: "Den"}, false},
		"token wins over the TVs": {app.Settings{Port: 9000, Name: "Den", DLNA: true, RequireToken: true}, false},
	} {
		t.Run(name, func(t *testing.T) {
			a, st := newTestAppWithStreaming(&fakeLibrary{})
			a.settings = &fakeSettings{view: app.SettingsView{Settings: tc.in}}
			if err := a.StartServer(); err != nil {
				t.Fatal(err)
			}
			if st.startCfg.DLNA != tc.want {
				t.Errorf("DLNA = %v, want %v", st.startCfg.DLNA, tc.want)
			}
		})
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
	want := ServerStateDTO{State: "failed", Name: "Den", Error: "port in use", LANURLs: []string{}, NameURLs: []string{}, Discovery: []string{}}
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
		NameURLs: []string{}, Discovery: []string{},
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

type fakeConversions struct {
	running chan struct{}
	stopped int
}

func (f *fakeConversions) Run(ctx context.Context) {
	close(f.running)
	<-ctx.Done()
}
func (f *fakeConversions) StopAll() { f.stopped++ }

func TestConversions_RunWithTheAppAndStopOnShutdown(t *testing.T) {
	conv := &fakeConversions{running: make(chan struct{})}
	a := newTestApp(&fakeLibrary{})
	a.convert = conv
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a.startup(ctx)
	select {
	case <-conv.running:
	case <-time.After(2 * time.Second):
		t.Fatal("startup did not start the conversion sweeper")
	}

	a.shutdown(ctx)
	if conv.stopped != 1 {
		t.Errorf("StopAll called %d times on shutdown, want 1", conv.stopped)
	}
}

func TestShutdown_WithoutConversionsIsFine(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	a.shutdown(context.Background()) // must not panic on a nil Conversions
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

	// The DTO has no field for the library folders: they are not settings the form edits.
	fs.view = app.SettingsView{Settings: app.Settings{Port: 9000, Name: "Den", Libraries: []media.Library{media.NewLibrary("/private/path")}}}
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
	if err := a.UpdateSettings(SettingsInputDTO{Port: 9100, Name: "Den", DLNA: true}); err != nil || !fs.gotInput.DLNA {
		t.Fatalf("the TV setting did not reach the settings service: %v %+v", err, fs.gotInput)
	}
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

func TestCheckPort(t *testing.T) {
	t.Run("maps holders to DTOs", func(t *testing.T) {
		a := newTestApp(&fakeLibrary{})
		ports := &fakePorts{holders: []portfree.Holder{
			{PortHolder: app.PortHolder{PID: 300, Name: "node"}},
			{PortHolder: app.PortHolder{PID: 42, Name: "eincast"}, Self: true},
		}}
		a.ports = ports

		got, err := a.CheckPort(8765)
		if err != nil {
			t.Fatal(err)
		}
		want := []PortHolderDTO{{PID: 300, Name: "node"}, {PID: 42, Name: "eincast", Self: true}}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(ports.checked, []int{8765}) {
			t.Fatalf("CheckPort = %+v (checked %v), want %+v", got, ports.checked, want)
		}
	})

	t.Run("a free port is an empty list, not nil", func(t *testing.T) {
		got, err := newTestApp(&fakeLibrary{}).CheckPort(8765)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("CheckPort = %#v, %v; want empty non-nil slice", got, err)
		}
	})

	t.Run("explains a bad port in plain words", func(t *testing.T) {
		a := newTestApp(&fakeLibrary{})
		a.ports = &fakePorts{checkErr: portfree.ErrInvalidPort}
		if _, err := a.CheckPort(80); err == nil || err.Error() != "Enter a port between 1024 and 65535." {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestFreePort(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"success", nil, ""},
		{"own process", portfree.ErrOwnProcess, "That is EinCast itself. Stop the server instead."},
		{"stale screen", portfree.ErrNotHolding, "That program is no longer using this port. Check again."},
		{"survivor", portfree.ErrStillHeld, "The program would not let go of the port."},
		{"permission", fmt.Errorf("end x: %w", app.ErrNotPermitted), "Not allowed: that program belongs to another user or to the system. Close it yourself, or run EinCast as an administrator."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(&fakeLibrary{})
			ports := &fakePorts{freeErr: tt.err}
			a.ports = ports

			err := a.FreePort(8765, 300)
			switch {
			case tt.wantMsg == "" && err != nil:
				t.Fatalf("error = %v", err)
			case tt.wantMsg != "" && (err == nil || err.Error() != tt.wantMsg):
				t.Fatalf("error = %v, want %q", err, tt.wantMsg)
			}
			if !reflect.DeepEqual(ports.freed, [][2]int{{8765, 300}}) {
				t.Fatalf("freed = %v", ports.freed)
			}
		})
	}
}

func TestStartServer_KeepAwakeFollowsTheSetting(t *testing.T) {
	for _, on := range []bool{true, false} {
		a, st := newTestAppWithStreaming(&fakeLibrary{})
		a.settings = &fakeSettings{view: app.SettingsView{Settings: app.Settings{Port: 9000, Name: "Den", KeepAwake: on}}}
		if err := a.StartServer(); err != nil {
			t.Fatal(err)
		}
		if st.startCfg.KeepAwake != on {
			t.Errorf("KeepAwake = %v, want %v", st.startCfg.KeepAwake, on)
		}
	}
}

func autoStartApp(autoStart bool, libraries int, state server.State) (*App, *fakeStreaming, *fakeLibrary) {
	lib := &fakeLibrary{state: make([]app.LibraryState, libraries)}
	a, st := newTestAppWithStreaming(lib)
	a.settings = &fakeSettings{view: app.SettingsView{Settings: app.Settings{Port: 9000, Name: "Den", AutoStart: autoStart}}}
	st.status = app.ServerStatus{State: state}
	return a, st, lib
}

func TestAutoStart(t *testing.T) {
	tests := []struct {
		name      string
		autoStart bool
		libraries int
		state     server.State
		wantStart int
	}{
		{"on, with a library, idle", true, 1, server.Idle, 1},
		{"off", false, 1, server.Idle, 0},
		{"nothing to share yet", true, 0, server.Idle, 0},
		{"already running", true, 1, server.Running, 0},
		{"on its way up", true, 1, server.Starting, 0},
		{"failed: the person decides", true, 1, server.Failed, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, st, _ := autoStartApp(tt.autoStart, tt.libraries, tt.state)
			a.autoStart()
			if st.starts != tt.wantStart {
				t.Errorf("starts = %d, want %d", st.starts, tt.wantStart)
			}
		})
	}
}

func TestAutoStart_FollowsRestoringTheLibrary(t *testing.T) {
	a, st, lib := autoStartApp(true, 1, server.Idle)
	a.restoreLibrary(context.Background())
	if lib.restores != 1 || st.starts != 1 {
		t.Errorf("restores=%d starts=%d, want the server started after the folders are back", lib.restores, st.starts)
	}
}

func TestAutoStart_FollowsChoosingTheFirstFolder(t *testing.T) {
	a, st, _ := autoStartApp(true, 1, server.Idle)
	a.pickFolder = func(context.Context) (string, error) { return "/media", nil }
	if err := a.ChooseFolder(); err != nil {
		t.Fatal(err)
	}
	if st.starts != 1 {
		t.Errorf("starts = %d, want sharing to begin right after the folder is chosen", st.starts)
	}
}

func TestAutoStart_NotAfterAFolderThatFailed(t *testing.T) {
	a, st, lib := autoStartApp(true, 1, server.Idle)
	lib.addErr = errors.New("boom")
	a.pickFolder = func(context.Context) (string, error) { return "/media", nil }
	if err := a.ChooseFolder(); err == nil {
		t.Fatal("want the error")
	}
	if st.starts != 0 {
		t.Errorf("starts = %d after a folder that failed", st.starts)
	}
}

func TestAutoStart_AFailedStartIsOnlyLogged(t *testing.T) {
	a, st, _ := autoStartApp(true, 1, server.Idle)
	st.startErr = errors.New("address already in use")
	a.autoStart() // must not panic or surface: the server:state event carries it
	if st.starts != 1 {
		t.Errorf("starts = %d, want 1", st.starts)
	}
}

func TestRefreshAddresses(t *testing.T) {
	t.Run("restarts a running server with the saved settings", func(t *testing.T) {
		a, st, _ := autoStartApp(false, 1, server.Running)
		if err := a.RefreshAddresses(); err != nil {
			t.Fatal(err)
		}
		if st.stops != 1 || st.starts != 1 || st.startCfg.Port != 9000 {
			t.Errorf("stops=%d starts=%d cfg=%+v", st.stops, st.starts, st.startCfg)
		}
	})
	t.Run("does nothing when the server is not running", func(t *testing.T) {
		a, st, _ := autoStartApp(false, 1, server.Idle)
		if err := a.RefreshAddresses(); err != nil || st.stops+st.starts != 0 {
			t.Errorf("err=%v stops=%d starts=%d", err, st.stops, st.starts)
		}
	})
	t.Run("a stop that fails does not start a second server", func(t *testing.T) {
		a, st, _ := autoStartApp(false, 1, server.Running)
		st.stopErr = errors.New("stuck")
		if err := a.RefreshAddresses(); err == nil || st.starts != 0 {
			t.Errorf("err=%v starts=%d", err, st.starts)
		}
	})
}

func TestUpdateSettings_PassesTheStartupAndPowerChoices(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	fs := &fakeSettings{}
	a.settings = fs
	err := a.UpdateSettings(SettingsInputDTO{Port: 9000, Name: "Den", AutoStart: true, KeepAwake: true, OpenAtLogin: true, HideOnClose: true})
	if err != nil {
		t.Fatal(err)
	}
	want := settings.Input{Port: 9000, Name: "Den", AutoStart: true, KeepAwake: true, OpenAtLogin: true, HideOnClose: true}
	if fs.gotInput != want {
		t.Errorf("input = %+v, want %+v", fs.gotInput, want)
	}
}

func TestGetFFmpegStatus(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	a.settings = &fakeSettings{view: app.SettingsView{Settings: app.Settings{FFmpegPath: "/old/ffmpeg"}}}
	a.ffmpeg = &fakeFFmpeg{path: "/opt/homebrew/bin/ffmpeg"}
	got := a.GetFFmpegStatus()
	if !got.Available || got.Path != "/opt/homebrew/bin/ffmpeg" || got.Chosen != "/old/ffmpeg" || got.InstallCommand != ffmpegInstallCommand(runtime.GOOS) {
		t.Errorf("status = %+v", got)
	}

	a.ffmpeg = &fakeFFmpeg{}
	if got := a.GetFFmpegStatus(); got.Available || got.Path != "" {
		t.Errorf("status with no ffmpeg = %+v", got)
	}
	a.ffmpeg = nil
	if got := a.GetFFmpegStatus(); got.Available {
		t.Errorf("status with no adapter = %+v", got)
	}
}

func TestFFmpegInstallCommand(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "brew install ffmpeg", "windows": "winget install ffmpeg", "linux": ""} {
		if got := ffmpegInstallCommand(goos); got != want {
			t.Errorf("ffmpegInstallCommand(%q) = %q, want %q", goos, got, want)
		}
	}
}

func TestChooseFFmpeg(t *testing.T) {
	tests := []struct {
		name     string
		picked   string
		pickErr  error
		checkErr error
		wantErr  bool
		wantSet  []string
	}{
		{name: "a usable program is remembered", picked: "/opt/ffmpeg", wantSet: []string{"/opt/ffmpeg"}},
		{name: "cancelling is not an error", picked: ""},
		{name: "a file that cannot run is refused and not remembered", picked: "/opt/notes.txt", checkErr: errors.New("not executable"), wantErr: true},
		{name: "the dialog failing is an error", pickErr: errors.New("no window"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(&fakeLibrary{})
			fs := &fakeSettings{}
			a.settings = fs
			a.ffmpeg = &fakeFFmpeg{checkErr: tt.checkErr}
			a.pickFile = func(context.Context, string) (string, error) { return tt.picked, tt.pickErr }

			if err := a.ChooseFFmpeg(); (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(fs.ffmpegSet, tt.wantSet) {
				t.Errorf("remembered = %v, want %v", fs.ffmpegSet, tt.wantSet)
			}
		})
	}
}

func TestClearFFmpegPath(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	fs := &fakeSettings{}
	a.settings = fs
	if err := a.ClearFFmpegPath(); err != nil || !reflect.DeepEqual(fs.ffmpegSet, []string{""}) {
		t.Errorf("err=%v remembered=%v", err, fs.ffmpegSet)
	}
}

// ---- profiles ---------------------------------------------------------------------

type fakeProfiles struct {
	entries []app.ProfileEntry
	saveErr error
	saved   []profile.Profile
	deleted []profile.ID
	pins    map[profile.ID]string
	err     error
}

func (f *fakeProfiles) Entries() []app.ProfileEntry { return f.entries }
func (f *fakeProfiles) Save(p profile.Profile) (profile.Profile, error) {
	f.saved = append(f.saved, p)
	if f.saveErr != nil {
		return profile.Profile{}, f.saveErr
	}
	if p.ID == "" {
		p.ID = "new1"
	}
	return p, nil
}
func (f *fakeProfiles) Delete(id profile.ID) error { f.deleted = append(f.deleted, id); return f.err }
func (f *fakeProfiles) SetPIN(id profile.ID, pin string) error {
	if f.pins == nil {
		f.pins = map[profile.ID]string{}
	}
	f.pins[id] = pin
	return f.err
}
func (f *fakeProfiles) ClearPIN(id profile.ID) error { delete(f.pins, id); return f.err }

func appWithProfiles(fp *fakeProfiles) *App {
	a := newTestApp(&fakeLibrary{})
	a.profiles = fp
	return a
}

func TestListProfiles_NeverCarriesAPINAndIsNeverNull(t *testing.T) {
	fp := &fakeProfiles{entries: []app.ProfileEntry{
		{Profile: profile.Profile{ID: "k", Name: "Kids", Color: "green", Libraries: []media.LibraryID{"a"}, Kinds: []media.Kind{media.KindVideo}, HiddenFolders: []string{"18*"}}, HasPIN: true},
		{Profile: profile.Everyone()},
	}}
	got := appWithProfiles(fp).ListProfiles()
	if len(got) != 2 || got[0].ID != "k" || !got[0].HasPIN || got[0].Libraries[0] != "a" || got[0].Kinds[0] != "video" || got[0].HiddenFolders[0] != "18*" {
		t.Fatalf("ListProfiles() = %+v", got)
	}
	if got[1].Libraries == nil || got[1].Kinds == nil || got[1].HiddenFolders == nil {
		t.Errorf("an empty list must be [] not null: %+v", got[1])
	}
	if out := newTestApp(&fakeLibrary{}).ListProfiles(); out == nil || len(out) != 0 {
		t.Errorf("without profiles: %#v", out)
	}
}

func TestSaveProfile_PassesTheFormAndReturnsWhatWasStored(t *testing.T) {
	fp := &fakeProfiles{}
	a := appWithProfiles(fp)
	got, err := a.SaveProfile(ProfileDTO{Name: "Kids", Color: "green", Libraries: []string{"a"}, Kinds: []string{"video"}, HiddenFolders: []string{"x"}})
	if err != nil || got.ID != "new1" {
		t.Fatalf("SaveProfile() = %+v, %v", got, err)
	}
	want := profile.Profile{Name: "Kids", Color: "green", Libraries: []media.LibraryID{"a"}, Kinds: []media.Kind{media.KindVideo}, HiddenFolders: []string{"x"}}
	if len(fp.saved) != 1 || !reflect.DeepEqual(fp.saved[0], want) {
		t.Errorf("saved %+v, want %+v", fp.saved, want)
	}
}

func TestProfileErrors_AreAdviceInPlainWords(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"last":     {profiles.ErrLastProfile, "at least one profile"},
		"too many": {profiles.ErrTooMany, "at most"},
		"unknown":  {profiles.ErrUnknownProfile, "no longer exists"},
		"invalid":  {fmt.Errorf("%w: the name must be 1 to 32 characters", profile.ErrInvalid), "the name must be 1 to 32 characters"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			fp := &fakeProfiles{saveErr: c.err, err: c.err}
			a := appWithProfiles(fp)
			for label, err := range map[string]error{
				"save":   func() error { _, e := a.SaveProfile(ProfileDTO{}); return e }(),
				"delete": a.DeleteProfile("x"),
				"pin":    a.SetProfilePIN("x", "1234"),
				"clear":  a.ClearProfilePIN("x"),
			} {
				if err == nil || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), "invalid profile") {
					t.Errorf("%s: error = %v, want advice containing %q", label, err, c.want)
				}
			}
		})
	}
}

func TestProfileCommands_GoToTheService(t *testing.T) {
	fp := &fakeProfiles{}
	a := appWithProfiles(fp)
	if err := a.DeleteProfile("k"); err != nil || len(fp.deleted) != 1 || fp.deleted[0] != "k" {
		t.Errorf("delete: %v %v", err, fp.deleted)
	}
	if err := a.SetProfilePIN("k", "1234"); err != nil || fp.pins["k"] != "1234" {
		t.Errorf("set pin: %v %v", err, fp.pins)
	}
	if err := a.ClearProfilePIN("k"); err != nil || fp.pins["k"] != "" {
		t.Errorf("clear pin: %v %v", err, fp.pins)
	}
}

func TestProfileCommands_RefuseWithoutTheFeature(t *testing.T) {
	a := newTestApp(&fakeLibrary{})
	if _, err := a.SaveProfile(ProfileDTO{}); err == nil {
		t.Error("SaveProfile worked with no service")
	}
	if a.DeleteProfile("x") == nil || a.SetProfilePIN("x", "1234") == nil || a.ClearProfilePIN("x") == nil {
		t.Error("a command worked with no service")
	}
}

func TestSetLibraryHiddenFromTVs(t *testing.T) {
	lib := &fakeLibrary{}
	a := newTestApp(lib)
	if err := a.SetLibraryHiddenFromTVs("L1", true); err != nil || len(lib.hidden) != 1 || lib.hidden[0] != "L1=true" {
		t.Errorf("hidden = %v, %v", lib.hidden, err)
	}
	lib.hideErr = errors.New("nope")
	if err := a.SetLibraryHiddenFromTVs("L1", false); err == nil {
		t.Error("the library's refusal was swallowed")
	}
}

func TestLibraryState_CarriesTheTVFlag(t *testing.T) {
	lib := media.NewLibrary("/m")
	lib.HiddenFromTVs = true
	got := toLibraryStateDTO(app.LibraryState{Library: lib})
	if !got.HiddenFromTVs {
		t.Error("the Settings screen cannot show that the library is kept off TVs")
	}
}

func TestShutdown_RunsTheClosersAfterTheServerStops(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	var order []string
	a.closers = []func(){func() { order = append(order, fmt.Sprintf("closed after %d stops", st.stops)) }}
	a.shutdown(context.Background())
	if len(order) != 1 || order[0] != "closed after 1 stops" {
		t.Errorf("order = %v, want the closer to run once the server has stopped", order)
	}
}
