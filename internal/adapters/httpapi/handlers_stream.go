package httpapi

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// handleStream serves a file's bytes. http.ServeContent gives us Range
// requests (seeking), HEAD, If-Modified-Since and 206 responses for free.
//
// The URL carries only an ID. The path comes from our own repository, never
// from the request (ADR 0005).
func (a *api) handleStream(w http.ResponseWriter, r *http.Request) {
	it, ok := a.lookup(w, r)
	if !ok {
		return
	}

	realPath, err := resolveInside(a.lib.Root(), it.Path)
	if err != nil {
		// Possible if a file was swapped for a symlink after the scan. Log the
		// real reason; tell the client nothing beyond "not found".
		a.log.WarnContext(r.Context(), "refusing to serve file",
			errAttr(err), pathAttr(it.Path))
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}

	f, err := os.Open(realPath)
	if err != nil {
		a.fileError(w, r, err)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}

	w.Header().Set("Content-Type", contentType(it.Name))
	http.ServeContent(w, r, it.Name, info.ModTime(), f)
}

func (a *api) fileError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, fs.ErrNotExist) {
		// Deleted since the last scan.
		writeError(w, http.StatusNotFound, codeNotFound, "media not found")
		return
	}
	a.log.ErrorContext(r.Context(), "open media failed", errAttr(err))
	writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}

var errOutsideRoot = errors.New("path resolves outside the library root")

// resolveInside returns the real (symlink-free) path of file, provided it is
// still inside root. This is defense in depth on top of ID lookup: even if a
// bad path ever reached the repository, it could not be served.
func resolveInside(root, file string) (string, error) {
	if root == "" {
		return "", errors.New("no library loaded")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	realFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		return "", fmt.Errorf("resolve file: %w", err)
	}
	rel, err := filepath.Rel(realRoot, realFile)
	if err != nil {
		return "", errOutsideRoot
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errOutsideRoot
	}
	return realFile, nil
}

// contentType picks a MIME type by extension. Types that operating systems
// often get wrong or omit (notably .mkv) are pinned first, so playback does not
// depend on the host's mime database.
func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mkv":
		return "video/x-matroska"
	case ".m4v":
		return "video/x-m4v"
	case ".ts":
		return "video/mp2t"
	case ".avi":
		return "video/x-msvideo"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	case ".3gp":
		return "video/3gpp"
	case ".flac":
		return "audio/flac"
	case ".opus":
		return "audio/ogg; codecs=opus"
	case ".m4a":
		return "audio/mp4"
	case ".wma":
		return "audio/x-ms-wma"
	case ".aif", ".aiff":
		return "audio/aiff"
	case ".heic":
		return "image/heic"
	case ".avif":
		return "image/avif"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}
