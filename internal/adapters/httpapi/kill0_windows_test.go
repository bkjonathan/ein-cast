//go:build windows

package httpapi

import "errors"

// probeProcess: Windows has no signal 0; the end-to-end test that uses it is skipped there.
func probeProcess(string) error { return errors.New("unsupported") }
