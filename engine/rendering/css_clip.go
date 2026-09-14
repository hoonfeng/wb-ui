package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/engine/platform/graphics"

	"github.com/hoonfeng/goskia/skia"
)

// parseCSSClipInset parses clip-path: inset(top right bottom left) into a
// rectangle relative to the box at (bx,by,bw,bh). 1-4 values follow the
// CSS margin shorthand. Returns ok=false when the value is not inset().
func parseCSSClipInset(s string, bx, by, bw, bh float64) (graphics.Rect, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "inset(") || !strings.HasSuffix(s, ")") {
		return graphics.Rect{}, false
	}
	inner := strings.TrimSpace(s[len("inset(") : len(s)-1])
	// strip round(...) suffix (ignored)
	if i := strings.Index(inner, "round"); i >= 0 {
		inner = strings.TrimSpace(inner[:i])
	}
	parts := splitSpaceComma(inner)
	if len(parts) == 0 {
		return graphics.Rect{}, true
	}
	lens := make([]float64, 0, 4)
	for _, p := range parts {
		lens = append(lens, resolveInsetLen(p, bw, bh))
	}
	top, right, bottom, left := lens[0], lens[0], lens[0], lens[0]
	switch len(lens) {
	case 2:
		right, left = lens[1], lens[1]
	case 3:
		right, left = lens[1], lens[1]
		bottom = lens[2]
	case 4:
		right, bottom, left = lens[1], lens[2], lens[3]
	}
	return graphics.Rect{X: bx + left, Y: by + top,
		Width: math.Max(0, bw-left-right), Height: math.Max(0, bh-top-bottom)}, true
}

// resolveInsetLen converts an inset() component ("10px", "5%") to a length;
// percentages resolve against the box's width (horizontal) or height
// (vertical), but we use the larger reference for simplicity (matches common
// usage with square boxes).
func resolveInsetLen(s string, bw, bh float64) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64); err == nil {
			return v / 100 * math.Max(bw, bh)
		}
		return 0
	}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64); err == nil {
		return v
	}
	return 0
}

// parseCSSClipShape parses clip-path: circle(r% at cx cy) / polygon(points)
// into a skia path. Returns nil for inset() (handled by parseCSSClipInset) or
// unrecognized shapes.
func parseCSSClipShape(s string, bx, by, bw, bh float64) *skia.Path {
	s = strings.TrimSpace(s)
	// circle(50% at 50% 50%)
	if strings.HasPrefix(s, "circle(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[len("circle(") : len(s)-1])
		r := math.Min(bw, bh) / 2
		cx, cy := bw/2, bh/2
		if at := strings.Index(inner, "at"); at >= 0 {
			posPart := strings.TrimSpace(inner[at+2:])
			posVals := splitSpaceComma(posPart)
			if len(posVals) >= 1 {
				cx = resolvePosLen(posVals[0], bw)
			}
			if len(posVals) >= 2 {
				cy = resolvePosLen(posVals[1], bh)
			}
			radStr := strings.TrimSpace(inner[:at])
			if v := resolvePosLen(radStr, math.Max(bw, bh)); v >= 0 {
				r = v
			}
		} else {
			if v := resolvePosLen(inner, math.Max(bw, bh)); v >= 0 {
				r = v
			}
		}
		path := skia.NewPath()
		const steps = 32
		for i := 0; i < steps; i++ {
			a := 2 * math.Pi * float64(i) / steps
			x := bx + cx + r*math.Cos(a)
			y := by + cy + r*math.Sin(a)
			if i == 0 {
				path.MoveTo(float32(x), float32(y))
			} else {
				path.LineTo(float32(x), float32(y))
			}
		}
		path.Close()
		return path
	}
	// polygon(x% y%, …)
	if strings.HasPrefix(s, "polygon(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSpace(s[len("polygon(") : len(s)-1])
		parts := strings.Split(inner, ",") // comma-separated point pairs
		if len(parts) < 3 {
			return nil
		}
		path := skia.NewPath()
		for i, p := range parts {
			vals := strings.Fields(p)
			if len(vals) < 2 {
				return nil
			}
			x := bx + resolvePosLen(vals[0], bw)
			y := by + resolvePosLen(vals[1], bh)
			if i == 0 {
				path.MoveTo(float32(x), float32(y))
			} else {
				path.LineTo(float32(x), float32(y))
			}
		}
		path.Close()
		return path
	}
	return nil
}

// resolvePosLen converts a length or percentage ("50%", "10px", "0") to an
// absolute value against the reference length.
func resolvePosLen(s string, ref float64) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64); err == nil {
			return v / 100 * ref
		}
		return 0
	}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 64); err == nil {
		return v
	}
	return 0
}
