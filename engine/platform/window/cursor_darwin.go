//go:build darwin

// Package window — Cocoa cursor support.
//
// The pure-Cocoa backend (window_macos.go) has no GLFW Window to delegate to,
// so cursor shapes map onto NSCursor class cursors. macOS has no dedicated
// diagonal resize cursor; both NWSE and NESW fall back to the arrow, matching
// what AppKit itself does outside of window resize areas.

package window

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void pc_set_cursor(int which) {
    NSCursor* c = nil;
    switch (which) {
    case 1: c = [NSCursor IBeamCursor]; break;          // cursor:text
    case 2: c = [NSCursor pointingHandCursor]; break;   // cursor:pointer
    case 5: c = [NSCursor resizeUpDownCursor]; break;   // cursor:ns-resize
    case 6: c = [NSCursor resizeLeftRightCursor]; break;// cursor:ew-resize
    default: c = [NSCursor arrowCursor]; break;         // default / diagonals
    }
    [c set];
}
*/
import "C"

// SetCursorShape sets the window cursor to the given shape.
func (w *Window) SetCursorShape(shape CursorShape) {
	if w == nil {
		return
	}
	C.pc_set_cursor(C.int(shape))
}
