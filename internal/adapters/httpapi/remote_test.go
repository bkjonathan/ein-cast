package httpapi

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

const (
	goodPIN     = "correct horse"
	publicHost  = "abc.trycloudflare.com"
	fakeSession = "session-token"
	clientAddr  = "203.0.113.7"
)

// fakeLinkExpiry is far in the future so the fake never has to read a clock.
var fakeLinkExpiry = time.Unix(2_000_000_000, 0)

// fakeRemote is a hand-written Remote. The real rules (rate limits, HMACs) are
// tested in app/remote; here the question is only what the HTTP layer does with
// each answer.
type fakeRemote struct {
	mu        sync.Mutex
	public    bool
	host      string
	pin       string
	lockedFor time.Duration
	logins    []loginCall
}

type loginCall struct{ client, pin string }

func (f *fakeRemote) PublicActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.public
}

func (f *fakeRemote) HostAllowed(host string) bool {
	return f.host != "" && strings.EqualFold(host, f.host)
}

func (f *fakeRemote) Login(client, pin string) (app.RemoteSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logins = append(f.logins, loginCall{client, pin})
	switch {
	case !f.public:
		return app.RemoteSession{}, app.ErrNotPublic
	case f.lockedFor > 0:
		return app.RemoteSession{}, &app.LockedError{RetryAfter: f.lockedFor}
	case pin != f.pin:
		return app.RemoteSession{}, app.ErrWrongPIN
	}
	return app.RemoteSession{Token: fakeSession, Expires: fakeLinkExpiry}, nil
}

func (f *fakeRemote) ValidSession(token string) bool { return token == fakeSession }

func (f *fakeRemote) SignStream(id media.ID) app.SignedLink {
	return app.SignedLink{Sig: "sig-" + string(id), Expires: fakeLinkExpiry}
}

func (f *fakeRemote) VerifyStream(id media.ID, sig string, exp int64) bool {
	return sig == "sig-"+string(id) && exp == fakeLinkExpiry.Unix()
}

func (f *fakeRemote) loginCalls() []loginCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]loginCall(nil), f.logins...)
}

// tunnelFixture is a server with public access on and the LAN token also
// required, so tests can show the LAN token does not open the public address.
func newTunnelFixture(t *testing.T) (*fixture, *fakeRemote) {
	t.Helper()
	rem := &fakeRemote{public: true, host: publicHost, pin: goodPIN}
	f := newFixtureFull(t, &fakeAccess{required: true, token: goodToken}, rem)
	return f, rem
}

// viaProxy marks a request as coming through a tunnel, the way one does.
func viaProxy(r *http.Request) { r.Header.Set("X-Forwarded-For", clientAddr) }

func withSession(r *http.Request) {
	viaProxy(r)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: fakeSession})
}

func (f *fixture) postForm(path string, form url.Values, mod func(*http.Request)) *http.Response {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(form.Encode()))
	if err != nil {
		f.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	viaProxy(req)
	if mod != nil {
		mod(req)
	}
	client := *f.srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func linkQuery(id media.ID) string {
	return "exp=" + strconv.FormatInt(fakeLinkExpiry.Unix(), 10) + "&sig=sig-" + string(id)
}

func TestTunnel_RequestsWithoutASessionAreRefused(t *testing.T) {
	f, _ := newTunnelFixture(t)
	movie := string(f.movie.ID)
	paths := []string{"/", "/api/health", "/api/library", "/api/library/" + movie, "/stream/" + movie, "/playlist.m3u", "/static/app.js", "/nope"}

	creds := map[string]func(*http.Request){
		"nothing":            viaProxy,
		"LAN token in query": func(r *http.Request) { viaProxy(r); r.URL.RawQuery = "t=" + goodToken },
		"LAN token as bearer": func(r *http.Request) {
			viaProxy(r)
			r.Header.Set("Authorization", "Bearer "+goodToken)
		},
		"LAN token cookie": func(r *http.Request) {
			viaProxy(r)
			r.AddCookie(&http.Cookie{Name: cookieName, Value: goodToken})
		},
		"session value in the LAN cookie": func(r *http.Request) {
			viaProxy(r)
			r.AddCookie(&http.Cookie{Name: cookieName, Value: fakeSession})
		},
		"wrong session": func(r *http.Request) {
			viaProxy(r)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "forged"})
		},
		"empty session": func(r *http.Request) {
			viaProxy(r)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: ""})
		},
		"signed link for another item": func(r *http.Request) { viaProxy(r); r.URL.RawQuery = linkQuery("someone-else") },
		"signature without expiry":     func(r *http.Request) { viaProxy(r); r.URL.RawQuery = "sig=sig-" + movie },
		"expiry changed": func(r *http.Request) {
			viaProxy(r)
			r.URL.RawQuery = "exp=" + strconv.FormatInt(fakeLinkExpiry.Unix()+1, 10) + "&sig=sig-" + movie
		},
	}
	for name, mod := range creds {
		for _, path := range paths {
			t.Run(name+" "+path, func(t *testing.T) {
				assertErrorShape(t, f.do("GET", path, mod), 401, "unauthorized")
			})
		}
	}
}

