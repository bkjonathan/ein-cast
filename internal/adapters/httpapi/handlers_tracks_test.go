package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

type fakeTracks struct {
	available bool
	streams   app.Streams
	err       error
	subtitle  string
	subErr    error
	asked     []int
}

func (f *fakeTracks) Available() bool { return f.available }
func (f *fakeTracks) Streams(context.Context, media.ID) (app.Streams, error) {
	return f.streams, f.err
}
func (f *fakeTracks) Subtitle(_ context.Context, _ media.ID, index int) ([]byte, error) {
	f.asked = append(f.asked, index)
	if f.subErr != nil {
		return nil, f.subErr
	}
	return []byte(f.subtitle), nil
}

func newTracksFixture(t *testing.T, access Access, tr *fakeTracks) (*fixture, *httptest.Server) {
	t.Helper()
	f := newFixtureWith(t, access)
	a := New(f.lib, access, &fakeRemote{}, "1.2.3", discardLog(), WithTracks(tr)).
		newAPI(app.ServeConfig{Name: "Den"})
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestTracks_ListsWhatIsInsideAVideo(t *testing.T) {
	tr := &fakeTracks{available: true, streams: app.Streams{
		Audio: []app.AudioStream{
			{Index: 0, Language: "eng", Codec: "ac3", Channels: 6, Default: true},
			{Index: 1, Language: "jpn", Title: "Commentary", Codec: "aac", Channels: 2},
			{Index: 2, Codec: "dts", Channels: 8},
		},
		Subtitles: []app.SubtitleStream{
			{Index: 0, Language: "fre", Title: "SDH", Codec: "subrip", Text: true},
			{Index: 1, Language: "eng", Codec: "hdmv_pgs_subtitle"},
		},
	}}
	f, srv := newTracksFixture(t, &fakeAccess{}, tr)

	resp := getFrom(t, srv, "/api/library/"+string(f.movie.ID)+"/tracks")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got tracksJSON
	if err := json.Unmarshal(readBody(t, resp), &got); err != nil {
		t.Fatal(err)
	}
	wantAudio := []audioTrackJSON{
		{Index: 0, Label: "English · 5.1 · Dolby Digital", Lang: "eng", Default: true},
		{Index: 1, Label: "Japanese · Commentary · Stereo · AAC", Lang: "jpn"},
		{Index: 2, Label: "Track 3 · 7.1 · DTS"},
	}
	for i, w := range wantAudio {
		if got.Audio[i] != w {
			t.Errorf("Audio[%d] = %+v, want %+v", i, got.Audio[i], w)
		}
	}
	wantSubs := []embeddedTrackJSON{
		{Index: 0, Label: "French (SDH)", Lang: "fre", Supported: true},
		{Index: 1, Label: "English", Lang: "eng", Supported: false},
	}
	for i, w := range wantSubs {
		if got.Subtitles[i] != w {
			t.Errorf("Subtitles[%d] = %+v, want %+v", i, got.Subtitles[i], w)
		}
	}
}

func TestTracks_WithoutTheFeatureOrAnswerThereAreNoChoices(t *testing.T) {
	// Not wired in at all, and a probe that failed: both are an empty list, not an error.
	f := newFixture(t)
	for name, get := range map[string]func() *http.Response{
		"not wired": func() *http.Response { return f.get("/api/library/" + string(f.movie.ID) + "/tracks") },
		"probe failed": func() *http.Response {
			ff, srv := newTracksFixture(t, &fakeAccess{}, &fakeTracks{available: true, err: errors.New("ffprobe: boom at /secret")})
			return getFrom(t, srv, "/api/library/"+string(ff.movie.ID)+"/tracks")
		},
	} {
		resp := get()
		body := string(readBody(t, resp))
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"audio":[]`) || !strings.Contains(body, `"subtitles":[]`) {
			t.Errorf("%s: status = %d body = %s", name, resp.StatusCode, body)
		}
		if strings.Contains(body, "/secret") {
			t.Errorf("%s: the body leaks detail: %s", name, body)
		}
	}
}

func TestTracks_UnknownVideoIsANotFoundAndSitsBehindAccess(t *testing.T) {
	access := &fakeAccess{}
	access.set(true, "secret")
	f, srv := newTracksFixture(t, access, &fakeTracks{available: true})

	if got := getFrom(t, srv, "/api/library/"+string(f.movie.ID)+"/tracks").StatusCode; got != http.StatusUnauthorized {
		t.Errorf("without the token: status = %d", got)
	}
	assertErrorShape(t, getFrom(t, srv, "/api/library/nope/tracks", "Authorization", "Bearer secret"), http.StatusNotFound, codeNotFound)
}

func TestEmbeddedSubtitle_ServesWebVTT(t *testing.T) {
	tr := &fakeTracks{available: true, subtitle: "WEBVTT\n\n00:01.000 --> 00:02.000\nhi\n"}
	f, srv := newTracksFixture(t, &fakeAccess{}, tr)

	resp := getFrom(t, srv, "/subtitle/"+string(f.movie.ID)+"/embedded/1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/vtt; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := string(readBody(t, resp)); got != tr.subtitle {
		t.Errorf("body = %q", got)
	}
	if len(tr.asked) != 1 || tr.asked[0] != 1 {
		t.Errorf("asked for tracks %v", tr.asked)
	}
}

func TestEmbeddedSubtitle_ErrorsMapToStatuses(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		err    error
		status int
		code   string
	}{
		{"no such track", "/embedded/9", app.ErrNoSubtitleTrack, http.StatusNotFound, codeNotFound},
		{"no ffmpeg", "/embedded/0", app.ErrSubtitlesUnavailable, http.StatusNotFound, codeNotFound},
		{"not a number", "/embedded/x", nil, http.StatusNotFound, codeNotFound},
		{"negative", "/embedded/-1", nil, http.StatusNotFound, codeNotFound},
		{"something else", "/embedded/0", errors.New("disk exploded at /secret/path"), http.StatusInternalServerError, codeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &fakeTracks{available: true, subErr: tt.err}
			f, srv := newTracksFixture(t, &fakeAccess{}, tr)
			resp := getFrom(t, srv, "/subtitle/"+string(f.movie.ID)+tt.path)
			body := string(readBody(t, resp))
			if resp.StatusCode != tt.status || !strings.Contains(body, `"code":"`+tt.code+`"`) {
				t.Fatalf("status = %d body = %s", resp.StatusCode, body)
			}
			if strings.Contains(body, "/secret/path") {
				t.Errorf("the body leaks internal detail: %s", body)
			}
		})
	}
}

func TestEmbeddedSubtitle_NotWiredAndBehindAccess(t *testing.T) {
	f := newFixture(t)
	assertErrorShape(t, f.get("/subtitle/"+string(f.movie.ID)+"/embedded/0"), http.StatusNotFound, codeNotFound)

	access := &fakeAccess{}
	access.set(true, "secret")
	tr := &fakeTracks{available: true, subtitle: "WEBVTT\n"}
	ff, srv := newTracksFixture(t, access, tr)
	if got := getFrom(t, srv, "/subtitle/"+string(ff.movie.ID)+"/embedded/0").StatusCode; got != http.StatusUnauthorized {
		t.Errorf("without the token: status = %d", got)
	}
	if len(tr.asked) != 0 {
		t.Error("a subtitle was read without the token")
	}
}

func TestHealth_SaysWhetherTracksCanBeListed(t *testing.T) {
	for _, available := range []bool{true, false} {
		_, srv := newTracksFixture(t, &fakeAccess{}, &fakeTracks{available: available})
		body := string(readBody(t, getFrom(t, srv, "/api/health")))
		want := `"tracks":false`
		if available {
			want = `"tracks":true`
		}
		if !strings.Contains(body, want) {
			t.Errorf("health = %s, want %s", body, want)
		}
	}
}

func TestLanguageName(t *testing.T) {
	for in, want := range map[string]string{
		"eng": "English", "ENG": "English", " fre ": "French", "fra": "French", "pt-BR": "Portuguese",
		"bur": "Burmese", "mya": "Burmese", "und": "", "": "", "zzz": "",
	} {
		if got := languageName(in); got != want {
			t.Errorf("languageName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPageCSP_AllowsBlobMediaOnly(t *testing.T) {
	if !strings.Contains(pageCSP, "media-src 'self' blob:") {
		t.Errorf("pageCSP lacks blob: media: %s", pageCSP)
	}
	for _, directive := range strings.Split(pageCSP, ";") {
		d := strings.TrimSpace(directive)
		if strings.Contains(d, "blob:") && !strings.HasPrefix(d, "media-src") {
			t.Errorf("blob: is allowed outside media-src: %q", d)
		}
	}
}
