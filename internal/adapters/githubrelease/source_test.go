package githubrelease

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
)

const (
	sumMac = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	sumWin = "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
)

// fakeGitHub serves the two addresses the adapter may fetch and remembers every
// request it saw, so the tests can say exactly what left the computer.
type fakeGitHub struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []*http.Request

	status    int    // for the release call; 0 means 200
	body      string // for the release call; "" means the standard release
	sumsCode  int    // for the checksum call; 0 means 200
	sums      string
	releaseFn func(w http.ResponseWriter, r *http.Request)
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{t: t, sums: sumMac + "  eincast-1.1.2-macos-universal.zip\n" + strings.ToLower(sumWin) + " *eincast-1.1.2-windows-amd64.exe\n"}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(context.Background()))
		f.mu.Unlock()
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			if f.releaseFn != nil {
				f.releaseFn(w, r)
				return
			}
			if f.status != 0 {
				w.WriteHeader(f.status)
				return
			}
			body := f.body
			if body == "" {
				body = f.standard()
			}
			fmt.Fprint(w, body)
		case "/dl/SHA256SUMS.txt":
			if f.sumsCode != 0 {
				w.WriteHeader(f.sumsCode)
				return
			}
			fmt.Fprint(w, f.sums)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitHub) standard() string {
	return fmt.Sprintf(`{
	  "tag_name": "v1.1.2", "name": "EinCast 1.1.2", "body": "## What's new\r\n- a thing",
	  "html_url": "https://github.com/o/r/releases/tag/v1.1.2", "published_at": "2026-11-01T10:00:00Z",
	  "draft": false, "prerelease": false, "author": {"login": "someone"},
	  "assets": [
	    {"name": "eincast-1.1.2-macos-universal.zip", "browser_download_url": "https://x/mac", "size": 111},
	    {"name": "eincast-1.1.2-windows-amd64.exe", "browser_download_url": "https://x/win", "size": 222},
	    {"name": "eincast-1.1.2-windows-amd64-installer.exe", "browser_download_url": "https://x/inst", "size": 333},
	    {"name": "SHA256SUMS.txt", "browser_download_url": "%s/dl/SHA256SUMS.txt", "size": 300}
	  ]}`, f.srv.URL)
}

func (f *fakeGitHub) source() *Source {
	return New("o/r", WithBaseURL(f.srv.URL), WithHTTPClient(f.srv.Client()))
}

func (f *fakeGitHub) seen() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.requests...)
}

func TestLatest_ReadsTheReleaseAndItsChecksums(t *testing.T) {
	gh := newFakeGitHub(t)

	rel, err := gh.source().Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if rel.Tag != "v1.1.2" || rel.Name != "EinCast 1.1.2" || rel.URL != "https://github.com/o/r/releases/tag/v1.1.2" {
		t.Errorf("release = %+v", rel)
	}
	if want := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC); !rel.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt = %v", rel.PublishedAt)
	}
	if rel.Notes != "## What's new\n- a thing" {
		t.Errorf("Notes = %q", rel.Notes)
	}
	want := []app.ReleaseAsset{
		{Name: "eincast-1.1.2-macos-universal.zip", Size: 111, SHA256: sumMac},
		{Name: "eincast-1.1.2-windows-amd64.exe", Size: 222, SHA256: strings.ToLower(sumWin)}, // "*" and upper case tolerated
		{Name: "eincast-1.1.2-windows-amd64-installer.exe", Size: 333},                        // no line in the file: shown without one
	}
	if !reflect.DeepEqual(rel.Assets, want) {
		t.Errorf("Assets = %+v\nwant     %+v (the checksum file itself is not a download)", rel.Assets, want)
	}
}

func TestLatest_SendsNothingThatIdentifiesThePerson(t *testing.T) {
	gh := newFakeGitHub(t)
	if _, err := gh.source().Latest(context.Background()); err != nil {
		t.Fatal(err)
	}

	reqs := gh.seen()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want exactly 2 (the release, then its checksum file)", len(reqs))
	}
	for _, r := range reqs {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s: only GET is used", r.Method, r.URL)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("%s has a query string %q; nothing may be sent in it", r.URL.Path, r.URL.RawQuery)
		}
		if ua := r.Header.Get("User-Agent"); ua != "EinCast" {
			t.Errorf("User-Agent = %q, want the same fixed value for everyone, with no version", ua)
		}
		for _, h := range []string{"Authorization", "Cookie", "Referer", "X-Forwarded-For"} {
			if v := r.Header.Get(h); v != "" {
				t.Errorf("header %s = %q; none of these may be sent", h, v)
			}
		}
	}
	if got := reqs[0].Header.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q", got)
	}
}

