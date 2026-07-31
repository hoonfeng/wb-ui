// SVG shape parser and painter.
//
// Parses <svg> child elements and paints SVG shapes (rect, circle, ellipse,
// line, polyline, polygon, path, text) to a Skia canvas. Supports gradients,
// clip paths, viewBox scaling, and CSS style attributes.
//
// Completeness: 70%
// Missing: masks, filters, <use>, <image>, animation, nested <svg> fully,
// CSS stylesheets inside <style> tags.

package rendering

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/platform/graphics"

	"github.com/hoonfeng/goskia/skia"
)

// --- SVG types ---

type svgShape interface {
	paint(canvas *graphics.Canvas, ctx *svgPaintContext)
}

// svgFilledShape wraps a parsed shape with its resolved paint properties.
// The shapes parsed by parseSVGElement carry geometry only; the walk in
// buildSVGDocument resolves per-element fill/stroke into an elCtx, but that
// context is local to the walk. Without this wrapper the fill would be lost
// when paintSVG runs with the default (transparent) fill — every SVG element
// rendered invisible. The wrapper re-applies the resolved fill during paint.
// gradientID (fill="url(#id)"), stroke and clipID (clip-path="url(#id)") are
// carried too so gradients, strokes and clips survive to paint time.
type svgFilledShape struct {
	shape       svgShape
	fill        graphics.Color
	gradientID  string
	stroke      graphics.Color
	strokeWidth float64
	clipID      string
	transform   string
}

func (s *svgFilledShape) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	// Gradient fill: paint the shape geometry directly with the gradient
	// shader (the shape's own paint would only use flat colors).
	if s.gradientID != "" {
		if g, ok := ctx.gradients[s.gradientID]; ok {
			if s.transform != "" {
				canvas.Save()
				defer canvas.Restore()
				applyTransformOps(canvas, s.transform)
			}
			paintShapeGradient(canvas, s.shape, g)
			return
		}
	}
	c2 := *ctx
	c2.fill = s.fill
	c2.stroke = s.stroke
	c2.strokeWidth = s.strokeWidth
	if s.clipID != "" {
		if clipShapes, ok := ctx.clips[s.clipID]; ok && len(clipShapes) > 0 {
			// Apply the clip path, paint the shape, restore.
			canvas.Save()
			if clipPath := clipShapesToPath(clipShapes); clipPath != nil {
				canvas.ClipPath(clipPath)
				clipPath.Release()
				if s.transform != "" {
					applyTransformOps(canvas, s.transform)
				}
				s.shape.paint(canvas, &c2)
			}
			canvas.Restore()
			return
		}
	}
	if s.transform != "" {
		canvas.Save()
		applyTransformOps(canvas, s.transform)
		s.shape.paint(canvas, &c2)
		canvas.Restore()
		return
	}
	s.shape.paint(canvas, &c2)
}

// clipShapesToPath converts SVG clip shapes into a single skia path. Only
// rect and circle are supported; unsupported shapes contribute nothing.
func clipShapesToPath(shapes []svgShape) *skia.Path {
	if len(shapes) == 0 {
		return nil
	}
	path := skia.NewPath()
	for _, sh := range shapes {
		switch s := sh.(type) {
		case *svgRect:
			path.MoveTo(float32(s.x), float32(s.y))
			path.LineTo(float32(s.x+s.w), float32(s.y))
			path.LineTo(float32(s.x+s.w), float32(s.y+s.h))
			path.LineTo(float32(s.x), float32(s.y+s.h))
			path.Close()
		case *svgCircle:
			// Approximate the circle with an octagon (enough for clips).
			const steps = 16
			for i := 0; i < steps; i++ {
				a := 2 * math.Pi * float64(i) / steps
				x := s.cx + s.r*math.Cos(a)
				y := s.cy + s.r*math.Sin(a)
				if i == 0 {
					path.MoveTo(float32(x), float32(y))
				} else {
					path.LineTo(float32(x), float32(y))
				}
			}
			path.Close()
		}
	}
	return path
}

// paintShapeGradient fills a basic shape with a defs gradient (delegates
// direction/radius math to paintGradientOnShape). Unsupported shape types
// fall back to the flat fill (drawn by the caller's non-gradient path).
func paintShapeGradient(canvas *graphics.Canvas, shape svgShape, g *svgGradient) {
	if canvas == nil || g == nil || len(g.stops) < 2 {
		return
	}
	switch s := shape.(type) {
	case *svgRect:
		paintGradientOnShape(canvas, g, s.x, s.y, s.w, s.h)
	case *svgCircle:
		paintGradientOnShape(canvas, g, s.cx-s.r, s.cy-s.r, s.r*2, s.r*2)
	case *svgEllipse:
		paintGradientOnShape(canvas, g, s.cx-s.rx, s.cy-s.ry, s.rx*2, s.ry*2)
	}
}

