//go:build !windows && !linux && !darwin

package window

import "github.com/go-gl/glfw/v3.3/glfw"

// platformHWND returns 0 on non-Windows, non-Linux platforms (no Win32 HWND,
// and Linux uses the X11 backend which doesn't need GLFW).
func platformHWND(_ *glfw.Window) uintptr { return 0 }
