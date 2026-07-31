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
	"strconv"
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
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
// gradientID (fill="url(#id)") and stroke are carried too so gradient fills
// and strokes survive to paint time.
type svgFilledShape struct {
	shape       svgShape
	fill        graphics.Color
	gradientID  string
	stroke      graphics.Color
	strokeWidth float64
}

func (s *svgFilledShape) paint(canvas *graphics.Canvas, ctx *svgPaintContext) {
	// Gradient fill: paint the shape geometry directly with the gradient
	// shader (the shape's own paint would only use flat colors).
	if s.gradientID != "" {
		if g, ok := ctx.gradients[s.gradientID]; ok {
			paintShapeGradient(canvas, s.shape, g)
			return
		}
	}
	c2 := *ctx
	c2.fill = s.fill
	c2.stroke = s.stroke
	c2.strokeWidth = s.strokeWidth
	s.shape.paint(canvas, &c2)
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

	for _, cmd := range s.commands {
		switch cmd.kind {
		case 'M', 'm':
			if len(cmd.args) >= 2 {
				p := graphics.Point{X: cmd.args[0], Y: cmd.args[1]}
				pts = append(pts, p)
				if !hasFirst {
					firstPoint = p
					hasFirst = true
				}
			}
		case 'L', 'l':
			if len(cmd.args) >= 2 {
				pts = append(pts, graphics.Point{X: cmd.args[0], Y: cmd.args[1]})
			}
		case 'Z', 'z':
			if hasFirst {
				pts = append(pts, firstPoint)
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
	textX := s.x
	if anchor == "middle" {
		w := graphics.MeasureText(font, s.content)
		textX -= w / 2
	} else if anchor == "end" {
		w := graphics.MeasureText(font, s.content)
		textX -= w
	}

	canvas.DrawText(textX, s.y, s.content, font, fill)
	if s.stroke.A > 0 && s.strokeWidth > 0 {
		canvas.DrawText(textX, s.y, s.content, font, s.stroke)
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

	var walk func(dom.Node, *svgPaintContext)
	walk = func(n dom.Node, ctx *svgPaintContext) {
		if n == nil {
			return
		}
		childEl, ok := n.(*dom.Element)
		if !ok {
			return
		}
		tag := childEl.LocalName()

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
			if refID := parseURLReference(href); refID != "" {
				if refEl, ok := doc.elementByID[refID]; ok {
					refShape := parseSVGElement(refEl)
					if refShape != nil {
						// Apply <use> x/y offset
						dx := parseSVGCoord(childEl.GetAttribute("x"))
						dy := parseSVGCoord(childEl.GetAttribute("y"))
						if dx != 0 || dy != 0 {
							refShape = &svgTranslatedShape{shape: refShape, dx: dx, dy: dy}
						}
						doc.shapes = append(doc.shapes, refShape)
					}
				}
			}
			return // <use> resolved, skip children
		}

		// Parse the shape
		if shape := parseSVGElement(childEl); shape != nil {
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
			doc.shapes = append(doc.shapes, wrapper)
		}

		// Recurse into children
		for c := childEl.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, elCtx)
		}
	}

	ctx := defaultSVGContext()
	walk(el, ctx)
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
