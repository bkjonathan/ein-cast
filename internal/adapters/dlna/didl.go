package dlna

import (
	"encoding/xml"
	"fmt"
	"strings"

	"eincast/internal/domain/media"
)

// didlHeader opens a DIDL-Lite document, the XML a ContentDirectory answers
// with. The three namespaces are the ones every client expects.
const didlHeader = `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" ` +
	`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
	`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
	`xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`

// streamFlags is the DLNA.ORG_FLAGS value of a file served over HTTP with byte
// ranges: streaming transfer mode, background transfer, connection stall and
// DLNA 1.5. TVs are picky about the exact digits, so this is the one value
// every media server of this kind sends.
const streamFlags = "01700000000000000000000000000000"

// didl encodes nodes as DIDL-Lite. base is the address the client reached us at
// ("http://192.168.1.20:8765"): files are given as absolute URLs on it.
func didl(nodes []*node, base string) string {
	var b strings.Builder
	b.WriteString(didlHeader)
	for _, n := range nodes {
		if n.isContainer() {
			writeContainer(&b, n)
		} else {
			writeItem(&b, n, base)
		}
	}
	b.WriteString("</DIDL-Lite>")
	return b.String()
}

func writeContainer(b *strings.Builder, n *node) {
	fmt.Fprintf(b, `<container id="%s" parentID="%s" restricted="1" searchable="0" childCount="%d">`,
		attr(n.id), attr(n.parentID), len(n.children))
	fmt.Fprintf(b, `<dc:title>%s</dc:title><upnp:class>object.container.storageFolder</upnp:class></container>`, text(n.title))
}

func writeItem(b *strings.Builder, n *node, base string) {
	it := n.item
	fmt.Fprintf(b, `<item id="%s" parentID="%s" restricted="1">`, attr(n.id), attr(n.parentID))
	fmt.Fprintf(b, `<dc:title>%s</dc:title>`, text(n.title))
	fmt.Fprintf(b, `<dc:date>%s</dc:date>`, it.ModTime.UTC().Format("2006-01-02"))
	fmt.Fprintf(b, `<upnp:class>%s</upnp:class>`, upnpClass(it.Kind))
	fmt.Fprintf(b, `<res protocolInfo="%s" size="%d">%s/stream/%s</res>`,
		attr(protocolInfo(it)), it.Size, text(base), text(string(it.ID)))
	b.WriteString(`</item>`)
}

func upnpClass(k media.Kind) string {
	switch k {
	case media.KindAudio:
		return "object.item.audioItem.musicTrack"
	case media.KindImage:
		return "object.item.imageItem.photo"
	default:
		return "object.item.videoItem"
	}
}

// protocolInfo says what the file is and how it may be fetched. Video and audio
// support byte ranges (OP=01) so a TV can seek; photos are fetched whole.
func protocolInfo(it *media.Item) string {
	mime := media.ContentType(it.Name)
	if it.Kind == media.KindImage {
		return "http-get:*:" + mime + ":DLNA.ORG_OP=00;DLNA.ORG_CI=0"
	}
	return "http-get:*:" + mime + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=" + streamFlags
}

// sinkProtocols is what GetProtocolInfo lists as the formats this server can
// provide: one entry per MIME type seen in the library would be exact, but TVs
// only need the families, so the common ones are listed.
func sourceProtocols() []string {
	types := []string{
		"video/mp4", "video/x-matroska", "video/x-msvideo", "video/quicktime", "video/webm", "video/mpeg", "video/mp2t",
		"audio/mpeg", "audio/mp4", "audio/flac", "audio/x-wav", "audio/ogg", "audio/aac",
		"image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp",
	}
	out := make([]string, len(types))
	for i, m := range types {
		out[i] = "http-get:*:" + m + ":*"
	}
	return out
}

// text escapes character data for XML.
func text(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return "" // cannot happen: a strings.Builder never fails to write
	}
	return b.String()
}

// attr escapes a value for use inside a double-quoted XML attribute.
func attr(s string) string { return text(s) }
