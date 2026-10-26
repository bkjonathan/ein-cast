package dlna

import (
	"crypto/sha1"
	"fmt"
)

// DeviceUUID derives the server's identity from the machine's name, so it is the
// same on every run: a TV that has seen "this" server before does not list a
// second copy of it after a restart, and nothing has to be stored. It is a
// version 5 (name-based) UUID in the namespace of this app.
func DeviceUUID(hostname string) string {
	// namespace: a fixed UUID chosen for EinCast, as RFC 4122 asks of version 5.
	ns := [16]byte{0x6e, 0x1c, 0xa5, 0x7e, 0x3b, 0x10, 0x4c, 0x2a, 0x9d, 0x55, 0x0e, 0x1d, 0xca, 0x57, 0x00, 0x01}
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte("eincast:" + hostname))
	sum := h.Sum(nil)
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
