// Translation of: Source/WebCore/animation/AnimationTimeline.cpp
//                  Source/WebCore/animation/KeyframeEffect.cpp
// Completeness: 20%
// Simplifications:
//   - only opacity property is animated (most common use case)
//   - only "infinite" iteration count is supported; finite counts stop at the end
//   - timing-function is ignored (linear interpolation)
//   - no animation-delay support
//   - no Animation reverse / alternate direction

package rendering

import (
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// AnimationTime is the global animation clock in seconds, set by the embedder
// (e.g. app.Host) each frame before calling Paint. It drives @keyframes-based
// opacity animations.
var AnimationTime float64

// KeyframesLookup is a function that returns the @keyframes rule with the
// given name, or nil if not found. The embedder sets this to bridge to the
// style resolver's keyframes collection.
var KeyframesLookup func(name string) *css.KeyframesRule

// ApplyAnimations walks the render tree and updates the effective Opacity on
// each element's ComputedStyle based on the current AnimationTime and the
// element's animation properties. This must be called before Paint each frame.
func ApplyAnimations(rv *RenderView) {
	if rv == nil || KeyframesLookup == nil {
		return
	}
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if o == nil {
			return
		}
		st := o.Style()
		if st != nil && st.AnimationName != "" {
			opacity := computeAnimatedOpacity(st, AnimationTime)
			if opacity >= 0 {
				st.Opacity = opacity
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
}

// computeAnimatedOpacity looks up the @keyframes rule for the animation name
// and computes the interpolated opacity at the given time. Returns -1 if no
// keyframes found or no opacity property in keyframes.
func computeAnimatedOpacity(st *style.ComputedStyle, time float64) float64 {
	if KeyframesLookup == nil || st.AnimationName == "" {
		return -1
	}
	kf := KeyframesLookup(st.AnimationName)
	if kf == nil || len(kf.Keyframes) == 0 {
		return -1
	}
	duration := st.AnimationDuration
	if duration <= 0 {
		return 1.0
	}
	// Compute progress (0.0–1.0) within the current iteration.
	progress := 0.0
	if st.AnimationIterationCount == 0 {
		// Infinite: loop forever.
		progress = mod(time, duration) / duration
	} else {
		total := duration * float64(st.AnimationIterationCount)
		if time >= total {
			progress = 1.0 // stopped at last frame
		} else {
			progress = mod(time, duration) / duration
		}
	}
	return interpolateOpacity(kf, progress)
}

// mod is a floating-point modulo that always returns a non-negative result.
func mod(a, b float64) float64 {
	if b <= 0 {
		return 0
	}
	r := a - b*float64(int(a/b))
	if r < 0 {
		r += b
	}
	return r
}

// interpolateOpacity finds the two surrounding keyframes for the given progress
// (0.0–1.0) and linearly interpolates the opacity between them.
func interpolateOpacity(kf *css.KeyframesRule, progress float64) float64 {
	type kfPoint struct {
		offset  float64
		opacity float64
		hasOp   bool
	}
	var points []kfPoint
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			opacity, hasOp := findOpacityInDecls(rule.Declarations)
			points = append(points, kfPoint{offset: offset, opacity: opacity, hasOp: hasOp})
		}
	}
	if len(points) == 0 {
		return 1.0
	}
	// Sort by offset.
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
	// If progress is before the first keyframe, use the first keyframe's opacity.
	if progress <= points[0].offset {
		if points[0].hasOp {
			return points[0].opacity
		}
		return 1.0
	}
	// If progress is after the last keyframe, use the last keyframe's opacity.
	if progress >= points[len(points)-1].offset {
		if points[len(points)-1].hasOp {
			return points[len(points)-1].opacity
		}
		return 1.0
	}
	// Find surrounding keyframes and interpolate.
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			if !points[i].hasOp || !points[i+1].hasOp {
				return 1.0
			}
			if points[i+1].offset == points[i].offset {
				return points[i+1].opacity
			}
			t := (progress - points[i].offset) / (points[i+1].offset - points[i].offset)
			return points[i].opacity + (points[i+1].opacity-points[i].opacity)*t
		}
	}
	return 1.0
}

// parseKeyframeOffset converts a keyframe selector like "0%", "50%", "from",
// "to" into a 0.0–1.0 offset. Returns -1 for unrecognized selectors.
func parseKeyframeOffset(key string) float64 {
	key = strings.TrimSpace(strings.ToLower(key))
	switch key {
	case "from":
		return 0.0
	case "to":
		return 1.0
	}
	if strings.HasSuffix(key, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(key, "%"), 64); err == nil {
			return v / 100.0
		}
	}
	return -1
}

// findOpacityInDecls searches a keyframe rule's declarations for an opacity
// property and returns its value. Returns (1.0, false) if not found.
func findOpacityInDecls(decls []css.Declaration) (float64, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, "opacity") {
			val := strings.TrimSpace(d.ValueString())
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				return v, true
			}
		}
	}
	return 1.0, false
}

// CumulativeOpacity returns the effective opacity for a render object by
// multiplying its own opacity with all ancestors' opacities. This is needed
// because ApplyAnimations sets the animated opacity on the animated element
// only; child elements' cached ComputedStyle still has the pre-animation
// inherited value (1.0). By walking up the ancestor chain we capture the
// animated parent's opacity. Painters use this to multiply color alpha.
func CumulativeOpacity(o RenderObject) float64 {
	if o == nil {
		return 1.0
	}
	op := 1.0
	for cur := o; cur != nil; cur = cur.Parent() {
		st := cur.Style()
		if st == nil {
			continue
		}
		op *= st.Opacity
	}
	if op < 0 {
		op = 0
	}
	if op > 1 {
		op = 1
	}
	return op
}

// ApplyOpacityToColor multiplies the alpha channel of a graphics color by the
// given opacity factor. Used by painters to apply CSS opacity to individual
// draw calls when SaveLayer is not used.
func ApplyOpacityToColor(c graphics.Color, opacity float64) graphics.Color {
	if opacity >= 1.0 {
		return c
	}
	if opacity < 0 {
		opacity = 0
	}
	alpha := float64(c.A) * opacity
	if alpha > 255 {
		alpha = 255
	}
	return graphics.Color{R: c.R, G: c.G, B: c.B, A: uint8(alpha)}
}
