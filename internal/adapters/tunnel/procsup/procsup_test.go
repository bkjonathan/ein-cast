package procsup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain lets the test binary double as the supervised process: when
// PROCSUP_HELPER is set it behaves as a small, scriptable service instead of
// running tests. Real processes exercise real pipes and signals.
func TestMain(m *testing.M) {
	if os.Getenv("PROCSUP_HELPER") != "" {
		runHelper()
		return
	}
	os.Exit(m.Run())
}

// runHelper behaves as the environment says:
//
//	H_PIDFILE     write our pid here
//	H_IGNORE_INT  ignore interrupts (so only a kill ends us)
//	H_COUNTER     a file counting runs; with H_FAIL_FIRST=n, the first n runs exit 1 at once
//	H_STDOUT      text for stdout, H_STDERR for stderr (written exactly as given)
//	H_RUN_MS      then run this long
//	H_EXIT        then exit with this code; without it, hang until killed
func runHelper() {
	if p := os.Getenv("H_PIDFILE"); p != "" {
		_ = os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o644)
	}
	if os.Getenv("H_IGNORE_INT") != "" {
		signal.Ignore(os.Interrupt)
	}
	if c := os.Getenv("H_COUNTER"); c != "" {
		b, _ := os.ReadFile(c)
		n, _ := strconv.Atoi(string(b))
		_ = os.WriteFile(c, []byte(strconv.Itoa(n+1)), 0o644)
		if failFirst, _ := strconv.Atoi(os.Getenv("H_FAIL_FIRST")); n < failFirst {
			os.Exit(1)
		}
	}
	fmt.Fprint(os.Stdout, os.Getenv("H_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("H_STDERR"))
	if ms, err := strconv.Atoi(os.Getenv("H_RUN_MS")); err == nil {
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
	if code, ok := os.LookupEnv("H_EXIT"); ok {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	// Not `select {}`: with no other goroutine the runtime aborts a program that
	// blocks forever ("all goroutines are asleep"), which would look like a crash.
	time.Sleep(time.Hour)
}

// recorder collects what the callbacks report.
type recorder struct {
	mu     sync.Mutex
	lines  []string
	starts []time.Time
	exits  []exit
}

type exit struct {
	err        error
	restarting bool
}

func (r *recorder) onLine(s Stream, l string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, s.String()+": "+l)
}

func (r *recorder) onStart() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, time.Now())
}

func (r *recorder) onExit(err error, restarting bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exits = append(r.exits, exit{err, restarting})
}

func (r *recorder) snapshot() (lines []string, starts []time.Time, exits []exit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...), append([]time.Time(nil), r.starts...), append([]exit(nil), r.exits...)
}

// fast makes backoffs short so restart tests take milliseconds.
func fast(s *Spec) {
	s.InitialBackoff = 5 * time.Millisecond
	s.MaxBackoff = 20 * time.Millisecond
	s.GracePeriod = 2 * time.Second
}

func helper(t *testing.T, env []string, mod ...func(*Spec)) (*Supervisor, *recorder) {
	t.Helper()
	rec := &recorder{}
	spec := Spec{
		Name:    "helper",
		Path:    os.Args[0],
		Env:     append([]string{"PROCSUP_HELPER=1"}, env...),
		OnStart: rec.onStart, OnLine: rec.onLine, OnExit: rec.onExit,
	}
	fast(&spec)
	for _, m := range mod {
		m(&spec)
	}
	sup := New(spec, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sup.Stop(ctx)
	})
	return sup, rec
}

func waitDone(t *testing.T, s *Supervisor) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("supervisor did not finish")
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func noRestart(s *Spec) { s.MaxRestarts = -1 }

func TestOutputArrivesAsLinesFromBothStreams(t *testing.T) {
	sup, rec := helper(t, []string{
		"H_STDOUT=one\r\ntwo\n\n\nlast without newline",
		"H_STDERR=err1\nerr2 partial",
		"H_EXIT=0",
	}, noRestart)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	lines, _, _ := rec.snapshot()
	want := map[string]bool{
		"stdout: one": true, "stdout: two": true, "stdout: last without newline": true,
		"stderr: err1": true, "stderr: err2 partial": true,
	}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want exactly %d (blank lines dropped, \\r removed, final partial line kept)", lines, len(want))
	}
	for _, l := range lines {
		if !want[l] {
			t.Errorf("unexpected line %q", l)
		}
	}
}

