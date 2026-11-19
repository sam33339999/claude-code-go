//go:build windows

package tui

import (
	"syscall"
	"unsafe"
)

// GetProcessMemoryInfo lives in psapi.dll (also exported by kernel32 as
// K32GetProcessMemoryInfo on Windows 7+). Loaded lazily via the standard
// library's syscall package; no cgo, no extra dependency.
var procGetProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryCounters mirrors the Win32 PROCESS_MEMORY_COUNTERS struct.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// processRSS returns this process's working set — Windows' equivalent of
// RSS (Task Manager's "Working set" column).
func processRSS() (uint64, bool) {
	if procGetProcessMemoryInfo.Find() != nil {
		return 0, false
	}
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, false
	}
	var c processMemoryCounters
	c.cb = uint32(unsafe.Sizeof(c))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	if r == 0 || c.workingSetSize == 0 {
		return 0, false
	}
	return uint64(c.workingSetSize), true
}
