// CSS filter property — Skia-backed implementation.
//
// Uses goskia's ImageFilter and ColorFilter to implement the full set of
// CSS filter functions. Supported:
//
//	filter: blur(5px)          — NewBlurImageFilter
//	filter: grayscale(100%)    — NewColorMatrixFilter
//	filter: sepia(100%)        — NewColorMatrixFilter
//	filter: brightness(1.5)    — NewColorMatrixFilter
//	filter: contrast(2)        — NewColorMatrixFilter
//	filter: saturate(2)        — NewColorMatrixFilter
//	filter: hue-rotate(90deg)  — NewColorMatrixFilter
//	filter: invert(100%)       — NewColorMatrixFilter
//	filter: opacity(50%)       — NewColorMatrixFilter (or layer opacity)
//
// Multiple filters can be combined with spaces.

package rendering

import (
	"math"
	"strconv"
	"strings"

	"github.com/hoonfeng/goskia/skia"
)

// CSSFilter describes a single CSS filter function.
type CSSFilter struct {
	Name  string // blur, brightness, contrast, grayscale, sepia, invert, opacity, saturate, hue-rotate
	Value float64
	Valid bool
}

// parseCSSFilters parses the CSS filter property value.
func parseCSSFilters(s string) []CSSFilter {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return nil
	}
	var filters []CSSFilter
	tokens := tokenizeFilterFuncs(s)
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		paren := strings.IndexByte(tok, '(')
		if paren < 0 || !strings.HasSuffix(tok, ")") {
			continue
		}
		name := strings.ToLower(tok[:paren])
		args := strings.TrimSpace(tok[paren+1 : len(tok)-1])
		val := parseFilterArg(args)
		switch name {
		case "blur", "brightness", "contrast", "grayscale",
			"sepia", "invert", "opacity", "saturate", "hue-rotate":
			filters = append(filters, CSSFilter{Name: name, Value: val, Valid: true})
		}
	}
	return filters
}

