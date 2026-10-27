package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"eincast/internal/app"
	"eincast/internal/app/profiles"
	"eincast/internal/app/resume"
	"eincast/internal/app/thumbnails"
	"eincast/internal/domain/media"
	"eincast/internal/domain/profile"
)

// ---- in-memory stand-ins for the stores ------------------------------------

type memProfileStore struct {
	mu   sync.Mutex
	recs []app.ProfileRecord
}

func (m *memProfileStore) Load() ([]app.ProfileRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recs == nil {
		return nil, app.ErrNoProfiles
	}
	return m.recs, nil
}
func (m *memProfileStore) Save(r []app.ProfileRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append([]app.ProfileRecord(nil), r...)
	return nil
}

type profSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *profSecrets) Get(k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[k]; ok {
		return v, nil
	}
	return "", app.ErrSecretNotFound
}
func (s *profSecrets) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[k] = v
	return nil
}
func (s *profSecrets) Delete(k string) error { return nil }

type memResumeStore struct{}

func (memResumeStore) Load() (map[profile.ID]map[media.ID]app.ResumePosition, error) {
	return nil, nil
}
func (memResumeStore) Save(map[profile.ID]map[media.ID]app.ResumePosition) error { return nil }

type memEvents struct{}

func (memEvents) Publish(app.Event) {}

type memLibraries struct{ libs []media.Library }

func (m memLibraries) Libraries() []media.Library { return m.libs }

// ---- the rig ----------------------------------------------------------------

const (
	cartoons media.LibraryID = "cartoons"
	movies   media.LibraryID = "movies"
)

// profRig is a real profile service and a real HTTP handler over a library of
// kids' and adults' titles, with a browser's cookie jar.
type profRig struct {
	t      *testing.T
	root   string
	srv    *httptest.Server
	client *http.Client
	svc    *profiles.Service
	resume *resume.Service
	thumbs *fakeThumbs
	hls    *fakeHLS
	api    *api

	bear, heat, song media.Item // kids' video, adults' video, adults' song
	kids, adults     profile.Profile
}

