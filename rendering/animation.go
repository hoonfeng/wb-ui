// Translation of: Source/WebCore/animation/AnimationTimeline.cpp
//                  Source/WebCore/animation/KeyframeEffect.cpp
//                  Source/WebCore/animation/KeyframeModel.cpp
//                  Source/WebCore/animation/CSSAnimation.cpp
// Completeness: 45%
// Simplifications:
//   - timing-function uses simplified cubic-bezier approximation (non-analytic)
//   - no animation-composition (replace/add/accumulate)
//   - no animation-range (scroll-timeline based)
//   - no animation-play-state (always running)
//   - @keyframes are found via the global KeyframesLookup callback
package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// AnimationTime is the global animation clock in seconds, set by the embedder
// (e.g. app.Host) each frame before calling Paint. It drives @keyframes-based
// animations.
var AnimationTime float64

// KeyframesLookup is a function that returns the @keyframes rule with the
// given name, or nil if not found. The embedder sets this to bridge to the
// style resolver's keyframes collection.
var KeyframesLookup func(name string) *css.KeyframesRule

// --- Top-level driver -------------------------------------------------------

// ApplyAnimations walks the render tree and updates each element's animated
// properties (opacity, transform translate/scale, color, background-color)
// based on the current AnimationTime and the element's animation properties.
// This must be called before Paint each frame.
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
			applyAnimationToStyle(st, AnimationTime)
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
}

// applyAnimationToStyle computes all animated properties for a single element
// and writes them directly into its ComputedStyle.
func applyAnimationToStyle(st *style.ComputedStyle, time float64) {
	kf := KeyframesLookup(st.AnimationName)
	if kf == nil || len(kf.Keyframes) == 0 {
		return
	}
	duration := st.AnimationDuration
	if duration <= 0 {
		duration = 0.001 // prevent division by zero
	}

	// --- 1. Compute effective progress --------------------------------------

	// Apply delay.
	effectiveTime := time - st.AnimationDelay
	if effectiveTime < 0 {
		// Still in delay phase.
		if st.AnimationFillMode == "backwards" || st.AnimationFillMode == "both" {
			// Apply first keyframe.
			progress := 0.0
			applyProgressToStyle(st, kf, progress)
		}
		return
	}

	// Determine iteration count and total duration.
	iterationCount := st.AnimationIterationCount
	infinite := iterationCount == 0
	var totalDuration float64
	if infinite {
		totalDuration = math.Inf(1)
	} else {
		totalDuration = duration * float64(iterationCount)
	}

	// Check if the animation has ended.
	ended := !infinite && effectiveTime > totalDuration
	if ended {
		if st.AnimationFillMode == "forwards" || st.AnimationFillMode == "both" {
			progress := 1.0
			applyProgressToStyle(st, kf, progress)
		}
		return
	}

	// Compute which iteration and local progress.
	iter := int(effectiveTime / duration)
	// Clamp iteration to the last valid one (0-based) to ensure progress == 1.0
	// at the exact end of the final iteration.
	if !infinite && iter >= int(iterationCount) {
		iter = int(iterationCount) - 1
	}
	localT := effectiveTime - float64(iter)*duration
	if localT > duration {
		localT = duration
	}
	progress := localT / duration

	// Apply direction.
	direction := st.AnimationDirection
	if direction == "" {
		direction = "normal"
	}
	reverse := false
	switch direction {
	case "reverse":
		reverse = true
	case "alternate":
		reverse = (iter % 2) == 1
	case "alternate-reverse":
		reverse = (iter % 2) == 0
	}
	if reverse {
		progress = 1.0 - progress
	}

	// Apply timing-function.
	progress = applyTimingFunction(progress, st.AnimationTimingFunction)

	// Clamp.
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	// --- 2. Interpolate keyframe values -------------------------------------
	applyProgressToStyle(st, kf, progress)
}

