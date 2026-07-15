// Translation of: Source/WebCore/platform/graphics/skia/* (Skia bridge)
//                  Source/WebCore/platform/graphics/GraphicsContextSkia.cpp
// Completeness: 80%
// Simplifications:
//   - The Skia bridge is now fully wired via goskia cgo bindings.
//   - Canvas creation, drawing, text rendering, and pixel readback all go
//     through Skia's real rasterizer (see canvas.go).
//   - No GPU/compositor surface; only raster (CPU) surfaces are used.

package graphics

// Skia is initialized automatically in NewCanvas via skia.Init().
// This file exists to document the Skia integration point and to provide
// any package-level Skia configuration helpers.
