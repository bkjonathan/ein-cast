package loginitem

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestItem(t *testing.T, exe string) *Item {
	t.Helper()
	return &Item{log: slog.New(slog.DiscardHandler), dir: filepath.Join(t.TempDir(), "LaunchAgents"), exe: exe}
}

func TestSet_WritesAndRemovesTheLaunchAgent(t *testing.T) {
	i := newTestItem(t, "/Applications/EinCast.app/Contents/MacOS/eincast")

	if err := i.Set(true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(i.path())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<string>com.bkthomas.eincast</string>", "<string>/usr/bin/open</string>",
		"<string>/Applications/EinCast.app</string>", "<string>--background</string>", "<key>RunAtLoad</key>"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("plist lacks %q:\n%s", want, data)
		}
	}

	if err := i.Set(true); err != nil { // turning on twice rewrites, never fails
		t.Fatal(err)
	}
	if err := i.Set(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(i.path()); !os.IsNotExist(err) {
		t.Errorf("the LaunchAgent is still there after turning it off: %v", err)
	}
	if err := i.Set(false); err != nil {
		t.Errorf("turning off what is not on: %v", err)
	}
}

func TestProgramArguments(t *testing.T) {
	tests := []struct {
		name string
		exe  string
		want string
	}{
		{"app bundle", "/Applications/EinCast.app/Contents/MacOS/eincast", "/usr/bin/open -a /Applications/EinCast.app --args --background"},
		{"bundle path with a space", "/Users/a b/Apps/EinCast.app/Contents/MacOS/eincast", "/usr/bin/open -a /Users/a b/Apps/EinCast.app --args --background"},
		{"development build", "/Users/me/ein-cast/build/bin/eincast", "/Users/me/ein-cast/build/bin/eincast --background"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(programArguments(tt.exe), " "); got != tt.want {
				t.Errorf("programArguments(%q) = %q, want %q", tt.exe, got, tt.want)
			}
		})
	}
}

func TestPlist_EscapesTheProgramPath(t *testing.T) {
	got := plist([]string{"/Users/a&b/<x>/eincast"})
	if !strings.Contains(got, "/Users/a&amp;b/&lt;x&gt;/eincast") {
		t.Errorf("path not escaped:\n%s", got)
	}
}
