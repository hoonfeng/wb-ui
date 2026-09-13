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
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/debugenv"
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
	patternID   string
	stroke      graphics.Color
	strokeWidth float64
	strokeGradientID string // stroke="url(#gradient)" — gradient stroke
	clipID      string
	transform   string
	dashArray   []float64
	lineCap     string
	lineJoin    string
	opacity     float64 // element opacity multiplier; 0 = unset (1.0)
	fillRule    string  // nonzero|evenodd
	dashOffset  float64 // stroke-dashoffset (dash phase)
}

func (s *svgFilledShape) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	// Pattern fill: tile the pattern's shapes across the shape bbox.
	if s.patternID != "" {
		if pat, ok := ctx.patterns[s.patternID]; ok && len(pat.shapes) > 0 && pat.w > 0 && pat.h > 0 {
			px, py, pw, ph := shapeBBox(s.shape)
			if pw > 0 && ph > 0 {
				canvas.Save()
				defer canvas.Restore()
				if s.transform != "" {
					applyTransformOps(canvas, s.transform)
				}
				for ty := py; ty < py+ph; ty += pat.h {
					for tx := px; tx < px+pw; tx += pat.w {
						for _, ps := range pat.shapes {
							canvas.Save()
							canvas.Translate(tx, ty)
							ps.paint(canvas, ctx)
							canvas.Restore()
						}
					}
				}
				return
			}
		}
	}
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
	c2.lineCap = s.lineCap
	c2.lineJoin = s.lineJoin
	c2.fillRule = s.fillRule
	if s.strokeGradientID != "" {
		if g, ok := ctx.gradients[s.strokeGradientID]; ok && len(g.stops) >= 2 {
			c2.strokeGradient = g
		}
	}
	if s.opacity > 0 {
		c2.opacity = s.opacity
	}
	// Element opacity multiplies both fill and stroke alpha.
	if c2.opacity > 0 && c2.opacity < 1 {
		c2.fill.A = uint8(float64(c2.fill.A) * c2.opacity)
		c2.stroke.A = uint8(float64(c2.stroke.A) * c2.opacity)
	}
	if len(s.dashArray) > 0 {
		c2.dashArray = s.dashArray
		c2.dashOffset = s.dashOffset
	}
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

// clipShapesToPath converts SVG clip shapes into a single skia path. Supports
// rect, circle, ellipse and polygon; unsupported shapes contribute nothing.
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
			// Approximate the circle with a polygon (enough for clips).
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
		case *svgEllipse:
			const esteps = 24
			for i := 0; i < esteps; i++ {
				a := 2 * math.Pi * float64(i) / esteps
				x := s.cx + s.rx*math.Cos(a)
				y := s.cy + s.ry*math.Sin(a)
				if i == 0 {
					path.MoveTo(float32(x), float32(y))
				} else {
					path.LineTo(float32(x), float32(y))
				}
			}
			path.Close()
		case *svgPolygon:
			if len(s.points) < 3 {
				continue
			}
			path.MoveTo(float32(s.points[0].X), float32(s.points[0].Y))
			for i := 1; i < len(s.points); i++ {
				path.LineTo(float32(s.points[i].X), float32(s.points[i].Y))
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
	case *svgPath:
		pts := s.samplePoints()
		if len(pts) >= 3 {
			ax, ay, bx, by, colors, pos := gradientParamsForPts(g, pts)
			canvas.FillPathGradient(pts, ax, ay, bx, by, colors, pos, true)
		}
	case *svgPolygon:
		pts := s.points
		if s.closed && len(pts) >= 3 {
			ax, ay, bx, by, colors, pos := gradientParamsForPts(g, pts)
			canvas.FillPathGradient(pts, ax, ay, bx, by, colors, pos, false)
		}
	}
}