func newProfRig(t *testing.T, tvOn bool) *profRig {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "toons/bear.mp4", bytes.Repeat([]byte("b"), 2000))
	writeFile(t, root, "adult/heat.mkv", bytes.Repeat([]byte("h"), 3000))
	writeFile(t, root, "adult/song.mp3", []byte("mp3"))
	writeFile(t, root, "adult/heat.en.srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))

	mk := func(lib media.LibraryID, rel string, kind media.Kind) media.Item {
		it := itemFor(t, root, rel, kind)
		it.LibraryID, it.ID = lib, media.NewID(lib, rel)
		return it
	}
	r := &profRig{t: t, root: root}
	r.bear = mk(cartoons, "toons/bear.mp4", media.KindVideo)
	r.heat = mk(movies, "adult/heat.mkv", media.KindVideo)
	r.heat.Subtitles = []media.Subtitle{{Lang: "en", Label: "English", RelPath: "adult/heat.en.srt", Path: root + "/adult/heat.en.srt"}}
	r.song = mk(movies, "adult/song.mp3", media.KindAudio)
	lib := &fakeLibrary{root: root, items: []media.Item{r.bear, r.heat, r.song}}

	libs := memLibraries{libs: []media.Library{{ID: cartoons, Root: root}, {ID: movies, Root: root, HiddenFromTVs: true}}}
	r.svc = profiles.NewService(&memProfileStore{}, &profSecrets{m: map[string]string{}}, libs, memEvents{}, slog.New(slog.DiscardHandler))
	r.svc.Load()
	r.resume = resume.NewService(memResumeStore{}, slog.New(slog.DiscardHandler))
	r.thumbs = &fakeThumbs{available: true, byID: map[media.ID]thumbnails.Thumbnail{}}
	r.hls = &fakeHLS{available: true, playlist: "#EXTM3U\n", segments: map[string]string{}}

	// Two profiles besides the default: kids (no PIN, cartoons only) and adults (PIN, everything).
	var err error
	if r.kids, err = r.svc.Save(profile.Profile{Name: "Kids", Color: "green", Libraries: []media.LibraryID{cartoons}}); err != nil {
		t.Fatal(err)
	}
	if r.adults, err = r.svc.Save(profile.Profile{Name: "Adults", Color: "red", AllLibraries: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.svc.SetPIN(r.adults.ID, "1234"); err != nil {
		t.Fatal(err)
	}
	// The default profile stays: three profiles, so a choice is needed.

	srv := New(lib, &fakeAccess{}, &fakeRemote{}, "1.2.3", slog.New(slog.DiscardHandler),
		WithProfiles(r.svc), WithResume(r.resume), WithThumbnails(r.thumbs), WithTranscoding(r.hls), WithDLNA("uuid"))
	r.api = srv.newAPI(app.ServeConfig{Name: "Den", DLNA: tvOn})
	r.srv = httptest.NewServer(r.api.handler())
	t.Cleanup(r.srv.Close)

	jar, _ := cookiejar.New(nil)
	r.client = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return r
}

func (r *profRig) do(method, path string, body any, headers ...string) (*http.Response, []byte) {
	r.t.Helper()
	return r.doWith(r.client, method, path, body, headers...)
}

func (r *profRig) doWith(c *http.Client, method, path string, body any, headers ...string) (*http.Response, []byte) {
	r.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, r.srv.URL+path, rd)
	if err != nil {
		r.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (r *profRig) get(path string, headers ...string) (*http.Response, []byte) {
	r.t.Helper()
	return r.do(http.MethodGet, path, nil, headers...)
}

// choose picks a profile like the player page does.
func (r *profRig) choose(p profile.Profile, pin string) *http.Response {
	r.t.Helper()
	resp, _ := r.do(http.MethodPost, "/api/profile", map[string]string{"id": string(p.ID), "pin": pin})
	return resp
}

func (r *profRig) listed() []string {
	r.t.Helper()
	resp, body := r.get("/api/library")
	if resp.StatusCode != http.StatusOK {
		r.t.Fatalf("GET /api/library = %d: %s", resp.StatusCode, body)
	}
	var items []struct{ Name string }
	_ = json.Unmarshal(body, &items)
	var names []string
	for _, i := range items {
		names = append(names, i.Name)
	}
	return names
}

func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("not an error body: %s", body)
	}
	return e.Error.Code
}

// ---- who gets what ------------------------------------------------------------

func TestProfiles_AGuestGetsNoListAndNoItems(t *testing.T) {
	r := newProfRig(t, false)
	for _, path := range []string{"/api/library", "/playlist.m3u", "/api/resume"} {
		resp, body := r.get(path)
		if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != codeProfileRequired {
			t.Errorf("GET %s as a guest = %d %s, want 401 profile_required", path, resp.StatusCode, body)
		}
	}
	for _, it := range []media.Item{r.bear, r.heat} {
		resp, _ := r.get("/stream/" + string(it.ID))
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("/stream/%s as a guest = %d, want 404: a guest is not Everyone", it.Name, resp.StatusCode)
		}
	}
}

func TestProfiles_TheHelpersAnAdultsPageNeedsAreOpenToAGuest(t *testing.T) {
	r := newProfRig(t, false)
	for _, path := range []string{"/", "/static/app.js", "/api/health", "/api/profiles"} {
		if resp, _ := r.get(path, "Accept", "text/html"); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d: the picker has to be able to load", path, resp.StatusCode)
		}
	}
	_, body := r.get("/api/profiles")
	var got profilesJSON
	_ = json.Unmarshal(body, &got)
	if !got.Active || len(got.Profiles) != 3 || got.Current != "" {
		t.Errorf("profiles = %+v", got)
	}
	for _, p := range got.Profiles {
		if p.Name == "Adults" && !p.HasPIN {
			t.Error("the picker is not told that Adults has a PIN")
		}
	}
	if strings.Contains(string(body), "1234") || strings.Contains(strings.ToLower(string(body)), "hash") || strings.Contains(string(body), string(cartoons)) {
		t.Errorf("the picker leaks more than names and colours: %s", body)
	}
}

