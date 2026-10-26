package remote

import (
	"strings"
	"testing"
)

func TestPINEncoding_RoundTripsAndRejectsDamage(t *testing.T) {
	p := newPINStore(newSecrets())
	p.cost = 1 << 4
	if err := p.set(testPIN); err != nil {
		t.Fatal(err)
	}
	stored := p.secrets.(*fakeSecrets).stored(pinKey)

	salt, hash, err := decodePIN(stored)
	if err != nil || len(salt) != saltLen || len(hash) != hashLen {
		t.Fatalf("decode(%q) = %d, %d, %v", stored, len(salt), len(hash), err)
	}

	parts := strings.Split(stored, "$")
	bad := map[string]string{
		"empty":         "",
		"plain PIN":     testPIN,
		"future format": "v2$" + parts[1] + "$" + parts[2],
		"missing hash":  "v1$" + parts[1],
		"extra field":   stored + "$x",
		"short salt":    "v1$AAAA$" + parts[2],
		"short hash":    "v1$" + parts[1] + "$AAAA",
		"not base64":    "v1$%%%$%%%",
	}
	for name, s := range bad {
		if _, _, err := decodePIN(s); err == nil {
			t.Errorf("%s: decoded without error", name)
		}
	}
}

func TestPINs_GetFreshSalts(t *testing.T) {
	secrets := newSecrets()
	p := newPINStore(secrets)
	p.cost = 1 << 4
	_ = p.set(testPIN)
	first := secrets.stored(pinKey)
	_ = p.set(testPIN)
	if secrets.stored(pinKey) == first {
		t.Error("the same PIN produced the same stored value twice; salts are not random")
	}
}

func TestPINVerify_DoesNotAcceptNearMisses(t *testing.T) {
	p := newPINStore(newSecrets())
	p.cost = 1 << 4
	if p.verify("") || p.verify(testPIN) {
		t.Fatal("verified before any PIN was set")
	}
	_ = p.set(testPIN)
	if !p.verify(testPIN) {
		t.Fatal("the right PIN failed")
	}
	for _, near := range []string{"", testPIN + " ", strings.ToUpper(testPIN), testPIN[1:]} {
		if p.verify(near) {
			t.Errorf("accepted %q", near)
		}
	}
}
