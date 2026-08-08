//go:build !linux && !darwin

// Package window — cursor support.
//
// GLFW only ships arrow/ibeam/hand/h/v-resize standard cursors; the browser's
// nwse-resize (45° diagonal double-arrow, shown over a textarea resize handle)
// is not among them, so we synthesize it as a 32×32 cursor bitmap with a
// white outline + black fill (matching the Windows classic resize cursor look).
package window

import (
	"image"
	"image/color"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// CursorShape enumerates the cursor shapes the Host may request, mirroring
// the CSS cursor values the renderer resolves.
type CursorShape int

const (
	CursorArrow   CursorShape = iota // default
	CursorIBeam                      // text / textarea body
	CursorHand                       // pointer (links, buttons, checkbox/select…)
	CursorNWSE                       // nwse-resize — textarea bottom-right resize handle
)

// SetCursorShape sets the window cursor to the given shape. Repeated calls
// with the same shape are cheap (cursor objects are cached in the window).
func (w *Window) SetCursorShape(shape CursorShape) {
	if w == nil || w.win == nil {
		return
	}
	cur := w.cursorFor(shape)
	if cur != nil {
		w.win.SetCursor(cur)
	}
}

func (w *Window) cursorFor(shape CursorShape) *glfw.Cursor {
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	if c := w.cursors[shape]; c != nil {
		return c
	}
	var c *glfw.Cursor
	switch shape {
	case CursorIBeam:
		c = glfw.CreateStandardCursor(glfw.IBeamCursor)
	case CursorHand:
		c = glfw.CreateStandardCursor(glfw.HandCursor)
	case CursorNWSE:
		c = makeNWSECursor()
	default:
		c = glfw.CreateStandardCursor(glfw.ArrowCursor)
	}
	w.cursors[shape] = c
	return c
}

// makeNWSECursor draws the nwse-resize cursor: a 45° double arrow (↖↘) —
// white outline (offset +1,+1) under a black body, 32×32, hotspot at center.
func makeNWSECursor() *glfw.Cursor {
	const S = 32
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	draw := func(ox, oy int, col color.NRGBA) {
		line := func(x0, y0, x1, y1 int) {
			dx := x1 - x0
			dy := y1 - y0
			steps := dx
			if a := dy; a > steps {
				steps = a
			}
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			if steps == 0 {
				img.SetNRGBA(x0+ox, y0+oy, col)
				return
			}
			for s := 0; s <= steps; s++ {
				x := x0 + (x1-x0)*s/steps
				y := y0 + (y1-y0)*s/steps
				img.SetNRGBA(x+ox, y+oy, col)
			}
		}
		// Shaft: 3px-thick diagonal from (11,11) to (21,21).
		for i := -1; i <= 1; i++ {
			line(11, 11+i, 21, 21+i)
		}
		// Head A (top-left): tip (5,5), wings to the shaft.
		line(5, 5, 11, 9)
		line(5, 5, 9, 11)
		// Head B (bottom-right): tip (27,27), wings to the shaft.
		line(21, 23, 27, 27)
		line(23, 21, 27, 27)
	}
	// White outline first (shifted +1), then black body on top.
	draw(1, 1, white)
	draw(0, 0, black)
	return glfw.CreateCursor(img, 16, 16)
}
