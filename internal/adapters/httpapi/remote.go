package httpapi

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// Remote is what the HTTP layer needs from remote access. It is declared here,
// at the consumer; remote.Service implements it.
type Remote interface {
	// PublicActive reports whether a public tunnel is, or might be, carrying traffic.
	PublicActive() bool
	// HostAllowed reports whether host is the address of the online tunnel.
	HostAllowed(host string) bool
	// Login exchanges the PIN for a session. client is used for rate limiting.
	// It returns app.ErrWrongPIN, app.ErrNotPublic or *app.LockedError.
	Login(client, pin string) (app.RemoteSession, error)
	// ValidSession reports whether a session cookie value is a live sign-in.
	ValidSession(token string) bool
	// SignStream signs a stream link for one media item.
	SignStream(id media.ID) app.SignedLink
	// VerifyStream reports whether sig authorises id until the unix time exp.
	VerifyStream(id media.ID, sig string, exp int64) bool
}

const (
	// sessionCookie holds the remote sign-in. It is separate from the LAN
	// token's cookie: the two credentials are never interchangeable.
	sessionCookie = "eincast_session"

	// Query parameters of a signed stream link.
	sigParam = "sig"
	expParam = "exp"

	// maxLoginBody bounds the sign-in form; a PIN is at most 64 characters.
	maxLoginBody = 4 << 10
)

// viaTunnel reports whether r reached us through a tunnel.
//
// Tunnels connect from this machine, so such a request arrives on loopback.
// Loopback alone does not prove it (the owner's own browser is loopback too),
// so a loopback request counts as tunnelled when a public tunnel is up, or when
// it carries the forwarding headers only a proxy adds. Both fail closed: the
// worst outcome of a wrong guess is that the owner is asked for the PIN.
//
// A request from any other address is never tunnelled, whatever headers it
// carries: those are the ones someone on the network could forge.
func (a *api) viaTunnel(r *http.Request) bool {
	peer, ok := peerAddr(r)
	if !ok || !peer.IsLoopback() {
		return false
	}
	return a.remote.PublicActive() || hasForwardingHeaders(r)
}

func hasForwardingHeaders(r *http.Request) bool {
	for _, h := range []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded"} {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// requestScheme is the scheme the client used, for building absolute URLs. A
// tunnel serves https at its edge and talks plain http to us, and says so in
// X-Forwarded-Proto, which is believed only on a loopback connection.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if peer, ok := peerAddr(r); ok && peer.IsLoopback() &&
		strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

// requireSession is the rule for tunnelled requests: a PIN sign-in (cookie) or
// a signed stream link, nothing else. The LAN token is not accepted here, so a
// token that leaked through a QR code on the home network does not open the
// public address.
func (a *api) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			// The only thing a stranger may do. It is rate limited in the service.
		case a.hasSession(r), a.hasSignedLink(r):
		default:
			a.denyTunnel(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *api) hasSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	return err == nil && c.Value != "" && a.remote.ValidSession(c.Value)
}

// hasSignedLink accepts "/stream/{id}?exp=…&sig=…" for exactly that id, so a
// link for one film cannot be turned into a link for another.
func (a *api) hasSignedLink(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	id, ok := strings.CutPrefix(r.URL.Path, "/stream/")
	if !ok || id == "" || strings.Contains(id, "/") {
		return false
	}
	q := r.URL.Query()
	exp, err := strconv.ParseInt(q.Get(expParam), 10, 64)
	if err != nil || q.Get(sigParam) == "" {
		return false
	}
	return a.remote.VerifyStream(media.ID(id), q.Get(sigParam), exp)
}

// denyTunnel answers a tunnelled request with no valid credential: the sign-in
// page for a browser opening the address, a plain 401 for everything else. The
// access log already records the request, so nothing more is logged here.
func (a *api) denyTunnel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet && r.URL.Path == "/" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		a.writeLoginPage(w, r, http.StatusUnauthorized, func(lang string) string { return a.text.text(lang, keySignInPrompt, nil) })
		return
	}
	writeError(w, http.StatusUnauthorized, codeUnauthorized, "sign in required")
}

