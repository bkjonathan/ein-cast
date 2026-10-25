package httpapi

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

const goodToken = "good-token_123"

func newSecured(t *testing.T) (*fixture, *fakeAccess) {
	t.Helper()
	acc := &fakeAccess{required: true, token: goodToken}
	return newFixtureWith(t, acc), acc
}

// do sends a request without following redirects, so redirects can be inspected.
func (f *fixture) do(method, path string, mod func(*http.Request)) *http.Response {
	f.t.Helper()
	req, err := http.NewRequest(method, f.srv.URL+path, nil)
	if err != nil {
		f.t.Fatal(err)
	}
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

func TestAccess_OffMeansOpen(t *testing.T) {
	f := newFixture(t)
	if resp := f.get("/api/library"); resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200 with access off", resp.StatusCode)
	}
}

func TestAccess_RequestsWithoutAValidTokenAreRefused(t *testing.T) {
	f, _ := newSecured(t)
	paths := []string{"/api/health", "/api/library", "/api/library/" + string(f.movie.ID), "/stream/" + string(f.movie.ID), "/playlist.m3u", "/nope"}

	creds := map[string]func(*http.Request){
		"no token":          nil,
		"wrong query":       func(r *http.Request) { r.URL.RawQuery = "t=wrong" },
		"wrong bearer":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
		"wrong cookie":      func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: "wrong"}) },
		"empty query":       func(r *http.Request) { r.URL.RawQuery = "t=" },
		"token as a prefix": func(r *http.Request) { r.URL.RawQuery = "t=" + goodToken[:5] },
		"token with suffix": func(r *http.Request) { r.URL.RawQuery = "t=" + goodToken + "x" },
		"wrong scheme":      func(r *http.Request) { r.Header.Set("Authorization", "Basic "+goodToken) },
		"token in a header": func(r *http.Request) { r.Header.Set("X-Token", goodToken) },
		"wrong query param": func(r *http.Request) { r.URL.RawQuery = "token=" + goodToken },
	}
	for name, mod := range creds {
		for _, path := range paths {
			t.Run(name+" "+path, func(t *testing.T) {
				resp := f.do("GET", path, mod)
				assertErrorShape(t, resp, 401, "unauthorized")
				if resp.Header.Get("WWW-Authenticate") == "" {
					t.Error("401 without WWW-Authenticate")
				}
			})
		}
	}
}

