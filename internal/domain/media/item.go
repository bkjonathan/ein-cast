// Package media models the files EinCast can serve. It is pure: no filesystem,
// no HTTP, no Wails.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// idBytes is how much of the SHA-256 digest becomes the ID: 8 bytes → 16 hex
// chars. 64 bits makes an accidental collision negligible for a home library
// while keeping URLs short.
const idBytes = 8

// ID identifies a media item in URLs (/stream/{id}). It is derived from the
// item's relative path, never from the absolute one, so it is stable across
// machines and leaks nothing about the host's directory layout.
type ID string

// NewID derives the stable ID for a slash-separated path relative to the
// library root.
func NewID(relPath string) ID {
	sum := sha256.Sum256([]byte(relPath))
	return ID(hex.EncodeToString(sum[:idBytes]))
}

// Item is one servable file in the library.
type Item struct {
	ID ID
	// Name is the file name including its extension.
	Name string
	// RelPath is slash-separated and relative to the library root.
	RelPath string
	// Path is the absolute location on disk. It must never be sent to clients:
	// they only ever see ID (see ADR 0005).
	Path    string
	Kind    Kind
	Size    int64
	ModTime time.Time
}