// applyProgressToStyle applies the interpolated keyframe values at the given
// progress (0.0–1.0) to the ComputedStyle.
func applyProgressToStyle(st *style.ComputedStyle, kf *css.KeyframesRule, progress float64) {
	// Opacity
	if opacity, ok := interpolateKeyframeFloat(kf, progress, "opacity"); ok {
		st.Opacity = opacity
	}

	// Transform: translateX / translateY
	if tx, ok := interpolateTransformTranslate(kf, progress, "translateX"); ok {
		st.TranslateX = tx
	}
	if ty, ok := interpolateTransformTranslate(kf, progress, "translateY"); ok {
		st.TranslateY = ty
	}

	// Transform: scale / scaleX / scaleY
	if s, ok := interpolateTransformScale(kf, progress); ok {
		st.ScaleX = s
		st.ScaleY = s
	}
	if sx, ok := interpolateKeyframeFloat(kf, progress, "scaleX"); ok {
		st.ScaleX = sx
	}
	if sy, ok := interpolateKeyframeFloat(kf, progress, "scaleY"); ok {
		st.ScaleY = sy
	}

	// Color
	if c, ok := interpolateKeyframeColor(kf, progress, "color"); ok {
		st.AnimatedColor = c
	}
	if bg, ok := interpolateKeyframeColor(kf, progress, "background-color"); ok {
		st.AnimatedBackgroundColor = bg
	}
}

// --- Timing function --------------------------------------------------------

// applyTimingFunction applies a CSS timing function to a progress value [0,1].
func applyTimingFunction(t float64, tf string) float64 {
	switch tf {
	case "", "linear":
		return t
	case "ease":
		// cubic-bezier(0.25, 0.1, 0.25, 1.0)
		return cubicBezier(t, 0.25, 0.1, 0.25, 1.0)
	case "ease-in":
		// cubic-bezier(0.42, 0.0, 1.0, 1.0)
		return cubicBezier(t, 0.42, 0.0, 1.0, 1.0)
	case "ease-out":
		// cubic-bezier(0.0, 0.0, 0.58, 1.0)
		return cubicBezier(t, 0.0, 0.0, 0.58, 1.0)
	case "ease-in-out":
		// cubic-bezier(0.42, 0.0, 0.58, 1.0)
		return cubicBezier(t, 0.42, 0.0, 0.58, 1.0)
	default:
		return t
	}
}

// cubicBezier evaluates a cubic Bezier curve at parameter t using
// de Casteljau's algorithm. This is a simplified numeric approximation
// (not the analytic root-finding that WebKit uses).
func cubicBezier(t, x1, y1, x2, y2 float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	// Binary search to find the Bezier parameter u such that X(u) = t.
	// Then evaluate Y(u).
	lo, hi := 0.0, 1.0
	for i := 0; i < 20; i++ {
		mid := (lo + hi) / 2
		x := bezierComponent(mid, x1, x2)
		if x < t {
			lo = mid
		} else {
			hi = mid
		}
	}
	u := (lo + hi) / 2
	return bezierComponent(u, y1, y2)
}

// bezierComponent evaluates the X or Y component of a cubic Bezier with
// control points (0,0), (x1,y1), (x2,y2), (1,1) at parameter u.
func bezierComponent(u, c1, c2 float64) float64 {
	// B(u) = 3(1-u)²u·c1 + 3(1-u)u²·c2 + u³
	u2 := u * u
	u3 := u2 * u
	omu := 1 - u
	omu2 := omu * omu
	return 3*omu2*u*c1 + 3*omu*u2*c2 + u3
}

// --- Keyframe interpolation helpers -----------------------------------------

// keyframePointFloat holds a parsed numeric value at a keyframe offset.
type keyframePointFloat struct {
	offset float64
	value  float64
	valid  bool
}

// keyframePointColor holds a parsed color at a keyframe offset.
type keyframePointColor struct {
	offset float64
	value  graphics.Color
	valid  bool
}

// collectKeyframeFloats collects all keyframe offsets for a given property
// name from the @keyframes rule.
func collectKeyframeFloats(kf *css.KeyframesRule, propName string) []keyframePointFloat {
	seen := map[float64]bool{}
	var points []keyframePointFloat
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findFloatInDecls(rule.Declarations, propName)
			points = append(points, keyframePointFloat{offset: offset, value: val, valid: ok})
		}
	}
	sortKeyframes(points)
	return points
}

// collectKeyframeColors collects all keyframe offsets for a color property.
func collectKeyframeColors(kf *css.KeyframesRule, propName string) []keyframePointColor {
	seen := map[float64]bool{}
	var points []keyframePointColor
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findColorInDecls(rule.Declarations, propName)
			points = append(points, keyframePointColor{offset: offset, value: val, valid: ok})
		}
	}
	sortKeyframesColor(points)
	return points
}

// interpolateKeyframeFloat interpolates a numeric property at the given progress.
// Returns (value, true) if keyframes for that property exist.
func interpolateKeyframeFloat(kf *css.KeyframesRule, progress float64, propName string) (float64, bool) {
	points := collectKeyframeFloats(kf, propName)
	return interpolateFloatAt(points, progress)
}