func TestAccess_ValidTokenFromEachSource(t *testing.T) {
	f, _ := newSecured(t)
	sources := map[string]func(*http.Request){
		"query":             func(r *http.Request) { r.URL.RawQuery = "t=" + goodToken },
		"bearer":            func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+goodToken) },
		"bearer (any case)": func(r *http.Request) { r.Header.Set("Authorization", "bEaReR "+goodToken) },
		"cookie":            func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: goodToken}) },
		"stale query, good cookie": func(r *http.Request) {
			r.URL.RawQuery = "t=old-rotated-token"
			r.AddCookie(&http.Cookie{Name: cookieName, Value: goodToken})
		},
	}
	for name, mod := range sources {
		t.Run(name, func(t *testing.T) {
			if resp := f.do("GET", "/api/library", mod); resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

func TestAccess_RefusalMessageDoesNotRevealWhetherATokenWasClose(t *testing.T) {
	f, _ := newSecured(t)
	a := readBody(t, f.do("GET", "/api/library", nil))
	b := readBody(t, f.do("GET", "/api/library", func(r *http.Request) { r.URL.RawQuery = "t=" + goodToken[:len(goodToken)-1] }))
	if string(a) != string(b) {
		t.Errorf("missing-token and wrong-token bodies differ:\n%s\n%s", a, b)
	}
}

func TestAccess_StaticFilesStayPublic(t *testing.T) {
	f, _ := newSecured(t)
	for _, p := range []string{"/static/app.js", "/static/style.css", "/static/icon.svg"} {
		if resp := f.do("GET", p, nil); resp.StatusCode != 200 {
			t.Errorf("%s: status = %d, want 200 (static files hold nothing private)", p, resp.StatusCode)
		}
	}
}

func TestAccess_PageWithoutTokenExplainsWhatToDo(t *testing.T) {
	f, _ := newSecured(t)
	resp := f.do("GET", "/", func(r *http.Request) { r.Header.Set("Accept", "text/html,application/xhtml+xml") })
	body := string(readBody(t, resp))

	if resp.StatusCode != 401 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(body, "Access token needed") || !strings.Contains(body, "QR code") {
		t.Errorf("body does not tell the visitor what to do:\n%s", body)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Errorf("CSP = %q, want a script-free lockdown", csp)
	}
	if strings.Contains(body, "<script") {
		t.Error("the unauthorized page must not contain script")
	}
}

func TestAccess_QRLinkSignsTheBrowserInAndHidesTheToken(t *testing.T) {
	f, _ := newSecured(t)

	// Step 1: the QR link. A redirect that sets the cookie and drops the token.
	resp := f.do("GET", "/?t="+goodToken, func(r *http.Request) { r.Header.Set("Accept", "text/html") })
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("status=%d Location=%q, want 303 to /", resp.StatusCode, resp.Header.Get("Location"))
	}
	var ck *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			ck = c
		}
	}
	if ck == nil || ck.Value != goodToken {
		t.Fatalf("no access cookie set: %v", resp.Cookies())
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.MaxAge <= 0 {
		t.Errorf("cookie = %+v, want HttpOnly, SameSite=Lax, Path=/, with a lifetime", ck)
	}
	if strings.Contains(resp.Header.Get("Location"), goodToken) {
		t.Error("the redirect leaks the token into the address bar")
	}

	// Step 2: a real cookie jar follows it, then keeps working with no token in any URL.
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	page, err := client.Get(f.srv.URL + "/?t=" + goodToken)
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.StatusCode != 200 || page.Request.URL.RawQuery != "" {
		t.Fatalf("after the redirect: status=%d url=%s", page.StatusCode, page.Request.URL)
	}
	for _, p := range []string{"/api/library", "/api/health", "/stream/" + string(f.movie.ID)} {
		r, err := client.Get(f.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Errorf("%s with the cookie: status = %d, want 200", p, r.StatusCode)
		}
	}
}

func TestAccess_OnlyThePageRedirects(t *testing.T) {
	f, _ := newSecured(t)
	// VLC-style request: token in the URL on a non-page path is simply served.
	resp := f.do("GET", "/stream/"+string(f.movie.ID)+"?t="+goodToken, func(r *http.Request) { r.Header.Set("Range", "bytes=0-9") })
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if len(resp.Cookies()) != 0 {
		t.Error("a stream request must not set cookies")
	}
}

func TestAccess_PlaylistCarriesTheTokenForPlayersWithoutCookies(t *testing.T) {
	f, _ := newSecured(t)
	host := strings.TrimPrefix(f.srv.URL, "http://")

	if resp := f.do("GET", "/playlist.m3u", nil); resp.StatusCode != 401 {
		t.Fatalf("playlist without a token: status = %d, want 401", resp.StatusCode)
	}

	body := string(readBody(t, f.do("GET", "/playlist.m3u?t="+goodToken, nil)))
	want := "http://" + host + "/stream/" + string(f.movie.ID) + "?t=" + url.QueryEscape(goodToken)
	if !strings.Contains(body, want) {
		t.Errorf("playlist lacks an authenticated entry %q:\n%s", want, body)
	}

	// Each entry, fetched exactly as VLC would (no cookie, no header), works.
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "http://") {
			continue
		}
		resp, err := http.Get(line)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("playlist entry %s: status = %d", line, resp.StatusCode)
		}
	}
}

func TestAccess_PlaylistEscapesAwkwardTokens(t *testing.T) {
	odd := "a+b/c=d e&f#g"
	f := newFixtureWith(t, &fakeAccess{required: true, token: odd})

	resp := f.do("GET", "/playlist.m3u", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+odd) })
	body := string(readBody(t, resp))
	if strings.Contains(body, odd) || !strings.Contains(body, url.QueryEscape(odd)) {
		t.Fatalf("token is not URL-escaped in the playlist:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "http://") {
			r, err := http.Get(line)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			if r.StatusCode != 200 {
				t.Errorf("escaped entry rejected: %d (%s)", r.StatusCode, line)
			}
		}
	}
}

func TestAccess_PlaylistHasNoTokenWhenAccessIsOff(t *testing.T) {
	f := newFixture(t)
	if body := string(readBody(t, f.get("/playlist.m3u"))); strings.Contains(body, "?t=") {
		t.Errorf("open-access playlist carries a token parameter:\n%s", body)
	}
}