func TestEndlessLinesAreChunked(t *testing.T) {
	sup, rec := helper(t, []string{"H_STDOUT=" + strings.Repeat("x", 40_000) + "\n", "H_EXIT=0"}, noRestart)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	lines, _, _ := rec.snapshot()
	var sizes []int
	for _, l := range lines {
		sizes = append(sizes, len(strings.TrimPrefix(l, "stdout: ")))
	}
	if fmt.Sprint(sizes) != fmt.Sprint([]int{maxLine, maxLine, 40_000 - 2*maxLine}) {
		t.Errorf("chunk sizes = %v, want [%d %d %d]", sizes, maxLine, maxLine, 40_000-2*maxLine)
	}
}

func TestLineWriter_SplitsAcrossWrites(t *testing.T) {
	var got []string
	w := newLineWriter(func(l string) { got = append(got, l) })
	for _, chunk := range []string{"ab", "c\nde", "f\ng", "h", "\n", "tail"} {
		if n, err := w.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	w.flush()
	if fmt.Sprint(got) != "[abc def gh tail]" {
		t.Errorf("got %q", got)
	}
}

func TestStopEndsTheProcess(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	sup, rec := helper(t, []string{"H_PIDFILE=" + pidfile})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child to start", func() bool { b, _ := os.ReadFile(pidfile); return len(b) > 0 })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sup.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case <-sup.Done():
	default:
		t.Fatal("Stop returned before the supervisor finished")
	}
	if err := sup.Err(); err != nil {
		t.Errorf("Err = %v, want nil for a requested stop", err)
	}
	assertGone(t, pidfile)
	if _, _, exits := rec.snapshot(); len(exits) != 1 || exits[0].err != nil || exits[0].restarting {
		t.Errorf("exits = %+v, want one clean, final exit", exits)
	}
}

func TestCancellingTheContextEndsTheProcess(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	sup, _ := helper(t, []string{"H_PIDFILE=" + pidfile})
	ctx, cancel := context.WithCancel(context.Background())
	if err := sup.Start(ctx); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child to start", func() bool { b, _ := os.ReadFile(pidfile); return len(b) > 0 })

	cancel()

	waitDone(t, sup)
	assertGone(t, pidfile)
}

// assertGone checks that the process whose pid is in pidfile no longer exists.
func assertGone(t *testing.T, pidfile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // no signal 0 to probe with
	}
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		t.Errorf("process %d is still running", pid)
	}
}

func TestAProcessThatIgnoresTheInterruptIsKilled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows kills outright")
	}
	pidfile := filepath.Join(t.TempDir(), "pid")
	sup, _ := helper(t, []string{"H_PIDFILE=" + pidfile, "H_IGNORE_INT=1"}, func(s *Spec) {
		s.GracePeriod = 150 * time.Millisecond
	})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child to start", func() bool { b, _ := os.ReadFile(pidfile); return len(b) > 0 })
	time.Sleep(50 * time.Millisecond) // let it install the ignore

	begin := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sup.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	if took := time.Since(begin); took < 100*time.Millisecond {
		t.Errorf("stopped in %v: it should have waited out the grace period", took)
	}
	assertGone(t, pidfile)
}

func TestACrashingProcessIsRestartedWithBackoffThenGivenUpOn(t *testing.T) {
	sup, rec := helper(t, []string{"H_EXIT=1"}, func(s *Spec) {
		s.MaxRestarts = 3
		s.InitialBackoff = 20 * time.Millisecond
		s.MaxBackoff = time.Second
	})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	_, starts, exits := rec.snapshot()
	if len(starts) != 4 || len(exits) != 4 {
		t.Fatalf("starts=%d exits=%d, want the first run plus 3 restarts", len(starts), len(exits))
	}
	for i, e := range exits {
		if e.err == nil || e.restarting != (i < 3) {
			t.Errorf("exit %d = %+v, want a failure with restarting=%v", i, e, i < 3)
		}
	}
	// Waits are 20, 40 and 80 ms. Only lower bounds: a loaded machine runs late, never early.
	for i, min := range []time.Duration{20, 40, 80} {
		if gap := starts[i+1].Sub(starts[i]); gap < min*time.Millisecond {
			t.Errorf("restart %d came after %v, want at least %v (the backoff doubles)", i+1, gap, min*time.Millisecond)
		}
	}
	err := sup.Err()
	if err == nil || !strings.Contains(err.Error(), "giving up") || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("Err = %v, want it to say it gave up and why", err)
	}
}

