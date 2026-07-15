// Minimal SVG shape parser and painter.
//
// Parses <svg> child elements and paints basic SVG shapes (rect, circle,
// ellipse, line, polyline, polygon, path) to a Skia canvas.
//
// Completeness: 30%
// Missing: text, gradients, clipping, masks, filters, <use>, <defs>,
// viewBox, nested <svg>, CSS styling on SVG elements.

package rendering

import (
	"strconv"
	"strings"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

type svgShape interface {
	paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64)
}

type svgRect struct {
	x, y, w, h, rx, ry float64
}

func (s *svgRect) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
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
	if stroke.A > 0 && sw > 0 {
		if s.rx > 0 || s.ry > 0 {
			r := s.rx
			if r == 0 {
				r = s.ry
			}
			canvas.StrokeRoundRect(s.x, s.y, s.w, s.h, r, sw, stroke)
		} else {
			canvas.StrokeRect(s.x, s.y, s.w, s.h, sw, stroke)
		}
	}
}

type svgCircle struct{ cx, cy, r float64 }

func (s *svgCircle) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, s.r, fill)
	}
	if stroke.A > 0 && sw > 0 {
		canvas.StrokeCircle(s.cx, s.cy, s.r, sw, stroke)
	}
}

type svgEllipse struct{ cx, cy, rx, ry float64 }

func (s *svgEllipse) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
	r := (s.rx + s.ry) / 2
	if fill.A > 0 {
		canvas.FillCircle(s.cx, s.cy, r, fill)
	}
	if stroke.A > 0 && sw > 0 {
		canvas.StrokeCircle(s.cx, s.cy, r, sw, stroke)
	}
}

type svgLine struct{ x1, y1, x2, y2 float64 }

func (s *svgLine) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
	if stroke.A > 0 && sw > 0 {
		canvas.StrokeLine(s.x1, s.y1, s.x2, s.y2, sw, stroke)
	}
}

type svgPolygon struct {
	points []graphics.Point
	closed bool
}

func (s *svgPolygon) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
	if fill.A > 0 && len(s.points) >= 3 {
		canvas.FillTriangle(s.points[0].X, s.points[0].Y,
			s.points[1].X, s.points[1].Y,
			s.points[2].X, s.points[2].Y, fill)
	}
}

type pathCmd struct {
	kind byte
	args []float64
}

type svgPath struct{ commands []pathCmd }

func (s *svgPath) paint(canvas *graphics.Canvas, fill, stroke graphics.Color, sw float64) {
	if len(s.commands) == 0 {
		return
	}
	var pts []graphics.Point
	for _, cmd := range s.commands {
		switch cmd.kind {
		case 'M', 'm':
			if len(cmd.args) >= 2 {
				pts = append(pts, graphics.Point{X: cmd.args[0], Y: cmd.args[1]})
			}
		case 'L', 'l':
			if len(cmd.args) >= 2 {
				pts = append(pts, graphics.Point{X: cmd.args[0], Y: cmd.args[1]})
			}
		case 'Z', 'z':
			if len(pts) >= 3 && fill.A > 0 {
				canvas.FillTriangle(pts[0].X, pts[0].Y,
					pts[len(pts)-2].X, pts[len(pts)-2].Y,
					pts[len(pts)-1].X, pts[len(pts)-1].Y, fill)
			}
		}
	}
}

type svgDocument struct {
	shapes []svgShape
	width  float64
	height float64
}

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

func parseSVGElement(el *dom.Element) svgShape {
	tag := el.LocalName()
	switch tag {
	case "rect":
		return &svgRect{
			x: parseSVGCoord(el.GetAttribute("x")), y: parseSVGCoord(el.GetAttribute("y")),
			w: parseSVGCoord(el.GetAttribute("width")), h: parseSVGCoord(el.GetAttribute("height")),
			rx: parseSVGCoord(el.GetAttribute("rx")), ry: parseSVGCoord(el.GetAttribute("ry")),
		}
	case "circle":
		return &svgCircle{
			cx: parseSVGCoord(el.GetAttribute("cx")), cy: parseSVGCoord(el.GetAttribute("cy")),
			r: parseSVGCoord(el.GetAttribute("r")),
		}
	case "ellipse":
		return &svgEllipse{
			cx: parseSVGCoord(el.GetAttribute("cx")), cy: parseSVGCoord(el.GetAttribute("cy")),
			rx: parseSVGCoord(el.GetAttribute("rx")), ry: parseSVGCoord(el.GetAttribute("ry")),
		}
	case "line":
		return &svgLine{
			x1: parseSVGCoord(el.GetAttribute("x1")), y1: parseSVGCoord(el.GetAttribute("y1")),
			x2: parseSVGCoord(el.GetAttribute("x2")), y2: parseSVGCoord(el.GetAttribute("y2")),
		}
	case "polygon", "polyline":
		return &svgPolygon{points: parseSVGPoints(el.GetAttribute("points")), closed: tag == "polygon"}
	case "path":
		return &svgPath{commands: parseSVGPathData(el.GetAttribute("d"))}
	}
	return nil
}

func buildSVGDocument(el *dom.Element) *svgDocument {
	doc := &svgDocument{
		width:  parseSVGCoord(el.GetAttribute("width")),
		height: parseSVGCoord(el.GetAttribute("height")),
	}
	var walk func(dom.Node)
	walk = func(n dom.Node) {
		if n == nil {
			return
		}
		if childEl, ok := n.(*dom.Element); ok {
			if shape := parseSVGElement(childEl); shape != nil {
				doc.shapes = append(doc.shapes, shape)
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(el)
	return doc
}

func paintSVG(canvas *graphics.Canvas, doc *svgDocument, x, y float64, defaultFill graphics.Color) {
	for _, s := range doc.shapes {
		s.paint(canvas, defaultFill, graphics.Color{}, 0)
	}
}
