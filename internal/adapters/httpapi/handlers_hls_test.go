package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"eincast/internal/app"
	"eincast/internal/app/transcode"
	"eincast/internal/domain/media"
)

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

func rsc(s string) io.ReadSeekCloser { return nopCloser{bytes.NewReader([]byte(s))} }

type fakeHLS struct {
	mu        sync.Mutex
	available bool
	playlist  string
	segments  map[string]string
	err       error // returned by Playlist
	fileErr   error // returned by File
	asked     []string
}

func (f *fakeHLS) Available() bool { return f.available }

func (f *fakeHLS) Playlist(_ context.Context, id media.ID) (io.ReadSeekCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, "playlist:"+string(id))
	if f.err != nil {
		return nil, f.err
	}
	return rsc(f.playlist), nil
}

func (f *fakeHLS) File(id media.ID, name string) (io.ReadSeekCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, "file:"+string(id)+"/"+name)
	if f.fileErr != nil {
		return nil, f.fileErr
	}
	body, ok := f.segments[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return rsc(body), nil
}

func newHLSFixture(t *testing.T, access Access, h *fakeHLS) (*fixture, *httptest.Server) {
	t.Helper()
	f := newFixtureWith(t, access)
	a := New(f.lib, access, &fakeRemote{}, "1.2.3", discardLog(), WithTranscoding(h)).
		newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestHLS_ServesThePlaylistAndSegments(t *testing.T) {
	h := &fakeHLS{available: true, playlist: "#EXTM3U\n#EXTINF:6,\nseg00000.ts\n", segments: map[string]string{"seg00000.ts": "ts-bytes"}}
	f, srv := newHLSFixture(t, &fakeAccess{}, h)
	base := "/hls/" + string(f.movie.ID) + "/"

	pl := getFrom(t, srv, base+"index.m3u8")
	if pl.StatusCode != http.StatusOK {
		t.Fatalf("playlist status = %d", pl.StatusCode)
	}
	if got := pl.Header.Get("Content-Type"); got != "application/vnd.apple.mpegurl" {
		t.Errorf("playlist Content-Type = %q", got)
	}
	if got := pl.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("playlist Cache-Control = %q: it grows while converting, so it must never be reused", got)
	}
	if got := string(readBody(t, pl)); !strings.Contains(got, "seg00000.ts") {
		t.Errorf("playlist = %q", got)
	}

	seg := getFrom(t, srv, base+"seg00000.ts")
	if seg.StatusCode != http.StatusOK || seg.Header.Get("Content-Type") != "video/mp2t" {
		t.Fatalf("segment status = %d, type = %q", seg.StatusCode, seg.Header.Get("Content-Type"))
	}
	if got := string(readBody(t, seg)); got != "ts-bytes" {
		t.Errorf("segment = %q", got)
	}
}

func TestHLS_SegmentsSupportRangeRequests(t *testing.T) {
	h := &fakeHLS{available: true, segments: map[string]string{"seg00000.ts": "0123456789"}}
	f, srv := newHLSFixture(t, &fakeAccess{}, h)

	resp := getFrom(t, srv, "/hls/"+string(f.movie.ID)+"/seg00000.ts", "Range", "bytes=2-5")
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", resp.StatusCode)
	}
	if got := string(readBody(t, resp)); got != "2345" {
		t.Errorf("body = %q", got)
	}
}

func TestHLS_ErrorsMapToStatuses(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unknown video", fmt.Errorf("x: %w", media.ErrNotFound), http.StatusNotFound, codeNotFound},
		{"no ffmpeg", app.ErrTranscodingUnavailable, http.StatusNotFound, codeNotFound},
		{"all slots busy", transcode.ErrBusy, http.StatusServiceUnavailable, codeBusy},
		{"cannot convert", fmt.Errorf("%w: Invalid data at /secret/path", app.ErrNotConvertible), http.StatusUnprocessableEntity, codeNotConvert},
		{"something else", errors.New("disk exploded at /secret/path"), http.StatusInternalServerError, codeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &fakeHLS{available: true, err: tt.err}
			f, srv := newHLSFixture(t, &fakeAccess{}, h)
			resp := getFrom(t, srv, "/hls/"+string(f.movie.ID)+"/index.m3u8")
			body := string(readBody(t, resp))
			if resp.StatusCode != tt.status || !strings.Contains(body, `"code":"`+tt.code+`"`) {
				t.Fatalf("status = %d body = %s, want %d %s", resp.StatusCode, body, tt.status, tt.code)
			}
			if strings.Contains(body, "/secret/path") {
				t.Errorf("the body leaks internal detail: %s", body)
			}
			if tt.status == http.StatusServiceUnavailable && resp.Header.Get("Retry-After") == "" {
				t.Error("a busy answer should say when to retry")
			}
		})
	}
}

func TestHLS_ASegmentOfAConversionThatIsGoneIsANotFound(t *testing.T) {
	for _, err := range []error{transcode.ErrNotStarted, fs.ErrNotExist} {
		h := &fakeHLS{available: true, fileErr: err}
		f, srv := newHLSFixture(t, &fakeAccess{}, h)
		assertErrorShape(t, getFrom(t, srv, "/hls/"+string(f.movie.ID)+"/seg00003.ts"), http.StatusNotFound, codeNotFound)
	}
}

func TestHLS_OnlyThePlaylistStartsAConversion(t *testing.T) {
	h := &fakeHLS{available: true, playlist: "#EXTM3U\n", segments: map[string]string{}}
	f, srv := newHLSFixture(t, &fakeAccess{}, h)
	id := string(f.movie.ID)

	getFrom(t, srv, "/hls/"+id+"/seg00000.ts")
	getFrom(t, srv, "/hls/"+id+"/anything-else")
	if len(h.asked) != 2 || strings.HasPrefix(h.asked[0], "playlist:") || strings.HasPrefix(h.asked[1], "playlist:") {
		t.Fatalf("asked = %v: a segment request must never start a conversion", h.asked)
	}
}

func TestHLS_NotConfiguredIsANotFound(t *testing.T) {
	f := newFixture(t)
	assertErrorShape(t, f.get("/hls/"+string(f.movie.ID)+"/index.m3u8"), http.StatusNotFound, codeNotFound)
}

func TestHLS_SitsBehindTheAccessToken(t *testing.T) {
	access := &fakeAccess{}
	access.set(true, "secret")
	h := &fakeHLS{available: true, playlist: "#EXTM3U\n"}
	f, srv := newHLSFixture(t, access, h)
	path := "/hls/" + string(f.movie.ID) + "/index.m3u8"

	if got := getFrom(t, srv, path).StatusCode; got != http.StatusUnauthorized {
		t.Errorf("without the token: status = %d, want 401", got)
	}
	if len(h.asked) != 0 {
		t.Error("a conversion was started without the token")
	}
	if got := getFrom(t, srv, path, "Authorization", "Bearer secret").StatusCode; got != http.StatusOK {
		t.Errorf("with the token: status = %d, want 200", got)
	}
}

func TestHealth_SaysWhetherConversionWorks(t *testing.T) {
	for _, available := range []bool{true, false} {
		_, srv := newHLSFixture(t, &fakeAccess{}, &fakeHLS{available: available})
		body := string(readBody(t, getFrom(t, srv, "/api/health")))
		want := fmt.Sprintf(`"transcoding":%v`, available)
		if !strings.Contains(body, want) {
			t.Errorf("health = %s, want %s", body, want)
		}
	}
}
