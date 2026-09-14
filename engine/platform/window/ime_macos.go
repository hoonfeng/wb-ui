//go:build darwin

package window

import "github.com/go-gl/glfw/v3.3/glfw"

// platformHWND returns 0 on macOS (no Win32 HWND). This function
// exists only for GLFW-based code paths (ime_windows.go uses it).
// The Cocoa native backend (window_macos.go) does not call this.
func platformHWND(_ *glfw.Window) uintptr { return 0 }
