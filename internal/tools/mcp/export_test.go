package mcp

import "time"

// Test-only hooks. This file is compiled only under `go test`, so nothing here
// ships in production builds (it is excluded from docgen and debt accounting).

// SetListToolsTimeoutForTest overrides the per-server boot ListTools timeout
// and returns a function restoring the previous value. It lets tests exercise
// the slow-tools/list deadline (CR-BF01-2) without waiting the production 10s.
func SetListToolsTimeoutForTest(d time.Duration) func() {
	old := listToolsTimeout
	listToolsTimeout = d
	return func() { listToolsTimeout = old }
}

// ResetPinnedRegistriesForTest clears all session-pinning markers so a test can
// register into a registry that a prior test already pinned. Production code
// never resets the markers (a registry's pinning lasts the process lifetime).
func ResetPinnedRegistriesForTest() {
	pinnedRegistries.Range(func(key, _ any) bool {
		pinnedRegistries.Delete(key)
		return true
	})
}