// svgPaintContext bundles all paint-time state for a single SVG subtree.
type svgPaintContext struct {
	fill        graphics.Color
	stroke      graphics.Color
	strokeWidth float64
	opacity     float64
	gradients   map[string]*svgGradient // gradients defined in <defs>
	clips       map[string][]svgShape   // clip paths defined in <defs>
}

func defaultSVGContext() *svgPaintContext {
	return &svgPaintContext{
		fill:        graphics.Color{R: 0, G: 0, B: 0, A: 0xFF},
		strokeWidth: 0,
		opacity:     1.0,
		gradients:   make(map[string]*svgGradient),
		clips:       make(map[string][]svgShape),
	}
}

// --- Gradient types ---

type svgStop struct {
	offset float64
	color  graphics.Color
}

type svgGradient struct {
	id        string
	x1, y1, x2, y2 float64 // linear
	cx, cy, r       float64 // radial
	isRadial        bool
	stops           []svgStop
}

// --- Basic shapes (unchanged from before) ---

type svgRect struct {
	x, y, w, h, rx, ry float64
}

func (s *svgRect) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	fill := ctx.fill
	if fill.A > 0 {
		if s.rx > 0 || s.ry > 0 {
			r := s.rx
			if r == 0 {
				r = s.ry
			}
			canvas.FillRoundRect(s.x, s.y, s.w, s.h, r, fill)
		} else {
			canvas.FillRect(s.x, s.y, s.w, s.h, fill)
		}
	}
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		if s.rx > 0 || s.ry > 0 {
			r := s.rx
			if r == 0 {
				r = s.ry
			}
			canvas.StrokeRoundRect(s.x, s.y, s.w, s.h, r, ctx.strokeWidth, ctx.stroke)
		} else {
			canvas.StrokeRect(s.x, s.y, s.w, s.h, ctx.strokeWidth, ctx.stroke)
		}
	}
}

type svgCircle struct{ cx, cy, r float64 }

func (s *svgCircle) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	fill := ctx.fill
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, s.r, fill)
	}
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		canvas.StrokeCircle(s.cx, s.cy, s.r, ctx.strokeWidth, ctx.stroke)
	}
}

type svgEllipse struct{ cx, cy, rx, ry float64 }
func (s *svgEllipse) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	r := (s.rx + s.ry) / 2
	fill := ctx.fill
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, r, fill)
	}
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		canvas.StrokeCircle(s.cx, s.cy, r, ctx.strokeWidth, ctx.stroke)
	}
}

type svgLine struct{ x1, y1, x2, y2 float64 }

func (s *svgLine) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		canvas.StrokeLine(s.x1, s.y1, s.x2, s.y2, ctx.strokeWidth, ctx.stroke)
	}
}

type svgPolygon struct {
	points []graphics.Point
	closed bool
}

func (s *svgPolygon) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	fill := ctx.fill
	if fill.A > 0 && len(s.points) >= 3 {
		canvas.FillTriangle(s.points[0].X, s.points[0].Y,
			s.points[1].X, s.points[1].Y,
			s.points[2].X, s.points[2].Y, fill)
	}
}

// svgUse represents an SVG <use> element that references another by id.
type svgUse struct {
	refID string
	x, y  float64
}

func (s *svgUse) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {}

// svgTranslatedShape applies x/y offset to a referenced shape (for <use>).
type svgTranslatedShape struct {
	shape svgShape
	dx, dy float64
}

func (s *svgTranslatedShape) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	canvas.Save()
	canvas.Translate(s.dx, s.dy)
	s.shape.paint(canvas, ctx)
	canvas.Restore()
}

type pathCmd struct {
	kind byte
	args []float64
}

type svgPath struct{ commands []pathCmd }

