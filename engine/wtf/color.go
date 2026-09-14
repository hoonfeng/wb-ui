// Translation of: Source/WebCore/platform/graphics/Color.h
// Completeness: 90%
// Simplifications:
//   - Only the 8-bit-per-channel RGBA subset is modeled (no float16, extended sRGB,
//     or HDR color spaces). This matches the CSS color model used almost everywhere
//     in style resolution.
//   - Named colors and color space conversions are omitted; use NewColorFromRGBA
//     or NewColorFromHexString for CSS-style color values.
//   - No color matrix / color filter support.

package wtf

import "fmt"

// Color is the Go translation of WebCore::Color (the 8-bit-per-channel RGBA subset).
// It represents an RGBA color where each channel is 0–255.
type Color struct {
	R, G, B, A uint8
}

// NewColorRGBA creates a Color from individual RGBA components.
func NewColorRGBA(r, g, b, a uint8) Color {
	return Color{R: r, G: g, B: b, A: a}
}

// NewColorFromHexString parses a CSS hex color string (#RGB, #RRGGBB, #RGBA, #RRGGBBAA).
// Returns a zero Color (transparent black) on parse failure.
func NewColorFromHexString(hex string) Color {
	if len(hex) == 0 || hex[0] != '#' {
		return Color{}
	}
	hex = hex[1:]
	var r, g, b, a uint8 = 0, 0, 0, 255
	switch len(hex) {
	case 3: // #RGB
		r = parseHexNibble(hex[0])
		g = parseHexNibble(hex[1])
		b = parseHexNibble(hex[2])
		r = r*16 + r
		g = g*16 + g
		b = b*16 + b
	case 4: // #RGBA
		r = parseHexNibble(hex[0])
		g = parseHexNibble(hex[1])
		b = parseHexNibble(hex[2])
		a = parseHexNibble(hex[3])
		r = r*16 + r
		g = g*16 + g
		b = b*16 + b
		a = a*16 + a
	case 6: // #RRGGBB
		r = parseHexByte(hex[0:2])
		g = parseHexByte(hex[2:4])
		b = parseHexByte(hex[4:6])
	case 8: // #RRGGBBAA
		r = parseHexByte(hex[0:2])
		g = parseHexByte(hex[2:4])
		b = parseHexByte(hex[4:6])
		a = parseHexByte(hex[6:8])
	}
	return Color{R: r, G: g, B: b, A: a}
}

func parseHexNibble(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func parseHexByte(b string) uint8 {
	return parseHexNibble(b[0])*16 + parseHexNibble(b[1])
}

// String returns the CSS hex representation (#RRGGBBAA if alpha < 255, else #RRGGBB).
func (c Color) String() string {
	if c.A < 255 {
		return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// WithAlpha returns a copy of the color with the alpha channel replaced.
func (c Color) WithAlpha(a uint8) Color {
	return Color{R: c.R, G: c.G, B: c.B, A: a}
}

// IsOpaque reports whether the alpha channel is 255 (fully opaque).
func (c Color) IsOpaque() bool { return c.A == 255 }

// IsTransparent reports whether the alpha channel is 0 (fully transparent).
func (c Color) IsTransparent() bool { return c.A == 0 }

// ToSkColor converts to a 32-bit packed Skia color (AARRGGBB).
func (c Color) ToSkColor() uint32 {
	return uint32(c.A)<<24 | uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

// Common named colors.
var (
	ColorBlack       = Color{R: 0, G: 0, B: 0, A: 255}
	ColorWhite       = Color{R: 255, G: 255, B: 255, A: 255}
	ColorRed         = Color{R: 255, G: 0, B: 0, A: 255}
	ColorGreen       = Color{R: 0, G: 255, B: 0, A: 255}
	ColorBlue        = Color{R: 0, G: 0, B: 255, A: 255}
	ColorTransparent = Color{R: 0, G: 0, B: 0, A: 0}
)
