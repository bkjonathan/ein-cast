// Package diagnostics builds the text a person pastes into a bug report: what
// version and system this is, how EinCast is set up, and the latest log lines.
// It is the answer to "what do I send you?" without any telemetry: nothing
// leaves the computer until the person pastes it somewhere (Phase 20.11).
package diagnostics

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// LogLines is how many of the latest log lines a report carries.
const LogLines = 200

// Settings, Server, Remote, Libraries, Logs and FFmpeg are the parts of the rest
// of the app this service reads. Declared here, at the consumer.
type (
	Settings  interface{ Get() app.SettingsView }
	Server    interface{ Status() app.ServerStatus }
	Remote    interface{ Status() app.RemoteStatus }
	Libraries interface{ State() []app.LibraryState }
	Logs      interface{ History() []app.LogEntry }
	// FFmpeg says where ffmpeg was found, or "" when it was not.
	FFmpeg interface{ Path() string }
)

// Identity says which build this is and where it runs.
type Identity struct {
	Version string
	OS      string // runtime.GOOS
	Arch    string // runtime.GOARCH
	// Home is the person's home folder. It is replaced by "~" wherever it appears,
	// because a report is often pasted into a public issue and the folder name is
	// usually the person's name.
	Home string
}

// Deps are the parts of the app a report is made from. FFmpeg may be nil.
type Deps struct {
	Settings  Settings
	Server    Server
	Remote    Remote
	Libraries Libraries
	Logs      Logs
	FFmpeg    FFmpeg
}

// Service builds reports.
type Service struct {
	id  Identity
	d   Deps
	now func() time.Time // a field so tests control the clock
}

// NewService returns a Service.
func NewService(id Identity, d Deps) *Service {
	return &Service{id: id, d: d, now: time.Now}
}

// Report returns the diagnostics text. It names no secret (the token, PINs and
// tunnel tokens are never read), no library folder and no file name the person
// chose; anything in the log that looks like one is replaced. It cannot promise
// to catch everything a log line might say, so the text begins by asking the
// person to read it first.
func (s *Service) Report() string {
	var b strings.Builder
	b.WriteString("EinCast diagnostics\n")
	b.WriteString("Read this before you post it: it is a copy of what EinCast knows, with folder names hidden.\n\n")

	fmt.Fprintf(&b, "Version:   %s\n", s.id.Version)
	fmt.Fprintf(&b, "System:    %s/%s\n", s.id.OS, s.id.Arch)
	fmt.Fprintf(&b, "Created:   %s\n\n", s.now().Format(time.RFC3339))

	libs := s.d.Libraries.State()
	redact := s.redactor(libs)

	s.writeServer(&b, redact)
	s.writeRemote(&b, redact)
	s.writeSettings(&b)
	s.writeLibraries(&b, libs, redact)
	s.writeLog(&b, redact)
	return b.String()
}

func (s *Service) writeServer(b *strings.Builder, redact func(string) string) {
	st := s.d.Server.Status()
	b.WriteString("Server\n")
	fmt.Fprintf(b, "  state:      %s\n", st.State)
	if st.Port != 0 {
		fmt.Fprintf(b, "  port:       %d\n", st.Port)
	}
	if len(st.Discovery) > 0 {
		fmt.Fprintf(b, "  discovery:  %s\n", strings.Join(st.Discovery, ", "))
	}
	fmt.Fprintf(b, "  addresses:  %d on the local network\n", len(st.LANURLs))
	if st.Error != "" {
		fmt.Fprintf(b, "  error:      %s\n", redact(st.Error))
	}
	b.WriteString("\n")
}

func (s *Service) writeRemote(b *strings.Builder, redact func(string) string) {
	r := s.d.Remote.Status()
	b.WriteString("Remote access\n")
	if r.Provider == "" {
		b.WriteString("  off\n\n")
		return
	}
	// The address itself is not written: a tunnel address is a way in to the library.
	fmt.Fprintf(b, "  provider:   %s (%s)\n", r.Provider, map[bool]string{true: "public", false: "private"}[r.Public])
	fmt.Fprintf(b, "  state:      %s\n", r.State)
	fmt.Fprintf(b, "  PIN set:    %s\n", yesNo(r.PINSet))
	if r.Error != "" {
		fmt.Fprintf(b, "  error:      %s\n", redact(r.Error))
	}
	b.WriteString("\n")
}

