// Translation of: Source/WebCore/platform/graphics/skia/* (Skia bridge)
//                  Source/WebCore/platform/graphics/GraphicsContextSkia.cpp
// Completeness: 80%
// Simplifications:
//   - The Skia bridge is now fully wired via goskia cgo bindings.
//   - Canvas creation, drawing, text rendering, and pixel readback all go
//     through Skia's real rasterizer (see canvas.go).
//   - No GPU/compositor surface; only raster (CPU) surfaces are used.

package graphics

import "github.com/hoonfeng/goskia/skia"

// SkiaImage is an alias for the goskia Image type, exposed for use by
// the rendering package's image pipeline.
type SkiaImage = skia.Image

// DecodeImage decodes encoded image bytes (PNG, JPEG, WEBP, GIF, ...) into
// a SkiaImage using Skia's built-in decoder (which bundles libpng, libjpeg,
// libwebp, etc.). Returns nil on failure (empty data, unsupported format
// or corrupt image).
func DecodeImage(data []byte) *SkiaImage {
	if len(data) == 0 {
		return nil
	}
	img, err := skia.DecodeImage(data)
	if err != nil {
		return nil
	}
	return img
}

// This file exists to document the Skia integration point and to provide
// any package-level Skia configuration helpers.
