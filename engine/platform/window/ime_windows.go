//go:build windows

package window

import (
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// platformHWND returns the Win32 HWND of the GLFW window as a uintptr,
// suitable for passing to the IME handler. Windows-only.
func platformHWND(win *glfw.Window) uintptr {
	return uintptr(unsafe.Pointer(win.GetWin32Window()))
}
