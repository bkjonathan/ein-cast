package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

// syncBuffer is a bytes.Buffer safe for the server goroutine to write while the
// test reads: the access log line is written after the response is sent.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fakeLibrary struct {
	root  string
	items []media.Item
}

func (f *fakeLibrary) List() []media.Item { return f.items }
func (f *fakeLibrary) Root() string       { return f.root }
func (f *fakeLibrary) Get(id media.ID) (media.Item, error) {
	for _, it := range f.items {
		if it.ID == id {
			return it, nil
		}
	}
	return media.Item{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
}

// fixture is a temp library on disk plus a running test server over it.
type fixture struct {
	t       *testing.T
	root    string
	lib     *fakeLibrary
	srv     *httptest.Server
	logs    *syncBuffer
	movie   media.Item
	movieBy []byte
}

func writeFile(t *testing.T, root, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func itemFor(t *testing.T, root, rel string, kind media.Kind) media.Item {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return media.Item{
		ID: media.NewID(rel), Name: filepath.Base(rel), RelPath: rel,
		Path: p, Kind: kind, Size: info.Size(), ModTime: info.ModTime(),
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()

	movieBytes := make([]byte, 10_000)
	for i := range movieBytes {
		movieBytes[i] = byte(i % 251)
	}
	writeFile(t, root, "movies/film.mkv", movieBytes)
	writeFile(t, root, "music/song.mp3", []byte("mp3-bytes"))
	writeFile(t, root, "photos/pic.jpg", []byte("jpg-bytes"))

	f := &fixture{t: t, root: root, logs: &syncBuffer{}, movieBy: movieBytes}
	f.movie = itemFor(t, root, "movies/film.mkv", media.KindVideo)
	f.lib = &fakeLibrary{root: root, items: []media.Item{
		f.movie,
		itemFor(t, root, "music/song.mp3", media.KindAudio),
		itemFor(t, root, "photos/pic.jpg", media.KindImage),
	}}

	log := slog.New(slog.NewJSONHandler(f.logs, nil))
	// Build through the real constructor so the test sees what production logs,
	// including the source=http tag.
	a := New(f.lib, "1.2.3", log).newAPI(app.ServeConfig{Name: "Den"})
	f.srv = httptest.NewServer(a.handler())
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) get(path string, headers ...string) *http.Response {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertErrorShape(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d", resp.StatusCode, status)
	}
	var body errorBody
	if err := json.Unmarshal(readBody(t, resp), &body); err != nil {
		t.Fatalf("error body is not the standard JSON shape: %v", err)
	}
	if body.Error.Code != code || body.Error.Message == "" {
		t.Fatalf("error = %+v, want code %q and a message", body.Error, code)
	}
}

func TestHealth(t *testing.T) {
	f := newFixture(t)
	resp := f.get("/api/health")

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body healthJSON
	if err := json.Unmarshal(readBody(t, resp), &body); err != nil {
		t.Fatal(err)
	}
	if body != (healthJSON{Status: "ok", Version: "1.2.3", Name: "Den"}) {
		t.Errorf("body = %+v", body)
	}
	if got := resp.Header.Get("Server"); got != "EinCast/1.2.3" {
		t.Errorf("Server header = %q, want EinCast/1.2.3", got)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("missing X-Request-ID")
	}
}

func TestLibrary_ListHidesInternalFields(t *testing.T) {
	f := newFixture(t)
	resp := f.get("/api/library")

	var items []map[string]any
	if err := json.Unmarshal(readBody(t, resp), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	for _, it := range items {
		keys := make([]string, 0, len(it))
		for k := range it {
			keys = append(keys, k)
		}
		for _, want := range []string{"id", "name", "kind", "size", "modTime"} {
			if _, ok := it[want]; !ok {
				t.Errorf("item missing %q: %v", want, it)
			}
		}
		if len(keys) != 5 {
			t.Errorf("item has keys %v; only id, name, kind, size, modTime may be exposed", keys)
		}
	}
	if strings.Contains(fmt.Sprint(items), f.root) {
		t.Error("response leaks the absolute library path")
	}
}

func TestLibrary_EmptyIsArrayNotNull(t *testing.T) {
	f := newFixture(t)
	f.lib.items = nil
	if got := strings.TrimSpace(string(readBody(t, f.get("/api/library")))); got != "[]" {
		t.Fatalf("body = %q, want []", got)
	}
}

func TestLibraryItem(t *testing.T) {
	f := newFixture(t)

	resp := f.get("/api/library/" + string(f.movie.ID))
	var got itemJSON
	if err := json.Unmarshal(readBody(t, resp), &got); err != nil || resp.StatusCode != 200 {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	if got.ID != f.movie.ID || got.Name != "film.mkv" || got.Kind != media.KindVideo || got.Size != 10_000 {
		t.Errorf("item = %+v", got)
	}

	assertErrorShape(t, f.get("/api/library/doesnotexist"), 404, "not_found")
}

func TestStream_FullFile(t *testing.T) {
	f := newFixture(t)
	resp := f.get("/stream/" + string(f.movie.ID))

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "video/x-matroska" {
		t.Errorf("Content-Type = %q, want video/x-matroska", got)
	}
	if got := resp.Header.Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", got)
	}
	if !bytes.Equal(readBody(t, resp), f.movieBy) {
		t.Error("body differs from the file")
	}
}

func TestStream_RangeRequests(t *testing.T) {
	f := newFixture(t)
	url := "/stream/" + string(f.movie.ID)

	tests := []struct {
		name        string
		rangeHeader string
		wantStatus  int
		wantRange   string
		wantBody    []byte
	}{
		{"middle slice", "bytes=10-19", 206, "bytes 10-19/10000", f.movieBy[10:20]},
		{"open ended", "bytes=9990-", 206, "bytes 9990-9999/10000", f.movieBy[9990:]},
		{"suffix", "bytes=-5", 206, "bytes 9995-9999/10000", f.movieBy[9995:]},
		{"past the end", "bytes=20000-", 416, "bytes */10000", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := f.get(url, "Range", tt.rangeHeader)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if got := resp.Header.Get("Content-Range"); got != tt.wantRange {
				t.Errorf("Content-Range = %q, want %q", got, tt.wantRange)
			}
			if tt.wantBody != nil && !bytes.Equal(readBody(t, resp), tt.wantBody) {
				t.Error("range body mismatch")
			}
		})
	}
}

func TestStream_Head(t *testing.T) {
	f := newFixture(t)
	req, _ := http.NewRequest(http.MethodHead, f.srv.URL+"/stream/"+string(f.movie.ID), nil)
	resp, err := f.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 || resp.ContentLength != 10_000 {
		t.Errorf("status=%d length=%d", resp.StatusCode, resp.ContentLength)
	}
	if len(readBody(t, resp)) != 0 {
		t.Error("HEAD returned a body")
	}
}

// The ID is the only thing a client controls. None of these may reach the disk.
func TestStream_PathTraversalIs404(t *testing.T) {
	f := newFixture(t)
	secret := writeFile(t, t.TempDir(), "secret.mkv", []byte("TOP-SECRET"))

	paths := []string{
		"/stream/../../etc/passwd",
		"/stream/..%2F..%2Fetc%2Fpasswd",
		"/stream/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"/stream/" + secret,
		"/stream/%2Fetc%2Fpasswd",
		"/stream/..%5C..%5Cwindows%5Cwin.ini",
		"/stream/movies/film.mkv", // a real relative path is not an ID
		"/stream/",
		"/stream",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp := f.get(p) // the client follows the mux's clean-path redirect
			body := readBody(t, resp)
			if resp.StatusCode != 404 {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
			if bytes.Contains(body, []byte("TOP-SECRET")) || bytes.Contains(body, []byte("root:")) {
				t.Fatal("response contains file contents")
			}
		})
	}
}

func TestStream_RefusesFilesOutsideRoot(t *testing.T) {
	f := newFixture(t)
	outside := writeFile(t, t.TempDir(), "secret.mkv", []byte("TOP-SECRET"))

	t.Run("item whose path was tampered", func(t *testing.T) {
		f.lib.items = append(f.lib.items, media.Item{ID: "bad", Name: "secret.mkv", Path: outside, Kind: media.KindVideo})
		resp := f.get("/stream/bad")
		if body := readBody(t, resp); resp.StatusCode != 404 || bytes.Contains(body, []byte("TOP-SECRET")) {
			t.Fatalf("status=%d body=%q", resp.StatusCode, body)
		}
	})

	t.Run("file swapped for a symlink after the scan", func(t *testing.T) {
		link := filepath.Join(f.root, "linked.mkv")
		if err := os.Symlink(outside, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		f.lib.items = append(f.lib.items, media.Item{ID: "link", Name: "linked.mkv", Path: link, Kind: media.KindVideo})
		resp := f.get("/stream/link")
		if body := readBody(t, resp); resp.StatusCode != 404 || bytes.Contains(body, []byte("TOP-SECRET")) {
			t.Fatalf("status=%d body=%q", resp.StatusCode, body)
		}
	})
}

func TestStream_FileDeletedSinceScan(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.movie.Path); err != nil {
		t.Fatal(err)
	}
	assertErrorShape(t, f.get("/stream/"+string(f.movie.ID)), 404, "not_found")
}

func TestStream_NoLibraryLoaded(t *testing.T) {
	f := newFixture(t)
	f.lib.root = ""
	assertErrorShape(t, f.get("/stream/"+string(f.movie.ID)), 404, "not_found")
}

func TestPlaylist(t *testing.T) {
	f := newFixture(t)
	tricky := writeFile(t, f.root, "weird\nname.flac", []byte("x"))
	f.lib.items = append(f.lib.items, media.Item{
		ID: "weird", Name: filepath.Base(tricky), Path: tricky, Kind: media.KindAudio,
	})

	resp := f.get("/playlist.m3u")
	body := string(readBody(t, resp))
	host := strings.TrimPrefix(f.srv.URL, "http://")

	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "audio/x-mpegurl" {
		t.Fatalf("status=%d content-type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if lines[0] != "#EXTM3U" {
		t.Errorf("first line = %q, want #EXTM3U", lines[0])
	}
	for _, want := range []string{
		"#EXTINF:-1,film",
		"http://" + host + "/stream/" + string(f.movie.ID),
		"#EXTINF:-1,song",
		"#EXTINF:-1,weird name", // newline in the file name was neutralised
	} {
		if !strings.Contains(body, want) {
			t.Errorf("playlist missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "pic") {
		t.Error("playlist includes an image; only video and audio belong in it")
	}
	// 1 header + 3 entries x 2 lines: the hostile name added no extra lines.
	if len(lines) != 7 {
		t.Errorf("playlist has %d lines, want 7:\n%s", len(lines), body)
	}
}

func TestIndexAndUnknownRoutes(t *testing.T) {
	f := newFixture(t)

	resp := f.get("/")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("/: status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	assertErrorShape(t, f.get("/nope"), 404, "not_found")
	assertErrorShape(t, f.get("/api/nope"), 404, "not_found")

	resp, err := f.srv.Client().Post(f.srv.URL+"/api/health", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 && resp.StatusCode != 405 {
		t.Errorf("POST /api/health status = %d, want a 4xx", resp.StatusCode)
	}
}

func TestAccessLogLineForStreamedBytes(t *testing.T) {
	f := newFixture(t)
	path := "/stream/" + string(f.movie.ID)
	resp := f.get(path)
	_ = readBody(t, resp)

	// The line is written after the response completes, so wait for it.
	var entry map[string]any
	deadline := time.Now().Add(2 * time.Second)
	for entry == nil && time.Now().Before(deadline) {
		for _, line := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
			var e map[string]any
			if json.Unmarshal([]byte(line), &e) == nil && e["path"] == path {
				entry = e
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if entry == nil {
		t.Fatalf("no access log line for %s in:\n%s", path, f.logs.String())
	}

	if entry["status"] != float64(200) || entry["bytes"] != float64(10_000) || entry["method"] != "GET" {
		t.Errorf("entry = %v", entry)
	}
	for _, key := range []string{"request_id", "duration", "client_ip", "source"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("access log missing %q: %v", key, entry)
		}
	}
	if entry["source"] != "http" {
		t.Errorf("source = %v, want http", entry["source"])
	}
	if entry["request_id"] != resp.Header.Get("X-Request-ID") {
		t.Errorf("logged request_id %v != header %q", entry["request_id"], resp.Header.Get("X-Request-ID"))
	}
}

func TestPlayerPage(t *testing.T) {
	f := newFixture(t)
	resp := f.get("/")
	body := string(readBody(t, resp))

	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{"<video", "/static/app.js", "/static/style.css", "/playlist.m3u", "EinCast"} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html is missing %q", want)
		}
	}
	csp := resp.Header.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "media-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q is missing %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP allows unsafe script execution: %q", csp)
	}
}

func TestStaticFiles(t *testing.T) {
	f := newFixture(t)

	tests := []struct {
		path     string
		status   int
		typePart string
	}{
		{"/static/app.js", 200, "javascript"},
		{"/static/style.css", 200, "text/css"},
		{"/static/icon.svg", 200, "image/svg+xml"},
		{"/static/missing.js", 404, "application/json"},
		{"/static/index.html", 404, "application/json"}, // the page lives at /
		{"/static/..%2Fserver.go", 404, ""},
		{"/static/%2e%2e%2fserver.go", 404, ""},
		{"/static/", 404, ""}, // no directory listing
		{"/static", 404, ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := f.get(tt.path)
			body := readBody(t, resp)
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d (body %.80q)", resp.StatusCode, tt.status, body)
			}
			if tt.typePart != "" && !strings.Contains(resp.Header.Get("Content-Type"), tt.typePart) {
				t.Errorf("Content-Type = %q, want it to contain %q", resp.Header.Get("Content-Type"), tt.typePart)
			}
			if tt.status == 404 && (bytes.Contains(body, []byte("package httpapi")) || bytes.Contains(body, []byte("<a href"))) {
				t.Error("404 response leaks a source file or a directory listing")
			}
		})
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	f := newFixture(t)
	for _, path := range []string{"/", "/api/health", "/api/library", "/nope", "/stream/" + string(f.movie.ID)} {
		resp := f.get(path)
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", path, got)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q", path, got)
		}
	}
}

// File names are attacker-influenced text (a downloaded file can be called
// "<img src=x onerror=...>.mp4"). The page must only ever insert them as text.
func TestPlayerJSNeverInjectsHTML(t *testing.T) {
	src, err := webAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "setAttribute('on", `setAttribute("on`} {
		if bytes.Contains(src, []byte(banned)) {
			t.Errorf("app.js uses %q, which can turn a file name into markup or script", banned)
		}
	}
}
