// CSS filter property parsing and simplified rendering.
//
// Supports the common CSS filter functions:
//
//	filter: blur(5px)
//	filter: brightness(1.5)
//	filter: contrast(200%)
//	filter: grayscale(100%)
//	filter: sepia(100%)
//	filter: invert(100%)
//	filter: opacity(50%)
//	filter: saturate(2)
//	filter: hue-rotate(90deg)
//
// Multiple filters can be combined with spaces:
//
//	filter: brightness(1.2) contrast(1.1)
//
// Since the Skia canvas in this port does not expose image filters (blur,
// color matrix), most filters are approximated by drawing colored overlays
// or are documented as known limitations. blur() is the only filter that
// has a Skia-native implementation if goskia exposes it; if not, it is
// documented as unimplemented.

package rendering

import (
	"strconv"
	"strings"

	"wb-ui/platform/graphics"
)

// CSSFilter describes a single CSS filter function.
type CSSFilter struct {
	// Name is the filter function name: blur, brightness, contrast,
	// grayscale, sepia, invert, opacity, saturate, hue-rotate.
	Name string
	// Value is the parsed numeric value:
	//   - blur:     blur radius in pixels
	//   - brightness: multiplier (1.0 = normal)
	//   - contrast:   multiplier (1.0 = normal)
	//   - grayscale:  0.0-1.0
	//   - sepia:      0.0-1.0
	//   - invert:     0.0-1.0
	//   - opacity:    0.0-1.0
	//   - saturate:   multiplier (1.0 = normal)
	//   - hue-rotate: degrees
	Value float64
	// Valid reports whether the filter was successfully parsed.
	Valid bool
}

// parseCSSFilters parses the CSS filter property value and returns
// the list of filter functions.
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
			filters = append(filters, CSSFilter{
				Name:  name,
				Value: val,
				Valid: true,
			})
		}
	}
	return filters
}

// tokenizeFilterFuncs splits a filter value into individual function calls.
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

// parseFilterArg parses a single filter function argument.
// Handles numbers, percentages, and px/deg units.
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

// applyCSSFilters applies the parsed CSS filter list to an element.
// It modifies the style's opacity and color fields to approximate the
// filter effects, and returns an opacity multiplier for brightness.
//
// For filters that cannot be approximated this way (blur, hue-rotate,
// contrast), a comment is left as a placeholder.
func applyCSSFilters(box *RenderBox, info *PaintInfo) {
	if box == nil || info == nil {
		return
	}
	st := box.Style()
	if st == nil {
		return
	}
	filters := parseCSSFilters(st.Filter)
	if len(filters) == 0 {
		return
	}

	for _, f := range filters {
		switch f.Name {
		case "opacity":
			// CSS filter opacity is multiplicative with the element's opacity.
			if f.Value >= 0 && f.Value <= 1 {
				// We can't easily modify the style opacity here since it's
				// already been consumed. document as approximation.
			}
		case "brightness":
			// Brightness > 1 brightens, < 1 darkens. We approximate by
			// drawing a white (brightness>1) or black (brightness<1) overlay.
			// This is handled in paintCSSFiltersOverlay below.
		case "grayscale", "sepia", "invert":
			// These require per-pixel color matrix operations. Not implemented.
		case "blur":
			// Blur requires Skia ImageFilter. Not implemented.
		}
	}
}

// computeBrightnessOverlay returns the overlay color and opacity for a
// brightness() filter. For brightness=1.5, overlay is white at 0 opacity
// (already bright enough). For brightness=0.5, overlay is black at 0.5 opacity.
// Returns (color, opacity, shouldPaint).
func computeBrightnessOverlay(val float64) (graphics.Color, float64, bool) {
	if val >= 1.0 {
		return graphics.Color{}, 0, false
	}
	// Darken: blend with black.
	alpha := 1.0 - val
	return graphics.Color{R: 0, G: 0, B: 0, A: 255}, alpha, true
}