func TestProfiles_KidsSeeOnlyKidsTitles(t *testing.T) {
	r := newProfRig(t, false)
	if resp := r.choose(r.kids, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("choosing Kids = %d", resp.StatusCode)
	}
	if got := strings.Join(r.listed(), ","); got != "bear.mp4" {
		t.Errorf("listed %q, want only bear.mp4", got)
	}
	if resp, _ := r.get("/stream/" + string(r.bear.ID)); resp.StatusCode != http.StatusOK {
		t.Errorf("its own title = %d", resp.StatusCode)
	}
	if resp, _ := r.get("/stream/"+string(r.bear.ID), "Range", "bytes=0-9"); resp.StatusCode != http.StatusPartialContent {
		t.Errorf("a range request = %d, want 206", resp.StatusCode)
	}
}

// The core promise: an item the profile may not see is a 404 that looks exactly
// like an ID that does not exist, on every route that takes an ID.
func TestProfiles_ForbiddenIsIndistinguishableFromMissingOnEveryRoute(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")

	forbidden, missing := string(r.heat.ID), "0123456789abcdef"
	routes := []string{"/api/library/%s", "/stream/%s", "/thumb/%s", "/hls/%s/index.m3u8", "/hls/%s/seg00000.ts", "/subtitle/%s/0"}
	for _, route := range routes {
		fr, fb := r.get(fmt.Sprintf(route, forbidden))
		mr, mb := r.get(fmt.Sprintf(route, missing))
		if fr.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s for a forbidden item = %d, want 404", route, fr.StatusCode)
		}
		if mr.StatusCode != fr.StatusCode || errorCode(t, fb) != errorCode(t, mb) {
			t.Errorf("GET %s tells forbidden (%d %s) from missing (%d %s)", route, fr.StatusCode, fb, mr.StatusCode, mb)
		}
	}
	for _, asked := range r.thumbs.asked {
		if asked == r.heat.ID {
			t.Error("the thumbnail service was asked about a forbidden item: a picture would have been made")
		}
	}
	if len(r.hls.asked) != 0 {
		t.Errorf("a conversion was started for a forbidden item: %v", r.hls.asked)
	}
	if resp, _ := r.get("/stream/" + forbidden + "?t=anything&p=forged"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a forged credential opened it: %d", resp.StatusCode)
	}
}

func TestProfiles_TheItemRoutesWorkForAnAllowedItem(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	r.thumbs.byID[r.bear.ID] = thumbnails.Thumbnail{JPEG: []byte("\xff\xd8\xffjpeg"), ModTime: thumbModTime}
	for _, path := range []string{"/api/library/" + string(r.bear.ID), "/thumb/" + string(r.bear.ID), "/hls/" + string(r.bear.ID) + "/index.m3u8"} {
		if resp, body := r.get(path); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d %s", path, resp.StatusCode, body)
		}
	}
}

func TestProfiles_AdultsNeedThePINAndSeeEverything(t *testing.T) {
	r := newProfRig(t, false)
	for name, pin := range map[string]string{"no PIN": "", "a wrong PIN": "9999"} {
		resp := r.choose(r.adults, pin)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", name, resp.StatusCode)
		}
		if got := resp.Header.Get("Set-Cookie"); got != "" {
			t.Errorf("%s set a cookie: %s", name, got)
		}
	}
	if resp, _ := r.get("/api/library"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("a wrong PIN left the device signed in")
	}

	if resp := r.choose(r.adults, "1234"); resp.StatusCode != http.StatusOK {
		t.Fatalf("the right PIN = %d", resp.StatusCode)
	}
	if got := strings.Join(r.listed(), ","); got != "bear.mp4,heat.mkv,song.mp3" {
		t.Errorf("listed %q", got)
	}
	if resp, _ := r.get("/stream/" + string(r.heat.ID)); resp.StatusCode != http.StatusOK {
		t.Errorf("an adult title = %d", resp.StatusCode)
	}
	if resp, body := r.get("/subtitle/" + string(r.heat.ID) + "/0"); resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "WEBVTT") {
		t.Errorf("subtitle = %d %s", resp.StatusCode, body)
	}
}

func TestProfiles_TheCookieIsHttpOnlyAndSeparateFromTheOthers(t *testing.T) {
	r := newProfRig(t, false)
	resp := r.choose(r.kids, "")
	var c *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == profileCookie {
			c = k
		}
	}
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge <= 0 {
		t.Fatalf("cookie = %+v", c)
	}
	if profileCookie == cookieName || profileCookie == sessionCookie {
		t.Error("the profile cookie shares a name with a credential it must not be mistaken for")
	}
}

