package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

// The device player page. Embedded, so the binary is self-contained and there
// is no asset path to misconfigure or traverse.
//
//go:embed web
var webAssets embed.FS

// webFS is the embedded files without the "web/" prefix.
func webFS() fs.FS {
	sub, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err) // impossible: "web" is embedded above and the build fails without it
	}
	return sub
}

// pageCSP locks the player page down to its own origin. The page never needs
// inline script or third-party content, so refusing both turns any future
// injection bug into a harmless no-op instead of script execution. blob: is for
// media only: hls.js feeds the video element through a MediaSource, which the
// browser exposes as a blob: URL (ADR 0031).
const pageCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; " +
	"media-src 'self' blob:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// hlsScript is the vendored hls.js (ADR 0031), loaded only by browsers that cannot
// play HLS by themselves.
const hlsScript = "hls.min.js"

// handleIndex serves the player page.
func (a *api) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "no-cache") // embedded files have no mtime; revalidate after upgrades
	http.ServeFileFS(w, r, webFS(), "index.html")
}

// handleStatic serves one file of the player page. The route pattern has a
// single-segment wildcard, so the name can never contain a slash, and a
// directory or unknown name is a JSON 404 rather than a listing.
func (a *api) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	files := webFS()
	info, err := fs.Stat(files, name)
	if err != nil || info.IsDir() || name == "index.html" {
		writeError(w, http.StatusNotFound, codeNotFound, "no such file")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if name == hlsScript {
		// A big file that changes only when EinCast is upgraded: not worth fetching
		// again on every video. A day's staleness after an upgrade costs nothing.
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeFileFS(w, r, files, name)
}
