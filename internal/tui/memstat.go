package tui

import (
	"fmt"
	"runtime/metrics"
	"time"

	tea "charm.land/bubbletea/v2"
)

// memSampleInterval is how often the status bar's memory figure refreshes
// (the first sample is taken on the first tick after startup).
const memSampleInterval = 10 * time.Second

// memMetrics are read with runtime/metrics, which (unlike
// runtime.ReadMemStats) does not stop the world, so sampling every second
// costs nothing noticeable.
var memMetrics = []metrics.Sample{
	{Name: "/memory/classes/total:bytes"},
	{Name: "/memory/classes/heap/released:bytes"},
}

// memSampleMsg carries a finished memory sample into Update.
type memSampleMsg struct{ bytes uint64 }

// memSampleCmd samples process memory off the Update loop (on macOS this runs
// ps(1), which takes a few milliseconds).
//
// Sources per platform: macOS /bin/ps (RSS), Linux /proc/self/statm (RSS),
// Windows GetProcessMemoryInfo (working set); elsewhere the Go runtime figure.
func memSampleCmd() tea.Cmd {
	return func() tea.Msg { return memSampleMsg{bytes: sampleProcessMemory()} }
}

// sampleProcessMemory returns the process's resident memory (what Activity
// Monitor / top report as RSS), including sub-agents, which run in the same
// process. Where the OS figure is unavailable it falls back to the Go
// runtime's view: memory mapped from the OS minus heap pages already returned
// (this omits the binary's code pages, so it reads lower than RSS).
func sampleProcessMemory() uint64 {
	if rss, ok := processRSS(); ok {
		return rss
	}
	return goRuntimeMemory()
}

// goRuntimeMemory is the Go runtime's mapped-minus-released memory.
func goRuntimeMemory() uint64 {
	samples := make([]metrics.Sample, len(memMetrics))
	copy(samples, memMetrics)
	metrics.Read(samples)
	var total, released uint64
	if samples[0].Value.Kind() == metrics.KindUint64 {
		total = samples[0].Value.Uint64()
	}
	if samples[1].Value.Kind() == metrics.KindUint64 {
		released = samples[1].Value.Uint64()
	}
	if released > total {
		return 0
	}
	return total - released
}

// formatBytes renders a byte count as "512KB", "48MB" or "1.2GB".
func formatBytes(b uint64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1fGB", float64(b)/gb)
	case b >= 10*mb:
		return fmt.Sprintf("%dMB", b/mb)
	case b >= mb:
		return fmt.Sprintf("%.1fMB", float64(b)/mb)
	default:
		return fmt.Sprintf("%dKB", b/kb)
	}
}
