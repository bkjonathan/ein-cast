package dlna

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxSOAPBytes bounds a control request. Real ones are a few hundred bytes.
const maxSOAPBytes = 64 << 10

const (
	soapEnvelopeNS = "http://schemas.xmlsoap.org/soap/envelope/"
	soapEncoding   = "http://schemas.xmlsoap.org/soap/encoding/"
)

// UPnP error codes (UPnP Device Architecture 1.1, section 3.2.2, and the
// ContentDirectory specification).
const (
	errInvalidAction = 401
	errInvalidArgs   = 402
	errActionFailed  = 501
	errNoSuchObject  = 701
	errNoSuchContain = 710
)

// soapError is a failure reported to the client as a SOAP fault.
type soapError struct {
	code int
	msg  string
}

func (e *soapError) Error() string { return fmt.Sprintf("upnp error %d: %s", e.code, e.msg) }

// soapRequest is a control request read from the wire.
type soapRequest struct {
	action string            // "Browse"
	args   map[string]string // argument name → text
}

// readSOAP reads a control request: the action's name comes from the body's
// first element (and is checked against the SOAPACTION header when present),
// and its arguments are that element's children, as text.
func readSOAP(r *http.Request, serviceType string) (*soapRequest, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSOAPBytes+1))
	if err != nil {
		return nil, &soapError{errActionFailed, "could not read the request"}
	}
	if len(body) > maxSOAPBytes {
		return nil, &soapError{errInvalidArgs, "request too large"}
	}

	dec := xml.NewDecoder(strings.NewReader(string(body)))
	dec.Strict = false
	var req *soapRequest
	inBody := false
	var argName string
	var argText strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &soapError{errInvalidArgs, "malformed request"}
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "Body" && t.Name.Space == soapEnvelopeNS:
				inBody = true
			case inBody && req == nil:
				req = &soapRequest{action: t.Name.Local, args: map[string]string{}}
				if t.Name.Space != "" && t.Name.Space != serviceType {
					return nil, &soapError{errInvalidAction, "unknown service"}
				}
			case req != nil && argName == "":
				argName = t.Name.Local
				argText.Reset()
			}
		case xml.CharData:
			if argName != "" {
				argText.Write(t)
			}
		case xml.EndElement:
			if argName != "" && t.Name.Local == argName {
				req.args[argName] = argText.String()
				argName = ""
			}
		}
	}
	if req == nil {
		return nil, &soapError{errInvalidAction, "no action in the request"}
	}
	if h := actionFromHeader(r.Header.Get("SOAPACTION")); h != "" && h != req.action {
		return nil, &soapError{errInvalidAction, "SOAPACTION does not match the request"}
	}
	return req, nil
}

// actionFromHeader reads "urn:...:ContentDirectory:1#Browse" (quoted or not).
func actionFromHeader(h string) string {
	h = strings.Trim(strings.TrimSpace(h), `"`)
	if i := strings.LastIndexByte(h, '#'); i >= 0 {
		return h[i+1:]
	}
	return ""
}

// response is one output argument of an action, in the order the service
// description lists them.
type response struct {
	name, value string
}

// writeSOAPResponse answers an action with its output arguments.
func writeSOAPResponse(w http.ResponseWriter, serviceType, action string, out []response) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	fmt.Fprintf(&b, `<s:Envelope xmlns:s="%s" s:encodingStyle="%s"><s:Body>`, soapEnvelopeNS, soapEncoding)
	fmt.Fprintf(&b, `<u:%sResponse xmlns:u="%s">`, action, serviceType)
	for _, o := range out {
		fmt.Fprintf(&b, `<%s>%s</%s>`, o.name, text(o.value), o.name)
	}
	fmt.Fprintf(&b, `</u:%sResponse></s:Body></s:Envelope>`, action)
	writeXML(w, http.StatusOK, b.String())
}

// writeSOAPFault answers with a UPnP error. Faults travel with HTTP status 500.
func writeSOAPFault(w http.ResponseWriter, err *soapError) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	fmt.Fprintf(&b, `<s:Envelope xmlns:s="%s" s:encodingStyle="%s"><s:Body><s:Fault>`, soapEnvelopeNS, soapEncoding)
	b.WriteString(`<faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring><detail>`)
	fmt.Fprintf(&b, `<UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError>`,
		err.code, text(err.msg))
	b.WriteString(`</detail></s:Fault></s:Body></s:Envelope>`)
	writeXML(w, http.StatusInternalServerError, b.String())
}

func writeXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("EXT", "")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