// gradientParamsForPts resolves an svgGradient's axis against the point set's
// bounding box (objectBoundingBox units are percentages) and returns the world
// axis plus the Skia color/position arrays.
func gradientParamsForPts(g *svgGradient, pts []graphics.Point) (ax, ay, bx, by float64, colors []graphics.Color, positions []float32) {
	if g == nil || len(pts) == 0 {
		return 0, 0, 1, 1, nil, nil
	}
	minX, minY := pts[0].X, pts[0].Y
	maxX, maxY := pts[0].X, pts[0].Y
	for _, p := range pts[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	w, h := maxX-minX, maxY-minY
	if g.userSpaceOnUse {
		ax, ay, bx, by = g.x1, g.y1, g.x2, g.y2
	} else {
		ax = minX + g.x1*w
		ay = minY + g.y1*h
		bx = minX + g.x2*w
		by = minY + g.y2*h
	}
	colors = make([]graphics.Color, len(g.stops))
	positions = make([]float32, len(g.stops))
	for i, st := range g.stops {
		colors[i] = st.color
		positions[i] = float32(st.offset)
	}
	return
}

// svgPattern is a <pattern> element: child shapes tiled across the fill
// bounding box at (w,h) intervals.
type svgPattern struct {
	shapes []svgShape
	w, h   float64
}

// svgPaintContext bundles all paint-time state for a single SVG subtree.
type svgPaintContext struct {
	fill        graphics.Color
	stroke      graphics.Color
	strokeWidth float64
	strokeGradient *svgGradient // stroke="url(#gradient)" — gradient stroke
	opacity     float64
	gradients   map[string]*svgGradient // gradients defined in <defs>
	clips       map[string][]svgShape   // clip paths defined in <defs>
	dashArray   []float64               // stroke-dasharray pattern
	patterns    map[string]*svgPattern  // patterns defined in <defs>
	markers     map[string]*svgMarker   // markers defined in <defs>
	masks       map[string]*svgMask     // masks defined in <defs>
	lineCap     string                  // butt|round|square (stroke-linecap)
	lineJoin    string                  // miter|round|bevel (stroke-linejoin)
	fillRule    string                  // nonzero|evenodd (fill-rule)
	dashOffset  float64                 // stroke-dashoffset (dash phase)
}

func defaultSVGContext() *svgPaintContext {
	return &svgPaintContext{
		fill:        graphics.Color{R: 0, G: 0, B: 0, A: 0xFF},
		strokeWidth: 0,
		opacity:     1.0,
		gradients:   make(map[string]*svgGradient),
		clips:       make(map[string][]svgShape),
		patterns:    make(map[string]*svgPattern),
		markers:     make(map[string]*svgMarker),
		masks:       make(map[string]*svgMask),
	}
}

// --- Gradient types ---

type svgStop struct {
	offset float64
	color  graphics.Color
}

type svgGradient struct {
	id            string
	x1, y1, x2, y2 float64 // linear
	cx, cy, r       float64 // radial
	isRadial        bool
	userSpaceOnUse  bool // gradientUnits="userSpaceOnUse" (default: objectBoundingBox)
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
		if len(ctx.dashArray) > 0 {
			// ★ stroke-dasharray on rect (media 占位虚线边框等)：沿圆角矩形
			// 中心线折线采样，按累计弧长推进 dash 相位（与 circle/path 的
			// dash 一致）。此前 StrokeRoundRect/StrokeRect 只画实线，
			// dasharray 被静默忽略（浏览器为 8-6 虚线，wb-ui 画实线）。
			half := ctx.strokeWidth / 2
			cw := s.w - ctx.strokeWidth
			ch := s.h - ctx.strokeWidth
			if cw > 0 && ch > 0 {
				r := s.rx
				if r == 0 {
					r = s.ry
				}
				pts := rectCenterlineDashes(s.x+half, s.y+half, cw, ch, r)
				off := ctx.dashOffset
				for i := 0; i < len(pts)-1; i++ {
					dashLine(canvas, pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y,
						ctx.strokeWidth, ctx.stroke, ctx.dashArray, off, ctx.lineCap)
					off += math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
				}
			}
			return
		}
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

// rectCenterlineDashes 生成圆角矩形中心线折线点序列（闭合环），点位顺序：
// 上边左端 → 右上弧 → 右边 → 右下弧 → 下边 → 左下弧 → 左边 → 左上弧 →
// 回到起点。与 canvas.roundRectOutline 一致（r=0 退化为矩形四角直折）。
// 供 svgRect 的 stroke-dasharray 沿弧长采样（svg.go 不能直接调 canvas
// 包内未导出函数，此处本地实现同构折线）。
func rectCenterlineDashes(cx, cy, cw, ch, r float64) []graphics.Point {
	const arcSegs = 8.0
	hw := cw / 2
	hh := ch / 2
	if r > hw {
		r = hw
	}
	if r > hh {
		r = hh
	}
	arcPts := func(ox, oy, rr, a1, a2 float64) []graphics.Point {
		var out []graphics.Point
		for i := 1.0; i <= arcSegs; i++ {
			a := a1 + (a2-a1)*i/arcSegs
			out = append(out, graphics.Point{X: ox + rr*math.Cos(a), Y: oy + rr*math.Sin(a)})
		}
		return out
	}
	var pts []graphics.Point
	// 上边（左端 → 右上弧起点）
	pts = append(pts, graphics.Point{X: cx + r, Y: cy})
	// 右上弧（圆心 cx+cw-r, cy+r；-90° → 0°）
	pts = append(pts, arcPts(cx+cw-r, cy+r, r, -math.Pi/2, 0)...)
	// 右边（→ 右下弧起点）
	pts = append(pts, graphics.Point{X: cx + cw, Y: cy + ch - r})
	// 右下弧（圆心 cx+cw-r, cy+ch-r；0° → 90°）
	pts = append(pts, arcPts(cx+cw-r, cy+ch-r, r, 0, math.Pi/2)...)
	// 下边（右端 → 左下弧起点）
	pts = append(pts, graphics.Point{X: cx + r, Y: cy + ch})
	// 左下弧（圆心 cx+r, cy+ch-r；90° → 180°）
	pts = append(pts, arcPts(cx+r, cy+ch-r, r, math.Pi/2, math.Pi)...)
	// 左边（→ 左上弧起点）
	pts = append(pts, graphics.Point{X: cx, Y: cy + r})
	// 左上弧（圆心 cx+r, cy+r；180° → 270°）
	pts = append(pts, arcPts(cx+r, cy+r, r, math.Pi, math.Pi*1.5)...)
	return pts
}

type svgCircle struct{ cx, cy, r float64 }

func (s *svgCircle) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	fill := ctx.fill
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, s.r, fill)
	}
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		if len(ctx.dashArray) > 0 {
			// stroke-dasharray on a circle (e.g. a progress ring): sample the
			// outline and draw dashes with a cumulative arc-length offset so
			// the dash pattern is continuous around the whole circle.
			const segs = 96
			per := 2 * math.Pi * s.r / segs
			off := 0.0
			for i := 0; i < segs; i++ {
				a1 := 2 * math.Pi * float64(i) / segs
				a2 := 2 * math.Pi * float64(i+1) / segs
				x1 := s.cx + s.r*math.Cos(a1)
				y1 := s.cy + s.r*math.Sin(a1)
				x2 := s.cx + s.r*math.Cos(a2)
				y2 := s.cy + s.r*math.Sin(a2)
				dashLine(canvas, x1, y1, x2, y2, ctx.strokeWidth, ctx.stroke, ctx.dashArray, off, ctx.lineCap)
				off += per
			}
		} else {
			canvas.StrokeCircle(s.cx, s.cy, s.r, ctx.strokeWidth, ctx.stroke)
		}
	}
}

type svgEllipse struct{ cx, cy, rx, ry float64 }
func (s *svgEllipse) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	r := (s.rx + s.ry) / 2
	if debugenv.Enabled("WB_SVG_DEBUG") {
		log.Printf("[svg] ellipse/circle: c=(%.1f,%.1f) r=%.1f fill=#%02x%02x%02x stroke=#%02x%02x%02x w=%.1f dash=%d",
			s.cx, s.cy, r, ctx.fill.R, ctx.fill.G, ctx.fill.B,
			ctx.stroke.R, ctx.stroke.G, ctx.stroke.B, ctx.strokeWidth, len(ctx.dashArray))
	}
	fill := ctx.fill
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, r, fill)
	}
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		if len(ctx.dashArray) > 0 {
			// stroke-dasharray on a circle (e.g. a progress ring): sample the
			// outline and draw dashes with a cumulative arc-length offset so
			// the dash pattern is continuous around the whole circle.
			const segs = 96
			per := 2 * math.Pi * r / segs
			off := 0.0
			for i := 0; i < segs; i++ {
				a1 := 2 * math.Pi * float64(i) / segs
				a2 := 2 * math.Pi * float64(i+1) / segs
				x1 := s.cx + r*math.Cos(a1)
				y1 := s.cy + r*math.Sin(a1)
				x2 := s.cx + r*math.Cos(a2)
				y2 := s.cy + r*math.Sin(a2)
				dashLine(canvas, x1, y1, x2, y2, ctx.strokeWidth, ctx.stroke, ctx.dashArray, off, ctx.lineCap)
				off += per
			}
		} else {
			canvas.StrokeCircle(s.cx, s.cy, r, ctx.strokeWidth, ctx.stroke)
		}
	}
}

type svgLine struct{ x1, y1, x2, y2 float64 }

func (s *svgLine) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if ctx.stroke.A > 0 && ctx.strokeWidth > 0 {
		dashLine(canvas, s.x1, s.y1, s.x2, s.y2, ctx.strokeWidth, ctx.stroke, ctx.dashArray, ctx.dashOffset, ctx.lineCap)
	}
}

type svgPolygon struct {
	points []graphics.Point
	closed bool
}