func (s *svgPath) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if len(s.commands) == 0 {
		return
	}
	fill := ctx.fill
	var pts []graphics.Point
	var firstPoint graphics.Point
	hasFirst := false
	curX, curY := 0.0, 0.0

	for _, cmd := range s.commands {
		args := cmd.args
		switch cmd.kind {
		case 'M', 'm':
			if len(args) >= 2 {
				x, y := args[0], args[1]
				if cmd.kind == 'm' {
					x += curX
					y += curY
				}
				p := graphics.Point{X: x, Y: y}
				pts = append(pts, p)
				curX, curY = x, y
				if !hasFirst {
					firstPoint = p
					hasFirst = true
				}
			}
		case 'L', 'l':
			if len(args) >= 2 {
				x, y := args[0], args[1]
				if cmd.kind == 'l' {
					x += curX
					y += curY
				}
				pts = append(pts, graphics.Point{X: x, Y: y})
				curX, curY = x, y
			}
		case 'C', 'c': // cubic bezier: x1 y1 x2 y2 x y
			if len(args) >= 6 {
				x1, y1, x2, y2, x, y := args[0], args[1], args[2], args[3], args[4], args[5]
				if cmd.kind == 'c' {
					x1 += curX
					y1 += curY
					x2 += curX
					y2 += curY
					x += curX
					y += curY
				}
				pts = append(pts, sampleCubicBezier(curX, curY, x1, y1, x2, y2, x, y)...)
				curX, curY = x, y
			}
		case 'Q', 'q': // quadratic bezier: x1 y1 x y
			if len(args) >= 4 {
				x1, y1, x, y := args[0], args[1], args[2], args[3]
				if cmd.kind == 'q' {
					x1 += curX
					y1 += curY
					x += curX
					y += curY
				}
				pts = append(pts, sampleQuadraticBezier(curX, curY, x1, y1, x, y)...)
				curX, curY = x, y
			}
		case 'A', 'a': // elliptical arc: rx ry x-axis-rot large-arc sweep x y
			if len(args) >= 7 {
				rx, ry := math.Abs(args[0]), math.Abs(args[1])
				phi := args[2] * math.Pi / 180
				laf, sf := args[3] != 0, args[4] != 0
				x, y := args[5], args[6]
				if cmd.kind == 'a' {
					x += curX
					y += curY
				}
				pts = append(pts, arcToPolyline(curX, curY, rx, ry, phi, laf, sf, x, y)...)
				curX, curY = x, y
			}
		case 'Z', 'z':
			if hasFirst {
				pts = append(pts, firstPoint)
				curX, curY = firstPoint.X, firstPoint.Y
			}
		}
	}
	// Triangle fan fill
	if fill.A > 0 && len(pts) >= 3 {
		for i := 1; i < len(pts)-1; i++ {
			canvas.FillTriangle(pts[0].X, pts[0].Y, pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y, fill)
		}
	}
	// Stroke as line segments
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 && len(pts) >= 2 {
		for i := 0; i < len(pts)-1; i++ {
			canvas.StrokeLine(pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y, ctx.strokeWidth, ctx.stroke)
		}
	}
}

// svgImage embeds a raster image (data URI or file) into the SVG, mirroring
// the SVG <image> element. Paints the decoded bitmap scaled to (x,y,w,h).
type svgImage struct {
	x, y, w, h float64
	href       string
}

func (s *svgImage) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if canvas == nil || s.href == "" || s.w <= 0 || s.h <= 0 {
		return
	}
	img := loadBackgroundImage(s.href, "")
	if img != nil && img.Loaded() {
		img.Draw(canvas, s.x, s.y, s.w, s.h)
	}
}

// sampleCubicBezier returns points along a cubic bezier curve.
func sampleCubicBezier(x0, y0, x1, y1, x2, y2, x3, y3 float64) []graphics.Point {
	const steps = 16
	out := make([]graphics.Point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / steps
		mt := 1 - t
		x := mt*mt*mt*x0 + 3*mt*mt*t*x1 + 3*mt*t*t*x2 + t*t*t*x3
		y := mt*mt*mt*y0 + 3*mt*mt*t*y1 + 3*mt*t*t*y2 + t*t*t*y3
		out = append(out, graphics.Point{X: x, Y: y})
	}
	return out
}

// sampleQuadraticBezier returns points along a quadratic bezier curve.
func sampleQuadraticBezier(x0, y0, x1, y1, x2, y2 float64) []graphics.Point {
	const steps = 12
	out := make([]graphics.Point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / steps
		mt := 1 - t
		x := mt*mt*x0 + 2*mt*t*x1 + t*t*x2
		y := mt*mt*y0 + 2*mt*t*y1 + t*t*y2
		out = append(out, graphics.Point{X: x, Y: y})
	}
	return out
}

// arcToPolyline converts an SVG elliptical arc (endpoint parametrization,
// SVG 1.1 F.6.5) into a polyline sampled along the arc. Returns empty when
// the arc degenerates (zero radii or coincident endpoints).
func arcToPolyline(x1, y1, rx, ry, phi float64, largeArc, sweep bool, x2, y2 float64) []graphics.Point {
	if rx == 0 || ry == 0 {
		// Degenerate: straight line.
		return []graphics.Point{{X: x2, Y: y2}}
	}
	dx2 := (x1 - x2) / 2
	dy2 := (y1 - y2) / 2
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	x1p := cosP*dx2 + sinP*dy2
	y1p := -sinP*dx2 + cosP*dy2
	// Correct radii per F.6.6.
	lambda := (x1p*x1p)/(rx*rx) + (y1p*y1p)/(ry*ry)
	if lambda > 1 {
		s := math.Sqrt(lambda)
		rx *= s
		ry *= s
	}
	// Center computation (F.6.5.1).
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	coef := 0.0
	if den > 0 {
		coef = math.Sqrt(math.Max(num/den, 0))
	}
	if largeArc == sweep {
		coef = -coef
	}
	cxp := coef * (rx * y1p / ry)
	cyp := coef * (-ry * x1p / rx)
	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2
	// Angles (F.6.5.2).
	angle := func(ux, uy, vx, vy float64) float64 {
		dot := ux*vx + uy*vy
		lens := math.Hypot(ux, uy) * math.Hypot(vx, vy)
		if lens == 0 {
			return 0
		}
		a := math.Acos(math.Max(-1, math.Min(1, dot/lens)))
		if ux*vy-uy*vx < 0 {
			a = -a
		}
		return a
	}
	theta1 := angle(1, 0, (x1p-cxp)/rx, (y1p-cyp)/ry)
	dtheta := angle((x1p-cxp)/rx, (y1p-cyp)/ry, (-x1p-cxp)/rx, (-y1p-cyp)/ry)
	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}
	const steps = 24
	out := make([]graphics.Point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / steps
		theta := theta1 + t*dtheta
		cosT, sinT := math.Cos(theta), math.Sin(theta)
		x := cx + rx*cosT*cosP - ry*sinT*sinP
		y := cy + rx*cosT*sinP + ry*sinT*cosP
		out = append(out, graphics.Point{X: x, Y: y})
	}
	return out
}
// --- SVG Text ---