func tokenizeFilterFuncs(s string) []string {
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

func parseFilterArg(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	isPct := strings.HasSuffix(s, "%")
	if isPct {
		s = strings.TrimSuffix(s, "%")
	} else if strings.HasSuffix(s, "px") {
		s = strings.TrimSuffix(s, "px")
	} else if strings.HasSuffix(s, "deg") {
		s = strings.TrimSuffix(s, "deg")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if isPct {
		v /= 100.0
	}
	return v
}

// buildCSSFilterChain builds a Skia ImageFilter chain from a parsed CSS filter
// list. Filters are composed in order (outermost = first in list).
// Returns nil if no filters can be applied.
func buildCSSFilterChain(filters []CSSFilter) *skia.ImageFilter {
	var result *skia.ImageFilter
	// Process in reverse order so the first filter in the list is outermost.
	for i := len(filters) - 1; i >= 0; i-- {
		f := filters[i]
		if !f.Valid {
			continue
		}
		var imgFilter *skia.ImageFilter
		switch f.Name {
		case "blur":
			if f.Value > 0 {
				imgFilter = skia.NewBlurImageFilter(float32(f.Value), float32(f.Value), skia.TileModeClamp, result)
			}
		case "grayscale":
			imgFilter = skia.NewColorFilterImageFilter(grayscaleColorMatrix(), result)
		case "sepia":
			imgFilter = skia.NewColorFilterImageFilter(sepiaColorMatrix(f.Value), result)
		case "brightness":
			imgFilter = skia.NewColorFilterImageFilter(brightnessColorMatrix(f.Value), result)
		case "contrast":
			imgFilter = skia.NewColorFilterImageFilter(contrastColorMatrix(f.Value), result)
		case "invert":
			imgFilter = skia.NewColorFilterImageFilter(invertColorMatrix(f.Value), result)
		case "saturate":
			imgFilter = skia.NewColorFilterImageFilter(saturateColorMatrix(f.Value), result)
		case "hue-rotate":
			imgFilter = skia.NewColorFilterImageFilter(hueRotateColorMatrix(f.Value), result)
		case "opacity":
			imgFilter = skia.NewColorFilterImageFilter(opacityColorMatrix(f.Value), result)
		}
		if imgFilter != nil {
			result = imgFilter
		}
	}
	return result
}

// --- Color matrix helpers for CSS filter functions ---
// Each function returns a [20]float32 suitable for NewColorMatrixFilter.
// The matrix is row-major 4x5:
//
//	[ R' ]   [ a b c d e ] [ R ]
//	[ G' ] = [ f g h i j ] [ G ]
//	[ B' ]   [ k l m n o ] [ B ]
//	[ A' ]   [ p q r s t ] [ A ]
//	                   [ 1 ]

func grayscaleColorMatrix() *skia.ColorFilter {
	const r, g, b = 0.2126, 0.7152, 0.0722
	return skia.NewColorMatrixFilter([20]float32{
		r, g, b, 0, 0,
		r, g, b, 0, 0,
		r, g, b, 0, 0,
		0, 0, 0, 1, 0,
	})
}

func sepiaColorMatrix(pct float64) *skia.ColorFilter {
	t := float32(clamp(pct, 0, 1))
	r, g, b := 0.393, 0.769, 0.189
	sr, sg, sb := float32(r), float32(g), float32(b)
	// Interpolate between identity and sepia matrix by t.
	return skia.NewColorMatrixFilter([20]float32{
		lerpCF(1, sr, t), lerpCF(0, sg, t), lerpCF(0, sb, t), 0, 0,
		lerpCF(0, sr*0.7, t), lerpCF(1, sg*0.7, t), lerpCF(0, sb*0.7, t), 0, 0,
		lerpCF(0, sr*0.5, t), lerpCF(0, sg*0.5, t), lerpCF(1, sb*0.5, t), 0, 0,
		0, 0, 0, 1, 0,
	})
}

func brightnessColorMatrix(val float64) *skia.ColorFilter {
	v := float32(clamp(val, 0, 10))
	return skia.NewColorMatrixFilter([20]float32{
		v, 0, 0, 0, 0,
		0, v, 0, 0, 0,
		0, 0, v, 0, 0,
		0, 0, 0, 1, 0,
	})
}

func contrastColorMatrix(val float64) *skia.ColorFilter {
	v := float32(clamp(val, 0, 10))
	// Contrast matrix: interpolate between gray (v=0) and identity (v=1).
	t := v
	mid := float32(0.5)
	return skia.NewColorMatrixFilter([20]float32{
		t, 0, 0, 0, mid * (1 - t),
		0, t, 0, 0, mid * (1 - t),
		0, 0, t, 0, mid * (1 - t),
		0, 0, 0, 1, 0,
	})
}

func invertColorMatrix(pct float64) *skia.ColorFilter {
	t := float32(clamp(pct, 0, 1))
	return skia.NewColorMatrixFilter([20]float32{
		lerpCF(1, -1, t), 0, 0, 0, lerpCF(0, 1, t),
		0, lerpCF(1, -1, t), 0, 0, lerpCF(0, 1, t),
		0, 0, lerpCF(1, -1, t), 0, lerpCF(0, 1, t),
		0, 0, 0, 1, 0,
	})
}

func saturateColorMatrix(val float64) *skia.ColorFilter {
	v := float32(clamp(val, 0, 10))
	// Grayscale luminance weights.
	rw, gw, bw := float32(0.2126), float32(0.7152), float32(0.0722)
	// Saturate: interpolate between grayscale (v=0) and identity (v=1).
	return skia.NewColorMatrixFilter([20]float32{
		lerpCF(rw, 1, v), lerpCF(gw, 0, v), lerpCF(bw, 0, v), 0, 0,
		lerpCF(rw, 0, v), lerpCF(gw, 1, v), lerpCF(bw, 0, v), 0, 0,
		lerpCF(rw, 0, v), lerpCF(gw, 0, v), lerpCF(bw, 1, v), 0, 0,
		0, 0, 0, 1, 0,
	})
}

func hueRotateColorMatrix(deg float64) *skia.ColorFilter {
	rad := deg * math.Pi / 180.0
	cosA := float32(math.Cos(rad))
	sinA := float32(math.Sin(rad))
	// Simplified hue rotation using the standard matrix.
	rw, gw, bw := float32(0.213), float32(0.715), float32(0.072)
	return skia.NewColorMatrixFilter([20]float32{
		rw + cosA*(1-rw) + sinA*(-rw), gw + cosA*(-gw) + sinA*(-gw), bw + cosA*(-bw) + sinA*(1-bw), 0, 0,
		rw + cosA*(-rw) + sinA*(rw), gw + cosA*(1-gw) + sinA*(gw), bw + cosA*(-bw) + sinA*(-bw), 0, 0,
		rw + cosA*(-rw) + sinA*(-1+rw), gw + cosA*(-gw) + sinA*(gw), bw + cosA*(1-bw) + sinA*(bw), 0, 0,
		0, 0, 0, 1, 0,
	})
}

func opacityColorMatrix(pct float64) *skia.ColorFilter {
	t := float32(clamp(pct, 0, 1))
	return skia.NewColorMatrixFilter([20]float32{
		1, 0, 0, 0, 0,
		0, 1, 0, 0, 0,
		0, 0, 1, 0, 0,
		0, 0, 0, t, 0,
	})
}

// --- helpers ---

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func lerpCF(a, b float32, t float32) float32 {
	return a + (b-a)*t
}
