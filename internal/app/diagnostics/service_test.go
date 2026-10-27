package diagnostics

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
	"eincast/internal/domain/server"
)

type fakeSettings struct{ v app.SettingsView }

func (f fakeSettings) Get() app.SettingsView { return f.v }

type fakeServer struct{ s app.ServerStatus }

func (f fakeServer) Status() app.ServerStatus { return f.s }

type fakeRemote struct{ s app.RemoteStatus }

func (f fakeRemote) Status() app.RemoteStatus { return f.s }

type fakeLibs struct{ l []app.LibraryState }

func (f fakeLibs) State() []app.LibraryState { return f.l }

type fakeLogs struct{ e []app.LogEntry }

func (f fakeLogs) History() []app.LogEntry { return f.e }

type fakeFFmpeg string

func (f fakeFFmpeg) Path() string { return string(f) }

func newService(d Deps) *Service {
	s := NewService(Identity{Version: "1.1.1", OS: "darwin", Arch: "arm64", Home: "/Users/someone"}, d)
	s.now = func() time.Time { return time.Date(2026, 10, 27, 13, 0, 0, 0, time.UTC) }
	return s
}

func baseDeps() Deps {
	return Deps{
		Settings:  fakeSettings{app.SettingsView{Settings: app.Settings{Port: 8765, RequireToken: true, KeepAwake: true, CheckForUpdates: true}, Token: "TOPSECRETTOKEN"}},
		Server:    fakeServer{app.ServerStatus{State: server.Running, Port: 8765, LANURLs: []string{"http://192.168.1.20:8765"}, Discovery: []string{"mdns"}}},
		Remote:    fakeRemote{},
		Libraries: fakeLibs{},
		Logs:      fakeLogs{},
		FFmpeg:    fakeFFmpeg("/opt/homebrew/bin/ffmpeg"),
	}
}

func TestReport_SaysWhatIsRunningAndHowItIsSetUp(t *testing.T) {
	got := newService(baseDeps()).Report()
	for _, want := range []string{
		"Version:   1.1.1", "System:    darwin/arm64", "Created:   2026-10-27T13:00:00Z",
		"state:      running", "port:       8765", "discovery:  mdns", "1 on the local network",
		"access token:", "check for updates:", "ffmpeg:                     found",
		"Remote access\n  off", "Libraries (0)", "Log (last 0 lines)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

func TestReport_NeverCarriesTheTokenTheAddressesOrFolders(t *testing.T) {
	d := baseDeps()
	// NewLibrary cleans the path, which on Windows turns the slashes into
	// backslashes; the log lines below must carry the folder the way the app does.
	lib := media.NewLibrary("/Volumes/Family Movies/Kids")
	d.Libraries = fakeLibs{[]app.LibraryState{{Library: lib, Stats: media.Stats{Total: 3, Counts: map[media.Kind]int{media.KindVideo: 3}},
		Error: "cannot read " + lib.Root + ": permission denied"}}}
	d.Remote = fakeRemote{app.RemoteStatus{Provider: "ngrok", Public: true, State: server.Running, URL: "https://abc-123.ngrok.app", PINSet: true}}
	d.Server = fakeServer{app.ServerStatus{State: server.Failed, Error: "listen on /Users/someone/x: busy"}}
	d.Logs = fakeLogs{[]app.LogEntry{{
		Time: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), Level: slog.LevelInfo, Source: "library", Message: "scanned",
		Attrs: []app.LogAttr{{Key: "root", Value: lib.Root},{Key: "home", Value: "/Users/someone/Movies"}, {Key: "url", Value: "http://x/?t=abc123&y=1"}},
	}}}

	got := newService(d).Report()

	for _, secret := range []string{"TOPSECRETTOKEN", "192.168.1.20", "abc-123.ngrok.app", "Family Movies", "/Users/someone", "abc123"} {
		if strings.Contains(got, secret) {
			t.Errorf("report leaks %q:\n%s", secret, got)
		}
	}
	for _, want := range []string{"[library 1]", "~/Movies", "t=[hidden]", "provider:   ngrok (public)", "PIN set:    yes", "error:      listen on ~/x: busy"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

func TestReport_NestedLibraryFoldersAreHiddenWhole(t *testing.T) {
	d := baseDeps()
	outer, inner := media.NewLibrary("/Volumes/Media"), media.NewLibrary("/Volumes/Media/Private")
	d.Libraries = fakeLibs{[]app.LibraryState{{Library: outer}, {Library: inner}}}
	sep := string(filepath.Separator)
	d.Logs = fakeLogs{[]app.LogEntry{{Source: "library", Message: "scan " + inner.Root + sep + "secret.mkv"}}}

	got := newService(d).Report()

	if strings.Contains(got, "Private") || strings.Contains(got, "secret.mkv") && !strings.Contains(got, "[library 2]"+sep+"secret.mkv") {
		t.Errorf("the inner folder name leaked, probably replaced by the outer one first:\n%s", got)
	}
}

func TestReport_KeepsOnlyTheLatestLines(t *testing.T) {
	d := baseDeps()
	var entries []app.LogEntry
	for i := 0; i < LogLines+50; i++ {
		entries = append(entries, app.LogEntry{Source: "http", Message: fmt.Sprintf("line %03d", i)})
	}
	d.Logs = fakeLogs{entries}

	got := newService(d).Report()

	if !strings.Contains(got, fmt.Sprintf("Log (last %d lines)", LogLines)) {
		t.Error("the report does not say how many lines it carries")
	}
	if strings.Contains(got, "line 049") || !strings.Contains(got, "line 050") || !strings.Contains(got, fmt.Sprintf("line %03d", LogLines+49)) {
		t.Errorf("wrong slice of the log kept:\n%s", got)
	}
}

func TestReport_ChosenFFmpegIsMentionedButNotItsPath(t *testing.T) {
	d := baseDeps()
	d.Settings = fakeSettings{app.SettingsView{Settings: app.Settings{FFmpegPath: "/Users/someone/tools/ffmpeg"}}}
	d.FFmpeg = nil

	got := newService(d).Report()

	if !strings.Contains(got, "not found, location chosen by the person") || strings.Contains(got, "tools/ffmpeg") {
		t.Errorf("ffmpeg line wrong:\n%s", got)
	}
}
