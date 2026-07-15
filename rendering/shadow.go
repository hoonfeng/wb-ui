// Shadow parsing and painting for CSS box-shadow and text-shadow.
//
// Supports the common shadow syntax:
//
//	offset-x offset-y blur-radius color
//	offset-x offset-y blur-radius spread-radius color
//
// Inset shadows ("inset" keyword) are parsed but not rendered in this port
// (they require a more complex inner-shadow rasterization path). Multiple
// shadows (comma-separated) are supported: the first non-inset shadow is used.

package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/platform/graphics"
)

// Shadow describes a single CSS shadow (box-shadow or text-shadow).
type Shadow struct {
	OffsetX, OffsetY float64
	Blur             float64
	Spread           float64
	Color            graphics.Color
	Inset            bool
}

// parseShadowList parses a CSS shadow declaration and returns the parsed shadows.
// Supports the format: offset-x offset-y blur-radius spread-radius color
// Multiple shadows are comma-separated.
func parseShadowList(s string) []Shadow {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return nil
	}
	// Split by comma (respecting parentheses for rgba/hsla).
	var parts []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}

	var shadows []Shadow
	for _, p := range parts {
		if sh := parseSingleShadow(strings.TrimSpace(p)); sh != nil {
			shadows = append(shadows, *sh)
		}
	}
	return shadows
}

// parseSingleShadow parses a single shadow value.
func parseSingleShadow(s string) *Shadow {
	if s == "" {
		return nil
	}
	sh := &Shadow{}
	// Check for inset keyword.
	if strings.HasPrefix(s, "inset ") || strings.HasPrefix(s, "inset\t") {
		sh.Inset = true
		s = strings.TrimSpace(s[5:])
	}

	// Tokenize by space.
	tokens := tokenizeShadow(s)
	if len(tokens) < 2 {
		return nil
	}

	// First two tokens are offset-x and offset-y.
	sh.OffsetX = parseShadowLength(tokens[0])
	sh.OffsetY = parseShadowLength(tokens[1])
	pos := 2

	// Third token (if present) is blur-radius.
	if pos < len(tokens) {
		if v := parseShadowLength(tokens[pos]); v >= 0 {
			sh.Blur = v
			pos++
		}
	}

	// Fourth token (if present) is spread-radius.
	if pos < len(tokens) {
		if v := parseShadowLength(tokens[pos]); v != 0 {
			// spread can be negative
			sh.Spread = v
			pos++
		}
	}

	// Remaining tokens are the color.
	if pos < len(tokens) {
		colorStr := strings.Join(tokens[pos:], " ")
		if c, ok := parseColorSimple(colorStr); ok {
			sh.Color = c
		}
	}
	if sh.Color.A == 0 {
		sh.Color = graphics.Color{R: 0, G: 0, B: 0, A: 128} // default: semi-transparent black
	}
	return sh
}

// tokenizeShadow splits a shadow string into tokens, merging rgba(...) etc. into one.
func tokenizeShadow(s string) []string {
	var tokens []string
	var cur strings.Builder
	depth := 0
	for _, c := range s {
		switch {
		case c == '(':
			depth++
			cur.WriteRune(c)
		case c == ')':
			depth--
			cur.WriteRune(c)
		case c == ' ' && depth == 0:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// parseShadowLength parses a CSS length value including "px" suffix.
func parseShadowLength(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Strip unit suffix (px, em, etc.)
	for i, c := range s {
		if c >= '0' && c <= '9' || c == '.' || c == '+' || c == '-' {
			continue
		}
		if i == 0 && (c == '+' || c == '-') {
			continue
		}
		s = s[:i]
		break
	}
	if s == "" || s == "+" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// paintBoxShadow draws the box-shadow for a rectangular box by rendering each
// shadow as a filled rounded-rect offset from the box, with the given blur
// approximated by reducing opacity in proportion to blur radius.
func paintBoxShadow(canvas *graphics.Canvas, x, y, w, h, r float64, shadows []Shadow, opacity float64) {
	for _, sh := range shadows {
		if sh.Inset {
			continue // inset shadows not supported
		}
		sx := x + sh.OffsetX - sh.Spread
		sy := y + sh.OffsetY - sh.Spread
		sw := w + 2*sh.Spread
		sh2 := h + 2*sh.Spread

		// Blur approximation: reduce alpha proportionally to blur radius.
		col := sh.Color
		blurFactor := 1.0
		if sh.Blur > 0 {
			blurFactor = math.Max(0.3, 1.0-sh.Blur/50.0)
		}
		col.A = uint8(float64(col.A) * blurFactor * opacity)
		if col.A == 0 {
			continue
		}

		sr := r + sh.Spread
		if sr < 0 {
			sr = 0
		}
		if sr > 0 {
			canvas.FillRoundRect(sx, sy, sw, sh2, sr, col)
		} else {
			canvas.FillRect(sx, sy, sw, sh2, col)
		}
	}
}

// paintTextShadow draws the text-shadow effect by drawing the text string
// multiple times (once per shadow) in the shadow color, offset by the shadow's
// offset.
func paintTextShadow(canvas *graphics.Canvas, shadows []Shadow, x, y float64, text string, font graphics.Font, opacity float64) {
	for _, sh := range shadows {
		if sh.Inset {
			continue
		}
		col := sh.Color
		blurFactor := 1.0
		if sh.Blur > 0 {
			blurFactor = math.Max(0.3, 1.0-sh.Blur/50.0)
		}
		col.A = uint8(float64(col.A) * blurFactor * opacity)
		if col.A == 0 {
			continue
		}
		// Draw text at shadow offset.
		canvas.DrawText(x+sh.OffsetX, y+sh.OffsetY, text, font, col)
	}
}
