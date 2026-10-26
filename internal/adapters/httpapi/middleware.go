package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

// middleware wraps a handler with cross-cutting behaviour.
type middleware func(http.Handler) http.Handler

// chain applies middleware so the first one listed is the outermost.
func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type ctxKey struct{}

// requestIDFrom returns the ID requestID attached to ctx, or "" outside a request.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// requestID gives every request an ID, visible in the log and the
// X-Request-ID response header. A client-supplied ID is kept only if it is
// short and plain, so it cannot smuggle odd characters into log lines.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		ok := c == '-' || c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		if !ok {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	return hex.EncodeToString(b[:])
}

// serverHeader identifies the app on every response: "EinCast/<version>".
func serverHeader(version string) middleware {
	value := "EinCast/" + version
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", value)
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders applies headers that are right for every response.
// nosniff stops a browser from guessing a type and running, say, a media file
// as script; no-referrer keeps stream URLs out of Referer headers.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// recorder captures what a handler wrote so the access log can report it.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

// ReadFrom keeps the zero-copy path alive: wrapping a ResponseWriter hides its
// io.ReaderFrom, and without it http.ServeContent could not use sendfile(2),
// which matters for multi-gigabyte videos.
func (r *recorder) ReadFrom(src io.Reader) (int64, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	var n int64
	var err error
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		// writerOnly hides ReaderFrom so io.Copy cannot recurse back into us.
		n, err = io.Copy(struct{ io.Writer }{r.ResponseWriter}, src)
	}
	r.bytes += n
	return n, err
}

// Unwrap lets http.NewResponseController (used by ServeContent and friends)
// reach the real writer's Flusher, deadlines and so on.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// accessLog writes one structured line per request. It sits outside recoverer,
// so a panicking handler is still logged, with its 500.
func accessLog(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK // handler wrote nothing: net/http sends 200
			}
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			}
			log.LogAttrs(r.Context(), level, "request",
				// Most useful first: the UI shows each record on one truncated line.
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Duration("duration", time.Since(start)),
				slog.Int64("bytes", rec.bytes),
				slog.String("client_ip", clientIP(r)),
				slog.String("request_id", requestIDFrom(r.Context())),
			)
		})
	}
}

// clientIP is the address of whoever is really asking. Normally that is the TCP
// peer. A tunnel makes every request arrive from loopback, so for those (and
// only those) the forwarding headers name the real client; see forwardedIP.
// Anyone else's X-Forwarded-For is ignored: it is trivially forged.
func clientIP(r *http.Request) string {
	peer, ok := peerAddr(r)
	if !ok {
		return r.RemoteAddr
	}
	if peer.IsLoopback() {
		if fwd, ok := forwardedIP(r); ok {
			return fwd.String()
		}
	}
	return peer.String()
}

// peerAddr is the address at the other end of the TCP connection.
func peerAddr(r *http.Request) (netip.Addr, bool) {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	return ap.Addr().Unmap(), true
}

// forwardedIP reads the client address a tunnel reports: CF-Connecting-IP when
// present, else the last X-Forwarded-For entry. The last, not the first: the
// tunnel's edge appends the address it saw to whatever the client sent, so the
// first entry is the client's own claim and the last is the edge's observation.
// That holds for exactly one trusted hop, which is what every tunnel here is.
func forwardedIP(r *http.Request) (netip.Addr, bool) {
	if v := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); v != "" {
		if a, err := netip.ParseAddr(v); err == nil {
			return a.Unmap(), true
		}
	}
	xff := r.Header.Values("X-Forwarded-For")
	for i := len(xff) - 1; i >= 0; i-- {
		parts := strings.Split(xff[i], ",")
		for j := len(parts) - 1; j >= 0; j-- {
			if a, err := netip.ParseAddr(strings.TrimSpace(parts[j])); err == nil {
				return a.Unmap(), true
			}
		}
	}
	return netip.Addr{}, false
}

// recoverer turns a handler panic into a logged 500 so one bad request cannot
// take the server down. http.ErrAbortHandler is net/http's own "stop quietly"
// signal and must keep propagating.
func recoverer(log *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.ErrorContext(r.Context(), "handler panic",
					slog.String("request_id", requestIDFrom(r.Context())),
					slog.Any("panic", v),
					slog.String("stack", string(debug.Stack())),
				)
				writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// corsLAN lets web pages served from the local network (other apps, a dev
// server) call the API, and nobody else. Public origins get no CORS headers,
// so a random website cannot read your library through your browser.
func corsLAN(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
		}
		if origin == "" || !isLANOrigin(origin) {
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Range, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLANOrigin accepts localhost, *.local (mDNS), *.ts.net (Tailscale MagicDNS),
// and loopback, private, link-local or tailnet (100.64.0.0/10) IP literals.
func isLANOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".ts.net") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || tailnetV4.Contains(addr.Unmap())
}

// tailnetV4 is the carrier-grade NAT range Tailscale assigns addresses from.
// net/netip does not call it "private".
var tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")