// interpolateKeyframeColor interpolates a color property at the given progress.
func interpolateKeyframeColor(kf *css.KeyframesRule, progress float64, propName string) (graphics.Color, bool) {
	points := collectKeyframeColors(kf, propName)
	return interpolateColorAt(points, progress)
}

// interpolateTransformTranslate parses translateX or translateY from
// transform declarations at each keyframe.
func interpolateTransformTranslate(kf *css.KeyframesRule, progress float64, funcName string) (float64, bool) {
	seen := map[float64]bool{}
	var points []keyframePointFloat
	for _, rule := range kf.Keyframes {
		for _, key := range rule.Keys {
			offset := parseKeyframeOffset(key)
			if offset < 0 {
				continue
			}
			if seen[offset] {
				continue
			}
			seen[offset] = true
			val, ok := findTransformInDecls(rule.Declarations, funcName)
			points = append(points, keyframePointFloat{offset: offset, value: val, valid: ok})
		}
	}
	if len(points) == 0 {
		return 0, false
	}
	sortKeyframes(points)
	return interpolateFloatAt(points, progress)
}

// interpolateTransformScale parses scale from transform declarations.
func interpolateTransformScale(kf *css.KeyframesRule, progress float64) (float64, bool) {
	return interpolateTransformTranslate(kf, progress, "scale")
}

// interpolateFloatAt does linear interpolation between sorted keyframe points.
func interpolateFloatAt(points []keyframePointFloat, progress float64) (float64, bool) {
	if len(points) == 0 {
		return 0, false
	}
	// If progress is before the first keyframe or after the last, use nearest.
	if progress <= points[0].offset {
		return points[0].value, points[0].valid
	}
	last := points[len(points)-1]
	if progress >= last.offset {
		return last.value, last.valid
	}
	// Find the two surrounding keyframes.
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			if !points[i].valid || !points[i+1].valid {
				return 0, false
			}
			t := (progress - points[i].offset) / (points[i+1].offset - points[i].offset)
			return points[i].value + (points[i+1].value-points[i].value)*t, true
		}
	}
	return 0, false
}

// interpolateColorAt does linear interpolation between sorted keyframe color points.
func interpolateColorAt(points []keyframePointColor, progress float64) (graphics.Color, bool) {
	if len(points) == 0 {
		return graphics.Color{}, false
	}
	if progress <= points[0].offset {
		return points[0].value, points[0].valid
	}
	last := points[len(points)-1]
	if progress >= last.offset {
		return last.value, last.valid
	}
	for i := 0; i < len(points)-1; i++ {
		if progress >= points[i].offset && progress <= points[i+1].offset {
			if !points[i].valid || !points[i+1].valid {
				return graphics.Color{}, false
			}
			t := (progress - points[i].offset) / (points[i+1].offset - points[i].offset)
			return lerpColor(points[i].value, points[i+1].value, t), true
		}
	}
	return graphics.Color{}, false
}

// lerpColor linearly interpolates between two colors component-wise.
func lerpColor(a, b graphics.Color, t float64) graphics.Color {
	return graphics.Color{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

// --- Sorting helpers --------------------------------------------------------

func sortKeyframes(points []keyframePointFloat) {
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
}

func sortKeyframesColor(points []keyframePointColor) {
	for i := 0; i < len(points); i++ {
		for j := i + 1; j < len(points); j++ {
			if points[j].offset < points[i].offset {
				points[i], points[j] = points[j], points[i]
			}
		}
	}
}

// --- Parsing helpers --------------------------------------------------------

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

// findFloatInDecls searches declarations for a property and returns its
// numeric value. Returns (0, false) if not found.
func findFloatInDecls(decls []css.Declaration, propName string) (float64, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, propName) {
			val := strings.TrimSpace(d.ValueString())
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// findColorInDecls searches declarations for a color property.
func findColorInDecls(decls []css.Declaration, propName string) (graphics.Color, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, propName) {
			val := strings.TrimSpace(d.ValueString())
			if c, ok := parseColorSimple(val); ok {
				return c, true
			}
		}
	}
	return graphics.Color{}, false
}

// findTransformInDecls searches declarations for a transform property and
// extracts a specific function value (e.g. translateX(10px) → 10).
func findTransformInDecls(decls []css.Declaration, funcName string) (float64, bool) {
	for _, d := range decls {
		if strings.EqualFold(d.Name, "transform") {
			val := d.ValueString()
			return extractTransformFunc(val, funcName)
		}
	}
	return 0, false
}

// extractTransformFunc extracts the numeric value from a CSS transform
// function like translateX(10px), scale(1.5), rotate(45deg).
func extractTransformFunc(input, funcName string) (float64, bool) {
	lower := strings.ToLower(input)
	search := strings.ToLower(funcName) + "("
	idx := strings.Index(lower, search)
	if idx < 0 {
		return 0, false
	}
	start := idx + len(funcName) + 1
	if start >= len(input) {
		return 0, false
	}
	end := strings.IndexByte(input[start:], ')')
	if end < 0 {
		return 0, false
	}
	arg := strings.TrimSpace(input[start : start+end])
	return parseCSSNumber(arg)
}

// parseCSSNumber parses a CSS numeric value like "10px", "1.5", "45deg".
// Returns the numeric value (stripping the unit).
func parseCSSNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Find the boundary between number and unit.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	dotSeen := false
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			i++
		} else if s[i] == '.' && !dotSeen {
			dotSeen = true
			i++
		} else {
			break
		}
	}
	if i == 0 {
		return 0, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	return num, true
}

