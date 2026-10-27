package httpapi

import (
	"net/http"
	"net/url"
	"path"
	"strconv"
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
	base := requestScheme(r) + "://" + r.Host
	// VLC and TV players do not keep cookies, so entries carry their own
	// credential. On the home network that is the token the playlist was
	// requested with. Through a tunnel it is a signed link per item that expires,
	// never the PIN session or the LAN token.
	tunnel := a.viaTunnel(r)
	items, v, ok := a.visibleItems(w, r)
	if !ok {
		return
	}
	// Each entry also carries the profile credential when there is a choice of
	// profiles, so the player's own requests are checked as this profile too.
	var params []string
	if token := tokenFrom(r.Context()); token != "" && !tunnel {
		params = append(params, queryParam+"="+url.QueryEscape(token))
	}
	if !tunnel && !v.open && a.profiles.Active() {
		if cred, ok := a.profiles.Credential(v.id); ok {
			params = append(params, profileParam+"="+url.QueryEscape(cred))
		}
	}
	suffix := ""
	if len(params) > 0 {
		suffix = "?" + strings.Join(params, "&")
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, it := range items {
		if it.Kind != media.KindVideo && it.Kind != media.KindAudio {
			continue
		}
		b.WriteString("#EXTINF:-1,")
		b.WriteString(playlistTitle(it.Name))
		b.WriteByte('\n')
		b.WriteString(base + "/stream/" + string(it.ID))
		if tunnel {
			link := a.remote.SignStream(it.ID)
			b.WriteString("?" + expParam + "=" + strconv.FormatInt(link.Expires.Unix(), 10) +
				"&" + sigParam + "=" + link.Sig)
		} else {
			b.WriteString(suffix)
		}
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
