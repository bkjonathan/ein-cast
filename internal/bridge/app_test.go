package bridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/streaming"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

type fakeLibrary struct {
	scanErr    error
	scanned    []string
	summary    app.LibrarySummary
	hasSummary bool
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
}

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
		Streaming: st,
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

func TestStartServer_UsesAppContextAndDefaults(t *testing.T) {
	a, st := newTestAppWithStreaming(&fakeLibrary{})
	a.ctx = context.WithValue(context.Background(), appCtxKey{}, "app")

	if err := a.StartServer(); err != nil {
		t.Fatalf("StartServer() error = %v", err)
	}
	if st.startCtx != a.ctx {
		t.Error("server must live as long as the app context, not a per-call one")
	}
	if st.startCfg != streaming.NewConfig() {
		t.Errorf("config = %+v, want the defaults", st.startCfg)
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

	want := ServerStateDTO{State: "failed", Name: "Den", Error: "port in use"}
	if got := a.GetServerState(); got != want {
		t.Fatalf("GetServerState() = %+v, want %+v", got, want)
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