func TestProfiles_AForgedOrBorrowedCredentialIsAGuest(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	var kidCookie string
	u := r.srv.URL
	for _, k := range r.client.Jar.Cookies(mustURL(u)) {
		if k.Name == profileCookie {
			kidCookie = k.Value
		}
	}
	parts := strings.Split(kidCookie, ".")
	forged := map[string]string{
		"another profile's id, kids' mac": string(r.adults.ID) + "." + parts[1] + "." + parts[2],
		"a made-up mac":                   string(r.adults.ID) + ".1." + strings.Repeat("a", 64),
		"the default profile":             "everyone.1." + strings.Repeat("0", 64),
		"nothing but a name":              "adults",
	}
	for name, cred := range forged {
		resp, _ := r.doWith(&http.Client{}, http.MethodGet, "/api/library", nil, "Cookie", profileCookie+"="+cred)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: /api/library = %d, want 401", name, resp.StatusCode)
		}
	}
}

func TestProfiles_ChangingAPINEndsTheSignInsMadeBefore(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	if resp, _ := r.get("/api/library"); resp.StatusCode != http.StatusOK {
		t.Fatal("not signed in")
	}
	if err := r.svc.SetPIN(r.kids.ID, "4444"); err != nil {
		t.Fatal(err)
	}
	if resp, _ := r.get("/api/library"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a device kept its profile after the PIN changed: %d", resp.StatusCode)
	}
}

func TestProfiles_DeletingAProfileEndsItsSignIns(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	if err := r.svc.Delete(r.kids.ID); err != nil {
		t.Fatal(err)
	}
	if resp, _ := r.get("/api/library"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a deleted profile still works: %d", resp.StatusCode)
	}
}

func TestProfiles_WrongPINsLockTheClientOut(t *testing.T) {
	r := newProfRig(t, false)
	var last *http.Response
	for i := 0; i < 5; i++ {
		last = r.choose(r.adults, "0000")
	}
	if last.StatusCode != http.StatusUnauthorized {
		t.Fatalf("the fifth wrong PIN = %d", last.StatusCode)
	}
	resp := r.choose(r.adults, "1234")
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Errorf("after five wrong PINs, even the right one = %d (Retry-After %q), want 429", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestProfiles_LeavingBringsThePickerBack(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	_, body := r.get("/api/profiles")
	var got profilesJSON
	_ = json.Unmarshal(body, &got)
	if got.Current != r.kids.ID {
		t.Fatalf("current = %q", got.Current)
	}
	if resp, _ := r.do(http.MethodDelete, "/api/profile", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/profile = %d", resp.StatusCode)
	}
	if resp, _ := r.get("/api/library"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after leaving: %d", resp.StatusCode)
	}
}

func TestProfiles_ChoosingFromAnotherSiteIsRefused(t *testing.T) {
	r := newProfRig(t, false)
	resp, _ := r.do(http.MethodPost, "/api/profile", map[string]string{"id": string(r.kids.ID)}, "Sec-Fetch-Site", "cross-site")
	if resp.StatusCode != http.StatusForbidden || len(resp.Cookies()) != 0 {
		t.Errorf("cross-site choose = %d, cookies %v", resp.StatusCode, resp.Cookies())
	}
	resp, _ = r.do(http.MethodDelete, "/api/profile", nil, "Sec-Fetch-Site", "cross-site")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site leave = %d", resp.StatusCode)
	}
}

func TestProfiles_ChoosingRefusesNonsense(t *testing.T) {
	r := newProfRig(t, false)
	if resp, _ := r.do(http.MethodPost, "/api/profile", map[string]string{"id": "ghost"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown profile = %d", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, r.srv.URL+"/api/profile", strings.NewReader("not json"))
	if resp, _ := r.client.Do(req); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a body that is not JSON = %d", resp.StatusCode)
	}
	big := strings.NewReader(`{"id":"` + strings.Repeat("x", 5000) + `"}`)
	req, _ = http.NewRequest(http.MethodPost, r.srv.URL+"/api/profile", big)
	if resp, _ := r.client.Do(req); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an oversized body = %d", resp.StatusCode)
	}
}

// ---- the playlist, for players that cannot hold a cookie ------------------------

func TestProfiles_ThePlaylistHoldsOnlyWhatTheProfileMaySeeAndWorksWithoutACookie(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	resp, body := r.get("/playlist.m3u")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("playlist = %d", resp.StatusCode)
	}
	text := string(body)
	if !strings.Contains(text, string(r.bear.ID)) || strings.Contains(text, string(r.heat.ID)) || strings.Contains(text, string(r.song.ID)) {
		t.Fatalf("the kids' playlist is wrong:\n%s", text)
	}
	if !strings.Contains(text, "?"+profileParam+"=") {
		t.Fatalf("entries carry no profile credential, so VLC would be a guest:\n%s", text)
	}

	// VLC: no cookies, only the URL.
	var entry string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "http") {
			entry = line
		}
	}
	vlc := &http.Client{}
	resp2, _ := r.doWith(vlc, http.MethodGet, strings.TrimPrefix(entry, r.srv.URL), nil)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("an entry without a cookie = %d, want 200", resp2.StatusCode)
	}
	// The same credential does not open an adult title.
	cred := entry[strings.Index(entry, "?"):]
	resp3, _ := r.doWith(vlc, http.MethodGet, "/stream/"+string(r.heat.ID)+cred, nil)
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("the kids' credential opened an adult title: %d", resp3.StatusCode)
	}
}

