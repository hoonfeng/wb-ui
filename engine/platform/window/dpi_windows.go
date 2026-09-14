//go:build windows

package window

import (
	"syscall"
)

// enableDPIAwareness makes the process per-monitor DPI aware, mirroring GWui's
// dpi_windows.go. Must be called before glfw.Init(); otherwise GLFW creates
// windows in non-DPI-aware mode, causing GetFramebufferSize() to return logical
// pixels (= window size) and contentScale to be fixed at 1.0.
func enableDPIAwareness() {
	user32 := syscall.NewLazyDLL("user32.dll")
	procSetProcessDpiAwarenessContext := user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDpiAwareness := user32.NewProc("SetProcessDpiAwareness")
	procSetProcessDPIAware := user32.NewProc("SetProcessDPIAware")

	// SetProcessDpiAwarenessContext: returns BOOL, non-zero = success.
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4 (i.e. ^uintptr(3))
	r, _, _ := procSetProcessDpiAwarenessContext.Call(uintptr(^uintptr(3)))
	if r != 0 {
		return
	}
	// SetProcessDpiAwareness: returns HRESULT, 0 (S_OK) = success.
	// PROCESS_PER_MONITOR_DPI_AWARE = 2
	r, _, _ = procSetProcessDpiAwareness.Call(2)
	if r == 0 {
		return
	}
	// Legacy Vista API: SetProcessDPIAware returns BOOL, non-zero = success.
	_, _, _ = procSetProcessDPIAware.Call()
}
