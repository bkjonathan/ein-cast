package media

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// LibraryID names a library. It is derived from the library's folder, so the
// same folder always has the same ID: removing a library and adding it again
// brings back every item ID, and with them the resume positions and bookmarks
// that were built on them.
type LibraryID string

// NewLibraryID derives the ID for the library rooted at root. The path is
// cleaned first, so "/media/films/" and "/media/films" are one library.
func NewLibraryID(root string) LibraryID {
	sum := sha256.Sum256([]byte("library|" + filepath.Clean(root)))
	return LibraryID(hex.EncodeToString(sum[:idBytes]))
}

// Library is a folder the person added, with the name they see for it.
type Library struct {
	ID LibraryID
	// Name is what the UI and TVs call the library. It starts as the folder's name
	// and can be changed.
	Name string
	// Root is the absolute folder. Like Item.Path it never leaves the server.
	Root string
}

// NewLibrary builds the Library for a folder, named after it.
func NewLibrary(root string) Library {
	root = filepath.Clean(root)
	return Library{ID: NewLibraryID(root), Name: DefaultLibraryName(root), Root: root}
}

// DefaultLibraryName is the last part of the folder's path, or the whole path for
// a drive or filesystem root that has no name of its own.
func DefaultLibraryName(root string) string {
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || strings.HasSuffix(base, ":\\") || base == "" {
		return root
	}
	return base
}

// Overlaps reports whether two library roots are the same folder or one is
// inside the other. Overlapping libraries would list the same file twice under
// two IDs, so they are refused. The comparison ignores case, since the two
// platforms EinCast runs on have case-insensitive file systems by default.
func Overlaps(a, b string) bool {
	a, b = comparable(a), comparable(b)
	return a == b || strings.HasPrefix(a, b+sep) || strings.HasPrefix(b, a+sep)
}

const sep = string(filepath.Separator)

func comparable(p string) string {
	p = strings.ToLower(filepath.Clean(p))
	return strings.TrimSuffix(p, sep)
}