func TestProfiles_TheAdultsPlaylistHasAllOfThem(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.adults, "1234")
	_, body := r.get("/playlist.m3u")
	if !strings.Contains(string(body), string(r.heat.ID)) || !strings.Contains(string(body), string(r.bear.ID)) {
		t.Errorf("playlist:\n%s", body)
	}
}

// ---- smart TVs ------------------------------------------------------------------

func TestProfiles_ASmartTvIsAGuestThatSeesOnlyTVVisibleLibraries(t *testing.T) {
	r := newProfRig(t, true) // Smart TVs are on
	tv := &http.Client{}
	if resp, _ := r.doWith(tv, http.MethodGet, "/stream/"+string(r.bear.ID), nil); resp.StatusCode != http.StatusOK {
		t.Errorf("a TV-visible title = %d, want 200", resp.StatusCode)
	}
	if resp, _ := r.doWith(tv, http.MethodGet, "/stream/"+string(r.heat.ID), nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a title in a library kept off TVs = %d, want 404", resp.StatusCode)
	}
	if resp, _ := r.doWith(tv, http.MethodGet, "/api/library", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a guest listed the library: %d", resp.StatusCode)
	}
}

func TestProfiles_WithSmartTVsOffAGuestGetsNoItemEither(t *testing.T) {
	r := newProfRig(t, false)
	if resp, _ := r.doWith(&http.Client{}, http.MethodGet, "/stream/"+string(r.bear.ID), nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a guest got a title with Smart TVs off: %d", resp.StatusCode)
	}
}

func TestProfiles_TheTVListingHidesLibrariesKeptOffTVs(t *testing.T) {
	r := newProfRig(t, true)
	got := tvLibrary{lib: r.api.lib, tv: r.svc}.List()
	if len(got) != 1 || got[0].ID != r.bear.ID {
		t.Errorf("a TV is shown %+v, want only the kids' title", got)
	}
}

func TestProfiles_ASignedTunnelLinkOpensExactlyItsItem(t *testing.T) {
	r := newProfRig(t, false)
	exp := strconv.FormatInt(fakeLinkExpiry.Unix(), 10)
	guest := &http.Client{}
	ok := "/stream/" + string(r.heat.ID) + "?exp=" + exp + "&sig=sig-" + string(r.heat.ID)
	if resp, _ := r.doWith(guest, http.MethodGet, ok, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("a signed link = %d, want 200: it was only issued for an allowed item", resp.StatusCode)
	}
	other := "/stream/" + string(r.heat.ID) + "?exp=" + exp + "&sig=sig-" + string(r.bear.ID)
	if resp, _ := r.doWith(guest, http.MethodGet, other, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a link signed for another item = %d", resp.StatusCode)
	}
}

// ---- one profile ------------------------------------------------------------------

