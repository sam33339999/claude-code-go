//go:build linux

package tui

import (
	"os"
	"strconv"
	"strings"
)

// processRSS returns this process's resident set size from /proc/self/statm
// (second field: resident pages).
func processRSS() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}
