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
// injection bug into a harmless no-op instead of script execution.
const pageCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; " +
	"media-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

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
	http.ServeFileFS(w, r, files, name)
}
