package relaunch

import (
	"io"
	"log/slog"
	"os"
	"reflect"
	"testing"
	"time"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCommand(t *testing.T) {
	const bundleExe = "/Applications/EinCast.app/Contents/MacOS/eincast"
	tests := []struct {
		name, goos, exe string
		wantName        string
		wantArgs        []string
	}{
		{"mac bundle opens through the system", "darwin", bundleExe,
			"/usr/bin/open", []string{"-n", "/Applications/EinCast.app", "--args", "--relaunch-after=42"}},
		{"mac build from source runs the program", "darwin", "/tmp/build/eincast",
			"/tmp/build/eincast", []string{"--relaunch-after=42"}},
		{"windows runs the program", "windows", `C:\Program Files\EinCast\eincast.exe`,
			`C:\Program Files\EinCast\eincast.exe`, []string{"--relaunch-after=42"}},
		{"a path that only looks like a bundle is not one off the Mac", "windows", bundleExe,
			bundleExe, []string{"--relaunch-after=42"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := command(tt.goos, tt.exe, 42)
			if name != tt.wantName || !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("command() = %q %q, want %q %q", name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestParseAfter(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantPID int
		wantOK  bool
	}{
		{"normal launch", nil, 0, false},
		{"other flags", []string{"--background"}, 0, false},
		{"relaunched", []string{"--relaunch-after=1234"}, 1234, true},
		{"among others", []string{"--background", "--relaunch-after=7"}, 7, true},
		{"not a number", []string{"--relaunch-after=abc"}, 0, false},
		{"zero is no process", []string{"--relaunch-after=0"}, 0, false},
		{"negative is no process", []string{"--relaunch-after=-5"}, 0, false},
		{"no value", []string{"--relaunch-after"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, ok := parseAfter(tt.args)
			if pid != tt.wantPID || ok != tt.wantOK {
				t.Errorf("parseAfter(%q) = %d, %v; want %d, %v", tt.args, pid, ok, tt.wantPID, tt.wantOK)
			}
		})
	}
}

func TestWaitForExit_ReturnsWhenTheProcessIsGone(t *testing.T) {
	probes := 0
	running := func(int) bool { probes++; return probes < 4 }

	waitForExit(1, time.Second, time.Millisecond, running, quiet())

	if probes != 4 {
		t.Errorf("probed %d times, want 4 (three running, then gone)", probes)
	}
}

func TestWaitForExit_GivesUpAtTheLimit(t *testing.T) {
	done := make(chan struct{})
	go func() {
		waitForExit(1, 30*time.Millisecond, time.Millisecond, func(int) bool { return true }, quiet())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitForExit did not return after its limit")
	}
}

func TestAlive_SeesThisProcessAndNotAnImpossibleOne(t *testing.T) {
	if !alive(os.Getpid()) {
		t.Error("alive(own pid) = false")
	}
	if alive(1<<22 + 12345) { // above the pid range of every system we run on
		t.Error("alive(impossible pid) = true")
	}
}

func TestWaitIfRelaunched_NormalLaunchDoesNotWait(t *testing.T) {
	start := time.Now()
	WaitIfRelaunched([]string{"--background"}, quiet())
	if time.Since(start) > 500*time.Millisecond {
		t.Error("a normal launch waited")
	}
}