func TestTunnel_LoopbackAloneIsTunnelledWhilePublicAccessIsOn(t *testing.T) {
	// A tunnel that adds no forwarding headers must not become a way around sign-in.
	f, _ := newTunnelFixture(t)
	assertErrorShape(t, f.do("GET", "/api/library", nil), 401, "unauthorized")
}

func TestTunnel_SessionCookieIsAccepted(t *testing.T) {
	f, _ := newTunnelFixture(t)
	for _, path := range []string{"/", "/api/health", "/api/library", "/stream/" + string(f.movie.ID), "/playlist.m3u"} {
		if resp := f.do("GET", path, withSession); resp.StatusCode != 200 {
			t.Errorf("%s with a session: status = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestTunnel_SignedLinkOpensOnlyItsItem(t *testing.T) {
	f, _ := newTunnelFixture(t)
	movie, song := f.lib.items[0].ID, f.lib.items[1].ID

	resp := f.do("GET", "/stream/"+string(movie)+"?"+linkQuery(movie), viaProxy)
	if resp.StatusCode != 200 || string(readBody(t, resp)) != string(f.movieBy) {
		t.Fatalf("signed link: status = %d, want the file", resp.StatusCode)
	}

	ranged := f.do("GET", "/stream/"+string(movie)+"?"+linkQuery(movie), func(r *http.Request) {
		viaProxy(r)
		r.Header.Set("Range", "bytes=0-9")
	})
	if ranged.StatusCode != http.StatusPartialContent {
		t.Errorf("Range through a signed link: status = %d, want 206 (players seek)", ranged.StatusCode)
	}

	cases := map[string]struct{ method, path string }{
		"another item":                   {"GET", "/stream/" + string(song) + "?" + linkQuery(movie)},
		"the library":                    {"GET", "/api/library?" + linkQuery(movie)},
		"a path below the item":          {"GET", "/stream/" + string(movie) + "/x?" + linkQuery(movie)},
		"a method that is not read-only": {"POST", "/stream/" + string(movie) + "?" + linkQuery(movie)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := f.do(c.method, c.path, viaProxy); resp.StatusCode != 401 {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestTunnel_BrowserWithoutASessionSeesTheSignInPage(t *testing.T) {
	f, _ := newTunnelFixture(t)
	resp := f.do("GET", "/", func(r *http.Request) {
		viaProxy(r)
		r.Header.Set("Accept", "text/html,application/xhtml+xml")
	})
	body := string(readBody(t, resp))

	if resp.StatusCode != 401 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("status %d, type %q; want the 401 sign-in page", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, `<form method="post" action="/login">`) || !strings.Contains(body, `type="password"`) {
		t.Errorf("page has no PIN form:\n%s", body)
	}
	csp := resp.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Errorf("CSP = %q, want a policy that allows no script", csp)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", resp.Header.Get("Cache-Control"))
	}
}

func TestLogin_RightPINStartsASession(t *testing.T) {
	f, rem := newTunnelFixture(t)
	resp := f.postForm("/login", url.Values{"pin": {goodPIN}}, nil)

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("status %d → %q, want 303 → /", resp.StatusCode, resp.Header.Get("Location"))
	}
	var c *http.Cookie
	for _, got := range resp.Cookies() {
		if got.Name == sessionCookie {
			c = got
		}
	}
	switch {
	case c == nil:
		t.Fatal("no session cookie")
	case c.Value != fakeSession:
		t.Errorf("cookie = %q", c.Value)
	case !c.HttpOnly:
		t.Error("session cookie is readable by scripts")
	case c.SameSite != http.SameSiteLaxMode:
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	case c.Secure:
		t.Error("Secure set on a plain-http request")
	}
	if calls := rem.loginCalls(); len(calls) != 1 || calls[0].client != clientAddr {
		t.Errorf("Login got %+v, want the forwarded client address %s (not loopback)", calls, clientAddr)
	}
}

func TestLogin_CookieIsSecureBehindHTTPSTunnel(t *testing.T) {
	f, _ := newTunnelFixture(t)
	resp := f.postForm("/login", url.Values{"pin": {goodPIN}}, func(r *http.Request) {
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie && !c.Secure {
			t.Error("session cookie lacks Secure behind an https tunnel")
		}
	}
}

func TestLogin_RefusalsSetNoCookie(t *testing.T) {
	wrong := html.EscapeString(english(t, "signin.wrong")) // the page escapes what it prints
	cases := map[string]struct {
		form   url.Values
		query  string
		locked time.Duration
		status int
		body   string
	}{
		"wrong PIN":        {form: url.Values{"pin": {"nope"}}, status: 401, body: wrong},
		"no PIN":           {form: url.Values{}, status: 401, body: wrong},
		"PIN in the query": {form: url.Values{}, query: "?pin=" + url.QueryEscape(goodPIN), status: 401, body: wrong},
		"locked out":       {form: url.Values{"pin": {goodPIN}}, locked: 90 * time.Second, status: 429, body: "Try again in about 2 minute"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f, rem := newTunnelFixture(t)
			rem.lockedFor = c.locked
			resp := f.postForm("/login"+c.query, c.form, nil)
			body := string(readBody(t, resp))

			if resp.StatusCode != c.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, c.status)
			}
			if len(resp.Cookies()) != 0 {
				t.Errorf("refusal set cookies: %v", resp.Cookies())
			}
			if !strings.Contains(body, c.body) {
				t.Errorf("page does not say %q:\n%s", c.body, body)
			}
			if c.locked > 0 && resp.Header.Get("Retry-After") != "90" {
				t.Errorf("Retry-After = %q, want 90", resp.Header.Get("Retry-After"))
			}
		})
	}
}

func TestLogin_RefusesRequestsFromOtherSites(t *testing.T) {
	cases := map[string]func(*http.Request){
		"foreign Origin":        func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"cross-site fetch":      func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"same-site, not origin": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") },
		"unparseable Origin":    func(r *http.Request) { r.Header.Set("Origin", "://") },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			f, rem := newTunnelFixture(t)
			resp := f.postForm("/login", url.Values{"pin": {goodPIN}}, mod)
			assertErrorShape(t, resp, 403, "forbidden")
			if n := len(rem.loginCalls()); n != 0 {
				t.Errorf("the PIN was checked %d time(s) for a cross-site request", n)
			}
		})
	}
}

func TestLogin_AcceptsTheSiteItselfAndNonBrowsers(t *testing.T) {
	cases := map[string]func(*http.Request){
		"no Origin (curl)":      nil,
		"Origin matches Host":   func(r *http.Request) { r.Header.Set("Origin", "http://"+r.Host) },
		"Origin is the tunnel":  func(r *http.Request) { r.Header.Set("Origin", "https://"+publicHost) },
		"fetch metadata agrees": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			f, _ := newTunnelFixture(t)
			if resp := f.postForm("/login", url.Values{"pin": {goodPIN}}, mod); resp.StatusCode != http.StatusSeeOther {
				t.Errorf("status = %d, want 303", resp.StatusCode)
			}
		})
	}
}

