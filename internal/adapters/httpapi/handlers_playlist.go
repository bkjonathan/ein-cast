package httpapi

import (
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	"eincast/internal/domain/media"
)

// handlePlaylist serves an extended M3U of every video and audio item so VLC,
// Infuse or a TV can open the whole library from one URL.
//
// Entries use absolute URLs built from the request's Host header: that is, by
// definition, an address this client just used to reach us, so it works for
// whichever interface (LAN IP, mDNS name, tunnel) they came through. Go's
// server already rejects Host values with control characters.
func (a *api) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	// When access is protected, entries carry the token they were requested
	// with: VLC and TV players do not keep cookies.
	suffix := ""
	if token := tokenFrom(r.Context()); token != "" {
		suffix = "?" + queryParam + "=" + url.QueryEscape(token)
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, it := range a.lib.List() {
		if it.Kind != media.KindVideo && it.Kind != media.KindAudio {
			continue
		}
		b.WriteString("#EXTINF:-1,")
		b.WriteString(playlistTitle(it.Name))
		b.WriteByte('\n')
		b.WriteString(base + "/stream/" + string(it.ID) + suffix)
		b.WriteByte('\n')
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", `inline; filename="eincast.m3u"`)
	_, _ = w.Write([]byte(b.String()))
}

// playlistTitle drops the extension and any control characters: a file name
// containing a newline would otherwise inject extra playlist lines.
func playlistTitle(name string) string {
	title := strings.TrimSuffix(name, path.Ext(name))
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, title)
}
