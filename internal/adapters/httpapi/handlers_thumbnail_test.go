package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
)

type fakeThumbs struct {
	mu        sync.Mutex
	available bool
	byID      map[media.ID]thumbnails.Thumbnail
	err       error
	asked     []media.ID
}

func (f *fakeThumbs) Available() bool { return f.available }

func (f *fakeThumbs) Get(_ context.Context, id media.ID) (thumbnails.Thumbnail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, id)
	if f.err != nil {
		return thumbnails.Thumbnail{}, f.err
	}
	if th, ok := f.byID[id]; ok {
		return th, nil
	}
	return thumbnails.Thumbnail{}, fmt.Errorf("get %q: %w", id, media.ErrNotFound)
}

var thumbModTime = time.Date(2024, 3, 5, 12, 0, 0, 0, time.UTC)

// newThumbFixture is the usual fixture plus a second server whose API serves
// thumbnails from the fake, so both the route and the access rules are real.
func newThumbFixture(t *testing.T, access Access, th *fakeThumbs) (*fixture, *httptest.Server) {
	t.Helper()
	f := newFixtureWith(t, access)
	a := New(f.lib, access, &fakeRemote{}, "1.2.3", discardLog(), WithThumbnails(th)).
		newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func getFrom(t *testing.T, srv *httptest.Server, path string, headers ...string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestThumbnail_ServesTheJPEG(t *testing.T) {
	th := &fakeThumbs{available: true, byID: map[media.ID]thumbnails.Thumbnail{}}
	f, srv := newThumbFixture(t, &fakeAccess{}, th)
	th.byID[f.movie.ID] = thumbnails.Thumbnail{JPEG: []byte("\xff\xd8jpeg"), ModTime: thumbModTime}

	resp := getFrom(t, srv, "/thumb/"+string(f.movie.ID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("Cache-Control = %q, want private (it sits behind a sign-in)", got)
	}
	if !bytes.Equal(readBody(t, resp), []byte("\xff\xd8jpeg")) {
		t.Error("body is not the thumbnail")
	}
}

func TestThumbnail_RevalidatesCheaply(t *testing.T) {
	th := &fakeThumbs{available: true, byID: map[media.ID]thumbnails.Thumbnail{}}
	f, srv := newThumbFixture(t, &fakeAccess{}, th)
	th.byID[f.movie.ID] = thumbnails.Thumbnail{JPEG: []byte("\xff\xd8jpeg"), ModTime: thumbModTime}
	path := "/thumb/" + string(f.movie.ID)

	first := getFrom(t, srv, path)
	lastModified := first.Header.Get("Last-Modified")
	if lastModified == "" {
		t.Fatal("no Last-Modified header")
	}
	again := getFrom(t, srv, path, "If-Modified-Since", lastModified)
	if again.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", again.StatusCode)
	}
}

func TestThumbnail_NothingToShowIsANotFound(t *testing.T) {
	for name, err := range map[string]error{
		"no picture": fmt.Errorf("x: %w", app.ErrNoThumbnail),
		"no ffmpeg":  app.ErrThumbnailsUnavailable,
		"unknown id": fmt.Errorf("x: %w", media.ErrNotFound),
	} {
		t.Run(name, func(t *testing.T) {
			th := &fakeThumbs{available: true, err: err}
			_, srv := newThumbFixture(t, &fakeAccess{}, th)
			assertErrorShape(t, getFrom(t, srv, "/thumb/whatever"), http.StatusNotFound, codeNotFound)
		})
	}
}

func TestThumbnail_AnUnexpectedFailureIsA500WithoutDetails(t *testing.T) {
	th := &fakeThumbs{available: true, err: errors.New("disk exploded at /secret/path")}
	_, srv := newThumbFixture(t, &fakeAccess{}, th)

	resp := getFrom(t, srv, "/thumb/x")
	body := string(readBody(t, resp))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if bytes.Contains([]byte(body), []byte("/secret/path")) {
		t.Errorf("body leaks the internal error: %s", body)
	}
}

func TestThumbnail_NotConfiguredIsANotFound(t *testing.T) {
	f := newFixture(t) // built without WithThumbnails
	assertErrorShape(t, f.get("/thumb/"+string(f.movie.ID)), http.StatusNotFound, codeNotFound)
}

func TestThumbnail_SitsBehindTheAccessToken(t *testing.T) {
	access := &fakeAccess{}
	access.set(true, "secret")
	th := &fakeThumbs{available: true, byID: map[media.ID]thumbnails.Thumbnail{}}
	f, srv := newThumbFixture(t, access, th)
	th.byID[f.movie.ID] = thumbnails.Thumbnail{JPEG: []byte("\xff\xd8jpeg"), ModTime: thumbModTime}
	path := "/thumb/" + string(f.movie.ID)

	if got := getFrom(t, srv, path).StatusCode; got != http.StatusUnauthorized {
		t.Errorf("without the token: status = %d, want 401", got)
	}
	if len(th.asked) != 0 {
		t.Error("the thumbnail service was reached without the token")
	}
	if got := getFrom(t, srv, path, "Authorization", "Bearer secret").StatusCode; got != http.StatusOK {
		t.Errorf("with the token: status = %d, want 200", got)
	}
}

func TestThumbnail_CannotBeAskedForAPath(t *testing.T) {
	th := &fakeThumbs{available: true}
	_, srv := newThumbFixture(t, &fakeAccess{}, th)

	for _, path := range []string{"/thumb/../../etc/passwd", "/thumb/%2e%2e%2fetc%2fpasswd", "/thumb/a/b"} {
		resp := getFrom(t, srv, path)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMovedPermanently {
			t.Errorf("GET %s status = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestHealth_SaysWhetherThumbnailsWork(t *testing.T) {
	for _, tt := range []struct {
		name  string
		thumb *fakeThumbs
		want  bool
	}{
		{"working", &fakeThumbs{available: true}, true},
		{"no ffmpeg", &fakeThumbs{available: false}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, srv := newThumbFixture(t, &fakeAccess{}, tt.thumb)
			var body healthJSON
			if err := json.Unmarshal(readBody(t, getFrom(t, srv, "/api/health")), &body); err != nil {
				t.Fatal(err)
			}
			if body.Thumbnails != tt.want {
				t.Errorf("thumbnails = %v, want %v", body.Thumbnails, tt.want)
			}
		})
	}
}