func TestLogin_DoesNotExistOnTheHomeNetwork(t *testing.T) {
	// Public access off and no forwarding headers: an ordinary LAN request.
	f := newFixtureFull(t, &fakeAccess{}, &fakeRemote{pin: goodPIN})
	req, _ := http.NewRequest("POST", f.srv.URL+"/login", strings.NewReader("pin="+goodPIN))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertErrorShape(t, resp, 404, "not_found")
}

func TestLogin_PINsNeverReachTheLog(t *testing.T) {
	f, _ := newTunnelFixture(t)
	const wrong = "wrong-pin-xyz-9731"
	f.postForm("/login", url.Values{"pin": {wrong}}, nil)
	f.postForm("/login", url.Values{"pin": {goodPIN}}, nil)
	f.postForm("/login?pin="+url.QueryEscape(wrong), url.Values{}, nil)

	logs := waitForLog(t, f, "remote sign-in")
	for _, secret := range []string{wrong, goodPIN, url.QueryEscape(wrong), fakeSession} {
		if strings.Contains(logs, secret) {
			t.Errorf("log contains %q:\n%s", secret, logs)
		}
	}
	if !strings.Contains(logs, "remote sign-in failed") || !strings.Contains(logs, clientAddr) {
		t.Errorf("a failed sign-in should be logged with who tried:\n%s", logs)
	}
}

