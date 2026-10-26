package cloudflared

import (
	"log/slog"
	"testing"

	"eincast/internal/adapters/tunnel/procsup"
)

func TestFindQuickURL_RealisticBanners(t *testing.T) {
	cases := map[string]string{
		// The boxed banner of recent versions, with cloudflared's log prefix.
		"2026-05-01T10:00:00Z INF |  https://funny-lemon-tree.trycloudflare.com                                                |": "https://funny-lemon-tree.trycloudflare.com",
		"2026-05-01T10:00:00Z INF Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):":       "",
		// Older, bare form.
		"https://abc-def-ghi-jkl.trycloudflare.com":       "https://abc-def-ghi-jkl.trycloudflare.com",
		"Visit it at: https://abc.trycloudflare.com":      "https://abc.trycloudflare.com",
		"Visit it at: https://abc.trycloudflare.com.":     "https://abc.trycloudflare.com", // end of sentence
		"| https://abc.trycloudflare.com/some/path?x=1 |": "https://abc.trycloudflare.com",
		"  \"https://abc.trycloudflare.com\"  ":           "https://abc.trycloudflare.com",
		"HTTPS://ABC-DEF.TRYCLOUDFLARE.COM":               "https://abc-def.trycloudflare.com",
		"url=https://a1.trycloudflare.com,next":           "https://a1.trycloudflare.com",
	}
	for line, want := range cases {
		got, ok := findQuickURL(line)
		if want == "" {
			if ok {
				t.Errorf("%q: found %q, want nothing", line, got)
			}
			continue
		}
		if !ok || string(got) != want {
			t.Errorf("%q: got %q (%v), want %q", line, got, ok, want)
		}
	}
}

func TestFindQuickURL_NeverMistakesAnotherHostForOurs(t *testing.T) {
	for _, line := range []string{
		// cloudflared's own API, quoted in its error lines.
		`ERR failed to request quick Tunnel: Post "https://api.trycloudflare.com/tunnel": EOF`,
		`https://API.trycloudflare.com`,
		// Hosts that merely contain or continue the name.
		"https://abc.trycloudflare.com.evil.example",
		"https://abc.trycloudflare.com.evil.example/",
		"https://abc.trycloudflare.com@evil.example/",
		"https://abc.trycloudflare.com-evil.example",
		"https://x.abc.trycloudflare.com", // a sub-subdomain is not a quick tunnel
		"https://trycloudflare.com",
		"http://abc.trycloudflare.com", // plain http is never what cloudflared reports
		"https://abc_def.trycloudflare.com",
		"https://-abc.trycloudflare.com",
		"https://.trycloudflare.com",
		"https://abc.trycloudflare.org",
		"abc.trycloudflare.com",
		"",
	} {
		got, ok := findQuickURL(line)
		if ok {
			t.Errorf("%q: accepted as %q", line, got)
		}
	}
}

func TestLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"2026-05-01T10:00:00Z INF Registered tunnel connection connIndex=0": slog.LevelInfo,
		"2026-05-01T10:00:00Z WRN Retrying":                                 slog.LevelWarn,
		"2026-05-01T10:00:00Z ERR Failed to dial":                           slog.LevelError,
		"2026-05-01T10:00:00Z DBG details":                                  slog.LevelDebug,
		"some unstructured text":                                            slog.LevelInfo,
		"the word ERR alone":                                                slog.LevelInfo, // needs the tag's spaces
	}
	for line, want := range cases {
		if got := logLevel(procsup.Stderr, line); got != want {
			t.Errorf("%q: level %v, want %v", line, got, want)
		}
	}
}
