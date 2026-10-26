package dlna

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"eincast/internal/domain/media"
)

type fakeLib struct{ items []media.Item }

func (f *fakeLib) List() []media.Item { return f.items }

func libItem(rel string, kind media.Kind, size int64) media.Item {
	name := rel[lastSlash(rel)+1:]
	return media.Item{ID: media.NewID(testLib, rel), Name: name, RelPath: rel, Kind: kind, Size: size, ModTime: time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC)}
}

func newServer(t *testing.T, items ...media.Item) *httptest.Server {
	t.Helper()
	h := NewHandler(Config{Library: &fakeLib{items: items}, Name: "Living <room> & more", UUID: "11111111-2222-5333-8444-555555555555", Version: "1.2.3"})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// soapCall sends a control request the way a TV does.
func soapCall(t *testing.T, srv *httptest.Server, path, service, action, args string) (*http.Response, string) {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:%s xmlns:u="%s">%s</u:%s></s:Body></s:Envelope>`,
		action, service, args, action)
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"%s#%s"`, service, action))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// outputs reads the response arguments out of a SOAP response body.
func outputs(t *testing.T, body string) map[string]string {
	t.Helper()
	var env struct {
		Body struct {
			Inner struct {
				Items []struct {
					XMLName xml.Name
					Text    string `xml:",chardata"`
				} `xml:",any"`
			} `xml:",any"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("response is not well-formed SOAP: %v\n%s", err, body)
	}
	out := map[string]string{}
	for _, it := range env.Body.Inner.Items {
		out[it.XMLName.Local] = it.Text
	}
	return out
}

type didlDoc struct {
	Containers []struct {
		ID         string `xml:"id,attr"`
		ParentID   string `xml:"parentID,attr"`
		ChildCount int    `xml:"childCount,attr"`
		Title      string `xml:"title"`
		Class      string `xml:"class"`
	} `xml:"container"`
	Items []struct {
		ID       string `xml:"id,attr"`
		ParentID string `xml:"parentID,attr"`
		Title    string `xml:"title"`
		Class    string `xml:"class"`
		Res      struct {
			ProtocolInfo string `xml:"protocolInfo,attr"`
			Size         int64  `xml:"size,attr"`
			URL          string `xml:",chardata"`
		} `xml:"res"`
	} `xml:"item"`
}

func parseDIDL(t *testing.T, result string) didlDoc {
	t.Helper()
	var d didlDoc
	if err := xml.Unmarshal([]byte(result), &d); err != nil {
		t.Fatalf("Result is not well-formed DIDL-Lite: %v\n%s", err, result)
	}
	return d
}

func browse(t *testing.T, srv *httptest.Server, id, flag string, start, count int) (map[string]string, didlDoc) {
	t.Helper()
	args := fmt.Sprintf(`<ObjectID>%s</ObjectID><BrowseFlag>%s</BrowseFlag><Filter>*</Filter><StartingIndex>%d</StartingIndex><RequestedCount>%d</RequestedCount><SortCriteria></SortCriteria>`,
		id, flag, start, count)
	resp, body := soapCall(t, srv, cdControlPath, contentDirectory, "Browse", args)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Browse(%s) status = %d\n%s", id, resp.StatusCode, body)
	}
	out := outputs(t, body)
	return out, parseDIDL(t, out["Result"])
}

func TestBrowse_TheRootListsTheKindsThatExist(t *testing.T) {
	srv := newServer(t, libItem("a.mkv", media.KindVideo, 5), libItem("b.mp3", media.KindAudio, 5))
	out, d := browse(t, srv, "0", "BrowseDirectChildren", 0, 0)

	if out["NumberReturned"] != "2" || out["TotalMatches"] != "2" {
		t.Errorf("counts = %s/%s", out["NumberReturned"], out["TotalMatches"])
	}
	if len(d.Containers) != 2 || d.Containers[0].Title != "Videos" || d.Containers[1].Title != "Music" {
		t.Fatalf("containers = %+v", d.Containers)
	}
	if d.Containers[0].ParentID != "0" || d.Containers[0].ChildCount != 1 || d.Containers[0].Class != "object.container.storageFolder" {
		t.Errorf("Videos = %+v", d.Containers[0])
	}
}

func TestBrowse_AFileIsDescribedForAPlayer(t *testing.T) {
	srv := newServer(t, libItem("tv/Show & Co <1>.mkv", media.KindVideo, 12345))
	_, root := browse(t, srv, "0", "BrowseDirectChildren", 0, 0)
	_, tv := browse(t, srv, root.Containers[0].ID, "BrowseDirectChildren", 0, 0)
	_, files := browse(t, srv, tv.Containers[0].ID, "BrowseDirectChildren", 0, 0)

	if len(files.Items) != 1 {
		t.Fatalf("files = %+v", files)
	}
	it := files.Items[0]
	id := media.NewID(testLib, "tv/Show & Co <1>.mkv")
	if it.Title != "Show & Co <1>" {
		t.Errorf("title = %q: special characters must survive the double escaping", it.Title)
	}
	if it.Class != "object.item.videoItem" || it.ParentID != tv.Containers[0].ID {
		t.Errorf("item = %+v", it)
	}
	if want := srv.URL + "/stream/" + string(id); it.Res.URL != want {
		t.Errorf("res URL = %q, want %q (the address the TV used, and the ordinary stream route)", it.Res.URL, want)
	}
	if it.Res.Size != 12345 {
		t.Errorf("size = %d", it.Res.Size)
	}
	pi := it.Res.ProtocolInfo
	if !strings.HasPrefix(pi, "http-get:*:video/x-matroska:") || !strings.Contains(pi, "DLNA.ORG_OP=01") || !strings.Contains(pi, "DLNA.ORG_FLAGS=01700000000000000000000000000000") {
		t.Errorf("protocolInfo = %q", pi)
	}
}

func TestBrowse_KindsMapToUPnPClasses(t *testing.T) {
	srv := newServer(t, libItem("s.flac", media.KindAudio, 1), libItem("p.jpg", media.KindImage, 1))
	_, root := browse(t, srv, "0", "BrowseDirectChildren", 0, 0)
	classes := map[string]string{}
	infos := map[string]string{}
	for _, c := range root.Containers {
		_, d := browse(t, srv, c.ID, "BrowseDirectChildren", 0, 0)
		for _, it := range d.Items {
			classes[it.Title] = it.Class
			infos[it.Title] = it.Res.ProtocolInfo
		}
	}
	if classes["s"] != "object.item.audioItem.musicTrack" || classes["p"] != "object.item.imageItem.photo" {
		t.Errorf("classes = %v", classes)
	}
	if strings.Contains(infos["p"], "DLNA.ORG_OP=01") || !strings.Contains(infos["p"], "image/jpeg") {
		t.Errorf("photo protocolInfo = %q: photos are fetched whole", infos["p"])
	}
}

func TestBrowse_MetadataOfOneObject(t *testing.T) {
	srv := newServer(t, libItem("a.mkv", media.KindVideo, 5))
	out, d := browse(t, srv, "k-video", "BrowseMetadata", 0, 0)
	if out["TotalMatches"] != "1" || len(d.Containers) != 1 || d.Containers[0].ID != "k-video" || d.Containers[0].ChildCount != 1 {
		t.Errorf("metadata = %v %+v", out, d)
	}
	_, d = browse(t, srv, "i-"+string(media.NewID(testLib, "a.mkv")), "BrowseMetadata", 0, 0)
	if len(d.Items) != 1 || d.Items[0].Title != "a" {
		t.Errorf("item metadata = %+v", d)
	}
}

func TestBrowse_Paging(t *testing.T) {
	var items []media.Item
	for i := 1; i <= 25; i++ {
		items = append(items, libItem(fmt.Sprintf("f%02d.mkv", i), media.KindVideo, 1))
	}
	srv := newServer(t, items...)

	out, d := browse(t, srv, "k-video", "BrowseDirectChildren", 0, 10)
	if out["NumberReturned"] != "10" || out["TotalMatches"] != "25" || d.Items[0].Title != "f01" {
		t.Errorf("page 1: %v first=%s", out, d.Items[0].Title)
	}
	out, d = browse(t, srv, "k-video", "BrowseDirectChildren", 20, 10)
	if out["NumberReturned"] != "5" || d.Items[0].Title != "f21" || d.Items[4].Title != "f25" {
		t.Errorf("last page: %v", out)
	}
	out, _ = browse(t, srv, "k-video", "BrowseDirectChildren", 100, 10)
	if out["NumberReturned"] != "0" || out["TotalMatches"] != "25" {
		t.Errorf("past the end: %v", out)
	}
	out, _ = browse(t, srv, "k-video", "BrowseDirectChildren", 5, 0)
	if out["NumberReturned"] != "20" {
		t.Errorf("count 0 means all the rest: %v", out)
	}
}

func TestBrowse_EmptyLibrary(t *testing.T) {
	srv := newServer(t)
	out, d := browse(t, srv, "0", "BrowseDirectChildren", 0, 0)
	if out["NumberReturned"] != "0" || len(d.Containers) != 0 {
		t.Errorf("empty library = %v %+v", out, d)
	}
}

func TestBrowse_ErrorsAreUPnPFaults(t *testing.T) {
	srv := newServer(t, libItem("a.mkv", media.KindVideo, 5))
	file := "i-" + string(media.NewID(testLib, "a.mkv"))
	tests := []struct {
		name, args string
		code       int
	}{
		{"unknown object", `<ObjectID>nope</ObjectID><BrowseFlag>BrowseMetadata</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/>`, 701},
		{"a path is not an object", `<ObjectID>../../etc/passwd</ObjectID><BrowseFlag>BrowseMetadata</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/>`, 701},
		{"children of a file", `<ObjectID>` + file + `</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/>`, 710},
		{"bad flag", `<ObjectID>0</ObjectID><BrowseFlag>Everything</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/>`, 402},
		{"bad index", `<ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>-3</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria/>`, 402},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := soapCall(t, srv, cdControlPath, contentDirectory, "Browse", tt.args)
			m := regexp.MustCompile(`<errorCode>(\d+)</errorCode>`).FindStringSubmatch(body)
			if resp.StatusCode != http.StatusInternalServerError || m == nil || m[1] != strconv.Itoa(tt.code) {
				t.Fatalf("status = %d, body = %s; want a fault with code %d", resp.StatusCode, body, tt.code)
			}
		})
	}
}

func TestControl_SimpleActions(t *testing.T) {
	srv := newServer(t, libItem("a.mkv", media.KindVideo, 5))

	_, body := soapCall(t, srv, cdControlPath, contentDirectory, "GetSearchCapabilities", "")
	if got, ok := outputs(t, body)["SearchCaps"]; !ok || got != "" {
		t.Errorf("SearchCaps = %q, %v", got, ok)
	}
	_, body = soapCall(t, srv, cdControlPath, contentDirectory, "GetSortCapabilities", "")
	if got, ok := outputs(t, body)["SortCaps"]; !ok || got != "" {
		t.Errorf("SortCaps = %q, %v", got, ok)
	}
	_, body = soapCall(t, srv, cdControlPath, contentDirectory, "GetSystemUpdateID", "")
	if _, err := strconv.ParseUint(outputs(t, body)["Id"], 10, 32); err != nil {
		t.Errorf("Id = %q: %v", outputs(t, body)["Id"], err)
	}

	_, body = soapCall(t, srv, cmControlPath, connectionManager, "GetProtocolInfo", "")
	o := outputs(t, body)
	if !strings.Contains(o["Source"], "http-get:*:video/mp4:*") || !strings.Contains(o["Source"], "audio/mpeg") {
		t.Errorf("Source = %q", o["Source"])
	}
	if _, ok := o["Sink"]; !ok {
		t.Error("Sink missing")
	}
	_, body = soapCall(t, srv, cmControlPath, connectionManager, "GetCurrentConnectionIDs", "")
	if outputs(t, body)["ConnectionIDs"] != "0" {
		t.Errorf("ConnectionIDs = %q", outputs(t, body)["ConnectionIDs"])
	}
	_, body = soapCall(t, srv, cmControlPath, connectionManager, "GetCurrentConnectionInfo", "<ConnectionID>0</ConnectionID>")
	if o := outputs(t, body); o["Status"] != "OK" || o["Direction"] != "Output" {
		t.Errorf("connection info = %v", o)
	}
}

func TestSystemUpdateID_ChangesWithTheLibrary(t *testing.T) {
	a := updateID([]media.Item{libItem("a.mkv", media.KindVideo, 5)})
	same := updateID([]media.Item{libItem("a.mkv", media.KindVideo, 5)})
	grown := updateID([]media.Item{libItem("a.mkv", media.KindVideo, 6)})
	more := updateID([]media.Item{libItem("a.mkv", media.KindVideo, 5), libItem("b.mkv", media.KindVideo, 1)})
	if a != same || a == grown || a == more {
		t.Errorf("ids: a=%d same=%d grown=%d more=%d", a, same, grown, more)
	}
}

func TestControl_UnknownActionAndServiceAreRefused(t *testing.T) {
	srv := newServer(t)
	_, body := soapCall(t, srv, cdControlPath, contentDirectory, "Search", "<ContainerID>0</ContainerID>")
	if !strings.Contains(body, "<errorCode>401</errorCode>") {
		t.Errorf("unknown action: %s", body)
	}
	_, body = soapCall(t, srv, cdControlPath, "urn:schemas-upnp-org:service:Other:1", "Browse", "")
	if !strings.Contains(body, "<errorCode>401</errorCode>") {
		t.Errorf("wrong service: %s", body)
	}
}

func TestControl_MalformedAndMismatchedRequests(t *testing.T) {
	srv := newServer(t)
	post := func(headers map[string]string, body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+cdControlPath, strings.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code, body := post(nil, "this is not xml <<<"); code != http.StatusInternalServerError || !strings.Contains(body, "UPnPError") {
		t.Errorf("garbage: %d %s", code, body)
	}
	if code, _ := post(nil, ""); code != http.StatusInternalServerError {
		t.Errorf("empty: %d", code)
	}
	env := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetSystemUpdateID xmlns:u="` + contentDirectory + `"/></s:Body></s:Envelope>`
	if code, body := post(map[string]string{"SOAPACTION": `"` + contentDirectory + `#Browse"`}, env); code != http.StatusInternalServerError || !strings.Contains(body, "401") {
		t.Errorf("SOAPACTION that disagrees with the body: %d %s", code, body)
	}
	if code, _ := post(map[string]string{"SOAPACTION": contentDirectory + "#GetSystemUpdateID"}, env); code != http.StatusOK {
		t.Errorf("unquoted SOAPACTION: %d", code)
	}
	if code, _ := post(nil, env); code != http.StatusOK {
		t.Errorf("no SOAPACTION header: %d, want the body alone to decide", code)
	}
	if code, body := post(nil, strings.Repeat("x", maxSOAPBytes+10)); code != http.StatusInternalServerError || !strings.Contains(body, "402") {
		t.Errorf("oversized: %d %s", code, body)
	}
}

func TestDocuments(t *testing.T) {
	srv := newServer(t)
	get := func(path string) (*http.Response, string) {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}

	resp, body := get(devicePath)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/xml") {
		t.Fatalf("device.xml: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	var dev struct {
		Device struct {
			DeviceType   string `xml:"deviceType"`
			FriendlyName string `xml:"friendlyName"`
			UDN          string `xml:"UDN"`
			Services     []struct {
				Type        string `xml:"serviceType"`
				SCPDURL     string `xml:"SCPDURL"`
				ControlURL  string `xml:"controlURL"`
				EventSubURL string `xml:"eventSubURL"`
			} `xml:"serviceList>service"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal([]byte(body), &dev); err != nil {
		t.Fatalf("device.xml is not well-formed: %v\n%s", err, body)
	}
	d := dev.Device
	if d.DeviceType != deviceType || d.FriendlyName != "Living <room> & more" || d.UDN != "uuid:11111111-2222-5333-8444-555555555555" {
		t.Errorf("device = %+v", d)
	}
	if len(d.Services) != 2 {
		t.Fatalf("services = %+v", d.Services)
	}
	// Every URL the description hands out must work on this server.
	for _, s := range d.Services {
		if r, _ := get(s.SCPDURL); r.StatusCode != 200 {
			t.Errorf("SCPD %s: %d", s.SCPDURL, r.StatusCode)
		}
		if r, err := srv.Client().Post(srv.URL+s.ControlURL, "text/xml", strings.NewReader("<x/>")); err != nil || r.StatusCode == 404 {
			t.Errorf("control URL %s is not served: %v %v", s.ControlURL, r, err)
		}
		req, _ := http.NewRequest("SUBSCRIBE", srv.URL+s.EventSubURL, nil)
		if r, err := srv.Client().Do(req); err != nil || r.StatusCode != 200 {
			t.Errorf("event URL %s: %v %v", s.EventSubURL, r, err)
		}
	}

	for _, path := range []string{cdSCPDPath, cmSCPDPath} {
		_, body := get(path)
		var doc struct {
			Actions []struct {
				Name string `xml:"name"`
			} `xml:"actionList>action"`
		}
		if err := xml.Unmarshal([]byte(body), &doc); err != nil || len(doc.Actions) == 0 {
			t.Errorf("%s: %v actions=%d", path, err, len(doc.Actions))
		}
	}
	if resp, _ := get("/dlna/unknown"); resp.StatusCode != 404 {
		t.Errorf("unknown path: %d", resp.StatusCode)
	}
	if resp, _ := get(cdControlPath); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET on a control URL: %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Server") == "" {
		t.Error("no Server header")
	}
}

func TestEvents_SubscribeSucceedsAndNeverCallsBack(t *testing.T) {
	srv := newServer(t)
	// The callback points at a server that fails the test if it is ever called.
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the server called back %s: it must never contact an address a client supplied", r.URL)
	}))
	defer cb.Close()

	req, _ := http.NewRequest("SUBSCRIBE", srv.URL+cdEventPath, nil)
	req.Header.Set("CALLBACK", "<"+cb.URL+"/notify>")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", "Second-300")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("SID"), "uuid:") || resp.Header.Get("TIMEOUT") == "" {
		t.Fatalf("SUBSCRIBE: %d SID=%q TIMEOUT=%q", resp.StatusCode, resp.Header.Get("SID"), resp.Header.Get("TIMEOUT"))
	}

	req, _ = http.NewRequest("UNSUBSCRIBE", srv.URL+cdEventPath, nil)
	req.Header.Set("SID", resp.Header.Get("SID"))
	if r, err := srv.Client().Do(req); err != nil || r.StatusCode != 200 {
		t.Errorf("UNSUBSCRIBE: %v %v", r, err)
	}
	time.Sleep(50 * time.Millisecond) // a wrongly sent callback would land in this window
}

func TestDeviceUUID(t *testing.T) {
	a, b := DeviceUUID("studio-mac"), DeviceUUID("studio-mac")
	if a != b || a == DeviceUUID("other-pc") {
		t.Errorf("uuids: %s %s", a, b)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(a) {
		t.Errorf("%q is not a version 5 UUID", a)
	}
}