// waitForLog returns the log once it mentions want: the access log line is
// written after the response is sent.
func waitForLog(t *testing.T, f *fixture, want string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := f.logs.String(); strings.Contains(s, want) {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	return f.logs.String()
}

func TestHostGuard_AcceptsTheOnlineTunnelAndNoOtherName(t *testing.T) {
	f, _ := newTunnelFixture(t)
	withHost := func(host string) func(*http.Request) {
		return func(r *http.Request) { withSession(r); r.Host = host }
	}
	cases := []struct {
		host   string
		status int
	}{
		{publicHost, 200},
		{strings.ToUpper(publicHost), 200},
		{"evil.example", 403}, // DNS rebinding
		{"abc.trycloudflare.com.evil.example", 403},
		{"trycloudflare.com", 403},
		{"127.0.0.1:8765", 200}, // the machine's own address stays valid
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if resp := f.do("GET", "/api/health", withHost(c.host)); resp.StatusCode != c.status {
				t.Errorf("Host %q: status = %d, want %d", c.host, resp.StatusCode, c.status)
			}
		})
	}
}

func TestPlaylist_ThroughATunnelUsesSignedLinks(t *testing.T) {
	f, _ := newTunnelFixture(t)
	resp := f.do("GET", "/playlist.m3u", func(r *http.Request) {
		withSession(r)
		r.Host = publicHost
		r.Header.Set("X-Forwarded-Proto", "https")
	})
	body := string(readBody(t, resp))

	want := "https://" + publicHost + "/stream/" + string(f.movie.ID) + "?" + linkQuery(f.movie.ID)
	if !strings.Contains(body, want) {
		t.Errorf("playlist lacks the signed https link %q:\n%s", want, body)
	}
	for _, leak := range []string{"t=" + goodToken, fakeSession, "http://" + publicHost} {
		if strings.Contains(body, leak) {
			t.Errorf("playlist contains %q:\n%s", leak, body)
		}
	}
}

