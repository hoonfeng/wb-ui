// Package window — cursor shape vocabulary shared by all backends.
//
// The shape enum lives in a platform-neutral file so that callers (app/host.go)
// can reference window.CursorShape on every OS: the GLFW implementation lives
// in cursor.go (!linux && !darwin), the X11 backend in cursor_linux.go and the
// Cocoa backend in cursor_darwin.go.
package window

// CursorShape enumerates the cursor shapes the Host may request, mirroring
// the CSS cursor values the renderer resolves.
type CursorShape int

const (
	CursorArrow CursorShape = iota // default / cursor:default
	CursorIBeam                    // cursor:text — text input / textarea body
	CursorHand                     // cursor:pointer — links (a[href]) and explicit pointer
	CursorNWSE                     // cursor:nwse-resize — ↘ double arrow (textarea handle)
	CursorNESW                     // cursor:nesw-resize — ↙ double arrow
	CursorNS                       // cursor:ns-resize — vertical double arrow
	CursorEW                       // cursor:ew-resize — horizontal double arrow
)
