package dlna

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"eincast/internal/domain/media"
)

// Library is the part of the library the handler reads. Declared here, at the
// consumer.
type Library interface {
	List() []media.Item
}

// Config is what the handler needs to describe and serve one run of the server.
type Config struct {
	Library Library
	// Name is what a TV lists the server as.
	Name string
	// UUID is the server's stable identity on the network (without "uuid:").
	UUID    string
	Version string
}

// Handler serves everything under /dlna/: the device description, the two
// service descriptions, their control endpoints, and the event endpoints. It
// is mounted by the HTTP server only while Smart TV discovery is on.
type Handler struct {
	cfg    Config
	server string // the SERVER header value
}

// NewHandler returns a Handler for cfg.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg, server: ServerHeader(cfg.Version)}
}

// ServerHeader is the SERVER string UPnP wants: "OS/version UPnP/1.0 Product/version".
func ServerHeader(version string) string {
	return runtime.GOOS + "/1.0 UPnP/1.0 EinCast/" + version
}

// ServeHTTP routes by exact path. Clients learn these paths from the device
// description, and nothing in a request is ever used as a file path.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", h.server)
	switch r.URL.Path {
	case devicePath:
		h.serveDoc(w, r, deviceXML(h.cfg.Name, h.cfg.UUID, h.cfg.Version))
	case cdSCPDPath:
		h.serveDoc(w, r, contentDirectorySCPD())
	case cmSCPDPath:
		h.serveDoc(w, r, connectionManagerSCPD())
	case cdControlPath:
		h.control(w, r, contentDirectory, h.contentDirectoryAction)
	case cmControlPath:
		h.control(w, r, connectionManager, h.connectionManagerAction)
	case cdEventPath, cmEventPath:
		h.events(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) serveDoc(w http.ResponseWriter, r *http.Request, body string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", deviceXMLContent)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.WriteString(w, body)
}

// actionFunc runs one action and returns its output arguments.
type actionFunc func(req *soapRequest, r *http.Request) ([]response, *soapError)

func (h *Handler) control(w http.ResponseWriter, r *http.Request, serviceType string, run actionFunc) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := readSOAP(r, serviceType)
	if err != nil {
		writeSOAPFault(w, asSOAPError(err))
		return
	}
	out, serr := run(req, r)
	if serr != nil {
		writeSOAPFault(w, serr)
		return
	}
	writeSOAPResponse(w, serviceType, req.action, out)
}

func asSOAPError(err error) *soapError {
	if se, ok := err.(*soapError); ok {
		return se
	}
	return &soapError{errActionFailed, "action failed"}
}

func (h *Handler) contentDirectoryAction(req *soapRequest, r *http.Request) ([]response, *soapError) {
	switch req.action {
	case "GetSearchCapabilities":
		return []response{{"SearchCaps", ""}}, nil
	case "GetSortCapabilities":
		return []response{{"SortCaps", ""}}, nil
	case "GetSystemUpdateID":
		return []response{{"Id", strconv.FormatUint(uint64(updateID(h.cfg.Library.List())), 10)}}, nil
	case "Browse":
		return h.browse(req, r)
	default:
		return nil, &soapError{errInvalidAction, "no such action: " + req.action}
	}
}

// browse answers a folder listing or a single object's metadata, one page at a
// time. The whole tree is built from the current library on each call: TVs ask
// rarely, and a fresh tree can never be stale.
func (h *Handler) browse(req *soapRequest, r *http.Request) ([]response, *soapError) {
	id := req.args["ObjectID"]
	flag := req.args["BrowseFlag"]
	start, err1 := parseCount(req.args["StartingIndex"])
	count, err2 := parseCount(req.args["RequestedCount"])
	if err1 != nil || err2 != nil {
		return nil, &soapError{errInvalidArgs, "StartingIndex and RequestedCount must be whole numbers"}
	}

	items := h.cfg.Library.List()
	t := buildTree(items)
	n, ok := t.byID[id]
	if !ok {
		return nil, &soapError{errNoSuchObject, "no such object"}
	}

	var page []*node
	total := 1
	switch flag {
	case "BrowseMetadata":
		page = []*node{n}
	case "BrowseDirectChildren":
		if !n.isContainer() {
			return nil, &soapError{errNoSuchContain, "not a container"}
		}
		total = len(n.children)
		page = window(n.children, start, count)
	default:
		return nil, &soapError{errInvalidArgs, "BrowseFlag must be BrowseMetadata or BrowseDirectChildren"}
	}

	return []response{
		{"Result", didl(page, "http://"+r.Host)},
		{"NumberReturned", strconv.Itoa(len(page))},
		{"TotalMatches", strconv.Itoa(total)},
		{"UpdateID", strconv.FormatUint(uint64(updateID(items)), 10)},
	}, nil
}

// window is the page of nodes starting at index start with at most count items
// (0 means "all the rest"), clamped to what exists.
func window(nodes []*node, start, count uint64) []*node {
	if start >= uint64(len(nodes)) {
		return nil
	}
	rest := nodes[start:]
	if count > 0 && count < uint64(len(rest)) {
		rest = rest[:count]
	}
	return rest
}

func parseCount(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 32)
}

// updateID changes whenever the library does, which is what SystemUpdateID is
// for: a client that kept a listing compares it to know whether to ask again.
func updateID(items []media.Item) uint32 {
	h := fnv.New32a()
	for _, it := range items {
		fmt.Fprintf(h, "%s|%d|%d|%d;", it.ID, it.Size, it.ModTime.UnixNano(), len(it.Subtitles))
	}
	return h.Sum32()
}

func (h *Handler) connectionManagerAction(req *soapRequest, _ *http.Request) ([]response, *soapError) {
	switch req.action {
	case "GetProtocolInfo":
		return []response{{"Source", strings.Join(sourceProtocols(), ",")}, {"Sink", ""}}, nil
	case "GetCurrentConnectionIDs":
		return []response{{"ConnectionIDs", "0"}}, nil
	case "GetCurrentConnectionInfo":
		return []response{
			{"RcsID", "-1"}, {"AVTransportID", "-1"}, {"ProtocolInfo", ""},
			{"PeerConnectionManager", ""}, {"PeerConnectionID", "-1"},
			{"Direction", "Output"}, {"Status", "OK"},
		}, nil
	default:
		return nil, &soapError{errInvalidAction, "no such action: " + req.action}
	}
}

// subscriptionTimeout is how long a subscription is said to last.
const subscriptionTimeout = "Second-1800"

// events accepts SUBSCRIBE and UNSUBSCRIBE. Many TVs refuse a server whose
// event URL answers an error, so both succeed; but nothing is ever sent to the
// callback. The only state a client could be told about is the library changing,
// which it learns from SystemUpdateID when it browses, and delivering to an
// address supplied by a stranger on the network would let that stranger aim this
// computer at any other address.
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "SUBSCRIBE":
		w.Header().Set("SID", "uuid:"+randomUUID())
		w.Header().Set("TIMEOUT", subscriptionTimeout)
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	case "UNSUBSCRIBE":
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "SUBSCRIBE, UNSUBSCRIBE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
