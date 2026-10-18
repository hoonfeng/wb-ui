//go:build linux

// Package window — X11 cursor support.
//
// The pure-X11 backend (window_x11.go) has no GLFW Window to delegate to, so
// cursor shapes are mapped onto the X11 cursor font (cursorfont.h glyphs) and
// applied with XDefineCursor. The cursor resource is freed immediately after
// being defined: X11 keeps a reference server-side for as long as the window
// uses it.

package window

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/cursorfont.h>
*/
import "C"

// SetCursorShape sets the window cursor to the given shape.
func (w *Window) SetCursorShape(shape CursorShape) {
	if w == nil || w.display == nil || w.xwin == 0 {
		return
	}
	var glyph C.uint
	switch shape {
	case CursorIBeam:
		glyph = C.XC_xterm
	case CursorHand:
		glyph = C.XC_hand2
	case CursorNWSE:
		glyph = C.XC_bottom_right_corner
	case CursorNESW:
		glyph = C.XC_top_right_corner
	case CursorNS:
		glyph = C.XC_sb_v_double_arrow
	case CursorEW:
		glyph = C.XC_sb_h_double_arrow
	default:
		glyph = C.XC_left_ptr
	}
	cur := C.XCreateFontCursor(w.display, glyph)
	if cur == 0 {
		return
	}
	C.XDefineCursor(w.display, w.xwin, cur)
	C.XFreeCursor(w.display, cur)
	C.XFlush(w.display)
}