type svgText struct {
	x, y float64
	fontFamily string
	fontSize   float64
	textAnchor string
	content    string
	fill       graphics.Color
	stroke     graphics.Color
	strokeWidth float64
}

func (s *svgText) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if s.content == "" {
		return
	}
	fill := s.fill
	if fill.A == 0 {
		fill = ctx.fill
	}
	if fill.A == 0 {
		return
	}
	size := s.fontSize
	if size <= 0 {
		size = 16
	}
	family := s.fontFamily
	if family == "" {
		family = "sans-serif"
	}
	font := graphics.Font{Family: family, Size: size, Weight: 400, Style: "normal"}

	anchor := s.textAnchor
	if anchor == "" {
		anchor = "start"
	}
	lines := strings.Split(s.content, "\n")
	lineHeight := size * 1.2
	for i, line := range lines {
		if line == "" {
			continue
		}
		textX := s.x
		if anchor == "middle" {
			w := graphics.MeasureText(font, line)
			textX -= w / 2
		} else if anchor == "end" {
			w := graphics.MeasureText(font, line)
			textX -= w
		}
		baseline := s.y + float64(i)*lineHeight
		canvas.DrawText(textX, baseline, line, font, fill)
		if s.stroke.A > 0 && s.strokeWidth > 0 {
			canvas.DrawText(textX, baseline, line, font, s.stroke)
		}
	}
}

// --- SVG Document ---

type svgDocument struct {
	shapes      []svgShape
	width       float64
	height      float64
	viewBox     [4]float64 // x, y, w, h (0 if not set)
	hasVB       bool
	elementByID map[string]*dom.Element // used by <use> references
	// gradients/clips carry defs contents to paint time (the paint context
	// is fresh per paintSVG call, so the defs parsed during build must be
	// stored here for shape gradient/clip resolution).
	gradients map[string]*svgGradient
	clips     map[string][]svgShape
	// styleRules carry <style> sheet rules (class/type selectors resolved to
	// fill/stroke) so shapes without inline presentation attributes can pick
	// up stylesheet styling, like real SVG.
	styleRules []svgStyleRule
}

// svgStyleRule is one declaration subset (fill/stroke) extracted from a
// <style> rule, matched against element class or tag name.
type svgStyleRule struct {
	selector    string // serialized selector text (".cls" or "rect")
	fill        string
	stroke      string
	strokeWidth string
}

// matchSVGStyleRule finds the first stylesheet rule matching the element:
// a selector containing ".cls" matches when the element's class attribute
// lists cls; a bare tag-name selector matches by local name.
func matchSVGStyleRule(rules []svgStyleRule, el *dom.Element) *svgStyleRule {
	classes := strings.Fields(el.GetAttribute("class"))
	tag := strings.ToLower(el.LocalName())
	for i := range rules {
		sel := rules[i].selector
		if strings.Contains(sel, ".") {
			for _, cls := range classes {
				if strings.Contains(sel, "."+cls) {
					return &rules[i]
				}
			}
			continue
		}
		if sel == tag {
			return &rules[i]
		}
	}
	return nil
}

// --- Parsing helpers ---