func (s *Service) writeSettings(b *strings.Builder) {
	v := s.d.Settings.Get()
	b.WriteString("Settings\n")
	rows := []struct {
		label string
		on    bool
	}{
		{"access token", v.RequireToken},
		{"smart TVs (DLNA)", v.DLNA},
		{"start sharing when opened", v.AutoStart},
		{"keep computer awake", v.KeepAwake},
		{"open at login", v.OpenAtLogin},
		{"keep sharing when closed", v.HideOnClose},
		{"hardware encoding", v.HardwareEncoding},
		{"check for updates", v.CheckForUpdates},
	}
	for _, r := range rows {
		fmt.Fprintf(b, "  %-27s %s\n", r.label+":", onOff(r.on))
	}
	ffmpeg := "not found"
	if s.d.FFmpeg != nil && s.d.FFmpeg.Path() != "" {
		ffmpeg = "found"
	}
	if v.FFmpegPath != "" {
		ffmpeg += ", location chosen by the person"
	}
	fmt.Fprintf(b, "  %-27s %s\n\n", "ffmpeg:", ffmpeg)
}

func (s *Service) writeLibraries(b *strings.Builder, libs []app.LibraryState, redact func(string) string) {
	fmt.Fprintf(b, "Libraries (%d)\n", len(libs))
	for i, l := range libs {
		fmt.Fprintf(b, "  library %d: %s", i+1, counts(l.Stats))
		if l.Watching {
			b.WriteString(", watched")
		}
		if l.Scanning {
			b.WriteString(", scanning")
		}
		if l.Library.HiddenFromTVs {
			b.WriteString(", hidden from TVs")
		}
		if l.Error != "" {
			fmt.Fprintf(b, ", last scan failed: %s", redact(l.Error))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func (s *Service) writeLog(b *strings.Builder, redact func(string) string) {
	hist := s.d.Logs.History()
	if len(hist) > LogLines {
		hist = hist[len(hist)-LogLines:]
	}
	fmt.Fprintf(b, "Log (last %d lines)\n", len(hist))
	for _, e := range hist {
		b.WriteString(redact(formatLine(e)))
		b.WriteString("\n")
	}
}

// formatLine renders one entry the way the Logs screen reads: time, level, source,
// message, then key=value pairs.
func formatLine(e app.LogEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %-5s [%s] %s", e.Time.Format("15:04:05"), levelName(e.Level), e.Source, e.Message)
	for _, a := range e.Attrs {
		fmt.Fprintf(&b, " %s=%s", a.Key, a.Value)
	}
	return b.String()
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

func counts(st media.Stats) string {
	if st.Total == 0 {
		return "no media found yet"
	}
	return fmt.Sprintf("%d files (%d video, %d audio, %d image)",
		st.Total, st.Counts[media.KindVideo], st.Counts[media.KindAudio], st.Counts[media.KindImage])
}

// secretInText catches the forms a secret takes in a URL or a header, in case one
// ever reaches a log line. The app does not log them; this is the second lock.
var secretInText = regexp.MustCompile(`(?i)\b(t|token|sig|pin|authtoken|password)=[^\s&"]+`)

// redactor returns a function that hides what a report must not carry: library
// folders (longest first, so a folder inside another is hidden whole), the home
// folder, and anything shaped like a token.
func (s *Service) redactor(libs []app.LibraryState) func(string) string {
	type swap struct{ from, to string }
	var swaps []swap
	for i, l := range libs {
		if l.Library.Root != "" {
			swaps = append(swaps, swap{l.Library.Root, fmt.Sprintf("[library %d]", i+1)})
		}
	}
	sort.Slice(swaps, func(i, j int) bool { return len(swaps[i].from) > len(swaps[j].from) })
	if s.id.Home != "" {
		swaps = append(swaps, swap{s.id.Home, "~"})
	}
	return func(text string) string {
		for _, w := range swaps {
			text = strings.ReplaceAll(text, w.from, w.to)
		}
		return secretInText.ReplaceAllString(text, "$1=[hidden]")
	}
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
