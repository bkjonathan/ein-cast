package cloudflared

import (
	"log/slog"
	"regexp"
	"strings"

	"eincast/internal/adapters/tunnel/procsup"
	"eincast/internal/app"
)

// quickURL finds a quick-tunnel address in a line of cloudflared's output. It
// accepts only https://<label>.trycloudflare.com, with the match bounded on both
// sides so that "…trycloudflare.com.evil.example" and
// "…trycloudflare.com@evil.example" (another host) are not mistaken for it.
//
// What may follow the name is the end of the line, or a character that cannot
// continue a host name (space, "/", "|", a quote, ...), or one full stop and then
// such a character (the end of a sentence). A "." directly followed by a letter
// is another label of a different host, so it is not allowed.
var quickURL = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9.@-])https://([a-z0-9]+(?:-[a-z0-9]+)*)\.trycloudflare\.com(?:$|[^A-Za-z0-9.@-]|\.(?:$|[^A-Za-z0-9.@-]))`)

// apiLabel is the service cloudflared itself talks to when it asks for a quick
// tunnel. Its address appears in error lines and is not ours.
const apiLabel = "api"

// findQuickURL returns the quick-tunnel address in line, if there is one.
func findQuickURL(line string) (app.PublicURL, bool) {
	m := quickURL.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	label := strings.ToLower(m[1])
	if label == apiLabel {
		return "", false
	}
	return app.PublicURL("https://" + label + ".trycloudflare.com"), true
}

// severity matches cloudflared's log format: a timestamp, then a three-letter
// tag. Anchoring it there keeps the word "ERR" in ordinary text from counting.
var severity = regexp.MustCompile(`^\S+\s+(ERR|WRN|DBG)\s`)

// logLevel maps cloudflared's own severity tags to ours, so a real error stands
// out in the log.
func logLevel(_ procsup.Stream, line string) slog.Level {
	m := severity.FindStringSubmatch(line)
	if m == nil {
		return slog.LevelInfo
	}
	switch m[1] {
	case "ERR":
		return slog.LevelError
	case "WRN":
		return slog.LevelWarn
	default:
		return slog.LevelDebug
	}
}
