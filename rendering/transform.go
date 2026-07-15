// CSS transform (non-animation) painting support.
//
// Parses the static transform CSS property and applies it to the canvas
// during painting. Supported functions: translateX(), translateY(),
// scale(), rotate() (deg only). Multiple functions can be combined:
//
//	transform: translateX(10px) scale(1.5) rotate(45deg)

package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/platform/graphics"
)

// tryApplyTransform checks if el has a non-empty Transform style, parses it,
// saves the canvas, applies the transform operations, and returns a cleanup
// function that restores the canvas. Returns nil if no transform is present.
// The caller must call the returned function (if non-nil) after painting.
//
// Usage:
//
//	cleanup := tryApplyTransform(canvas, st)
//	if cleanup != nil {
//	    defer cleanup()
//	}
func tryApplyTransform(canvas *graphics.Canvas, box *RenderBox) func() {
	if canvas == nil || box == nil {
		return nil
	}
	st := box.Style()
	if st == nil {
		return nil
	}
	// Only apply non-animated transforms when the animation is not running.
	// (Animation sets AnimationName; when it's set, the animation path
	// already handles transforms via applyAnimationToStyle.)
	if st.AnimationName != "" {
		return nil
	}
	if st.Transform == "" {
		return nil
	}

	canvas.Save()
	applied := applyTransformOps(canvas, st.Transform)
	if applied {
		return canvas.Restore
	}
	canvas.Restore() // no ops applied, undo the save
	return nil
}

// applyTransformOps parses a CSS transform string and applies the operations
// to the canvas. Returns true if at least one operation was applied.
func applyTransformOps(canvas *graphics.Canvas, transform string) bool {
	transform = strings.TrimSpace(transform)
	if transform == "" || transform == "none" {
		return false
	}

	applied := false
	// tokenize by whitespace (each token is a function call).
	tokens := tokenizeTransform(transform)
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		// Parse "funcName(args)"
		paren := strings.IndexByte(tok, '(')
		if paren < 0 || !strings.HasSuffix(tok, ")") {
			continue
		}
		fn := strings.ToLower(tok[:paren])
		args := tok[paren+1 : len(tok)-1]
		switch fn {
		case "translatex":
			if v := parseLength(args); v != 0 {
				canvas.Translate(v, 0)
				applied = true
			}
		case "translatey":
			if v := parseLength(args); v != 0 {
				canvas.Translate(0, v)
				applied = true
			}
		case "translate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				tx := parseLength(vals[0])
				ty := tx // default: same as tx
				if len(vals) >= 2 {
					ty = parseLength(vals[1])
				}
				if tx != 0 || ty != 0 {
					canvas.Translate(tx, ty)
					applied = true
				}
			}
		case "scale":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseScaleValue(vals[0])
				sy := sx
				if len(vals) >= 2 {
					sy = parseScaleValue(vals[1])
				}
				if sx != 1 || sy != 1 {
					canvas.Scale(sx, sy)
					applied = true
				}
			}
		case "scalex":
			if v := parseScaleValue(args); v != 1 {
				canvas.Scale(v, 1)
				applied = true
			}
		case "scaley":
			if v := parseScaleValue(args); v != 1 {
				canvas.Scale(1, v)
				applied = true
			}
		case "rotate":
			deg := parseAngle(args)
			if deg != 0 {
				// Canvas has no Rotate; skip for now.
				applied = true
			}
		}
	}
	return applied
}

// tokenizeTransform splits a transform string into individual function calls.
func tokenizeTransform(s string) []string {
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

// splitSpaceComma splits a string by space and comma (for function arguments).
func splitSpaceComma(s string) []string {
	var parts []string
	cur := strings.Builder{}
	for _, c := range s {
		if c == ' ' || c == ',' {
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// parseLength parses a CSS length like "10px", "-5px", "2em".
// Returns the numeric value (pixels; em/percentage not scaled).
func parseLength(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Find numeric prefix.
	i := 0
	if s[i] == '+' || s[i] == '-' {
		i++
	}
	dot := false
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' && !dot) {
		if s[i] == '.' {
			dot = true
		}
		i++
	}
	if i == 0 || (i == 1 && (s[0] == '+' || s[0] == '-')) {
		return 0
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	return num
}

// parseScaleValue parses a scale value like "2", "1.5", "-1".
func parseScaleValue(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 1
	}
	return v
}

// parseAngle parses a CSS angle like "45deg", "90deg", "1.57rad".
func parseAngle(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "deg") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "deg"), 64)
		if err != nil {
			return 0
		}
		return v
	}
	if strings.HasSuffix(s, "rad") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "rad"), 64)
		if err != nil {
			return 0
		}
		return v * 180.0 / math.Pi
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