// parseColorSimple parses a simple CSS color string (#hex, rgb(), named).
func parseColorSimple(s string) (graphics.Color, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return graphics.Color{}, false
	}
	// #hex
	if s[0] == '#' {
		return parseHexColorSimple(s)
	}
	// rgb() / rgba()
	if strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")") {
		return parseRGBAFunc(s[5 : len(s)-1])
	}
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		c, ok := parseRGBAFunc(s[4 : len(s)-1])
		if ok {
			c.A = 255
		}
		return c, ok
	}
	// Named colors (basic set).
	return namedColorSimple(s)
}

// parseHexColorSimple parses #rgb / #rrggbb / #rrggbbaa.
func parseHexColorSimple(s string) (graphics.Color, bool) {
	if len(s) < 2 {
		return graphics.Color{}, false
	}
	hex := s[1:]
	var r, g, b, a uint8
	a = 255
	switch len(hex) {
	case 3:
		r = hexNibble(hex[0]) * 17
		g = hexNibble(hex[1]) * 17
		b = hexNibble(hex[2]) * 17
	case 6:
		r = hexNibble(hex[0])*16 + hexNibble(hex[1])
		g = hexNibble(hex[2])*16 + hexNibble(hex[3])
		b = hexNibble(hex[4])*16 + hexNibble(hex[5])
	case 8:
		r = hexNibble(hex[0])*16 + hexNibble(hex[1])
		g = hexNibble(hex[2])*16 + hexNibble(hex[3])
		b = hexNibble(hex[4])*16 + hexNibble(hex[5])
		a = hexNibble(hex[6])*16 + hexNibble(hex[7])
	default:
		return graphics.Color{}, false
	}
	return graphics.Color{R: r, G: g, B: b, A: a}, true
}

func hexNibble(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// parseRGBAFunc parses the inner arguments of rgb()/rgba().
func parseRGBAFunc(args string) (graphics.Color, bool) {
	parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == ' ' })
	if len(parts) < 3 {
		return graphics.Color{}, false
	}
	r := parseCSSByte(parts[0])
	g := parseCSSByte(parts[1])
	b := parseCSSByte(parts[2])
	a := uint8(255)
	if len(parts) >= 4 {
		a = parseCSSAlpha(parts[3])
	}
	return graphics.Color{R: r, G: g, B: b, A: a}, true
}

func parseCSSByte(s string) uint8 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v)
}

func parseCSSAlpha(s string) uint8 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 255
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return uint8(v * 255)
}

// namedColorSimple returns a basic named CSS color.
func namedColorSimple(name string) (graphics.Color, bool) {
	switch strings.ToLower(name) {
	case "black":
		return graphics.Color{0, 0, 0, 255}, true
	case "white":
		return graphics.Color{255, 255, 255, 255}, true
	case "red":
		return graphics.Color{255, 0, 0, 255}, true
	case "green", "lime":
		return graphics.Color{0, 255, 0, 255}, true
	case "blue":
		return graphics.Color{0, 0, 255, 255}, true
	case "yellow":
		return graphics.Color{255, 255, 0, 255}, true
	case "transparent":
		return graphics.Color{0, 0, 0, 0}, true
	}
	return graphics.Color{}, false
}

// --- Existing helpers (preserved) -------------------------------------------

// CumulativeOpacity returns the effective opacity for a render object by
// multiplying its own opacity with all ancestors' opacities.
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
// given opacity factor.
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