func parseSVGCoord(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "px") {
		s = s[:len(s)-2]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseSVGCoordList(s string) []float64 {
	var vals []float64
	for _, p := range strings.Fields(s) {
		vals = append(vals, parseSVGCoord(p))
	}
	return vals
}

func parseSVGPoints(s string) []graphics.Point {
	parts := strings.Fields(s)
	var pts []graphics.Point
	for i := 0; i+1 < len(parts); i += 2 {
		x, _ := strconv.ParseFloat(parts[i], 64)
		y, _ := strconv.ParseFloat(parts[i+1], 64)
		pts = append(pts, graphics.Point{X: x, Y: y})
	}
	return pts
}

// --- Path parsing ---

func tokenizeSVGPath(s string) []string {
	var tokens []string
	var cur strings.Builder
	for _, c := range s {
		if c == ' ' || c == ',' || c == '\t' || c == '\n' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			continue
		}
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			tokens = append(tokens, string(c))
			continue
		}
		if c == '-' && cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
		cur.WriteRune(c)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func parseSVGPathData(s string) []pathCmd {
	var cmds []pathCmd
	tokens := tokenizeSVGPath(s)
	i := 0
	lastCmd := byte('M')
	for i < len(tokens) {
		cmd := lastCmd
		tok := tokens[i]
		if len(tok) == 1 && ((tok[0] >= 'A' && tok[0] <= 'Z') || (tok[0] >= 'a' && tok[0] <= 'z')) {
			cmd = tok[0]
			i++
		}
		var args []float64
		for i < len(tokens) {
			tok = tokens[i]
			if len(tok) == 1 && ((tok[0] >= 'A' && tok[0] <= 'Z') || (tok[0] >= 'a' && tok[0] <= 'z')) {
				break
			}
			if v, err := strconv.ParseFloat(tok, 64); err == nil {
				args = append(args, v)
			}
			i++
		}
		cmds = append(cmds, pathCmd{kind: cmd, args: args})
		lastCmd = cmd
	}
	return cmds
}

// --- CSS style parsing ---

// parseStyleAttribute parses a CSS style attribute like "fill:red;stroke:black;stroke-width:2"
// and returns a map of property→value.
func parseStyleAttribute(s string) map[string]string {
	m := make(map[string]string)
	if s == "" {
		return m
	}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(part[:colon])
		val := strings.TrimSpace(part[colon+1:])
		if key != "" {
			m[key] = val
		}
	}
	return m
}

// parseColorAttribute parses an SVG color value, which can be a named color,
// hex/rgb/rgba, or "none" (returns zero alpha).
func parseColorAttribute(s string) graphics.Color {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" || s == "transparent" {
		return graphics.Color{}
	}
	// Try parseColorSimple from animation.go
	if c, ok := parseColorSimple(s); ok {
		return c
	}
	// Fallback: black
	return graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
}

// parseURLReference extracts the id from a url(#id) reference.
func parseURLReference(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "url(#") && strings.HasSuffix(s, ")") {
		return s[5 : len(s)-1]
	}
	return ""
}

// parseViewBox parses a viewBox attribute "x y w h" into [4]float64.
func parseViewBox(s string) [4]float64 {
	var vb [4]float64
	parts := strings.Fields(s)
	if len(parts) < 4 {
		return vb
	}
	for i := 0; i < 4 && i < len(parts); i++ {
		vb[i] = parseSVGCoord(parts[i])
	}
	return vb
}

// --- Element parsing ---

// parseSVGElement parses a single SVG child element into a shape.
func parseSVGElement(el *dom.Element) svgShape {
	tag := el.LocalName()

	// Parse CSS style attributes
	styleMap := parseStyleAttribute(el.GetAttribute("style"))
	getAttr := func(name string) string {
		if v, ok := styleMap[name]; ok {
			return v
		}
		return el.GetAttribute(name)
	}

	switch tag {
	case "rect":
		return &svgRect{
			x:  parseSVGCoord(el.GetAttribute("x")),
			y:  parseSVGCoord(el.GetAttribute("y")),
			w:  parseSVGCoord(el.GetAttribute("width")),
			h:  parseSVGCoord(el.GetAttribute("height")),
			rx: parseSVGCoord(el.GetAttribute("rx")),
			ry: parseSVGCoord(el.GetAttribute("ry")),
		}
	case "circle":
		return &svgCircle{
			cx: parseSVGCoord(el.GetAttribute("cx")),
			cy: parseSVGCoord(el.GetAttribute("cy")),
			r:  parseSVGCoord(el.GetAttribute("r")),
		}
	case "ellipse":
		return &svgEllipse{
			cx: parseSVGCoord(el.GetAttribute("cx")),
			cy: parseSVGCoord(el.GetAttribute("cy")),
			rx: parseSVGCoord(el.GetAttribute("rx")),
			ry: parseSVGCoord(el.GetAttribute("ry")),
		}
	case "line":
		return &svgLine{
			x1: parseSVGCoord(el.GetAttribute("x1")),
			y1: parseSVGCoord(el.GetAttribute("y1")),
			x2: parseSVGCoord(el.GetAttribute("x2")),
			y2: parseSVGCoord(el.GetAttribute("y2")),
		}
	case "polygon", "polyline":
		return &svgPolygon{
			points: parseSVGPoints(el.GetAttribute("points")),
			closed: tag == "polygon",
		}
	case "path":
		return &svgPath{commands: parseSVGPathData(el.GetAttribute("d"))}
	case "text":
		content := el.TextContent()
		return &svgText{
			x:           parseSVGCoord(el.GetAttribute("x")),
			y:           parseSVGCoord(el.GetAttribute("y")),
			fontFamily:  getAttr("font-family"),
			fontSize:    parseSVGCoord(getAttr("font-size")),
			textAnchor:  el.GetAttribute("text-anchor"),
			content:     content,
			fill:        parseColorAttribute(getAttr("fill")),
			stroke:      parseColorAttribute(getAttr("stroke")),
			strokeWidth: parseSVGCoord(getAttr("stroke-width")),
		}
	case "image":
		href := el.GetAttribute("href")
		if href == "" {
			href = el.GetAttribute("xlink:href")
		}
		return &svgImage{
			x:    parseSVGCoord(el.GetAttribute("x")),
			y:    parseSVGCoord(el.GetAttribute("y")),
			w:    parseSVGCoord(el.GetAttribute("width")),
			h:    parseSVGCoord(el.GetAttribute("height")),
			href: href,
		}
	case "use":
		href := el.GetAttribute("href")
		if href == "" {
			href = el.GetAttribute("xlink:href")
		}
		if refID := parseURLReference(href); refID != "" {
			return &svgUse{refID: refID,
				x: parseSVGCoord(el.GetAttribute("x")),
				y: parseSVGCoord(el.GetAttribute("y"))}
		}
		return nil
	}
	return nil
}