func TestProfiles_WithOneProfileNothingChanges(t *testing.T) {
	r := newProfRig(t, false)
	for _, p := range []profile.Profile{r.kids, r.adults} {
		if err := r.svc.Delete(p.ID); err != nil {
			t.Fatal(err)
		}
	}
	resp, body := r.get("/api/profiles")
	var got profilesJSON
	_ = json.Unmarshal(body, &got)
	if resp.StatusCode != http.StatusOK || got.Active || len(got.Profiles) != 0 {
		t.Errorf("profiles = %+v: with one profile there is no picker", got)
	}
	if got := strings.Join(r.listed(), ","); got != "bear.mp4,heat.mkv,song.mp3" {
		t.Errorf("listed %q with no cookie", got)
	}
	if resp, _ := r.get("/stream/" + string(r.heat.ID)); resp.StatusCode != http.StatusOK {
		t.Errorf("stream = %d", resp.StatusCode)
	}
	if resp, _ := r.do(http.MethodPost, "/api/profile", map[string]string{"id": "everyone"}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("choosing with one profile = %d, want 404", resp.StatusCode)
	}
	_, pl := r.get("/playlist.m3u")
	if strings.Contains(string(pl), "?"+profileParam+"=") {
		t.Error("the playlist carries a profile credential nobody needs")
	}
}

func TestProfiles_ALoneProfileThatHidesAFolderIsEnforcedForEveryone(t *testing.T) {
	r := newProfRig(t, false)
	for _, p := range []profile.Profile{r.kids, r.adults} {
		_ = r.svc.Delete(p.ID)
	}
	only := profile.Everyone()
	only.HiddenFolders = []string{"adult"}
	if _, err := r.svc.Save(only); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.listed(), ","); got != "bear.mp4" {
		t.Errorf("listed %q", got)
	}
	if resp, _ := r.get("/stream/" + string(r.heat.ID)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a hidden folder's title = %d, want 404", resp.StatusCode)
	}
}

func TestProfiles_HiddenFoldersAndKindsAreEnforcedOnTheServer(t *testing.T) {
	r := newProfRig(t, false)
	teen, err := r.svc.Save(profile.Profile{Name: "Teen", Color: "blue", AllLibraries: true, Kinds: []media.Kind{media.KindVideo}, HiddenFolders: []string{"ADULT"}})
	if err != nil {
		t.Fatal(err)
	}
	r.choose(teen, "")
	if got := strings.Join(r.listed(), ","); got != "bear.mp4" {
		t.Errorf("listed %q", got)
	}
	for _, it := range []media.Item{r.heat, r.song} {
		if resp, _ := r.get("/stream/" + string(it.ID)); resp.StatusCode != http.StatusNotFound {
			t.Errorf("/stream/%s = %d, want 404", it.Name, resp.StatusCode)
		}
	}
}

// ---- resume -------------------------------------------------------------------------

func TestResume_IsPerProfileAndNeverNamesWhatTheProfileCannotSee(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.adults, "1234")
	for _, it := range []media.Item{r.bear, r.heat} {
		resp, body := r.do(http.MethodPut, "/api/resume/"+string(it.ID), map[string]int64{"t": 600, "d": 3600, "m": it.ModTime.UnixMilli()})
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("PUT %s = %d %s", it.Name, resp.StatusCode, body)
		}
	}
	_, body := r.get("/api/resume")
	var adultsList []resumeJSON
	_ = json.Unmarshal(body, &adultsList)
	if len(adultsList) != 2 {
		t.Fatalf("adults' resume = %s", body)
	}

	// Kids on another device see their own positions only, and not the adult title.
	kid := newKidClient(t, r)
	_, kb := r.doWith(kid, http.MethodGet, "/api/resume", nil)
	if strings.TrimSpace(string(kb)) != "[]" {
		t.Errorf("kids see the adults' positions: %s", kb)
	}
	if resp, _ := r.doWith(kid, http.MethodPut, "/api/resume/"+string(r.heat.ID), map[string]int64{"t": 10, "d": 100}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("kids saved a position in an adult title: %d", resp.StatusCode)
	}
	if resp, _ := r.doWith(kid, http.MethodPut, "/api/resume/"+string(r.bear.ID), map[string]int64{"t": 20, "d": 100}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("kids PUT = %d", resp.StatusCode)
	}
	_, kb = r.doWith(kid, http.MethodGet, "/api/resume", nil)
	var kidsList []resumeJSON
	_ = json.Unmarshal(kb, &kidsList)
	if len(kidsList) != 1 || kidsList[0].T != 20 {
		t.Errorf("kids' list = %s", kb)
	}
	// The adults' position in the same title is untouched.
	_, body = r.get("/api/resume")
	_ = json.Unmarshal(body, &adultsList)
	for _, e := range adultsList {
		if e.ID == r.bear.ID && e.T != 600 {
			t.Errorf("the kids overwrote the adults' position: %+v", e)
		}
	}
}

