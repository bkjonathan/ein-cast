package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

const sampleSRT = "1\r\n00:00:01,000 --> 00:00:02,500\r\nHello\r\n\r\n2\r\n00:00:03,000 --> 00:00:04,000\r\nWorld\r\n"

// newSubtitleFixture serves a library holding one video with two tracks.
func newSubtitleFixture(t *testing.T, access Access) (*httptest.Server, media.Item, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "tv/ep.mkv", []byte("video"))
	writeFile(t, root, "tv/ep.en.srt", []byte(sampleSRT))
	writeFile(t, root, "tv/ep.fr.srt", []byte("1\n00:00:01,000 --> 00:00:02,000\ncaf\xe9\n")) // Windows-1252
	it := itemFor(t, root, "tv/ep.mkv", media.KindVideo)
	it.Subtitles = []media.Subtitle{
		{Lang: "en", Label: "English", RelPath: "tv/ep.en.srt", Path: filepath.Join(root, "tv", "ep.en.srt")},
		{Lang: "fr", Label: "French", RelPath: "tv/ep.fr.srt", Path: filepath.Join(root, "tv", "ep.fr.srt")},
	}
	lib := &fakeLibrary{root: root, items: []media.Item{it}}
	a := New(lib, access, &fakeRemote{}, "1.2.3", discardLog()).newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return srv, it, root
}

func TestSubtitle_ServesWebVTT(t *testing.T) {
	srv, it, _ := newSubtitleFixture(t, &fakeAccess{})

	resp := getFrom(t, srv, "/subtitle/"+string(it.ID)+"/0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	want := "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nHello\n\n00:00:03.000 --> 00:00:04.000\nWorld\n\n"
	if got := string(readBody(t, resp)); got != want {
		t.Errorf("body =\n%q\nwant\n%q", got, want)
	}
}

func TestSubtitle_LegacyEncodingBecomesUTF8(t *testing.T) {
	srv, it, _ := newSubtitleFixture(t, &fakeAccess{})
	body := string(readBody(t, getFrom(t, srv, "/subtitle/"+string(it.ID)+"/1")))
	if !strings.Contains(body, "café") {
		t.Errorf("body = %q, want the Windows-1252 text decoded", body)
	}
}

func TestSubtitle_NotFoundCases(t *testing.T) {
	srv, it, _ := newSubtitleFixture(t, &fakeAccess{})
	id := string(it.ID)
	for _, path := range []string{
		"/subtitle/" + id + "/2", "/subtitle/" + id + "/-1", "/subtitle/" + id + "/x",
		"/subtitle/" + id + "/0.vtt", "/subtitle/nope/0",
	} {
		assertErrorShape(t, getFrom(t, srv, path), http.StatusNotFound, codeNotFound)
	}
}

func TestSubtitle_RefusesAFileThatLeftTheLibrary(t *testing.T) {
	srv, it, root := newSubtitleFixture(t, &fakeAccess{})
	secret := filepath.Join(t.TempDir(), "secret.srt")
	if err := os.WriteFile(secret, []byte("1\n00:00:01,000 --> 00:00:02,000\nsecret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	track := filepath.Join(root, "tv", "ep.en.srt")
	if err := os.Remove(track); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, track); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	resp := getFrom(t, srv, "/subtitle/"+string(it.ID)+"/0")
	body := string(readBody(t, resp))
	if resp.StatusCode != http.StatusNotFound || strings.Contains(body, "secret") {
		t.Fatalf("status = %d body = %q, want 404 without the file's content", resp.StatusCode, body)
	}
}

func TestSubtitle_ATrackThatDisappearedIsANotFound(t *testing.T) {
	srv, it, root := newSubtitleFixture(t, &fakeAccess{})
	if err := os.Remove(filepath.Join(root, "tv", "ep.en.srt")); err != nil {
		t.Fatal(err)
	}
	assertErrorShape(t, getFrom(t, srv, "/subtitle/"+string(it.ID)+"/0"), http.StatusNotFound, codeNotFound)
}

func TestSubtitle_AHugeFileIsRefused(t *testing.T) {
	srv, it, root := newSubtitleFixture(t, &fakeAccess{})
	big := make([]byte, maxSubtitleBytes+1)
	if err := os.WriteFile(filepath.Join(root, "tv", "ep.en.srt"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	assertErrorShape(t, getFrom(t, srv, "/subtitle/"+string(it.ID)+"/0"), http.StatusNotFound, codeNotFound)
}

func TestSubtitle_SitsBehindTheAccessToken(t *testing.T) {
	access := &fakeAccess{}
	access.set(true, "secret")
	srv, it, _ := newSubtitleFixture(t, access)
	path := "/subtitle/" + string(it.ID) + "/0"

	if got := getFrom(t, srv, path).StatusCode; got != http.StatusUnauthorized {
		t.Errorf("without the token: status = %d, want 401", got)
	}
	if got := getFrom(t, srv, path, "Authorization", "Bearer secret").StatusCode; got != http.StatusOK {
		t.Errorf("with the token: status = %d, want 200", got)
	}
}

func TestLibraryJSON_ListsTracksWithoutPaths(t *testing.T) {
	srv, it, root := newSubtitleFixture(t, &fakeAccess{})

	raw := readBody(t, getFrom(t, srv, "/api/library"))
	if strings.Contains(string(raw), root) || strings.Contains(string(raw), "ep.en.srt") {
		t.Fatalf("library JSON leaks a path or file name: %s", raw)
	}
	var items []itemJSON
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != it.ID {
		t.Fatalf("items = %+v", items)
	}
	want := []subtitleJSON{{Label: "English", Lang: "en"}, {Label: "French", Lang: "fr"}}
	if len(items[0].Subtitles) != 2 || items[0].Subtitles[0] != want[0] || items[0].Subtitles[1] != want[1] {
		t.Errorf("subtitles = %+v, want %+v", items[0].Subtitles, want)
	}
}

func TestLibraryJSON_ItemsWithoutTracksHaveAnEmptyList(t *testing.T) {
	f := newFixture(t)
	if raw := string(readBody(t, f.get("/api/library"))); !strings.Contains(raw, `"subtitles":[]`) || strings.Contains(raw, `"subtitles":null`) {
		t.Fatalf("library JSON = %s, want subtitles as []", raw)
	}
}