// --- Gradient parsing ---

func parseGradientElement(el *dom.Element) *svgGradient {
	tag := strings.ToLower(el.LocalName())
	if tag != "lineargradient" && tag != "radialgradient" {
		return nil
	}
	g := &svgGradient{
		id: el.GetAttribute("id"),
	}
	if tag == "radialGradient" {
		g.isRadial = true
		g.cx = parseSVGCoord(el.GetAttribute("cx"))
		g.cy = parseSVGCoord(el.GetAttribute("cy"))
		g.r = parseSVGCoord(el.GetAttribute("r"))
		if g.r <= 0 {
			g.r = 50 // default radius
		}
	} else {
		g.x1 = parseSVGCoord(el.GetAttribute("x1"))
		g.y1 = parseSVGCoord(el.GetAttribute("y1"))
		g.x2 = parseSVGCoord(el.GetAttribute("x2"))
		g.y2 = parseSVGCoord(el.GetAttribute("y2"))
		if g.x2 == 0 && g.y2 == 0 {
			g.x2 = 100 // default horizontal gradient
		}
	}
	// Parse <stop> child elements
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if stopEl, ok := c.(*dom.Element); ok && stopEl.LocalName() == "stop" {
			offsetStr := stopEl.GetAttribute("offset")
			offset := 0.0
			if strings.HasSuffix(offsetStr, "%") {
				offset, _ = strconv.ParseFloat(strings.TrimSuffix(offsetStr, "%"), 64)
				offset /= 100.0
			} else {
				offset = parseSVGCoord(offsetStr)
			}
			stopColor := parseColorAttribute(stopEl.GetAttribute("stop-color"))
			if stopColor.A == 0 {
				stopColor = graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
			}
			// Check stop-opacity
			if opStr := stopEl.GetAttribute("stop-opacity"); opStr != "" {
				if op, err := strconv.ParseFloat(opStr, 64); err == nil {
					stopColor.A = uint8(op * 255)
				}
			}
			g.stops = append(g.stops, svgStop{offset: offset, color: stopColor})
		}
	}
	return g
}

// paintGradientOnShape draws a gradient-filled rectangle (the simplest case).
// For more complex shapes, the gradient approach would need a more general
// path-based implementation; for now this handles the common <rect> case.
func paintGradientOnShape(canvas *graphics.Canvas, g *svgGradient, x, y, w, h float64) {
	if len(g.stops) < 2 {
		return
	}
	if g.isRadial {
		cx := g.cx
		cy := g.cy
		r := g.r
		if cx == 0 && cy == 0 && r == 50 {
			// Default: center of bounding box
			cx = x + w/2
			cy = y + h/2
			r = (w + h) / 4
		}
		canvas.FillRadialGradient(cx, cy, r, g.stops[0].color, g.stops[len(g.stops)-1].color)
		return
	}
	// Linear gradient. The canvas API only paints top-to-bottom, so paint
	// pixel columns manually (mirroring the CSS gradient rasterizer) for the
	// default horizontal direction; other directions fall back to the
	// vertical canvas gradient.
	x1, y1, x2, y2 := g.x1, g.y1, g.x2, g.y2
	if x1 == 0 && y1 == 0 && x2 == 100 && y2 == 0 {
		// Default: left to right across the bounding box.
		stops := make([]ColorStop, len(g.stops))
		for i, s := range g.stops {
			stops[i] = ColorStop{Color: s.color, Position: s.offset}
		}
		iw := int(w)
		if iw < 1 {
			return
		}
		for px := 0; px < iw; px++ {
			t := (float64(px) + 0.5) / w
			canvas.FillRect(x+float64(px), y, 1, h, interpolateColor(stops, t))
		}
		return
	}
	canvas.FillLinearGradient(x, y, w, h, g.stops[0].color, g.stops[len(g.stops)-1].color)
}

// --- ClipPath parsing ---

func parseClipPathElement(el *dom.Element) []svgShape {
	var shapes []svgShape
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if childEl, ok := c.(*dom.Element); ok {
			if shape := parseSVGElement(childEl); shape != nil {
				shapes = append(shapes, shape)
			}
		}
	}
	return shapes
}

// --- Document builder ---