// handleLogin checks the PIN and, if right, starts a session. It exists only
// for tunnelled requests; on the home network it does not.
func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.viaTunnel(r) {
		a.handleNotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	if !a.sameSite(r) {
		writeError(w, http.StatusForbidden, codeForbidden, "cross-site request refused")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read the form")
		return
	}

	ip := clientIP(r)
	sess, err := a.remote.Login(ip, r.PostFormValue("pin"))

	var locked *app.LockedError
	switch {
	case err == nil:
		a.log.InfoContext(r.Context(), "remote sign-in", slog.String("client_ip", ip))
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    sess.Token,
			Path:     "/",
			Expires:  sess.Expires,
			HttpOnly: true,
			Secure:   requestScheme(r) == "https",
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case errors.As(err, &locked):
		a.log.WarnContext(r.Context(), "remote sign-in locked out",
			slog.String("client_ip", ip), slog.Duration("retry_after", locked.RetryAfter))
		secs := int(math.Ceil(locked.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		minutes := int(math.Ceil(float64(secs) / 60))
		a.writeLoginPage(w, r, http.StatusTooManyRequests, func(lang string) string {
			return a.text.plural(lang, keySignInLocked, minutes, nil)
		})
	case errors.Is(err, app.ErrWrongPIN):
		a.log.WarnContext(r.Context(), "remote sign-in failed", slog.String("client_ip", ip))
		a.writeLoginPage(w, r, http.StatusUnauthorized, func(lang string) string { return a.text.text(lang, keySignInWrong, nil) })
	case errors.Is(err, app.ErrNotPublic):
		a.handleNotFound(w, r)
	default:
		a.log.ErrorContext(r.Context(), "remote sign-in error", errAttr(err))
		writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
	}
}

// sameSite refuses a sign-in posted from another website. Browsers say where a
// request came from in Sec-Fetch-Site and Origin; tools that send neither (curl)
// are let through, since they are not a browser being tricked.
func (a *api) sameSite(r *http.Request) bool {
	if s := r.Header.Get("Sec-Fetch-Site"); s != "" && s != "same-origin" && s != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	// The tunnel's own address is also fine, in case it rewrites the Host header.
	return strings.EqualFold(u.Host, r.Host) || a.remote.HostAllowed(u.Host)
}

// Keys of the words on the sign-in page (web/locales). They are fixed text:
// nothing a client sent is ever echoed into the page.
const (
	keySignInPrompt = "signin.prompt"
	keySignInWrong  = "signin.wrong"
	keySignInLocked = "signin.locked" // a plural: .one and .other
	keySignInButton = "signin.button"
)

// loginPage takes, in order: the language, the message, the button's text.
const loginPage = `<!doctype html>
<html lang="%s">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EinCast</title>
<body style="font-family:system-ui,sans-serif;max-width:24rem;margin:4rem auto;padding:0 1rem;line-height:1.5">
<h1>EinCast</h1>
<p>%s</p>
<form method="post" action="/login">
<input type="password" name="pin" autocomplete="current-password" autofocus required maxlength="64"
 style="width:100%%;padding:.6rem;font-size:1rem;box-sizing:border-box">
<button type="submit" style="margin-top:.8rem;padding:.6rem 1.2rem;font-size:1rem">%s</button>
</form>
`

// loginCSP allows the page's inline style and posting back to itself, and
// nothing else: no script, no images, no framing.
const loginCSP = "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// writeLoginPage answers in the visitor's language. The words come from our own
// embedded files, but are escaped anyway: the page's safety should not depend on
// what a translator typed.
func (a *api) writeLoginPage(w http.ResponseWriter, r *http.Request, status int, message func(lang string) string) {
	lang := a.langOf(r)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", loginCSP)
	h.Set("Content-Language", lang)
	h.Add("Vary", "Accept-Language")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, loginPage, lang, html.EscapeString(message(lang)),
		html.EscapeString(a.text.text(lang, keySignInButton, nil)))
}
