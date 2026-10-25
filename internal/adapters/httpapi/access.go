package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// Access decides whether requests must carry the access token and whether a
// given token is right. It is declared here, at the consumer; the settings
// service implements it.
type Access interface {
	// Required reports whether the access token is switched on.
	Required() bool
	// Verify reports whether token is acceptable. It must run in constant time.
	Verify(token string) bool
}

const (
	// cookieName holds the token for browsers after the first visit.
	cookieName = "eincast_access"
	// queryParam carries the token in links: QR codes, playlists, bookmarks.
	queryParam = "t"
	// cookieMaxAge is how long a browser stays signed in.
	cookieMaxAge = 30 * 24 * time.Hour
)

type tokenCtxKey struct{}

// tokenFrom returns the token this request authenticated with, or "" when the
// request is open (access is off).
func tokenFrom(ctx context.Context) string {
	t, _ := ctx.Value(tokenCtxKey{}).(string)
	return t
}

// credential is one token a request offers, and where it came from.
type credential struct {
	value    string
	viaQuery bool
}

// credentials lists every token the request offers, best source first: an
// "Authorization: Bearer" header, the "t" query parameter, then the cookie.
func credentials(r *http.Request) []credential {
	var out []credential
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		if v := strings.TrimSpace(h[7:]); v != "" {
			out = append(out, credential{value: v})
		}
	}
	if v := r.URL.Query().Get(queryParam); v != "" {
		out = append(out, credential{value: v, viaQuery: true})
	}
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		out = append(out, credential{value: c.Value})
	}
	return out
}

// requireAccess enforces the access token on everything except the player
// page's static files (which contain nothing private).
//
// Browsers arrive through the QR code (/?t=…). The first response sets a cookie
// and redirects to "/", so the token does not stay in the address bar or the
// history. Players like VLC cannot hold cookies, so playlist entries carry the
// token in their own URL.
func (a *api) requireAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.access.Required() || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		offered := credentials(r)
		for _, c := range offered {
			if !a.access.Verify(c.value) {
				continue
			}
			if c.viaQuery && r.Method == http.MethodGet && r.URL.Path == "/" {
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    c.value,
					Path:     "/",
					MaxAge:   int(cookieMaxAge / time.Second),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), tokenCtxKey{}, c.value)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		a.deny(w, r, len(offered) > 0)
	})
}

// unauthorizedPage is what a person sees when they open the address without the
// token. It has no dynamic content.
const unauthorizedPage = `<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EinCast</title>
<body style="font-family:system-ui,sans-serif;max-width:30rem;margin:4rem auto;padding:0 1rem;line-height:1.5">
<h1>Access token needed</h1>
<p>This EinCast server is protected. Open the link or scan the QR code shown in
the EinCast app on the computer that runs it.</p>
`

// unauthorizedCSP allows only the inline style the page above uses; it has no
// script and loads nothing.
const unauthorizedCSP = "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'"

// deny answers a request that has no valid token. The log records who asked and
// whether a token was offered, but never the token itself.
func (a *api) deny(w http.ResponseWriter, r *http.Request, offered bool) {
	reason := "no token"
	if offered {
		reason = "wrong token"
	}
	a.log.WarnContext(r.Context(), "access denied",
		slog.String("method", r.Method), slog.String("path", r.URL.Path),
		slog.String("client_ip", clientIP(r)), slog.String("reason", reason),
	)

	w.Header().Set("WWW-Authenticate", `Bearer realm="EinCast"`)
	if r.Method == http.MethodGet && r.URL.Path == "/" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", unauthorizedCSP)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(unauthorizedPage))
		return
	}
	// The same message whether the token was missing or wrong: do not help a guesser.
	writeError(w, http.StatusUnauthorized, codeUnauthorized, "access token required")
}

// hostGuard rejects requests whose Host header is not a way of reaching this
// machine on the local network: an IP address, a single-label name ("my-pc",
// "localhost"), or an mDNS name ("*.local").
//
// This blocks DNS rebinding. A malicious website can make its own domain
// resolve to the LAN address of this server, and the visitor's browser would
// then let that page read the library, because it is "the same site" to the
// browser. Such requests arrive with Host: evil.example, which a public domain
// name can never satisfy.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host) {
			writeError(w, http.StatusForbidden, codeForbidden, "unexpected Host header")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalHost accepts the Host values described on hostGuard. An empty Host (an
// HTTP/1.0 request) is allowed: no browser sends one, so it cannot be rebinding.
func isLocalHost(hostport string) bool {
	if hostport == "" {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	host = strings.TrimSuffix(host, ".")

	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	return host != "" && (!strings.Contains(host, ".") || strings.HasSuffix(host, ".local"))
}
