package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

// hlsJSSum is the SHA-256 of the vendored hls.min.js 1.6.16, as published to npm
// (hls.js-1.6.16.tgz, dist/hls.min.js). Upgrading the library means changing this
// line and ADR 0031 together, which is the point: nobody swaps a 540 KB script
// by accident.
const hlsJSSum = "442f599c34f103c3355b375a23bdff560592d7117d09a8c847242ea3de2d40e0"

func TestVendoredHlsJS_IsTheReleaseTheADRNames(t *testing.T) {
	src, err := webAssets.ReadFile("web/hls.min.js")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(src)
	if got := hex.EncodeToString(sum[:]); got != hlsJSSum {
		t.Errorf("web/hls.min.js sha256 = %s, want %s", got, hlsJSSum)
	}
	if !strings.Contains(string(src), "Hls=") {
		t.Error("web/hls.min.js does not define Hls")
	}
	licence, err := webAssets.ReadFile("web/hls.js.LICENSE")
	if err != nil || !strings.Contains(string(licence), "Apache License") {
		t.Errorf("hls.js's licence must ship with it (Apache-2.0): %v", err)
	}
}

func TestHlsJS_IsServedAsScriptAndKeptByTheBrowser(t *testing.T) {
	f := newFixture(t)
	resp := f.get("/static/hls.min.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "max-age=86400") {
		t.Errorf("Cache-Control = %q: a 540 KB script should not be fetched again for every video", got)
	}
	// The page itself is still revalidated, so an upgrade shows at once.
	if got := f.get("/static/app.js").Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("app.js Cache-Control = %q", got)
	}
}

func TestPlayerPage_LoadsHlsJSOnlyWhenNeeded(t *testing.T) {
	index, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "hls.min.js") {
		t.Error("index.html loads hls.js up front; browsers with their own HLS should never fetch it")
	}
	app, err := webAssets.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "/static/hls.min.js") {
		t.Error("app.js never loads hls.js")
	}
}