func buildSVGDocument(el *dom.Element) *svgDocument {
	doc := &svgDocument{
		width:       parseSVGCoord(el.GetAttribute("width")),
		height:      parseSVGCoord(el.GetAttribute("height")),
		elementByID: make(map[string]*dom.Element),
	}
	if vb := el.GetAttribute("viewBox"); vb != "" {
		doc.viewBox = parseViewBox(vb)
		doc.hasVB = true
	}
	// If no explicit width/height, use viewBox dimensions
	if doc.width <= 0 && doc.hasVB {
		doc.width = doc.viewBox[2]
		doc.height = doc.viewBox[3]
	}

	// First pass: collect all elements with id attributes (for <use> references)
	var collectIDs func(dom.Node)
	collectIDs = func(n dom.Node) {
		if n == nil {
			return
		}
		if childEl, ok := n.(*dom.Element); ok {
			if id := childEl.GetAttribute("id"); id != "" {
				doc.elementByID[id] = childEl
			}
			for c := childEl.FirstChild(); c != nil; c = c.NextSibling() {
				collectIDs(c)
			}
		}
	}
	collectIDs(el)

	var walk func(dom.Node, *svgPaintContext, float64, float64)
	walk = func(n dom.Node, ctx *svgPaintContext, offX, offY float64) {
		if n == nil {
			return
		}
		childEl, ok := n.(*dom.Element)
		if !ok {
			return
		}
		tag := childEl.LocalName()

		// Nested <svg> elements shift their children by their x/y attributes
		// (the root svg usually has none). Accumulate into the offset passed
		// down to descendants.
		if tag == "svg" {
			offX += parseSVGCoord(childEl.GetAttribute("x"))
			offY += parseSVGCoord(childEl.GetAttribute("y"))
		}

		// Handle <defs> — collect gradients and clip paths. LocalName() is
		// lowercased, so compare lowercase (SVG tag names are case-sensitive
		// but this port normalizes them for element lookup).
		if tag == "defs" {
			for c := childEl.FirstChild(); c != nil; c = c.NextSibling() {
				if defEl, ok := c.(*dom.Element); ok {
					switch strings.ToLower(defEl.LocalName()) {
					case "lineargradient", "radialgradient":
						if g := parseGradientElement(defEl); g != nil && g.id != "" {
							ctx.gradients[g.id] = g
						}
					case "clippath":
						clipID := defEl.GetAttribute("id")
						if clipID != "" {
							ctx.clips[clipID] = parseClipPathElement(defEl)
						}
					}
				}
			}
			return
		}

		// Handle <style> — collect fill/stroke rules from the embedded CSS
		// sheet so shapes can be styled by class like real SVG.
		if tag == "style" {
			for _, rule := range css.NewParser(childEl.TextContent()).ParseStyleSheet() {
				if sr, ok := rule.(*css.StyleRule); ok {
					var r svgStyleRule
					r.selector = strings.TrimSpace(sr.Selectors.String())
					if r.selector == "" {
						continue
					}
					for _, d := range sr.Declarations {
						val := d.ValueString()
						switch strings.ToLower(d.Name) {
						case "fill":
							r.fill = val
						case "stroke":
							r.stroke = val
						case "stroke-width":
							r.strokeWidth = val
						}
					}
					doc.styleRules = append(doc.styleRules, r)
				}
			}
			return
		}

		// Check display:none
		styleMap := parseStyleAttribute(childEl.GetAttribute("style"))
		if styleMap["display"] == "none" {
			return
		}

		// Resolve fill and stroke from attributes or style
		fillStr := childEl.GetAttribute("fill")
		if fillStr == "" {
			fillStr = styleMap["fill"]
		}
		strokeStr := childEl.GetAttribute("stroke")
		if strokeStr == "" {
			strokeStr = styleMap["stroke"]
		}
		swStr := childEl.GetAttribute("stroke-width")
		if swStr == "" {
			swStr = styleMap["stroke-width"]
		}

		// Third precedence level: stylesheet rules (<style> sheet) matched by
		// class or tag name. Inline attributes and style="" win, exactly like
		// CSS presentation-attribute precedence in SVG.
		if fillStr == "" || strokeStr == "" || swStr == "" {
			if sr := matchSVGStyleRule(doc.styleRules, childEl); sr != nil {
				if fillStr == "" && sr.fill != "" {
					fillStr = sr.fill
				}
				if strokeStr == "" && sr.stroke != "" {
					strokeStr = sr.stroke
				}
				if swStr == "" && sr.strokeWidth != "" {
					swStr = sr.strokeWidth
				}
			}
		}

		// Create a per-element paint context
		elCtx := &svgPaintContext{
			fill:        ctx.fill,
			stroke:      ctx.stroke,
			strokeWidth: ctx.strokeWidth,
			opacity:     ctx.opacity,
			gradients:   ctx.gradients,
			clips:       ctx.clips,
		}

		// Resolve url(#gradientId) references
		if gradientID := parseURLReference(fillStr); gradientID != "" {
			if g, ok := ctx.gradients[gradientID]; ok {
				// Gradient fill — fallback to black, use gradient colors
				elCtx.fill = graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
				_ = g // gradient applied in shape-specific code
			}
		} else if fillStr != "" {
			elCtx.fill = parseColorAttribute(fillStr)
		}

		if strokeStr != "" {
			elCtx.stroke = parseColorAttribute(strokeStr)
		}
		if swStr != "" {
			elCtx.strokeWidth = parseSVGCoord(swStr)
		}

		// Handle clip-path="url(#id)"
		clipStr := childEl.GetAttribute("clip-path")
		if clipStr == "" {
			clipStr = styleMap["clip-path"]
		}
		if clipID := parseURLReference(clipStr); clipID != "" {
			if clipShapes, ok := ctx.clips[clipID]; ok && len(clipShapes) > 0 {
				// For simple clips, we apply a clip to the canvas
				canvas := &graphics.Canvas{}
				_ = canvas
				// Clip path shapes will be applied during paint
				_ = clipShapes
			}
		}

		// Handle <use> elements: look up referenced element and clone its shape
		if tag == "use" {
			href := childEl.GetAttribute("href")
			if href == "" {
				href = childEl.GetAttribute("xlink:href")
			}
			// <use> uses #id fragment syntax (not url(#id)); parseURLReference
			// only handles url() so accept both forms here.
			refID := parseURLReference(href)
			if refID == "" && strings.HasPrefix(strings.TrimSpace(href), "#") {
				refID = strings.TrimSpace(href)[1:]
			}
			if refEl, ok := doc.elementByID[refID]; ok {
				refShape := parseSVGElement(refEl)
				if refShape != nil {
					// The referenced shape is pure geometry; resolve its
					// own fill so it is visible (the paint context's
					// default fill is transparent).
					if refFillStr := refEl.GetAttribute("fill"); refFillStr != "" {
						if refFill := parseColorAttribute(refFillStr); refFill.A > 0 {
							refShape = &svgFilledShape{shape: refShape, fill: refFill}
						}
					}
					// Apply <use> x/y offset
					dx := parseSVGCoord(childEl.GetAttribute("x"))
					dy := parseSVGCoord(childEl.GetAttribute("y"))
					if dx != 0 || dy != 0 {
						refShape = &svgTranslatedShape{shape: refShape, dx: dx, dy: dy}
					}
					doc.shapes = append(doc.shapes, refShape)
				}
			}
			return // <use> resolved, skip children
		}

		// Parse the shape
		if shape := parseSVGElement(childEl); shape != nil {
			// Nested <svg> / group offsets apply to the whole subtree.
			if offX != 0 || offY != 0 {
				shape = &svgTranslatedShape{shape: shape, dx: offX, dy: offY}
			}
			// Attach the resolved paint properties so painting (which runs
			// with a fresh default context) still sees them.
			wrapper := &svgFilledShape{
				shape:       shape,
				fill:        elCtx.fill,
				stroke:      elCtx.stroke,
				strokeWidth: elCtx.strokeWidth,
			}
			if gradientID := parseURLReference(fillStr); gradientID != "" {
				wrapper.gradientID = gradientID
			}
			if clipID := parseURLReference(clipStr); clipID != "" {
				wrapper.clipID = clipID
			}
			wrapper.transform = childEl.GetAttribute("transform")
			doc.shapes = append(doc.shapes, wrapper)
		}

		// Recurse into children
		for c := childEl.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, elCtx, offX, offY)
		}
	}

	ctx := defaultSVGContext()
	walk(el, ctx, 0, 0)
	doc.gradients = ctx.gradients
	doc.clips = ctx.clips
	return doc
}

