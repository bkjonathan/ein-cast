package streaming

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/server"
)

// fakeRun is a controllable app.RunningServer.
type fakeRun struct {
	addr    string
	stopErr error
	done    chan struct{}
	err     error

	mu       sync.Mutex
	stopped  int
	stopCtx  context.Context
	closeOne sync.Once
}

func newFakeRun() *fakeRun { return &fakeRun{addr: "127.0.0.1:8765", done: make(chan struct{})} }

func (f *fakeRun) Addr() string          { return f.addr }
func (f *fakeRun) Done() <-chan struct{} { return f.done }
func (f *fakeRun) Err() error            { return f.err }
func (f *fakeRun) Stop(ctx context.Context) error {
	f.mu.Lock()
	f.stopped++
	f.stopCtx = ctx
	f.mu.Unlock()
	f.end(nil)
	return f.stopErr
}

// end simulates the server finishing (idempotent).
func (f *fakeRun) end(err error) {
	f.closeOne.Do(func() { f.err = err; close(f.done) })
}

type fakeWeb struct {
	mu      sync.Mutex
	runs    []*fakeRun
	startFn func() (*fakeRun, error)
	gotCfg  app.ServeConfig
	gotCtx  context.Context
}

func (f *fakeWeb) Start(ctx context.Context, cfg app.ServeConfig) (app.RunningServer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gotCfg, f.gotCtx = cfg, ctx
	run, err := f.startFn()
	if err != nil {
		return nil, err
	}
	f.runs = append(f.runs, run)
	return run, nil
}

func (f *fakeWeb) last() *fakeRun {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs[len(f.runs)-1]
}

type fakeEvents struct {
	mu  sync.Mutex
	got []app.ServerStatus
}

func (f *fakeEvents) Publish(ev app.Event) {
	if e, ok := ev.(app.ServerStateChanged); ok {
		f.mu.Lock()
		f.got = append(f.got, e.Status)
		f.mu.Unlock()
	}
}

func (f *fakeEvents) states() []server.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]server.State, len(f.got))
	for i, s := range f.got {
		out[i] = s.State
	}
	return out
}

func newService(web *fakeWeb) (*Service, *fakeEvents) {
	ev := &fakeEvents{}
	return NewService(web, ev, slog.New(slog.DiscardHandler)), ev
}

func okWeb() *fakeWeb {
	return &fakeWeb{startFn: func() (*fakeRun, error) { return newFakeRun(), nil }}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func equalStates(a, b []server.State) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStart_Success(t *testing.T) {
	web := okWeb()
	svc, ev := newService(web)
	ctx := context.WithValue(context.Background(), struct{}{}, "marker")

	if err := svc.Start(ctx, NewConfig(WithPort(9000), WithName("Den"))); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if web.gotCfg != (app.ServeConfig{Port: 9000, Name: "Den"}) || web.gotCtx != ctx {
		t.Errorf("web got cfg=%+v ctx=%v", web.gotCfg, web.gotCtx)
	}
	st := svc.Status()
	if st.State != server.Running || st.Addr != "127.0.0.1:8765" || st.Name != "Den" || st.Error != "" {
		t.Errorf("Status() = %+v", st)
	}
	if want := []server.State{server.Starting, server.Running}; !equalStates(ev.states(), want) {
		t.Errorf("events = %v, want %v", ev.states(), want)
	}
}

func TestStart_BindFailureFailsAndCanRetry(t *testing.T) {
	boom := errors.New("address already in use")
	web := &fakeWeb{}
	web.startFn = func() (*fakeRun, error) { return nil, boom }
	svc, ev := newService(web)

	err := svc.Start(context.Background(), NewConfig())
	if !errors.Is(err, boom) {
		t.Fatalf("Start() error = %v, want it to wrap %v", err, boom)
	}
	st := svc.Status()
	if st.State != server.Failed || st.Error != boom.Error() || st.Addr != "" {
		t.Errorf("Status() = %+v, want failed with reason and no addr", st)
	}

	web.startFn = func() (*fakeRun, error) { return newFakeRun(), nil }
	if err := svc.Start(context.Background(), NewConfig()); err != nil {
		t.Fatalf("retry Start() error = %v", err)
	}
	if st := svc.Status(); st.State != server.Running || st.Error != "" {
		t.Errorf("Status() after retry = %+v, want running and no error", st)
	}
	want := []server.State{server.Starting, server.Failed, server.Starting, server.Running}
	if !equalStates(ev.states(), want) {
		t.Errorf("events = %v, want %v", ev.states(), want)
	}
}

func TestStart_InvalidConfigDoesNotTouchState(t *testing.T) {
	web := okWeb()
	svc, ev := newService(web)

	if err := svc.Start(context.Background(), NewConfig(WithPort(70000))); err == nil {
		t.Fatal("Start() error = nil for invalid port")
	}
	if svc.Status().State != server.Idle || len(ev.states()) != 0 || len(web.runs) != 0 {
		t.Errorf("state=%s events=%v runs=%d, want untouched", svc.Status().State, ev.states(), len(web.runs))
	}
}

func TestStart_WhileRunningIsRejected(t *testing.T) {
	web := okWeb()
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())

	err := svc.Start(context.Background(), NewConfig())
	if !errors.Is(err, server.ErrInvalidTransition) {
		t.Fatalf("second Start() error = %v, want ErrInvalidTransition", err)
	}
	if len(web.runs) != 1 {
		t.Errorf("web.Start called %d times, want 1", len(web.runs))
	}
}

