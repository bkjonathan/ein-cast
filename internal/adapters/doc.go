// Package adapters is the parent of all port implementations (filesystem
// scanner, in-memory repo, HTTP API, log bus, ...). Each lives in its own
// sub-package so a dependency on one never drags in the others.
package adapters
