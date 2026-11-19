//go:build !darwin && !linux && !windows

package tui

// processRSS is unavailable on this platform; callers fall back to the Go
// runtime's figure.
func processRSS() (uint64, bool) { return 0, false }