func TestAccess_CanBeSwitchedAtRuntime(t *testing.T) {
	f, acc := newSecured(t)
	if f.do("GET", "/api/library", nil).StatusCode != 401 {
		t.Fatal("setup: expected 401")
	}
	acc.set(false, "")
	if f.do("GET", "/api/library", nil).StatusCode != 200 {
		t.Error("turning access off must take effect without restarting the server")
	}
	acc.set(true, "rotated")
	if f.do("GET", "/api/library", func(r *http.Request) { r.URL.RawQuery = "t=" + goodToken }).StatusCode != 401 {
		t.Error("the previous token must stop working after rotation")
	}
	if f.do("GET", "/api/library", func(r *http.Request) { r.URL.RawQuery = "t=rotated" }).StatusCode != 200 {
		t.Error("the new token must work")
	}
}

func TestAccess_PreflightIsAnsweredBeforeAuthentication(t *testing.T) {
	f, _ := newSecured(t)
	resp := f.do("OPTIONS", "/api/library", func(r *http.Request) {
		r.Header.Set("Origin", "http://192.168.1.50:3000")
		r.Header.Set("Access-Control-Request-Method", "GET")
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204: browsers cannot send credentials on a preflight", resp.StatusCode)
	}
}

// The token travels in URLs, so nothing may write the query string to the log.
func TestAccess_TokenNeverReachesTheLog(t *testing.T) {
	f, _ := newSecured(t)
	secrets := []string{goodToken, "WRONG-TOKEN-9876", "COOKIE-TOKEN-5555", "BEARER-TOKEN-4444"}

	f.do("GET", "/api/library?t="+goodToken, nil)
	f.do("GET", "/api/library?t=WRONG-TOKEN-9876", nil)
	f.do("GET", "/playlist.m3u?t="+goodToken, nil)
	f.do("GET", "/api/library", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: "COOKIE-TOKEN-5555"}) })
	f.do("GET", "/api/library", func(r *http.Request) { r.Header.Set("Authorization", "Bearer BEARER-TOKEN-4444") })

	deadline := time.Now().Add(2 * time.Second)
	for strings.Count(f.logs.String(), `"msg":"request"`) < 5 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	logs := f.logs.String()
	if !strings.Contains(logs, "access denied") {
		t.Fatalf("expected refusals to be logged:\n%s", logs)
	}
	for _, s := range secrets {
		if strings.Contains(logs, s) {
			t.Errorf("the log contains the secret %q:\n%s", s, logs)
		}
	}
}

func TestHostGuard_BlocksDNSRebinding(t *testing.T) {
	f, _ := newSecured(t)
	// A valid token does not help: the guard runs first, on the Host alone.
	for _, host := range []string{"evil.example.com", "192.168.1.20.evil.com", "localhost.evil.com", "attacker.net:8765", "xn--e1afmkfd.xn--p1ai"} {
		t.Run(host, func(t *testing.T) {
			resp := f.do("GET", "/api/library?t="+goodToken, func(r *http.Request) { r.Host = host })
			assertErrorShape(t, resp, 403, "forbidden")
		})
	}
	// An ordinary request still works.
	if f.do("GET", "/api/library?t="+goodToken, nil).StatusCode != 200 {
		t.Error("a request with the real Host was refused")
	}
}

func TestIsLocalHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"", true},
		{"192.168.1.20", true},
		{"192.168.1.20:8765", true},
		{"127.0.0.1:8765", true},
		{"10.0.0.7", true},
		{"[::1]:8765", true},
		{"[::1]", true},
		{"::1", true},
		{"[fe80::1]:80", true},
		{"localhost", true},
		{"LOCALHOST:8765", true},
		{"my-macbook", true},
		{"DESKTOP-4F2K9:8765", true},
		{"eincast.local", true},
		{"MacBook-Pro.local:8765", true},
		{"eincast.local.", true},
		{"evil.com", false},
		{"evil.com:8765", false},
		{"192.168.1.20.evil.com", false},
		{"localhost.evil.com", false},
		{"local", true}, // a single label
		{"evil.local.com", false},
		{"example.org.", false},
		{".", false},
		{"[", false}, // nothing left once the brackets are trimmed: refuse rather than guess
	}
	for _, tt := range tests {
		if got := isLocalHost(tt.host); got != tt.want {
			t.Errorf("isLocalHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
