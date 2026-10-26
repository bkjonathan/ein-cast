package app

import "log/slog"

// Secret wraps a credential (PIN, token, signature) so that logging it by
// accident prints a placeholder instead. Wrap before the value gets anywhere
// near a log call: slog.Any("token", app.Secret(t)).
//
// The log pipeline deliberately does not scrub text after the fact (ADR 0004);
// this makes the safe thing the default at the call site.
type Secret string

// redacted is what a Secret logs and prints as.
const redacted = "[redacted]"

// LogValue implements slog.LogValuer.
func (Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// String implements fmt.Stringer, covering %s and %v formatting.
func (Secret) String() string { return redacted }

// GoString covers %#v.
func (Secret) GoString() string { return redacted }
