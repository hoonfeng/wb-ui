// Package fonts provides font-related types for the wb-ui platform layer.
//
// In the current wb-ui architecture, font management (FontManager, Typeface
// resolution, font loading) is implemented in platform/graphics/fontmgr.go,
// which wraps Skia's typeface APIs.
//
// This package is reserved for future extraction of font-specific types
// (FontDescription, FontMetrics, FontSelector) from the graphics package
// when they become complex enough to warrant a separate package. At that
// point the fontmgr.go code in graphics/ can be moved here.
//
// For now, use platform/graphics.Font and platform/graphics.FontManager for
// all font-related operations.
//
// See also:
//   - platform/graphics/fontmgr.go — FontManager (typeface loading + CSS family resolution)
//   - platform/graphics/canvas.go — Font struct (family, size, weight, style)
//   - WebKit Source/WebCore/platform/graphics/FontCascade.h
package fonts