func TestStop_Success(t *testing.T) {
	web := okWeb()
	svc, ev := newService(web)
	_ = svc.Start(context.Background(), NewConfig(WithName("Den")))

	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	run := web.last()
	if run.stopped != 1 {
		t.Errorf("run.Stop called %d times, want 1", run.stopped)
	}
	if dl, ok := run.stopCtx.Deadline(); !ok || time.Until(dl) > stopTimeout {
		t.Errorf("stop context deadline = %v, %v; want one within %s", dl, ok, stopTimeout)
	}
	st := svc.Status()
	if st.State != server.Idle || st.Addr != "" || st.Name != "Den" {
		t.Errorf("Status() = %+v, want idle with no addr (name remembered)", st)
	}
	want := []server.State{server.Starting, server.Running, server.Stopping, server.Idle}
	if !equalStates(ev.states(), want) {
		t.Errorf("events = %v, want %v", ev.states(), want)
	}
}

func TestStop_WhenIdleIsRejected(t *testing.T) {
	svc, ev := newService(okWeb())
	if err := svc.Stop(context.Background()); !errors.Is(err, server.ErrInvalidTransition) {
		t.Fatalf("Stop() error = %v, want ErrInvalidTransition", err)
	}
	if len(ev.states()) != 0 {
		t.Errorf("events = %v, want none", ev.states())
	}
}

func TestStop_FailureMovesToFailed(t *testing.T) {
	boom := errors.New("close failed")
	web := okWeb()
	web.startFn = func() (*fakeRun, error) {
		r := newFakeRun()
		r.stopErr = boom
		return r, nil
	}
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())

	if err := svc.Stop(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Stop() error = %v, want it to wrap %v", err, boom)
	}
	if st := svc.Status(); st.State != server.Failed || st.Error != boom.Error() {
		t.Errorf("Status() = %+v", st)
	}
}

func TestStop_FromFailedAcknowledgesTheFailure(t *testing.T) {
	web := &fakeWeb{startFn: func() (*fakeRun, error) { return nil, errors.New("nope") }}
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())

	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() from failed error = %v", err)
	}
	if st := svc.Status(); st.State != server.Idle || st.Error != "" {
		t.Errorf("Status() = %+v, want idle with the error cleared", st)
	}
}

func TestCrash_RunningServerThatDiesBecomesFailed(t *testing.T) {
	web := okWeb()
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())

	crash := errors.New("accept: too many open files")
	web.last().end(crash)

	eventually(t, func() bool { return svc.Status().State == server.Failed })
	if st := svc.Status(); st.Error != crash.Error() || st.Addr != "" {
		t.Errorf("Status() = %+v, want the crash reason and no addr", st)
	}
}

func TestCrash_CleanExitWithoutStopStillCountsAsFailure(t *testing.T) {
	web := okWeb()
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())

	web.last().end(nil) // e.g. parent context cancelled behind our back

	eventually(t, func() bool { return svc.Status().State == server.Failed })
	if svc.Status().Error == "" {
		t.Error("Failed status has no reason")
	}
}

// The watcher of a stopped run must not flip a later state to Failed.
func TestStop_LateWatcherDoesNothing(t *testing.T) {
	web := okWeb()
	svc, _ := newService(web)
	_ = svc.Start(context.Background(), NewConfig())
	_ = svc.Stop(context.Background())
	_ = svc.Start(context.Background(), NewConfig()) // a second, healthy run

	web.runs[0].end(errors.New("late noise")) // already ended; Do makes this a no-op
	time.Sleep(50 * time.Millisecond)

	if got := svc.Status().State; got != server.Running {
		t.Fatalf("state = %s, want running (old watcher must not interfere)", got)
	}
}

// Racing Start calls: exactly one run may be created. Run with -race.
func TestConcurrentStartsCreateOneRun(t *testing.T) {
	web := okWeb()
	svc, _ := newService(web)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Start(context.Background(), NewConfig())
		}()
	}
	wg.Wait()

	web.mu.Lock()
	defer web.mu.Unlock()
	if len(web.runs) != 1 {
		t.Fatalf("%d runs created, want 1", len(web.runs))
	}
}
