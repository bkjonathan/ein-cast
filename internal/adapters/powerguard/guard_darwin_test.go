package powerguard

import (
	"log/slog"
	"os/exec"
	"testing"
)

// alive reports whether the process behind cmd still runs.
func alive(cmd *exec.Cmd) bool {
	return cmd.ProcessState == nil
}

func TestHold_RunsCaffeinateUntilReleased(t *testing.T) {
	g := New(slog.New(slog.DiscardHandler))
	var started *exec.Cmd
	real := g.command
	g.command = func(pid int) *exec.Cmd { started = real(pid); return started }

	release, err := g.Hold("test")
	if err != nil {
		t.Skipf("caffeinate is not usable here: %v", err)
	}
	if started == nil || !alive(started) {
		t.Fatal("caffeinate is not running after Hold")
	}

	release()
	release() // releasing twice must be harmless
	if alive(started) {
		t.Error("caffeinate still running after release: the Mac could never sleep again")
	}
}

func TestHold_ReportsAProgramThatCannotStart(t *testing.T) {
	g := New(slog.New(slog.DiscardHandler))
	g.command = func(int) *exec.Cmd { return exec.Command("/nonexistent/caffeinate") }
	if _, err := g.Hold("test"); err == nil {
		t.Fatal("Hold() succeeded with no program to run")
	}
}

func TestHold_TiesCaffeinateToOurProcess(t *testing.T) {
	// -w makes caffeinate leave when we do; losing the flag would leave the Mac
	// awake after a crash, so the arguments are worth pinning.
	cmd := New(slog.New(slog.DiscardHandler)).command(4242)
	want := []string{"caffeinate", "-i", "-w", "4242"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Fatalf("args = %v, want %v", cmd.Args, want)
		}
	}
}