func TestBackoffIsCapped(t *testing.T) {
	sup, rec := helper(t, []string{"H_EXIT=1"}, func(s *Spec) {
		s.MaxRestarts = 5
		s.InitialBackoff = 10 * time.Millisecond
		s.MaxBackoff = 25 * time.Millisecond
	})
	begin := time.Now()
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	_, starts, _ := rec.snapshot()
	// Waits: 10, 20, 25, 25, 25 = 105 ms uncapped would be 10+20+40+80+160 = 310 ms.
	if total := starts[len(starts)-1].Sub(begin); total > 300*time.Millisecond {
		t.Errorf("five restarts took %v: the 25ms cap was not applied", total)
	}
}

func TestAProcessThatRanForAWhileGetsAFreshStart(t *testing.T) {
	// Each run lasts longer than StableAfter, so it counts as healthy and the
	// failure count restarts: MaxRestarts=1 would otherwise stop after 2 runs.
	sup, rec := helper(t, []string{"H_RUN_MS=60", "H_EXIT=1"}, func(s *Spec) {
		s.MaxRestarts = 1
		s.StableAfter = 30 * time.Millisecond
	})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "five starts", func() bool { _, starts, _ := rec.snapshot(); return len(starts) >= 5 })

	select {
	case <-sup.Done():
		t.Fatal("gave up although every run was healthy")
	default:
	}
}

func TestRestartsCanBeSwitchedOff(t *testing.T) {
	sup, rec := helper(t, []string{"H_EXIT=2"}, noRestart)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	_, starts, exits := rec.snapshot()
	if len(starts) != 1 || len(exits) != 1 || exits[0].restarting {
		t.Errorf("starts=%d exits=%+v, want one run and no restart", len(starts), exits)
	}
	if sup.Err() == nil {
		t.Error("a process that died with nothing to restart it is a failure")
	}
}

func TestRestartedProcessRunsAgainAndCanBeStopped(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	sup, rec := helper(t, []string{"H_COUNTER=" + counter, "H_FAIL_FIRST=2", "H_STDOUT=up\n"})
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the third run to come up", func() bool {
		lines, _, _ := rec.snapshot()
		return len(lines) == 1
	})
	select {
	case <-sup.Done():
		t.Fatal("finished although the third run is healthy")
	default:
	}
	_, starts, exits := rec.snapshot()
	if len(starts) != 3 || len(exits) != 2 {
		t.Errorf("starts=%d exits=%d, want 3 and 2", len(starts), len(exits))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sup.Stop(ctx); err != nil || sup.Err() != nil {
		t.Errorf("Stop = %v, Err = %v", err, sup.Err())
	}
}

func TestStartFailureIsReportedAtOnce(t *testing.T) {
	var started bool
	sup := New(Spec{
		Name: "ghost", Path: filepath.Join(t.TempDir(), "does-not-exist"),
		OnStart: func() { started = true },
	}, slog.New(slog.DiscardHandler))

	err := sup.Start(context.Background())

	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error = %v, want one naming the process", err)
	}
	if started {
		t.Error("OnStart ran for a process that never started")
	}
	select {
	case <-sup.Done():
	default:
		t.Error("Done is open for a supervisor that never ran: waiting on it would hang")
	}
	if err := sup.Stop(context.Background()); err != nil {
		t.Errorf("Stop after a failed Start: %v", err)
	}
}

func TestAFailedRestartCountsAsAFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	script := filepath.Join(t.TempDir(), "svc")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var starts int
	var mu sync.Mutex
	sup := New(Spec{
		Name: "svc", Path: script, MaxRestarts: 2,
		InitialBackoff: 5 * time.Millisecond, MaxBackoff: 10 * time.Millisecond,
		OnStart: func() { mu.Lock(); starts++; mu.Unlock() },
		// Remove the program after its first exit, so the restart cannot spawn.
		OnExit: func(error, bool) { _ = os.Remove(script) },
	}, slog.New(slog.DiscardHandler))
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	mu.Lock()
	defer mu.Unlock()
	if starts != 1 {
		t.Errorf("starts = %d, want 1: the restarts could not spawn", starts)
	}
	if err := sup.Err(); err == nil || !strings.Contains(err.Error(), "giving up") {
		t.Errorf("Err = %v, want it to give up", err)
	}
}

