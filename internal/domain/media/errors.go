package media

import "errors"

// Sentinel errors. Callers match them with errors.Is; adapters wrap them with
// context but must not replace them.
var (
	// ErrNotFound means no item exists for the requested ID.
	ErrNotFound = errors.New("media: not found")
	// ErrUnsupported means the file type is not one EinCast serves.
	ErrUnsupported = errors.New("media: unsupported file type")
)
