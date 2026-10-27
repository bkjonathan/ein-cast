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

// NewID derives the stable ID for a slash-separated path relative to the root of
// library lib. The library is part of the input, so the same relative path in two
// libraries is two items.
func NewID(lib LibraryID, relPath string) ID {
	sum := sha256.Sum256([]byte(string(lib) + "|" + relPath))
	return ID(hex.EncodeToString(sum[:idBytes]))
}

// LegacyID is the ID an item had before libraries existed (ADR 0021): the hash of
// the relative path alone. Browsers kept resume positions under it, so the player
// page uses it to carry those positions over to the current ID.
func LegacyID(relPath string) ID {
	sum := sha256.Sum256([]byte(relPath))
	return ID(hex.EncodeToString(sum[:idBytes]))
}

// Item is one servable file in the library.
type Item struct {
	ID ID
	// LibraryID is the library the file belongs to.
	LibraryID LibraryID
	// Root is the absolute folder of that library. Like Path it stays on the
	// server; whoever opens Path checks that it is still inside Root.
	Root string
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
	// Subtitles are the sidecar subtitle files found next to a video, in a
	// stable order. Clients refer to one by its position in this list.
	Subtitles []Subtitle
	// Meta is what the name and any .nfo file say about a video (a title, a year,
	// the series an episode belongs to). The zero Meta is every other kind.
	Meta Meta
	// Artwork is the poster found next to a video, if any.
	Artwork Artwork
}

// DisplayName is what lists show for the item: its title when it has one, else
// the file name.
func (it Item) DisplayName() string {
	if it.Meta.Title != "" {
		return it.Meta.Title
	}
	return it.Name
}