func (s *svgPolygon) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if debugenv.Enabled("WB_SVG_DEBUG") {
		log.Printf("[svg] polygon: pts=%d closed=%v fill=#%02x%02x%02x stroke=#%02x%02x%02x w=%.1f",
			len(s.points), s.closed, ctx.fill.R, ctx.fill.G, ctx.fill.B,
			ctx.stroke.R, ctx.stroke.G, ctx.stroke.B, ctx.strokeWidth)
	}
	fill := ctx.fill
	if fill.A > 0 && len(s.points) >= 3 {
		// Native path fill (handles concave shapes + fill-rule).
		canvas.FillPath(s.points, fill, ctx.fillRule == "evenodd")
	}
	// Stroke the outline (closed for polygon, open for polyline).
	if ctx.strokeWidth > 0 && len(s.points) >= 2 && (ctx.stroke.A > 0 || ctx.strokeGradient != nil) {
		if ctx.strokeGradient != nil {
			ax, ay, bx, by, colors, pos := gradientParamsForPts(ctx.strokeGradient, s.points)
			canvas.StrokePathGradient(s.points, ctx.strokeWidth, ax, ay, bx, by, colors, pos, ctx.lineCap, ctx.lineJoin)
		} else if len(ctx.dashArray) == 0 {
			canvas.StrokePath(s.points, ctx.strokeWidth, ctx.stroke, ctx.lineCap, ctx.lineJoin)
		} else {
			segs := len(s.points) - 1
			closeRing := s.closed
			if closeRing && len(s.points) >= 3 {
				segs = len(s.points)
			}
			for i := 0; i < segs; i++ {
				p1 := s.points[i]
				p2 := s.points[(i+1)%len(s.points)]
				dashLine(canvas, p1.X, p1.Y, p2.X, p2.Y, ctx.strokeWidth, ctx.stroke, ctx.dashArray, ctx.dashOffset, ctx.lineCap)
			}
		}
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

type svgPath struct {
	commands    []pathCmd
	markerStart string // url(#id) — drawn at the path start
	markerEnd   string // url(#id) — drawn at the path end
}

func (s *svgPath) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	if len(s.commands) == 0 {
		return
	}
	fill := ctx.fill
	pts := s.samplePoints()
	// Fill via a native Skia path so concave / self-intersecting paths (and
	// fill-rule="evenodd" stars etc.) rasterize like the browser — the old
	// triangle fan painted the wrong interior for concave shapes.
	if fill.A > 0 && len(pts) >= 3 {
		if debugenv.Enabled("WB_SVG_DEBUG") {
			log.Printf("[svg] path fill: pts=%d fill=#%02x%02x%02x rule=%s", len(pts), fill.R, fill.G, fill.B, ctx.fillRule)
		}
		canvas.FillPath(pts, fill, ctx.fillRule == "evenodd")
	}
	// Stroke. With a dash pattern we still walk segments (dashLine handles
	// the on/off phases); solid strokes use the native path stroke so
	// stroke-linecap / stroke-linejoin match the browser exactly. Gradient
	// strokes (stroke="url(#gradient)") use the shader-based path stroke.
	if ctx.strokeWidth > 0 && len(pts) >= 2 && (ctx.stroke.A > 0 || ctx.strokeGradient != nil) {
		if ctx.strokeGradient != nil {
			ax, ay, bx, by, colors, pos := gradientParamsForPts(ctx.strokeGradient, pts)
			canvas.StrokePathGradient(pts, ctx.strokeWidth, ax, ay, bx, by, colors, pos, ctx.lineCap, ctx.lineJoin)
		} else if len(ctx.dashArray) == 0 {
			canvas.StrokePath(pts, ctx.strokeWidth, ctx.stroke, ctx.lineCap, ctx.lineJoin)
		} else {
			for i := 0; i < len(pts)-1; i++ {
				dashLine(canvas, pts[i].X, pts[i].Y, pts[i+1].X, pts[i+1].Y, ctx.strokeWidth, ctx.stroke, ctx.dashArray, ctx.dashOffset, ctx.lineCap)
			}
		}
	} else if debugenv.Enabled("WB_SVG_DEBUG") && len(pts) >= 2 {
		log.Printf("[svg] path SKIPPED stroke: stroke.A=%d strokeWidth=%.1f pts=%d", ctx.stroke.A, ctx.strokeWidth, len(pts))
	}
	// Markers: paint the referenced <marker> templates at the path start and
	// end, rotated to the local path direction (orient=auto).
	if ctx.markers != nil {
		if s.markerStart != "" {
			if id := parseURLReference(s.markerStart); id != "" {
				if m, ok := ctx.markers[id]; ok && len(pts) >= 1 {
					dir := graphics.Point{X: pts[1].X - pts[0].X, Y: pts[1].Y - pts[0].Y}
					paintSVGMarker(canvas, ctx, m, pts[0], dir)
				}
			}
		}
		if s.markerEnd != "" {
			if id := parseURLReference(s.markerEnd); id != "" {
				if m, ok := ctx.markers[id]; ok && len(pts) >= 2 {
					last := pts[len(pts)-1]
					dir := graphics.Point{X: last.X - pts[len(pts)-2].X, Y: last.Y - pts[len(pts)-2].Y}
					paintSVGMarker(canvas, ctx, m, last, dir)
				}
			}
		}
	}
}