func TestStartTwiceIsRefused(t *testing.T) {
	sup, rec := helper(t, nil)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(context.Background()); err == nil {
		t.Error("second Start succeeded")
	}
	eventually(t, "the first start", func() bool { _, s, _ := rec.snapshot(); return len(s) == 1 })
}

func TestStopIsIdempotentAndSafeBeforeStart(t *testing.T) {
	sup, _ := helper(t, nil)
	ctx := context.Background()
	if err := sup.Stop(ctx); err != nil {
		t.Fatalf("before Start: %v", err)
	}
	if err := sup.Start(ctx); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := sup.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStopGivesUpWaitingWhenItsOwnContextExpires(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows kills outright")
	}
	sup, _ := helper(t, []string{"H_IGNORE_INT=1"}, func(s *Spec) { s.GracePeriod = time.Second })
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := sup.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop = %v, want the caller's deadline", err)
	}
	// Cleanup's Stop then waits the grace period out and the process is killed.
}

func TestLogsCarryLevelsAndNeverTheArgumentsOrEnvironment(t *testing.T) {
	var buf syncBuf
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sup := New(Spec{
		Name: "svc", Path: os.Args[0],
		Args: []string{"-token=SECRET-ARG"},
		Env:  []string{"PROCSUP_HELPER=1", "TUNNEL_TOKEN=SECRET-ENV", "H_STDERR=WRN careful\nERR broken\nplain\n", "H_EXIT=0"},
		Level: func(_ Stream, line string) slog.Level {
			switch {
			case strings.HasPrefix(line, "ERR"):
				return slog.LevelError
			case strings.HasPrefix(line, "WRN"):
				return slog.LevelWarn
			}
			return slog.LevelInfo
		},
		MaxRestarts: -1,
	}, log)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	out := buf.String()
	for _, want := range []string{
		`level=WARN msg="WRN careful" process=svc stream=stderr`,
		`level=ERROR msg="ERR broken" process=svc stream=stderr`,
		`level=INFO msg=plain process=svc stream=stderr`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"SECRET-ARG", "SECRET-ENV"} {
		if strings.Contains(out, secret) {
			t.Errorf("the log contains %q:\n%s", secret, out)
		}
	}
}

func TestSecretsAreScrubbedFromOutputBeforeAnyoneSeesIt(t *testing.T) {
	const secret = "eyJhIjoiU0VDUkVUIn0"
	var buf syncBuf
	log := slog.New(slog.NewTextHandler(&buf, nil))
	rec := &recorder{}
	sup := New(Spec{
		Name: "svc", Path: os.Args[0], MaxRestarts: -1,
		Env:     []string{"PROCSUP_HELPER=1", "H_STDERR=using token " + secret + " to connect\nplain\nagain " + secret + secret + "\n", "H_EXIT=0"},
		Secrets: []string{secret, ""}, // an empty secret must not blank every line
		OnLine:  rec.onLine,
	}, log)
	if err := sup.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitDone(t, sup)

	lines, _, _ := rec.snapshot()
	want := []string{"stderr: using token [redacted] to connect", "stderr: plain", "stderr: again [redacted][redacted]"}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("OnLine got %q, want %q", lines, want)
	}
	if strings.Contains(buf.String(), secret) {
		t.Errorf("the log contains the secret:\n%s", buf.String())
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestSpecDefaults(t *testing.T) {
	got := Spec{}.withDefaults()
	if got.MaxRestarts != 5 || got.InitialBackoff != time.Second || got.MaxBackoff != 30*time.Second ||
		got.StableAfter != 30*time.Second || got.GracePeriod != 5*time.Second {
		t.Errorf("defaults = %+v", got)
	}
	if got := (Spec{MaxRestarts: -1}).withDefaults(); got.MaxRestarts != -1 {
		t.Errorf("MaxRestarts = %d: a negative value must survive to mean 'never restart'", got.MaxRestarts)
	}
}
