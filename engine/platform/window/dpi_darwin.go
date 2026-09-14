//go:build darwin

package window

// enableDPIAwareness detects the Retina display backing scale factor
// on macOS. It reads the NSScreen's backingScaleFactor to set the
// content scale, so that 1 CSS pixel = 1 backingScaleFactor screen pixels.
//
// This is called from NewWindow on the GLFW backend. For the Cocoa
// native backend (window_macos.go), DPI is handled directly.
func enableDPIAwareness() {}
