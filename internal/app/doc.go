// Package app holds the use cases. It orchestrates the domain through small
// consumer-side interfaces (ports) and never imports Wails, net/http or os;
// adapters supply those capabilities from the outside.
//
// The rule is enforced by scripts/check-deps.sh.
package app
