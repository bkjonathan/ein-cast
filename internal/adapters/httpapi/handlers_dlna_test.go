package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eincast/internal/app"
)

func newDLNAFixture(t *testing.T, access Access, run app.ServeConfig, opts ...Option) (*fixture, *httptest.Server) {
	t.Helper()
	f := newFixtureWith(t, access)
	a := New(f.lib, access, &fakeRemote{}, "1.2.3", discardLog(), opts...).newAPI(run)
	srv := httptest.NewServer(a.handler())
	t.Cleanup(srv.Close)
	return f, srv
}

func TestDLNA_IsOffUnlessAskedFor(t *testing.T) {
	for name, tc := range map[string]struct {
		run  app.ServeConfig
		opts []Option
	}{
		"setting off": {app.ServeConfig{Name: "Den"}, []Option{WithDLNA("uuid-1")}},
		"no identity": {app.ServeConfig{Name: "Den", DLNA: true}, nil},
		"neither":     {app.ServeConfig{Name: "Den"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			_, srv := newDLNAFixture(t, &fakeAccess{}, tc.run, tc.opts...)
			for _, path := range []string{"/dlna/device.xml", "/dlna/cd/control", "/dlna/cd.xml"} {
				assertErrorShape(t, getFrom(t, srv, path), http.StatusNotFound, codeNotFound)
			}
		})
	}
}

func TestDLNA_ServesTheDeviceAndAListingWhenOn(t *testing.T) {
	f, srv := newDLNAFixture(t, &fakeAccess{}, app.ServeConfig{Name: "Den", DLNA: true}, WithDLNA("11111111-2222-5333-8444-555555555555"))

	resp := getFrom(t, srv, "/dlna/device.xml")
	body := string(readBody(t, resp))
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<friendlyName>Den</friendlyName>") || !strings.Contains(body, "uuid:11111111-2222-5333-8444-555555555555") {
		t.Fatalf("device.xml: %d %s", resp.StatusCode, body)
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/dlna/cd/control", strings.NewReader(
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">`+
			`<ObjectID>k-video</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/></u:Browse></s:Body></s:Envelope>`))
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	r, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	out := string(readBody(t, r))
	// The fixture's film is inside a "movies" folder, so that is what the video
	// root lists. (The per-file URLs are covered in the dlna package's tests.)
	if r.StatusCode != http.StatusOK || !strings.Contains(out, "movies") {
		t.Fatalf("Browse: %d %s", r.StatusCode, out)
	}
	if strings.Contains(out, f.root) {
		t.Error("the listing leaks the library's path on disk")
	}
}

func TestDLNA_FailsClosedWhenTheAccessTokenIsOn(t *testing.T) {
	access := &fakeAccess{}
	access.set(true, "secret")
	_, srv := newDLNAFixture(t, access, app.ServeConfig{Name: "Den", DLNA: true}, WithDLNA("uuid-1"))

	// A TV cannot send the token, so it is turned away rather than shown the library.
	for _, path := range []string{"/dlna/device.xml", "/dlna/cd.xml"} {
		if got := getFrom(t, srv, path).StatusCode; got != http.StatusUnauthorized {
			t.Errorf("GET %s with the token required: status = %d, want 401", path, got)
		}
	}
}

func TestDLNA_StreamAnswersForContentFeaturesOnlyWhenOn(t *testing.T) {
	ask := func(t *testing.T, on bool) *http.Response {
		t.Helper()
		var opts []Option
		if on {
			opts = append(opts, WithDLNA("uuid-1"))
		}
		f, srv := newDLNAFixture(t, &fakeAccess{}, app.ServeConfig{Name: "Den", DLNA: on}, opts...)
		return getFrom(t, srv, "/stream/"+string(f.movie.ID), "getcontentFeatures.dlna.org", "1")
	}

	on := ask(t, true)
	if on.Header.Get("transferMode.dlna.org") != "Streaming" || !strings.Contains(on.Header.Get("contentFeatures.dlna.org"), "DLNA.ORG_OP=01") {
		t.Errorf("with DLNA on, headers = %v", on.Header)
	}
	if off := ask(t, false); off.Header.Get("contentFeatures.dlna.org") != "" || off.Header.Get("transferMode.dlna.org") != "" {
		t.Errorf("with DLNA off, headers = %v", off.Header)
	}
}

func TestDLNA_StreamWithoutTheHeaderIsUnchanged(t *testing.T) {
	f, srv := newDLNAFixture(t, &fakeAccess{}, app.ServeConfig{Name: "Den", DLNA: true}, WithDLNA("uuid-1"))
	resp := getFrom(t, srv, "/stream/"+string(f.movie.ID))
	if resp.Header.Get("contentFeatures.dlna.org") != "" || resp.StatusCode != http.StatusOK {
		t.Errorf("status %d headers %v", resp.StatusCode, resp.Header)
	}
}
