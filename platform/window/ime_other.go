//go:build !windows

package window

import "github.com/go-gl/glfw/v3.3/glfw"

// platformHWND returns 0 on non-Windows platforms (no Win32 HWND).
func platformHWND(_ *glfw.Window) uintptr { return 0 }