func newKidClient(t *testing.T, r *profRig) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, _ := r.doWith(c, http.MethodPost, "/api/profile", map[string]string{"id": string(r.kids.ID)})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("choosing Kids = %d", resp.StatusCode)
	}
	return c
}

func TestResume_ListIsMostRecentFirstAndLeavesOutDeletedTitles(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.adults, "1234")
	r.do(http.MethodPut, "/api/resume/"+string(r.bear.ID), map[string]int64{"t": 1, "d": 100})
	time.Sleep(3 * time.Millisecond)
	r.do(http.MethodPut, "/api/resume/"+string(r.heat.ID), map[string]int64{"t": 2, "d": 100})
	_, body := r.get("/api/resume")
	var list []resumeJSON
	_ = json.Unmarshal(body, &list)
	if len(list) != 2 || list[0].ID != r.heat.ID {
		t.Errorf("list = %s, want the latest first", body)
	}

	// A position for a title that has left the library is not named.
	_ = r.resume.Put(r.adults.ID, "gone-from-the-library", 5, 10, 0)
	_, body = r.get("/api/resume")
	if strings.Contains(string(body), "gone-from-the-library") {
		t.Errorf("a vanished title is listed: %s", body)
	}
}

func TestResume_DeleteAndNonsense(t *testing.T) {
	r := newProfRig(t, false)
	r.choose(r.kids, "")
	path := "/api/resume/" + string(r.bear.ID)
	r.do(http.MethodPut, path, map[string]int64{"t": 50, "d": 100})
	if resp, _ := r.do(http.MethodDelete, path, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE = %d", resp.StatusCode)
	}
	if _, body := r.get("/api/resume"); strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("after delete: %s", body)
	}
	for name, b := range map[string]map[string]int64{
		"past the end": {"t": 101, "d": 100}, "negative": {"t": -1, "d": 100}, "no length": {"t": 0, "d": 0},
	} {
		if resp, _ := r.do(http.MethodPut, path, b); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, resp.StatusCode)
		}
	}
	if resp, _ := r.do(http.MethodPut, path, map[string]int64{"t": 5, "d": 100}, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site PUT = %d", resp.StatusCode)
	}
	if resp, _ := r.do(http.MethodDelete, path, nil, "Sec-Fetch-Site", "cross-site"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site DELETE = %d", resp.StatusCode)
	}
}

func TestResume_AGuestCannotReadOrWrite(t *testing.T) {
	r := newProfRig(t, false)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		path := "/api/resume"
		if m != http.MethodGet {
			path += "/" + string(r.bear.ID)
		}
		resp, _ := r.do(m, path, map[string]int64{"t": 1, "d": 10})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s as a guest = %d, want 401", m, path, resp.StatusCode)
		}
	}
}

func TestResume_WithOneProfileItIsEveryonesAndNeedsNoCookie(t *testing.T) {
	r := newProfRig(t, false)
	for _, p := range []profile.Profile{r.kids, r.adults} {
		_ = r.svc.Delete(p.ID)
	}
	path := "/api/resume/" + string(r.heat.ID)
	if resp, _ := r.do(http.MethodPut, path, map[string]int64{"t": 7, "d": 70}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT = %d", resp.StatusCode)
	}
	if got := r.resume.Get(profile.EveryoneID)[r.heat.ID]; got.T != 7 {
		t.Errorf("stored under Everyone: %+v", got)
	}
}

func mustURL(s string) *url.URL { u, _ := url.Parse(s); return u }
