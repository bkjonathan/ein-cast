package loginitem

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"eincast/internal/app"
)

// label names the LaunchAgent; it is the bundle ID so the entry is recognisable
// in System Settings → Login Items.
const label = "com.bkthomas.eincast"

// Item is a per-user LaunchAgent. It needs no privileges and is picked up at the
// next sign-in, so nothing has to be loaded now.
type Item struct {
	log *slog.Logger
	dir string // the LaunchAgents folder
	exe string // the running program
}

var _ app.LoginItem = (*Item)(nil)

// New returns an Item for the running program.
func New(log *slog.Logger) (*Item, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find the running program: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find the home folder: %w", err)
	}
	return &Item{
		log: log.With(slog.String("source", "login")),
		dir: filepath.Join(home, "Library", "LaunchAgents"),
		exe: exe,
	}, nil
}

func (i *Item) path() string { return filepath.Join(i.dir, label+".plist") }

// Set writes or removes the LaunchAgent.
func (i *Item) Set(enabled bool) error {
	if !enabled {
		if err := os.Remove(i.path()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove login entry: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(i.dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", i.dir, err)
	}
	// Written to a temporary file first so a crash cannot leave half a plist
	// that launchd would reject at every sign-in.
	tmp := i.path() + ".tmp"
	if err := os.WriteFile(tmp, []byte(plist(programArguments(i.exe))), 0o644); err != nil {
		return fmt.Errorf("write login entry: %w", err)
	}
	if err := os.Rename(tmp, i.path()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace login entry: %w", err)
	}
	return nil
}

// programArguments is what launchd runs. Inside an app bundle it opens the
// bundle, which is how the Finder starts it (Dock icon, single instance);
// outside one (a development build) it runs the program itself.
func programArguments(exe string) []string {
	if bundle, ok := bundleOf(exe); ok {
		return []string{"/usr/bin/open", "-a", bundle, "--args", BackgroundFlag}
	}
	return []string{exe, BackgroundFlag}
}

// bundleOf returns the .app folder a program path sits in.
func bundleOf(exe string) (string, bool) {
	const marker = ".app/Contents/MacOS/"
	i := strings.Index(exe, marker)
	if i < 0 {
		return "", false
	}
	return exe[:i+len(".app")], true
}

func plist(args []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range args {
		b.WriteString("\t\t<string>")
		_ = xml.EscapeText(&b, []byte(a))
		b.WriteString("</string>\n")
	}
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`)
	return b.String()
}
