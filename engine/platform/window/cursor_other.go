//go:build !windows && !linux && !darwin

package window

import "github.com/go-gl/glfw/v3.3/glfw"

// systemDiagCursor is not available on this platform; cursor.go falls back
// to the synthesized diagonal bitmap.
func systemDiagCursor(flip bool) *glfw.Cursor { return nil }
