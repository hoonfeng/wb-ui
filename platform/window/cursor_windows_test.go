//go:build windows

package window

import "testing"

// TestSystemDiagCursor verifies that the OS diagonal resize cursors
// (IDC_SIZENWSE / IDC_SIZENESW) can be read from the system theme — the
// exact pointer the browser shows over a textarea resize handle.
func TestSystemDiagCursor(t *testing.T) {
	for _, flip := range []bool{false, true} {
		img, hotX, hotY, ok := loadSystemCursorBitmap(flip)
		if !ok {
			t.Fatalf("loadSystemCursorBitmap(%v) failed, want the OS theme cursor", flip)
		}
		b := img.Bounds()
		if b.Dx() < 8 || b.Dy() < 8 {
			t.Fatalf("cursor bitmap too small: %dx%d", b.Dx(), b.Dy())
		}
		// The bitmap must contain some opaque (drawn) pixels.
		opaque := false
		for y := b.Min.Y; y < b.Max.Y && !opaque; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if img.NRGBAAt(x, y).A != 0 {
					opaque = true
					break
				}
			}
		}
		if !opaque {
			t.Fatalf("cursor bitmap fully transparent")
		}
		if hotX < 0 || hotY < 0 {
			t.Fatalf("bad hotspot: (%d,%d)", hotX, hotY)
		}
	}
}
