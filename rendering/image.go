// Translation of: Source/WebCore/platform/graphics/BitmapImage.h
//                  Source/WebCore/platform/graphics/BitmapImage.cpp
//                  Source/WebCore/platform/graphics/Image.h
// Completeness: 40%
// Simplifications:
//   - only static raster images (PNG/JPEG) are supported; animated images
//     (GIF, WebP, APNG) and vector images (SVG) are not yet implemented
//   - no ImageObserver / incremental-load / frame animation machinery
//   - no decoded data caching beyond the pixel buffer derived from the
//     skia::Image (goskia decodes on first use and caches in its internal
//     Skia raster backend)
//   - DecodedImage wraps a skia.Image directly; memory ownership follows
//     skia's reference-counting via Release()

package rendering

import (
	"sync"

	"wb-ui/platform/graphics"
)

// DecodedImage is the Go translation of the WebCore::Image / BitmapImage
// abstraction. It holds a decoded Skia image ready for painting. Images
// are decoded from raw bytes (PNG, JPEG) by the CachedResource pipeline
// and stored here for use by RenderImage and background-image painting.
//
// Thread-safety: a DecodedImage is safe for concurrent access once loaded,
// because the underlying skia.Image is immutable. The loading flag
// prevents concurrent decoding of the same image.
type DecodedImage struct {
	mu      sync.Mutex
	skImg   *graphics.SkiaImage
	loaded  bool
	width   int
	height  int
}

// SkiaImage is an alias for the goskia Image type, exposed here so
// that the rendering package can create and hold decoded images.
type SkiaImage = graphics.SkiaImage

// NewDecodedImage decodes raw image bytes (PNG, JPEG, WEBP, ...) into a
// DecodedImage using Skia's built-in decoder. Returns nil if decoding
// fails.
func NewDecodedImage(data []byte) *DecodedImage {
	if len(data) == 0 {
		return nil
	}
	img := graphics.DecodeImage(data)
	if img == nil {
		return nil
	}
	w, h := img.Width(), img.Height()
	return &DecodedImage{
		skImg:  img,
		loaded: true,
		width:  w,
		height: h,
	}
}

// Loaded reports whether the image has been successfully decoded.
func (di *DecodedImage) Loaded() bool {
	di.mu.Lock()
	defer di.mu.Unlock()
	return di.loaded
}

// Width returns the intrinsic width of the decoded image in pixels.
func (di *DecodedImage) Width() int {
	di.mu.Lock()
	defer di.mu.Unlock()
	return di.width
}

// Height returns the intrinsic height of the decoded image in pixels.
func (di *DecodedImage) Height() int {
	di.mu.Lock()
	defer di.mu.Unlock()
	return di.height
}

// Draw paints the decoded image onto the canvas at the given position and
// size. The image is scaled to fill the destination rect (aspect ratio is
// not preserved — the caller should compute the appropriate dest rect).
func (di *DecodedImage) Draw(canvas *graphics.Canvas, x, y, w, h float64) {
	di.mu.Lock()
	img := di.skImg
	di.mu.Unlock()
	if img == nil || canvas == nil {
		return
	}
	canvas.DrawImage(img, x, y, w, h)
}

// SkiaImage returns the underlying skia image, or nil if not loaded. Used by
// the paint pipeline to apply the image's alpha as a CSS mask-image.
func (di *DecodedImage) SkiaImage() *SkiaImage {
	di.mu.Lock()
	defer di.mu.Unlock()
	return di.skImg
}

// Release frees the underlying Skia image. After calling Release the
// DecodedImage must not be used for drawing.
func (di *DecodedImage) Release() {
	di.mu.Lock()
	defer di.mu.Unlock()
	if di.skImg != nil {
		di.skImg.Release()
		di.skImg = nil
	}
	di.loaded = false
}
