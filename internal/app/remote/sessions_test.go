package remote

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"eincast/internal/domain/media"
)

func newCreds() (*credentials, *clock) {
	clk := newClock()
	return newCredentials(clk.now), clk
}

func TestSession_LivesForItsTTLThenExpires(t *testing.T) {
	c, clk := newCreds()
	sess := c.issue("198.51.100.1")

	if !c.validSession(sess.Token) {
		t.Fatal("a fresh session is invalid")
	}
	clk.advance(sessionTTL - time.Second)
	if !c.validSession(sess.Token) {
		t.Error("session expired early")
	}
	clk.advance(2 * time.Second)
	if c.validSession(sess.Token) {
		t.Error("session works after its TTL")
	}
	if c.count() != 0 {
		t.Errorf("count = %d, want the expired session pruned", c.count())
	}
}

func TestSession_ForgeriesAreRefused(t *testing.T) {
	c, clk := newCreds()
	good := c.issue("198.51.100.1").Token
	parts := strings.Split(good, ".")
	id, exp, sig := parts[0], parts[1], parts[2]

	farFuture := strconv.FormatInt(clk.now().Add(100*24*time.Hour).Unix(), 10)
	other, _ := newCreds() // same code, different random keys

	cases := map[string]string{
		"empty":                    "",
		"no dots":                  "garbage",
		"two parts":                id + "." + exp,
		"four parts":               good + ".x",
		"empty id":                 "." + exp + "." + sig,
		"empty expiry":             id + ".." + sig,
		"empty signature":          id + "." + exp + ".",
		"signature not base64":     id + "." + exp + ".!!!",
		"signature changed":        id + "." + exp + "." + flip(sig),
		"id changed":               flip(id) + "." + exp + "." + sig,
		"expiry extended":          id + "." + farFuture + "." + sig,
		"expiry not a number":      id + ".soon." + sig,
		"issued under another key": other.issue("198.51.100.1").Token,
		"a link signature":         id + "." + exp + "." + c.signLink(media.ID(id)).Sig,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if c.validSession(token) {
				t.Errorf("accepted %q", token)
			}
		})
	}
}

func TestSession_AWellSignedIDWeNeverIssuedIsRefused(t *testing.T) {
	// Revocation seam: removing the id from the registry ends the session even
	// though the signature still checks out.
	c, _ := newCreds()
	sess := c.issue("198.51.100.1")
	id := strings.Split(sess.Token, ".")[0]

	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()

	if c.validSession(sess.Token) {
		t.Error("a session that is no longer registered still works")
	}
}

func TestSession_RotateSignsEveryoneOut(t *testing.T) {
	c, _ := newCreds()
	sess := c.issue("198.51.100.1")
	link := c.signLink("film")

	c.rotate()

	if c.validSession(sess.Token) || c.verifyLink("film", link.Sig, link.Expires.Unix()) {
		t.Error("credentials survived a key rotation")
	}
	if c.count() != 0 {
		t.Error("sessions survived a key rotation")
	}
}

func TestSession_TokensAreUnique(t *testing.T) {
	c, _ := newCreds()
	seen := map[string]bool{}
	for range 200 {
		tok := c.issue("198.51.100.1").Token
		if seen[tok] {
			t.Fatal("two sessions share a token")
		}
		seen[tok] = true
	}
}

func TestLink_AuthorisesExactlyOneItemUntilItExpires(t *testing.T) {
	c, clk := newCreds()
	link := c.signLink("film")
	exp := link.Expires.Unix()

	if !c.verifyLink("film", link.Sig, exp) {
		t.Fatal("a fresh link is invalid")
	}
	if want := clk.now().Add(linkTTL).Truncate(time.Second); !link.Expires.Equal(want) {
		t.Errorf("expires %v, want %v", link.Expires, want)
	}

	cases := map[string]struct {
		id  media.ID
		sig string
		exp int64
	}{
		"another item":         {"song", link.Sig, exp},
		"extended expiry":      {"film", link.Sig, exp + 3600},
		"shortened expiry":     {"film", link.Sig, exp - 1},
		"signature changed":    {"film", flip(link.Sig), exp},
		"signature not base64": {"film", "!!!", exp},
		"empty signature":      {"film", "", exp},
		"empty id":             {"", link.Sig, exp},
		"id with a suffix":     {"film2", link.Sig, exp},
	}
	for name, c2 := range cases {
		t.Run(name, func(t *testing.T) {
			if c.verifyLink(c2.id, c2.sig, c2.exp) {
				t.Error("accepted")
			}
		})
	}

	clk.advance(linkTTL)
	if !c.verifyLink("film", link.Sig, exp) {
		t.Error("link expired at its stated time, not after it")
	}
	clk.advance(time.Second)
	if c.verifyLink("film", link.Sig, exp) {
		t.Error("link works after it expired")
	}
}

func TestLink_FieldBoundariesAreNotAmbiguous(t *testing.T) {
	// ("ab", exp) and ("a", "b"+exp) must not sign the same bytes.
	c, _ := newCreds()
	link := c.signLink("ab")
	if c.verifyLink("a", link.Sig, link.Expires.Unix()) {
		t.Error("a signature for one id verified for a prefix of it")
	}
}

func TestSessionAndLinkKeysAreIndependent(t *testing.T) {
	c, _ := newCreds()
	sess := c.issue("198.51.100.1")
	sig := strings.Split(sess.Token, ".")[2]

	// A session's signature must not double as a link signature.
	if c.verifyLink("anything", sig, sess.Expires.Unix()) {
		t.Error("a session signature verified as a stream link")
	}
}

// flip changes one character, keeping the string valid base64url.
func flip(s string) string {
	if s == "" {
		return "A"
	}
	repl := "A"
	if s[0] == 'A' {
		repl = "B"
	}
	return repl + s[1:]
}
