package remote

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"

	"eincast/internal/app"
	"eincast/internal/domain/media"
)

const (
	// sessionTTL is how long a PIN sign-in lasts. After that the browser asks
	// for the PIN again.
	sessionTTL = 24 * time.Hour

	// linkTTL is how long a signed stream link plays. It must outlast the
	// longest film, but a leaked link should not work for days.
	linkTTL = 6 * time.Hour
)

var b64 = base64.RawURLEncoding

// credentials issues and checks the two things a remote client can present
// instead of the PIN: a session cookie (browsers) and a signed stream link
// (players that cannot hold cookies).
//
// Both are HMAC-SHA256 over their contents with keys that exist only in memory
// and are replaced by rotate. Turning remote access off, on again or changing
// the PIN therefore signs everyone out, and nothing about a session survives a
// restart of the app.
type credentials struct {
	now func() time.Time

	mu       sync.Mutex
	sessKey  []byte
	linkKey  []byte
	sessions map[string]sessionInfo
}

type sessionInfo struct {
	ip      string
	expires time.Time
}

func newCredentials(now func() time.Time) *credentials {
	c := &credentials{now: now}
	c.rotate()
	return c
}

// rotate replaces both keys and forgets every session. The keys are never
// empty: an empty HMAC key would make forgery trivial.
func (c *credentials) rotate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessKey = randomBytes(32)
	c.linkKey = randomBytes(32)
	c.sessions = map[string]sessionInfo{}
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b) // never fails: crypto/rand crashes the program instead
	return b
}

func mac(key []byte, parts ...string) []byte {
	h := hmac.New(sha256.New, key)
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0}) // separator, so ("ab","c") and ("a","bc") differ
	}
	return h.Sum(nil)
}

// issue starts a session for a client that just proved the PIN. The token is
// "<id>.<expiry>.<mac>"; the id also lets us count and list sessions.
func (c *credentials) issue(ip string) app.RemoteSession {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.pruneLocked(now)

	id := b64.EncodeToString(randomBytes(16))
	expires := now.Add(sessionTTL)
	c.sessions[id] = sessionInfo{ip: ip, expires: expires}

	exp := strconv.FormatInt(expires.Unix(), 10)
	sig := b64.EncodeToString(mac(c.sessKey, "session", id, exp))
	return app.RemoteSession{Token: id + "." + exp + "." + sig, Expires: expires}
}

// validSession reports whether token is a live session issued under the
// current key. The signature check comes first, so garbage costs no lock.
func (c *credentials) validSession(token string) bool {
	id, exp, sig, ok := splitToken(token)
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	want := mac(c.sessKey, "session", id, exp)
	if !hmac.Equal(sig, want) || c.now().Unix() > expires {
		return false
	}
	_, live := c.sessions[id] // an issued id, not merely a well-signed one
	return live
}

// splitToken breaks "<id>.<expiry>.<mac>" apart, decoding the MAC.
func splitToken(token string) (id, exp string, sig []byte, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return "", "", nil, false
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", "", nil, false
	}
	return parts[0], parts[1], sig, true
}

// count is the number of live sessions.
func (c *credentials) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(c.now())
	return len(c.sessions)
}

func (c *credentials) pruneLocked(now time.Time) {
	for id, s := range c.sessions {
		if !now.Before(s.expires) {
			delete(c.sessions, id)
		}
	}
}

// signLink authorises one media ID for linkTTL. The MAC covers the ID and the
// expiry, so a link works for that item only and cannot be extended.
func (c *credentials) signLink(id media.ID) app.SignedLink {
	c.mu.Lock()
	key := c.linkKey
	c.mu.Unlock()

	expires := c.now().Add(linkTTL).Truncate(time.Second)
	sig := mac(key, "stream", string(id), strconv.FormatInt(expires.Unix(), 10))
	return app.SignedLink{Sig: b64.EncodeToString(sig), Expires: expires}
}

// verifyLink reports whether sig authorises id until the unix time exp.
func (c *credentials) verifyLink(id media.ID, sig string, exp int64) bool {
	got, err := b64.DecodeString(sig)
	if err != nil {
		return false
	}
	c.mu.Lock()
	key := c.linkKey
	c.mu.Unlock()

	want := mac(key, "stream", string(id), strconv.FormatInt(exp, 10))
	return hmac.Equal(got, want) && c.now().Unix() <= exp
}