// samplePoints evaluates the path commands into a polyline point list,
// sampling beziers (C/S/Q/T) and arcs (A) exactly like paint used to inline,
// so gradient fill/stroke can reuse the same geometry.
func (s *svgPath) samplePoints() []graphics.Point {
	var pts []graphics.Point
	var firstPoint graphics.Point
	hasFirst := false
	curX, curY := 0.0, 0.0
	// lastCtrl / lastCtrlKind track the previous bezier control point so the
	// smooth variants S/s (cubic) and T/t (quadratic) can reflect it.
	var lastCtrl graphics.Point
	lastCtrlKind := byte(0)

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
		case 'H', 'h': // horizontal line (absolute / relative)
			if len(args) >= 1 {
				x := args[0]
				if cmd.kind == 'h' {
					x += curX
				}
				pts = append(pts, graphics.Point{X: x, Y: curY})
				curX = x
			}
		case 'V', 'v': // vertical line (absolute / relative)
			if len(args) >= 1 {
				y := args[0]
				if cmd.kind == 'v' {
					y += curY
				}
				pts = append(pts, graphics.Point{X: curX, Y: y})
				curY = y
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
				lastCtrl = graphics.Point{X: x2, Y: y2}
				lastCtrlKind = 'c'
			}
		case 'S', 's': // smooth cubic: x2 y2 x y (ctrl1 = reflection of prev ctrl2)
			if len(args) >= 4 {
				x1, y1 := curX, curY
				if lastCtrlKind == 'c' || lastCtrlKind == 's' {
					x1 = 2*curX - lastCtrl.X
					y1 = 2*curY - lastCtrl.Y
				}
				x2, y2, x, y := args[0], args[1], args[2], args[3]
				if cmd.kind == 's' {
					x2 += curX
					y2 += curY
					x += curX
					y += curY
				}
				pts = append(pts, sampleCubicBezier(curX, curY, x1, y1, x2, y2, x, y)...)
				curX, curY = x, y
				lastCtrl = graphics.Point{X: x2, Y: y2}
				lastCtrlKind = 's'
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
				lastCtrl = graphics.Point{X: x1, Y: y1}
				lastCtrlKind = 'q'
			}
		case 'T', 't': // smooth quadratic: x y (ctrl = reflection of prev q ctrl)
			if len(args) >= 2 {
				x1, y1 := curX, curY
				if lastCtrlKind == 'q' || lastCtrlKind == 't' {
					x1 = 2*curX - lastCtrl.X
					y1 = 2*curY - lastCtrl.Y
				}
				x, y := args[0], args[1]
				if cmd.kind == 't' {
					x += curX
					y += curY
				}
				pts = append(pts, sampleQuadraticBezier(curX, curY, x1, y1, x, y)...)
				curX, curY = x, y
				lastCtrl = graphics.Point{X: x1, Y: y1}
				lastCtrlKind = 't'
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
	return pts
}

// paintSVGMarker paints a <marker> template at a path vertex: translate to
// the vertex, rotate to the direction (orient=auto), then offset by refX/refY
// and paint the child shapes.
func paintSVGMarker(canvas *graphics.Canvas, ctx *svgPaintContext, m *svgMarker, at, dir graphics.Point) {
	canvas.Save()
	canvas.Translate(at.X, at.Y)
	if m.orientAuto {
		angle := math.Atan2(dir.Y, dir.X) * 180 / math.Pi
		canvas.Rotate(angle)
	}
	canvas.Translate(-m.refX, -m.refY)
	for _, sh := range m.shapes {
		sh.paint(canvas, ctx)
	}
	canvas.Restore()
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
	// Sample density adapts to the arc sweep: ~4° per segment (a full circle
	// gets ~90 points) so small gear arcs stay round when scaled down to
	// 14-18px icon sizes. 8° segments made short arcs visibly faceted and
	// the settings gear teeth look broken at 18px.
	segs := math.Ceil(math.Abs(dtheta) / (math.Pi / 45)) // 4° per segment
	if segs < 4 {
		segs = 4
	}
	steps := int(segs)
	out := make([]graphics.Point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
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
	x, y          float64
	fontFamily    string
	fontSize      float64
	textAnchor    string
	content       string
	fill          graphics.Color
	stroke        graphics.Color
	strokeWidth   float64
	rotate        float64       // rotate="45" degrees around the anchor point
	letterSpacing float64       // letter-spacing in px
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

	// Rotate the whole text block around the anchor point (SVG rotate attr).
	if s.rotate != 0 {
		canvas.Save()
		canvas.Translate(s.x, s.y)
		canvas.Rotate(s.rotate)
		canvas.Translate(-s.x, -s.y)
		defer canvas.Restore()
	}

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
		if s.letterSpacing != 0 {
			// Draw character by character with added spacing.
			cx := textX
			for _, ch := range line {
				chStr := string(ch)
				canvas.DrawText(cx, baseline, chStr, font, fill)
				if s.stroke.A > 0 && s.strokeWidth > 0 {
					canvas.DrawText(cx, baseline, chStr, font, s.stroke)
				}
				cx += graphics.MeasureText(font, chStr) + s.letterSpacing
			}
			continue
		}
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
	par         string // preserveAspectRatio 原始属性值（""=默认 xMidYMid meet）
	viewportW   float64 // 实际渲染视口宽（0=用 width）
	viewportH   float64 // 实际渲染视口高（0=用 height）
	elementByID map[string]*dom.Element // used by <use> references
	// gradients/clips carry defs contents to paint time (the paint context
	// is fresh per paintSVG call, so the defs parsed during build must be
	// stored here for shape gradient/clip resolution).
	gradients map[string]*svgGradient
	clips     map[string][]svgShape
	patterns  map[string]*svgPattern
	// styleRules carry <style> sheet rules (class/type selectors resolved to
	// fill/stroke) so shapes without inline presentation attributes can pick
	// up stylesheet styling, like real SVG.
	styleRules []svgStyleRule
	// markers carry <marker> templates referenced by marker-start/end.
	markers map[string]*svgMarker
	// masks carry <mask> templates referenced by CSS mask-image url(#id).
	masks map[string]*svgMask
	// currentColor is the resolved CSS color of the host element; SVG
	// fill/stroke="currentColor" resolves to it (set by the caller).
	currentColor graphics.Color
}

// svgMarker is a <marker> template: child shapes painted at a path vertex,
// translated by (refX,refY) and rotated to the path direction when orient=auto.
type svgMarker struct {
	shapes     []svgShape
	refX, refY float64
	orientAuto bool
}

// svgMask is a parsed <mask> element. Its child shapes' luminance (default) or
// alpha (mask-type: alpha) selects the mask value. x/y/width/height describe the
// mask region (default -10%,-10%,120%,120%); maskUnits selects the region's
// coordinate system (objectBoundingBox relative to the masked element, or
// userSpaceOnUse absolute); maskContentUnits selects the child shapes' system.
type svgMask struct {
	id               string
	x, y, w, h       float64 // mask region (fractions when maskUnits=objectBoundingBox)
	maskUnits        string  // objectBoundingBox (default) | userSpaceOnUse
	maskContentUnits string  // userSpaceOnUse (default) | objectBoundingBox
	maskType         string  // luminance (default) | alpha
	shapes           []svgShape
}

// parseMaskElement parses a <mask> element (from <defs>) into an svgMask.
// Extracted from buildSVGDocument so same-document mask-image: url(#id)
// references can reuse it directly against a mask element found by
// GetElementById (inline <svg><mask> in the same document).
func parseMaskElement(defEl *dom.Element) *svgMask {
	if defEl == nil {
		return nil
	}
	maskID := defEl.GetAttribute("id")
	if maskID == "" {
		return nil
	}
	mk := &svgMask{
		id:               maskID,
		x:                parseMaskFraction(defEl.GetAttribute("x"), -0.10),
		y:                parseMaskFraction(defEl.GetAttribute("y"), -0.10),
		w:                parseMaskFraction(defEl.GetAttribute("width"), 1.20),
		h:                parseMaskFraction(defEl.GetAttribute("height"), 1.20),
		maskUnits:        defEl.GetAttribute("maskUnits"),
		maskContentUnits: defEl.GetAttribute("maskContentUnits"),
		maskType:         defEl.GetAttribute("mask-type"),
	}
	for mc := defEl.FirstChild(); mc != nil; mc = mc.NextSibling() {
		if mEl, ok := mc.(*dom.Element); ok {
			if shape := parseSVGElement(mEl); shape != nil {
				fs := &svgFilledShape{shape: shape, fill: graphics.Color{R: 255, G: 255, B: 255, A: 255}}
				pm := parseStyleAttribute(mEl.GetAttribute("style"))
				if f := pm["fill"]; f != "" {
					fs.fill = parseColorAttribute(f)
				} else if f := mEl.GetAttribute("fill"); f != "" {
					fs.fill = parseColorAttribute(f)
				}
				if st := pm["stroke"]; st != "" {
					fs.stroke = parseColorAttribute(st)
				} else if st := mEl.GetAttribute("stroke"); st != "" {
					fs.stroke = parseColorAttribute(st)
				}
				fs.strokeWidth = parseSVGCoord(mEl.GetAttribute("stroke-width"))
				mk.shapes = append(mk.shapes, fs)
			}
		}
	}
	return mk
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

// parseMaskFraction parses an SVG mask region coordinate (x/y/width/height).
// A trailing "%" divides by 100 (so "10%" → 0.10, "120%" → 1.20); a bare number
// is the fraction/absolute value directly. Empty returns the default.
func parseMaskFraction(s string, def float64) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if strings.HasSuffix(s, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64); err == nil {
			return v / 100.0
		}
		return def
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return def
}

func parseSVGCoordList(s string) []float64 {
	var vals []float64
	for _, p := range strings.Fields(s) {
		vals = append(vals, parseSVGCoord(p))
	}
	return vals
}

func parseSVGPoints(s string) []graphics.Point {
	// Points may be separated by whitespace AND/OR commas ("0,0 24,0 12,24"
	// or "0,0,24,0,12,24"). Tokenize every numeric literal instead of
	// splitting on whitespace only — ParseFloat("0,0") fails and collapses
	// the whole polygon to a single (0,0) point.
	var nums []float64
	re := regexp.MustCompile(`-?\d*\.?\d+(?:[eE][-+]?\d+)?`)
	for _, m := range re.FindAllString(s, -1) {
		if v, err := strconv.ParseFloat(m, 64); err == nil {
			nums = append(nums, v)
		}
	}
	var pts []graphics.Point
	for i := 0; i+1 < len(nums); i += 2 {
		pts = append(pts, graphics.Point{X: nums[i], Y: nums[i+1]})
	}
	return pts
}

// --- Path parsing ---

// tokenizeSVGPath splits an SVG path "d" into command letters and numeric
// tokens, following the SVG 1.1 path grammar. Numbers allow IMPLICIT
// separators when the next token starts with '-'/'+'/'.' right after a
// complete number — e.g. `-1.82.33` is `-1.82` + `.33` (leading-dot
// fraction), `l-8-3-8 3` is four numbers, and `1e-5` is a single exponent
// number. A naive split on whitespace/comma/minus breaks the gear teeth
// arcs (`a1.65 1.65 0 0 0-1.82.33` → arc dropped) and scientific notation.
func tokenizeSVGPath(s string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == ',' || c == '\t' || c == '\n' || c == '\r':
			flush()
		case isDigit(c):
			cur.WriteByte(c)
		case c == '.':
			// A second '.' starts a new number (`1.82.33` → `1.82`, `.33`).
			if strings.Contains(cur.String(), ".") {
				flush()
			}
			cur.WriteByte(c)
		case c == '+' || c == '-':
			// After e/E this is the exponent sign; otherwise a new number.
			cs := cur.String()
			if cs != "" && (cs[len(cs)-1] == 'e' || cs[len(cs)-1] == 'E') {
				cur.WriteByte(c)
			} else {
				flush()
				cur.WriteByte(c)
			}
		case c == 'e' || c == 'E':
			// Exponent only when the current number is followed by a digit
			// (or a sign then a digit); otherwise it is a command letter.
			if cur.Len() > 0 {
				var next, afterNext byte
				if i+1 < len(s) {
					next = s[i+1]
				}
				if i+2 < len(s) {
					afterNext = s[i+2]
				}
				if isDigit(next) || ((next == '+' || next == '-') && isDigit(afterNext)) {
					cur.WriteByte(c)
				} else {
					flush()
					tokens = append(tokens, string(c))
				}
			} else {
				tokens = append(tokens, string(c))
			}
		default: // any other letter is a command
			flush()
			tokens = append(tokens, string(c))
		}
	}
	flush()
	return tokens
}