func TestClientIP_TrustsForwardingHeadersOnlyFromLoopback(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		header http.Header
		want   string
	}{
		{"LAN peer, no headers", "192.168.1.9:5000", nil, "192.168.1.9"},
		{"LAN peer forges X-Forwarded-For", "192.168.1.9:5000", http.Header{"X-Forwarded-For": {"10.0.0.1"}}, "192.168.1.9"},
		{"LAN peer forges CF-Connecting-IP", "192.168.1.9:5000", http.Header{"Cf-Connecting-Ip": {"10.0.0.1"}}, "192.168.1.9"},
		{"public peer forges both", "198.51.100.4:1", http.Header{"X-Forwarded-For": {"10.0.0.1"}, "Cf-Connecting-Ip": {"10.0.0.2"}}, "198.51.100.4"},
		{"loopback, no headers", "127.0.0.1:5000", nil, "127.0.0.1"},
		{"loopback with CF-Connecting-IP", "127.0.0.1:5000", http.Header{"Cf-Connecting-Ip": {clientAddr}}, clientAddr},
		{"loopback with X-Forwarded-For", "127.0.0.1:5000", http.Header{"X-Forwarded-For": {clientAddr}}, clientAddr},
		{"IPv6 loopback", "[::1]:5000", http.Header{"X-Forwarded-For": {clientAddr}}, clientAddr},
		{"the edge's entry wins over the client's claim", "127.0.0.1:5000", http.Header{"X-Forwarded-For": {"10.0.0.1, " + clientAddr}}, clientAddr},
		{"entries across repeated headers", "127.0.0.1:5000", http.Header{"X-Forwarded-For": {"10.0.0.1", clientAddr}}, clientAddr},
		{"CF-Connecting-IP beats X-Forwarded-For", "127.0.0.1:5000", http.Header{"Cf-Connecting-Ip": {clientAddr}, "X-Forwarded-For": {"10.0.0.1"}}, clientAddr},
		{"garbage falls back to loopback", "127.0.0.1:5000", http.Header{"X-Forwarded-For": {"not-an-ip"}}, "127.0.0.1"},
		{"bad CF header falls back to X-Forwarded-For", "127.0.0.1:5000", http.Header{"Cf-Connecting-Ip": {"junk"}, "X-Forwarded-For": {clientAddr}}, clientAddr},
		{"IPv4-mapped address", "127.0.0.1:5000", http.Header{"X-Forwarded-For": {"::ffff:" + clientAddr}}, clientAddr},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			for k, v := range c.header {
				r.Header[k] = v
			}
			if got := clientIP(r); got != c.want {
				t.Errorf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}

func TestViaTunnel(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		header http.Header
		public bool
		want   bool
	}{
		{"loopback, nothing special", "127.0.0.1:1", nil, false, false},
		{"loopback while a public tunnel is up", "127.0.0.1:1", nil, true, true},
		{"loopback with forwarding headers", "127.0.0.1:1", http.Header{"X-Forwarded-For": {clientAddr}}, false, true},
		{"loopback with X-Forwarded-Proto only", "127.0.0.1:1", http.Header{"X-Forwarded-Proto": {"https"}}, false, true},
		{"LAN peer while a public tunnel is up", "192.168.1.9:1", nil, true, false},
		{"LAN peer with forged forwarding headers", "192.168.1.9:1", http.Header{"X-Forwarded-For": {"127.0.0.1"}, "Cf-Connecting-Ip": {"127.0.0.1"}}, true, false},
		{"unparseable peer", "garbage", http.Header{"X-Forwarded-For": {clientAddr}}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &api{remote: &fakeRemote{public: c.public}}
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			for k, v := range c.header {
				r.Header[k] = v
			}
			if got := a.viaTunnel(r); got != c.want {
				t.Errorf("viaTunnel = %v, want %v", got, c.want)
			}
		})
	}
}

func TestRequestScheme(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		proto  string
		want   string
	}{
		{"plain", "192.168.1.9:1", "", "http"},
		{"tunnel says https", "127.0.0.1:1", "https", "https"},
		{"any case", "127.0.0.1:1", "HTTPS", "https"},
		{"LAN peer cannot claim https", "192.168.1.9:1", "https", "http"},
		{"tunnel says http", "127.0.0.1:1", "http", "http"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			if c.proto != "" {
				r.Header.Set("X-Forwarded-Proto", c.proto)
			}
			if got := requestScheme(r); got != c.want {
				t.Errorf("requestScheme = %q, want %q", got, c.want)
			}
		})
	}
}