func TestLatest_Failures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantNoRel  bool
		wantSubstr string
	}{
		{name: "no release yet", status: http.StatusNotFound, wantNoRel: true},
		{name: "rate limited", status: http.StatusForbidden, wantSubstr: "limiting requests"},
		{name: "too many requests", status: http.StatusTooManyRequests, wantSubstr: "limiting requests"},
		{name: "server error", status: http.StatusInternalServerError, wantSubstr: "HTTP 500"},
		{name: "not json", body: "<html>", wantSubstr: "could not be read"},
		{name: "json without a tag", body: `{"name": "x"}`, wantNoRel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gh := newFakeGitHub(t)
			gh.status, gh.body = tt.status, tt.body

			_, err := gh.source().Latest(context.Background())

			if err == nil {
				t.Fatal("expected an error")
			}
			if got := errors.Is(err, app.ErrNoRelease); got != tt.wantNoRel {
				t.Errorf("errors.Is(ErrNoRelease) = %v, want %v (err: %v)", got, tt.wantNoRel, err)
			}
			if tt.wantSubstr != "" && !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantSubstr)
			}
		})
	}
}

func TestLatest_AMissingChecksumFileOnlyMeansNoChecksums(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.sumsCode = http.StatusNotFound

	rel, err := gh.source().Latest(context.Background())
	if err != nil {
		t.Fatalf("a checksum file that cannot be read must not fail the check: %v", err)
	}
	if rel.Tag != "v1.1.2" || len(rel.Assets) != 3 {
		t.Fatalf("release = %+v", rel)
	}
	for _, a := range rel.Assets {
		if a.SHA256 != "" {
			t.Errorf("%s has checksum %q from a file that was not served", a.Name, a.SHA256)
		}
	}
}

func TestLatest_DoesNotFollowAChecksumAddressThatIsNotHTTPS(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.body = `{"tag_name": "v1.1.2", "assets": [
	  {"name": "SHA256SUMS.txt", "browser_download_url": "http://169.254.169.254/latest/meta-data", "size": 1},
	  {"name": "a.zip", "browser_download_url": "https://x/a", "size": 1}]}`

	rel, err := gh.source().Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := len(gh.seen()); n != 1 {
		t.Fatalf("%d requests; the plain-http address must not be fetched", n)
	}
	if len(rel.Assets) != 1 || rel.Assets[0].SHA256 != "" {
		t.Errorf("assets = %+v", rel.Assets)
	}
}

func TestLatest_ALongDescriptionIsCut(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.body = fmt.Sprintf(`{"tag_name": "v1.1.2", "body": %q}`, strings.Repeat("é", maxNotes+500))

	rel, err := gh.source().Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := []rune(rel.Notes); len(got) != maxNotes+1 || got[len(got)-1] != '…' {
		t.Fatalf("notes are %d runes, want %d plus a mark", len(got), maxNotes)
	}
}

func TestLatest_ATimeoutIsAContextError(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.releaseFn = func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := gh.source().Latest(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want one that matches context.DeadlineExceeded, so the app can say \"did not answer in time\"", err)
	}
	if strings.Contains(err.Error(), "https://") {
		t.Errorf("error %q shows an address", err)
	}
}

func TestLatest_AnUnreachableServerHidesTheAddress(t *testing.T) {
	gh := newFakeGitHub(t)
	src := gh.source()
	gh.srv.Close()

	_, err := src.Latest(context.Background())

	if err == nil || !strings.HasPrefix(err.Error(), "GitHub could not be reached") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "/repos/") {
		t.Errorf("error %q shows the request address", err)
	}
}

func TestParseChecksums(t *testing.T) {
	data := strings.Join([]string{
		sumMac + "  a.zip",
		strings.ToLower(sumWin) + " *b.exe",
		"",
		"# a comment",
		"abc123  too-short.zip",
		sumMac + "  ",
		strings.Repeat("g", 64) + "  not-hex.zip",
		"  " + sumMac + "  spaced name.zip  ",
	}, "\n")

	got := parseChecksums([]byte(data))

	want := map[string]string{"a.zip": sumMac, "b.exe": strings.ToLower(sumWin), "spaced name.zip": sumMac}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseChecksums = %v, want %v", got, want)
	}
}
