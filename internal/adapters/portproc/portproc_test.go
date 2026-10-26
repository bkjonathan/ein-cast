package portproc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"eincast/internal/app"
)

func TestParseLsof(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []app.PortHolder
	}{
		{"nothing", "", nil},
		{"one process", "p4321\ncnode\n", []app.PortHolder{{PID: 4321, Name: "node"}}},
		{
			"several, with a name containing spaces",
			"p10\ncGoogle Chrome Helper\np20\ncpython3\n",
			[]app.PortHolder{{PID: 10, Name: "Google Chrome Helper"}, {PID: 20, Name: "python3"}},
		},
		{"the same process twice", "p10\ncnode\np10\ncnode\n", []app.PortHolder{{PID: 10, Name: "node"}}},
		{"a bad pid does not lend its name to the previous process", "p10\ncnode\npxyz\ncother\n", []app.PortHolder{{PID: 10, Name: "node"}}},
		{"no name", "p10\n", []app.PortHolder{{PID: 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLsof([]byte(tt.in)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseLsof = %+v, want %+v", got, tt.want)
			}
		})
	}
}

const netstatSample = `
Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       1092
  TCP    0.0.0.0:8765           0.0.0.0:0              LISTENING       4321
  TCP    [::]:8765              [::]:0                 LISTENING       4321
  TCP    192.168.1.5:8765       192.168.1.9:51000      ESTABLISHED     4321
  TCP    0.0.0.0:18765          0.0.0.0:0              LISTENING       777
  TCP    127.0.0.1:8765         0.0.0.0:0              ABHÖREN         55
  UDP    0.0.0.0:8765           *:*                                    999
`

func TestParseNetstat(t *testing.T) {
	got := parseNetstat([]byte(netstatSample), 8765)
	want := []app.PortHolder{{PID: 4321}, {PID: 55}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNetstat = %+v, want %+v\n(a longer port number, a connected client, UDP and the word LISTENING in another language must all be handled)", got, want)
	}
	if got := parseNetstat([]byte(netstatSample), 9999); len(got) != 0 {
		t.Fatalf("a free port reported %+v", got)
	}
}

func TestParseTasklist(t *testing.T) {
	tests := map[string]string{
		`"node.exe","4321","Console","1","50,000 K"` + "\r\n":                "node.exe",
		"INFO: No tasks are running which match the specified criteria.\r\n": "",
		"": "",
	}
	for in, want := range tests {
		if got := parseTasklist([]byte(in)); got != want {
			t.Errorf("parseTasklist(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHoldersRunsTheRightTool(t *testing.T) {
	var gotName string
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte("p10\ncnode\n"), nil
	}
	got, err := (&Inspector{goos: "darwin", run: run}).Holders(context.Background(), 8765)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "lsof" || !strings.Contains(strings.Join(gotArgs, " "), "-iTCP:8765") {
		t.Fatalf("ran %s %v", gotName, gotArgs)
	}
	if len(got) != 1 || got[0].PID != 10 {
		t.Fatalf("Holders = %+v", got)
	}
}

func TestHoldersOnWindowsNamesTheProcess(t *testing.T) {
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "netstat" {
			return []byte(netstatSample), nil
		}
		return []byte(`"node.exe","4321","Console","1","50,000 K"`), nil
	}
	got, err := (&Inspector{goos: "windows", run: run}).Holders(context.Background(), 8765)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "node.exe" {
		t.Fatalf("Holders = %+v", got)
	}
}

func TestHoldersTreatsLsofExit1AsFree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	// A real non-zero exit with no output, as lsof does when nothing matches.
	cmd := exec.Command("sh", "-c", "exit 1")
	exitErr := cmd.Run()
	if exitErr == nil {
		t.Skip("could not produce an exit status 1")
	}
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, exitErr }
	got, err := (&Inspector{goos: "darwin", run: run}).Holders(context.Background(), 8765)
	if err != nil || len(got) != 0 {
		t.Fatalf("Holders = %+v, %v; want none and no error", got, err)
	}
}

func TestHoldersReportsAMissingTool(t *testing.T) {
	boom := errors.New("executable file not found")
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, boom }
	if _, err := (&Inspector{goos: "darwin", run: run}).Holders(context.Background(), 8765); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want it to wrap %v", err, boom)
	}
}

func TestSignalRefusesSystemProcesses(t *testing.T) {
	for _, pid := range []int{-5, 0, 1} {
		if err := (&Inspector{goos: "darwin"}).Terminate(pid); err == nil {
			t.Errorf("Terminate(%d) succeeded", pid)
		}
	}
}

// A real process, so the signalling code is exercised against the real system.
func TestTerminateAndKillEndARealProcess(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil || runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sleep command")
	}
	for _, kill := range []bool{false, true} {
		cmd := exec.Command("sleep", "60")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { _ = cmd.Wait(); close(exited) }()

		in := New()
		var err error
		if kill {
			err = in.Kill(cmd.Process.Pid)
		} else {
			err = in.Terminate(cmd.Process.Pid)
		}
		if err != nil {
			t.Fatalf("kill=%v: %v", kill, err)
		}
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("kill=%v: process still running", kill)
		}
		// Ending what is already gone is not an error.
		if err := in.Kill(cmd.Process.Pid); err != nil {
			t.Errorf("ending an already ended process: %v", err)
		}
	}
}

// Finds a real listener on this machine: the integration check for the tool the
// production code depends on.
func TestHoldersFindsOurOwnListener(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof not installed")
	}
	ln, port := listen(t)
	defer ln.Close()

	got, err := New().Holders(context.Background(), port)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range got {
		found = found || h.PID == os.Getpid()
	}
	if !found {
		t.Fatalf("Holders(%s) = %+v, want it to include pid %s", strconv.Itoa(port), got, strconv.Itoa(os.Getpid()))
	}
}