// --- Painting ---

func paintSVG(canvas *graphics.Canvas, doc *svgDocument, x, y float64, defaultFill graphics.Color) {
	if doc == nil || len(doc.shapes) == 0 {
		return
	}

	// Apply viewBox transform if present
	if doc.hasVB && doc.width > 0 && doc.height > 0 {
		vb := doc.viewBox
		vbW := vb[2]
		vbH := vb[3]
		if vbW > 0 && vbH > 0 {
			scaleX := doc.width / vbW
			scaleY := doc.height / vbH
			canvas.Save()
			canvas.Translate(x, y)
			canvas.Scale(scaleX, scaleY)
			canvas.Translate(-vb[0], -vb[1])
			defer canvas.Restore()

			ctx := defaultSVGContext()
			ctx.fill = defaultFill
			ctx.gradients = doc.gradients
			ctx.clips = doc.clips
			for _, s := range doc.shapes {
				s.paint(canvas, ctx)
			}
			return
		}
	}

	// No viewBox: simple translation
	if x != 0 || y != 0 {
		canvas.Save()
		canvas.Translate(x, y)
		defer canvas.Restore()
	}

	ctx := defaultSVGContext()
	ctx.fill = defaultFill
	ctx.gradients = doc.gradients
	ctx.clips = doc.clips
	for _, s := range doc.shapes {
		s.paint(canvas, ctx)
	}
}