// parseSVGPathData parses an SVG path "d" attribute into path commands. Per
// the SVG spec, consecutive parameter groups after a command are IMPLICIT
// repetitions of the same command — e.g. "l-8-3-8 3" is "l -8 -3 l -8 3".
// Many feather icons rely on this (settings gear arcs, shield outline).
// Each parameter group becomes its own pathCmd so painters can process one
// group per command.
func parseSVGPathData(s string) []pathCmd {
	var cmds []pathCmd
	tokens := tokenizeSVGPath(s)
	i := 0
	lastCmd := byte('M')
	// argsPerCmd maps a command letter to its parameter-group size.
	argsPerCmd := func(c byte) int {
		switch c {
		case 'M', 'm', 'L', 'l', 'T', 't':
			return 2
		case 'H', 'h', 'V', 'v':
			return 1
		case 'C', 'c':
			return 6
		case 'S', 's', 'Q', 'q':
			return 4
		case 'A', 'a':
			return 7
		}
		return 0
	}
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
		// Split the argument list into per-command groups. A trailing
		// incomplete group is dropped (lenient, matches browsers).
		per := argsPerCmd(cmd)
		if per > 0 {
			for start := 0; start+per <= len(args); start += per {
				kind := cmd
				// After an initial M/m, subsequent groups act as L/l.
				if start > 0 && (cmd == 'M' || cmd == 'm') {
					if cmd == 'M' {
						kind = 'L'
					} else {
						kind = 'l'
					}
				}
				cmds = append(cmds, pathCmd{kind: kind, args: args[start : start+per]})
			}
		} else if cmd == 'Z' || cmd == 'z' {
			cmds = append(cmds, pathCmd{kind: cmd})
		}
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

// resolveViewBoxTransform 按 preserveAspectRatio 计算 viewBox→viewport 变换。
// 返回 (sx, sy, dx, dy)，即 Scale(sx,sy) + Translate(dx,dy)（相对 viewport 原点）。
// 支持：none（独立拉伸填满）、xMin/xMid/xMax × yMin/yMid/yMax 对齐、
// meet（默认，等比缩放完整显示）/slice（等比缩放填满裁剪）。
func resolveViewBoxTransform(doc *svgDocument, vw, vh float64) (sx, sy, dx, dy float64) {
	vb := doc.viewBox
	vbW, vbH := vb[2], vb[3]
	if vbW <= 0 || vbH <= 0 || vw <= 0 || vh <= 0 {
		return 1, 1, 0, 0
	}
	align := "xMidYMid"
	slice := false
	if doc.par != "" {
		for _, f := range strings.Fields(doc.par) {
			switch f {
			case "defer":
				continue
			case "none":
				align = "none"
			case "meet":
				slice = false
			case "slice":
				slice = true
			default:
				if strings.HasPrefix(f, "x") || strings.Contains(f, "Y") {
					align = f
				}
			}
		}
	}
	if align == "none" {
		return vw / vbW, vh / vbH, 0, 0
	}
	ax := 0.5 // x 对齐：xMin=0 xMid=0.5 xMax=1
	switch {
	case strings.HasPrefix(align, "xMin"):
		ax = 0
	case strings.HasPrefix(align, "xMax"):
		ax = 1
	}
	ay := 0.5 // y 对齐：yMin=0 yMid=0.5 yMax=1
	switch {
	case strings.HasSuffix(align, "YMin"):
		ay = 0
	case strings.HasSuffix(align, "YMax"):
		ay = 1
	}
	scale := math.Min(vw/vbW, vh/vbH)
	if slice {
		scale = math.Max(vw/vbW, vh/vbH)
	}
	return scale, scale, (vw - vbW*scale) * ax, (vh - vbH*scale) * ay
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
		return &svgPath{
			commands:    parseSVGPathData(el.GetAttribute("d")),
			markerStart: el.GetAttribute("marker-start"),
			markerEnd:   el.GetAttribute("marker-end"),
		}
	case "text":
		content := el.TextContent()
		return &svgText{
			x:            parseSVGCoord(el.GetAttribute("x")),
			y:            parseSVGCoord(el.GetAttribute("y")),
			fontFamily:   getAttr("font-family"),
			fontSize:     parseSVGCoord(getAttr("font-size")),
			textAnchor:   el.GetAttribute("text-anchor"),
			content:      content,
			fill:         parseColorAttribute(getAttr("fill")),
			stroke:       parseColorAttribute(getAttr("stroke")),
			strokeWidth:  parseSVGCoord(getAttr("stroke-width")),
			rotate:       parseSVGCoord(el.GetAttribute("rotate")),
			letterSpacing: parseSVGCoord(getAttr("letter-spacing")),
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
	if strings.EqualFold(el.GetAttribute("gradientUnits"), "userSpaceOnUse") {
		g.userSpaceOnUse = true
	}
	if tag == "radialgradient" {
		g.isRadial = true
		g.cx = parseSVGCoord(el.GetAttribute("cx"))
		g.cy = parseSVGCoord(el.GetAttribute("cy"))
		g.r = parseSVGCoord(el.GetAttribute("r"))
		if g.r <= 0 {
			g.r = 50 // default radius
		}
	} else {
		if g.userSpaceOnUse {
			// Absolute user-space coordinates.
			g.x1 = parseSVGCoord(el.GetAttribute("x1"))
			g.y1 = parseSVGCoord(el.GetAttribute("y1"))
			g.x2 = parseSVGCoord(el.GetAttribute("x2"))
			g.y2 = parseSVGCoord(el.GetAttribute("y2"))
		} else {
			// objectBoundingBox: x1/y1/x2/y2 are fractions of the shape bbox;
			// both "100%" and bare "1" mean 100%.
			g.x1 = parseGradFraction(el.GetAttribute("x1"))
			g.y1 = parseGradFraction(el.GetAttribute("y1"))
			g.x2 = parseGradFraction(el.GetAttribute("x2"))
			g.y2 = parseGradFraction(el.GetAttribute("y2"))
		}
		if g.x2 == 0 && g.y2 == 0 {
			g.x2 = 1 // default horizontal gradient
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

// parseGradFraction parses a gradient axis coordinate in objectBoundingBox
// mode: percentages ("100%") and bare numbers ("1") both mean fractions of
// the shape bounding box (1 == 100%).
func parseGradFraction(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		return v / 100
	}
	return parseSVGCoord(s)
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
		if !g.userSpaceOnUse && cx == 0 && cy == 0 && r == 50 {
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
	// default horizontal direction; arbitrary axes are sampled per-pixel by
	// projecting each point onto the gradient axis (SVG objectBoundingBox:
	// x1/y1/x2/y2 are percentages of the shape bbox).
	x1, y1, x2, y2 := g.x1, g.y1, g.x2, g.y2
	if g.userSpaceOnUse {
		// Coordinates are absolute user-space units: project directly.
		ax, ay := x1, y1
		bx, by := x2, y2
		vx, vy := bx-ax, by-ay
		den := vx*vx + vy*vy
		if den == 0 {
			canvas.FillRect(x, y, w, h, g.stops[len(g.stops)-1].color)
			return
		}
		stops := make([]ColorStop, len(g.stops))
		for i, s := range g.stops {
			stops[i] = ColorStop{Color: s.color, Position: s.offset}
		}
		if w*h > 40000 {
			canvas.FillLinearGradient(x, y, w, h, g.stops[0].color, g.stops[len(g.stops)-1].color)
			return
		}
		for py := int(y); py < int(y+h); py++ {
			for px := int(x); px < int(x+w); px++ {
				pxc := float64(px) + 0.5
				pyc := float64(py) + 0.5
				t := ((pxc-ax)*vx + (pyc-ay)*vy) / den
				canvas.FillRect(float64(px), float64(py), 1, 1, interpolateColor(stops, t))
			}
		}
		return
	}
	if x1 == 0 && y1 == 0 && x2 == 1 && y2 == 0 {
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
	// Arbitrary axis: project each pixel onto the (x1,y1)→(x2,y2) axis.
	ax := x + x1*w
	ay := y + y1*h
	bx := x + x2*w
	by := y + y2*h
	vx, vy := bx-ax, by-ay
	den := vx*vx + vy*vy
	if den == 0 {
		// Degenerate axis: gradient collapses, fill with the last stop.
		canvas.FillRect(x, y, w, h, g.stops[len(g.stops)-1].color)
		return
	}
	stops := make([]ColorStop, len(g.stops))
	for i, s := range g.stops {
		stops[i] = ColorStop{Color: s.color, Position: s.offset}
	}
	// Keep per-pixel cost bounded: fall back to the canvas gradient for
	// very large boxes.
	if w*h > 40000 {
		canvas.FillLinearGradient(x, y, w, h, g.stops[0].color, g.stops[len(g.stops)-1].color)
		return
	}
	for py := int(y); py < int(y+h); py++ {
		for px := int(x); px < int(x+w); px++ {
			pxc := float64(px) + 0.5
			pyc := float64(py) + 0.5
			t := ((pxc-ax)*vx + (pyc-ay)*vy) / den
			canvas.FillRect(float64(px), float64(py), 1, 1, interpolateColor(stops, t))
		}
	}
}

// shapeBBox returns the bounding box of a basic shape (used by pattern
// tiling); zero w/h for shapes without a box.
func shapeBBox(s svgShape) (x, y, w, h float64) {
	switch sh := s.(type) {
	case *svgRect:
		return sh.x, sh.y, sh.w, sh.h
	case *svgCircle:
		return sh.cx - sh.r, sh.cy - sh.r, sh.r * 2, sh.r * 2
	case *svgEllipse:
		return sh.cx - sh.rx, sh.cy - sh.ry, sh.rx * 2, sh.ry * 2
	}
	return 0, 0, 0, 0
}

// dashLine strokes the segment (x1,y1)-(x2,y2) with a dash pattern.
// dashes alternates on/off lengths; offset shifts the pattern (common
// values like "5 3" or "4,4" are supported; offset usually absent).
func dashLine(canvas *graphics.Canvas, x1, y1, x2, y2, width float64, col graphics.Color, dashes []float64, offset float64, cap string) {
	// roundCap paints a filled circle (diameter = stroke width) at a point so
	// the line end is rounded — mirrors SVG stroke-linecap:round.
	roundCap := func(px, py float64) {
		if cap == "round" {
			canvas.FillCircle(px, py, width/2, col)
		}
	}
	if len(dashes) == 0 {
		canvas.StrokeLine(x1, y1, x2, y2, width, col)
		if cap == "round" {
			roundCap(x1, y1)
			roundCap(x2, y2)
		}
		return
	}
	dx, dy := x2-x1, y2-y1
	L := math.Hypot(dx, dy)
	if L == 0 {
		return
	}
	ux, uy := dx/L, dy/L
	total := totalDashes(dashes)
	// phase is the offset within the current on/off pattern element at the
	// start of THIS line segment (a short arc sample). We walk the line
	// position pos∈[0,L] in lockstep with the dash pattern, painting the
	// on-intervals. This correctly handles segments whose start phase lies
	// beyond the segment length (previously such segments were skipped even
	// when the phase was inside an on-interval, breaking progress rings).
	phase := math.Mod(offset, total)
	if phase < 0 {
		phase += total
	}
	i := 0
	// Advance to the pattern element containing `phase`.
	var acc float64
	for i < len(dashes) {
		if acc+dashes[i] > phase {
			phase -= acc
			break
		}
		acc += dashes[i]
		i++
	}
	if i >= len(dashes) {
		i = len(dashes) - 1
		phase = 0
	}
	pos := 0.0
	for pos < L {
		seg := dashes[i%len(dashes)]
		if seg <= 0 {
			seg = 0.001
		}
		remain := seg - phase
		if remain <= 0 {
			// Phase sits exactly at the pattern boundary: move to the next
			// element (toggling on/off).
			phase = 0
			i++
			continue
		}
		lineRemain := L - pos
		end := remain
		if end > lineRemain {
			end = lineRemain
		}
		if i%2 == 0 { // on-interval
			canvas.StrokeLine(x1+ux*pos, y1+uy*pos, x1+ux*(pos+end), y1+uy*(pos+end), width, col)
			if cap == "round" {
				roundCap(x1+ux*pos, y1+uy*pos)
				roundCap(x1+ux*(pos+end), y1+uy*(pos+end))
			}
		}
		pos += end
		phase += end
		if phase >= seg {
			phase = 0
			i++
		}
	}
}

func totalDashes(dashes []float64) float64 {
	var t float64
	for _, d := range dashes {
		t += d
	}
	if t == 0 {
		return 1
	}
	return t
}

// parseDashArray parses "5,3 2" / "5 3" / "4 4" into an alternating
// on/off pattern.
func parseDashArray(s string) []float64 {
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil
	}
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		if v, err := strconv.ParseFloat(f, 64); err == nil && v > 0 {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	// Odd count: the last entry is duplicated (SVG semantics).
	if len(out)%2 == 1 {
		out = append(out, out[len(out)-1])
	}
	return out
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

func buildSVGDocument(el *dom.Element, currentColors ...graphics.Color) *svgDocument {
	doc := &svgDocument{
		width:       parseSVGCoord(el.GetAttribute("width")),
		height:      parseSVGCoord(el.GetAttribute("height")),
		elementByID: make(map[string]*dom.Element),
		// currentColor default: black (SVG spec). The host element's CSS
		// color is passed in when the caller has style context — it MUST be
		// available BEFORE walk parses fill/stroke="currentColor", otherwise
		// every currentColor stroke resolves to transparent (A=0).
		currentColor: graphics.Color{R: 0, G: 0, B: 0, A: 0xFF},
	}
	if len(currentColors) > 0 {
		doc.currentColor = currentColors[0]
	}
	if vb := el.GetAttribute("viewBox"); vb != "" {
		doc.viewBox = parseViewBox(vb)
		doc.hasVB = true
	}
	doc.par = el.GetAttribute("preserveAspectRatio")
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
					case "pattern":
						patID := defEl.GetAttribute("id")
						if patID != "" {
							pat := &svgPattern{
								w: parseSVGCoord(defEl.GetAttribute("width")),
								h: parseSVGCoord(defEl.GetAttribute("height")),
							}
							for pc := defEl.FirstChild(); pc != nil; pc = pc.NextSibling() {
								if pEl, ok := pc.(*dom.Element); ok {
									if shape := parseSVGElement(pEl); shape != nil {
										// Wrap with fill/stroke from attributes so
										// pattern shapes paint colored.
										fs := &svgFilledShape{shape: shape, fill: graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}}
										pm := parseStyleAttribute(pEl.GetAttribute("style"))
										if f, ok2 := pm["fill"]; ok2 {
											fs.fill = parseColorAttribute(f)
										} else if f := pEl.GetAttribute("fill"); f != "" {
											fs.fill = parseColorAttribute(f)
										}
										if sw := pm["stroke-width"]; sw != "" {
											fs.strokeWidth = parseSVGCoord(sw)
										} else {
											fs.strokeWidth = parseSVGCoord(pEl.GetAttribute("stroke-width"))
										}
										if st := pm["stroke"]; st != "" {
											fs.stroke = parseColorAttribute(st)
										} else if st := pEl.GetAttribute("stroke"); st != "" {
											fs.stroke = parseColorAttribute(st)
										}
										if d := pEl.GetAttribute("stroke-dasharray"); d != "" {
											fs.dashArray = parseDashArray(d)
										}
										pat.shapes = append(pat.shapes, fs)
									}
								}
							}
							ctx.patterns[patID] = pat
						}
					case "marker":
						mid := defEl.GetAttribute("id")
						if mid != "" {
							m := &svgMarker{
								refX: parseSVGCoord(defEl.GetAttribute("refX")),
								refY: parseSVGCoord(defEl.GetAttribute("refY")),
							}
							if strings.EqualFold(defEl.GetAttribute("orient"), "auto") {
								m.orientAuto = true
							}
							for mc := defEl.FirstChild(); mc != nil; mc = mc.NextSibling() {
								if mEl, ok := mc.(*dom.Element); ok {
									if shape := parseSVGElement(mEl); shape != nil {
										fs := &svgFilledShape{shape: shape, fill: graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}}
										if f := mEl.GetAttribute("fill"); f != "" {
											fs.fill = parseColorAttribute(f)
										} else if pm := parseStyleAttribute(mEl.GetAttribute("style")); pm["fill"] != "" {
											fs.fill = parseColorAttribute(pm["fill"])
										}
										m.shapes = append(m.shapes, fs)
									}
								}
							}
							ctx.markers[mid] = m
						}
					case "mask":
						if mk := parseMaskElement(defEl); mk != nil {
							ctx.masks[mk.id] = mk
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
			patterns:    ctx.patterns,
			markers:     ctx.markers,
			masks:       ctx.masks,
		}

		// Resolve url(#gradientId) references
		if gradientID := parseURLReference(fillStr); gradientID != "" {
			if g, ok := ctx.gradients[gradientID]; ok {
				// Gradient fill — fallback to black, use gradient colors
				elCtx.fill = graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
				_ = g // gradient applied in shape-specific code
			}
		} else if fillStr == "currentColor" {
			elCtx.fill = doc.currentColor
		} else if fillStr != "" {
			elCtx.fill = parseColorAttribute(fillStr)
		}

		if strokeGradID := parseURLReference(strokeStr); strokeGradID != "" {
			// Gradient stroke: resolved at paint time via the gradient map;
			// keep stroke transparent so the shader path draws it.
			elCtx.stroke = graphics.Color{}
		} else if strokeStr == "currentColor" {
			elCtx.stroke = doc.currentColor
		} else if strokeStr != "" {
			elCtx.stroke = parseColorAttribute(strokeStr)
		}
		if swStr != "" {
			elCtx.strokeWidth = parseSVGCoord(swStr)
		}

		// stroke-linecap / stroke-linejoin (attribute or style="").
		attrOrStyle := func(name string) string {
			if v, ok := styleMap[name]; ok {
				return v
			}
			return childEl.GetAttribute(name)
		}
		elCtx.lineCap = attrOrStyle("stroke-linecap")
		elCtx.lineJoin = attrOrStyle("stroke-linejoin")
		elCtx.fillRule = attrOrStyle("fill-rule")
		// opacity / fill-opacity / stroke-opacity multiply the alpha.
		if opStr := attrOrStyle("opacity"); opStr != "" {
			if op, err := strconv.ParseFloat(opStr, 64); err == nil {
				elCtx.opacity = math.Max(0, math.Min(1, op))
			}
		}
		if fo := attrOrStyle("fill-opacity"); fo != "" {
			if op, err := strconv.ParseFloat(fo, 64); err == nil {
				elCtx.fill.A = uint8(float64(elCtx.fill.A) * math.Max(0, math.Min(1, op)))
			}
		}
		if so := attrOrStyle("stroke-opacity"); so != "" {
			if op, err := strconv.ParseFloat(so, 64); err == nil {
				elCtx.stroke.A = uint8(float64(elCtx.stroke.A) * math.Max(0, math.Min(1, op)))
			}
		}
		if dos := attrOrStyle("stroke-dashoffset"); dos != "" {
			if v, err := strconv.ParseFloat(dos, 64); err == nil {
				elCtx.dashOffset = v
			}
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

		// <symbol> is a template: never rendered directly, only via <use>.
		if tag == "symbol" {
			return
		}

		// <marker> is a template: never rendered directly; its child shapes
		// are painted at path vertices referenced by marker-start/end.
		if tag == "marker" {
			m := &svgMarker{
				refX: parseSVGCoord(childEl.GetAttribute("refX")),
				refY: parseSVGCoord(childEl.GetAttribute("refY")),
			}
			if strings.EqualFold(childEl.GetAttribute("orient"), "auto") {
				m.orientAuto = true
			}
			for c := childEl.FirstChild(); c != nil; c = c.NextSibling() {
				if subEl, ok := c.(*dom.Element); ok {
					if shape := parseSVGElement(subEl); shape != nil {
						m.shapes = append(m.shapes, shape)
					}
				}
			}
			if id := childEl.GetAttribute("id"); id != "" {
				if doc.markers == nil {
					doc.markers = make(map[string]*svgMarker)
				}
				doc.markers[id] = m
			}
			return
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
				// <symbol> is a template container: render each of its child
				// shapes, applying the <use> x/y offset to the whole set.
				if strings.ToLower(refEl.LocalName()) == "symbol" {
					dx := parseSVGCoord(childEl.GetAttribute("x"))
					dy := parseSVGCoord(childEl.GetAttribute("y"))
					for c := refEl.FirstChild(); c != nil; c = c.NextSibling() {
						if subEl, ok := c.(*dom.Element); ok {
							shape := parseSVGElement(subEl)
							if shape == nil {
								continue
							}
							if f := subEl.GetAttribute("fill"); f != "" {
								if col := parseColorAttribute(f); col.A > 0 {
									shape = &svgFilledShape{shape: shape, fill: col}
								}
							}
							if dx != 0 || dy != 0 {
								shape = &svgTranslatedShape{shape: shape, dx: dx, dy: dy}
							}
							doc.shapes = append(doc.shapes, shape)
						}
					}
					return
				}
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
				lineCap:     elCtx.lineCap,
				lineJoin:    elCtx.lineJoin,
				opacity:     elCtx.opacity,
				fillRule:    elCtx.fillRule,
				dashOffset:  elCtx.dashOffset,
			}
			if gradientID := parseURLReference(fillStr); gradientID != "" {
				// Same url() reference may resolve to a gradient OR a pattern.
				if _, ok := ctx.gradients[gradientID]; ok {
					wrapper.gradientID = gradientID
				} else if _, ok := ctx.patterns[gradientID]; ok {
					wrapper.patternID = gradientID
				}
			}
			if strokeGradID := parseURLReference(strokeStr); strokeGradID != "" {
				if _, ok := ctx.gradients[strokeGradID]; ok {
					wrapper.strokeGradientID = strokeGradID
				}
			}
			if clipID := parseURLReference(clipStr); clipID != "" {
				wrapper.clipID = clipID
			}
			wrapper.transform = childEl.GetAttribute("transform")
			if dashStr := childEl.GetAttribute("stroke-dasharray"); dashStr != "" {
				wrapper.dashArray = parseDashArray(dashStr)
			}
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
	doc.patterns = ctx.patterns
	doc.markers = ctx.markers
	doc.masks = ctx.masks
	return doc
}

// --- Painting ---

func paintSVG(canvas *graphics.Canvas, doc *svgDocument, x, y float64, defaultFill graphics.Color) {
	if doc == nil {
		return
	}
	// 兼容旧调用（无显式 viewport）：用 doc 上的 viewportW/H（若已设置），
	// 否则退回固有 width/height。生产代码一律走 paintSVGTo 显式传 viewport。
	paintSVGTo(canvas, doc, x, y, doc.viewportW, doc.viewportH, defaultFill)
}

// paintSVGTo paints the SVG document with an explicit viewport — the actual
// rendering size of the destination box. viewBox + preserveAspectRatio resolve
// against (vw,vh); without a viewBox the shapes are drawn at intrinsic
// coordinates, scaled to the viewport when the SVG declares an intrinsic size.
//
// ★ 这是唯一渲染路径：内联 <svg> 元素、<img src="*.svg">、
// background-image: url(data:image/svg+xml) 都必须调用本函数并把目标矩形
// 尺寸作为 viewport 传入，三条路径行为才一致。viewport 不得写成共享的
// svgDocument 字段（svgBackgroundCache 缓存复用的 doc 会被交叉污染，导致
// 图形不居中/超出边界）。
func paintSVGTo(canvas *graphics.Canvas, doc *svgDocument, x, y, vw, vh float64, defaultFill graphics.Color) {
	if doc == nil || len(doc.shapes) == 0 {
		return
	}
	if vw <= 0 || vh <= 0 {
		vw, vh = doc.width, doc.height
	}
	if doc.hasVB && vw > 0 && vh > 0 {
		vb := doc.viewBox
		if vb[2] > 0 && vb[3] > 0 {
			sx, sy, dx, dy := resolveViewBoxTransform(doc, vw, vh)
			canvas.Save()
			canvas.Translate(x, y)
			canvas.Translate(dx, dy)
			canvas.Scale(sx, sy)
			canvas.Translate(-vb[0], -vb[1])
			defer canvas.Restore()

			ctx := defaultSVGContext()
			ctx.fill = defaultFill
			ctx.gradients = doc.gradients
			ctx.clips = doc.clips
			ctx.markers = doc.markers
			for _, s := range doc.shapes {
				s.paint(canvas, ctx)
			}
			return
		}
	}

	// No viewBox: translate to (x,y); scale intrinsic coordinates to the
	// viewport when the SVG declares a size (matches <img> scaling semantics).
	if x != 0 || y != 0 || (doc.width > 0 && vw > 0 && vw != doc.width) ||
		(doc.height > 0 && vh > 0 && vh != doc.height) {
		canvas.Save()
		canvas.Translate(x, y)
		if doc.width > 0 && doc.height > 0 && vw > 0 && vh > 0 {
			canvas.Scale(vw/doc.width, vh/doc.height)
		}
		defer canvas.Restore()
	}

	ctx := defaultSVGContext()
	ctx.fill = defaultFill
	ctx.gradients = doc.gradients
	ctx.clips = doc.clips
	ctx.patterns = doc.patterns
	ctx.markers = doc.markers
	for _, s := range doc.shapes {
		s.paint(canvas, ctx)
	}
}

// renderSVGMask rasterizes a <mask> element's content into a decoded image.
// The image's RGB carries the child shapes' own colors and its alpha carries
// their coverage, so the caller extracts the mask value as luminance (default)
// or alpha per mask-type. The mask region is resolved against the masked
// element's box (targetW×targetH) per maskUnits; child shapes are drawn per
// maskContentUnits (userSpaceOnUse = target-element coordinates, or
// objectBoundingBox = 0..1 fractions scaled to the target box).
func renderSVGMask(m *svgMask, targetW, targetH float64) *DecodedImage {
	if m == nil || len(m.shapes) == 0 || targetW <= 0 || targetH <= 0 {
		return nil
	}
	// mask region in target-element coordinates.
	var mx, my, mw, mh float64
	if m.maskUnits == "userSpaceOnUse" {
		mx, my, mw, mh = m.x, m.y, m.w, m.h
	} else { // objectBoundingBox (default)
		mx, my = m.x*targetW, m.y*targetH
		mw, mh = m.w*targetW, m.h*targetH
	}
	if mw <= 0 || mh <= 0 {
		return nil
	}
	cw, ch := int(math.Ceil(mw)), int(math.Ceil(mh))
	if cw <= 0 || ch <= 0 {
		return nil
	}
	canvas := graphics.NewCanvas(cw, ch)
	defer canvas.Release()

	// Shift the mask region origin to the canvas origin, then (for
	// objectBoundingBox content units) scale child coords 0..1 to the target
	// box. Canvas transforms pre-concat, so this yields T·S (scale first).
	canvas.Translate(-mx, -my)
	if m.maskContentUnits == "objectBoundingBox" {
		canvas.Scale(targetW, targetH)
	}

	ctx := defaultSVGContext()
	ctx.fill = graphics.Color{R: 255, G: 255, B: 255, A: 255}
	for _, s := range m.shapes {
		s.paint(canvas, ctx)
	}

	img := canvas.Snapshot()
	if img == nil {
		return nil
	}
	return &DecodedImage{
		skImg:  img,
		loaded: true,
		width:  img.Width(),
		height: img.Height(),
	}
}
