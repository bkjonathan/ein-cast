// Package domain is the innermost layer: pure types and rules with no
// knowledge of Wails, HTTP, or the filesystem. It imports nothing from the
// rest of EinCast. Sub-packages (media, server) arrive in later phases.
//
// The rule is enforced by scripts/check-deps.sh.
package domain
