//go:build windows

package window

import (
	"image"
	"image/color"
	"syscall"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Windows system diagonal resize cursors. The browser shows the OS theme's
// IDC_SIZENWSE / IDC_SIZENESW over a textarea resize handle; GLFW has no
// diagonal standard cursors, so we read the system cursor bitmap via Win32
// and wrap it in a GLFW cursor — giving the exact system pointer (including
// user-customized themes), instead of a synthesized approximation.

var (
	user32W = syscall.NewLazyDLL("user32.dll")
	gdi32W  = syscall.NewLazyDLL("gdi32.dll")

	procLoadCursorW  = user32W.NewProc("LoadCursorW")
	procGetIconInfo  = user32W.NewProc("GetIconInfo")
	procGetDC        = user32W.NewProc("GetDC")
	procReleaseDC    = user32W.NewProc("ReleaseDC")
	procGetDIBits    = gdi32W.NewProc("GetDIBits")
	procGetObjectW   = gdi32W.NewProc("GetObjectW")
	procDeleteObject = gdi32W.NewProc("DeleteObject")
)

// Standard cursor resource ids (winuser.h).
const (
	idcSizeNWSE uintptr = 32642 // IDC_SIZENWSE
	idcSizeNESW uintptr = 32643 // IDC_SIZENESW
)

type iconInfo struct {
	fIcon    int32
	xHotspot int32
	yHotspot int32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmapInfo struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// systemDiagCursor loads the OS diagonal resize cursor (IDC_SIZENWSE for
// flip=false, IDC_SIZENESW for flip=true) and wraps it as a GLFW cursor.
// Returns nil if the system cursor can't be loaded (caller falls back to
// the synthesized bitmap).
func systemDiagCursor(flip bool) *glfw.Cursor {
	img, hotX, hotY, ok := loadSystemCursorBitmap(flip)
	if !ok {
		return nil
	}
	return glfw.CreateCursor(img, hotX, hotY)
}

// loadSystemCursorBitmap reads the OS diagonal resize cursor bitmap and its
// hotspot. Returns ok=false if the system cursor can't be loaded.
func loadSystemCursorBitmap(flip bool) (*image.NRGBA, int, int, bool) {
	id := idcSizeNESW
	if !flip {
		id = idcSizeNWSE
	}
	// LoadCursor(NULL, MAKEINTRESOURCE(id)) — the resource id sits in the
	// low 16 bits of the pointer argument.
	hcur, _, _ := procLoadCursorW.Call(0, id)
	if hcur == 0 {
		return nil, 0, 0, false
	}
	// LoadCursor returns a shared cursor — must NOT DeleteObject it.

	var ii iconInfo
	r, _, _ := procGetIconInfo.Call(hcur, uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return nil, 0, 0, false
	}
	// GetIconInfo copies the bitmaps — we must delete them.
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(ii.hbmMask)
	}
	if ii.hbmColor != 0 {
		defer procDeleteObject.Call(ii.hbmColor)
	}

	// Cursor bitmap size (16×16 at 100% DPI, larger on HiDPI).
	var bm bitmapInfo
	r, _, _ = procGetObjectW.Call(ii.hbmColor, uintptr(unsafe.Sizeof(bm)), uintptr(unsafe.Pointer(&bm)))
	if r == 0 || bm.bmWidth <= 0 || bm.bmHeight <= 0 {
		return nil, 0, 0, false
	}
	w, h := int(bm.bmWidth), int(bm.bmHeight)
	if w > 64 || h > 64 {
		return nil, 0, 0, false
	}

	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return nil, 0, 0, false
	}
	defer procReleaseDC.Call(0, dc)

	// 32bpp BGRA pixel buffer (top-down DIB).
	pixels := make([]byte, w*h*4)
	var bi bitmapInfoHeader
	bi.biSize = uint32(unsafe.Sizeof(bi))
	bi.biWidth = int32(w)
	bi.biHeight = -int32(h) // top-down
	bi.biPlanes = 1
	bi.biBitCount = 32
	bi.biCompression = 0 // BI_RGB
	r, _, _ = procGetDIBits.Call(dc, ii.hbmColor, 0, uintptr(h), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&bi)), 0)
	if r == 0 {
		return nil, 0, 0, false
	}

	// Alpha channel: modern theme cursors carry real alpha. If it is all
	// zero (classic AND-mask cursor), derive transparency from the mask.
	hasAlpha := false
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			hasAlpha = true
			break
		}
	}
	if !hasAlpha {
		// Read the 1bpp AND mask: bit=1 → transparent, bit=0 → opaque.
		rowBytes := (w + 31) / 32 * 4 // 1bpp rows are DWORD-aligned
		mask := make([]byte, rowBytes*h)
		var biM bitmapInfoHeader
		biM.biSize = uint32(unsafe.Sizeof(biM))
		biM.biWidth = int32(w)
		biM.biHeight = -int32(h)
		biM.biPlanes = 1
		biM.biBitCount = 1
		biM.biCompression = 0
		biM.biSizeImage = uint32(len(mask))
		r, _, _ = procGetDIBits.Call(dc, ii.hbmMask, 0, uintptr(h), uintptr(unsafe.Pointer(&mask[0])), uintptr(unsafe.Pointer(&biM)), 0)
		if r == 0 {
			return nil, 0, 0, false
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				byteIdx := y*rowBytes + x/8
				bit := (mask[byteIdx] >> uint(7-x%8)) & 1
				off := (y*w + x) * 4
				if bit == 1 {
					pixels[off+3] = 0
				} else {
					pixels[off+3] = 255
				}
			}
		}
	}

	// BGRA → RGBA for GLFW (non-premultiplied).
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 4
			img.SetNRGBA(x, y, color.NRGBA{
				R: pixels[off+2],
				G: pixels[off+1],
				B: pixels[off],
				A: pixels[off+3],
			})
		}
	}

	hotX, hotY := int(ii.xHotspot), int(ii.yHotspot)
	if hotX < 0 || hotX >= w {
		hotX = w / 2
	}
	if hotY < 0 || hotY >= h {
		hotY = h / 2
	}
	return img, hotX, hotY, true
}
