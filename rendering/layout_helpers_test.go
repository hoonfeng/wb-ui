package rendering

import (
	"fmt"
	"testing"

	"wb-ui/dom"
)

// indexHTMLCSS mirrors the theme CSS used by layout regression tests that
// build DOM trees programmatically (see layout_fix_test.go).
const indexHTMLCSS = `
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { width: 100%; height: 100%; overflow: hidden; }
:root, .theme-dark {
  --bg-primary: #0d1117; --bg-secondary: #161b22;
  --text-primary: #e6edf3; --text-secondary: #8b949e;
  --accent: #58a6ff; --border-color: #30363d;
  --activity-bar-bg: #0d1117; --sidebar-bg: #161b22;
  --font-ui: 'Inter', system-ui, sans-serif;
  --font-size-base: 13px;
}
body {
  font-family: var(--font-ui); font-size: var(--font-size-base);
  color: var(--text-primary); background-color: var(--bg-primary);
}
#app { display: flex; flex-direction: column; background-color: var(--bg-primary); }
`

// layoutInfo returns the border-box geometry of a render object from its
// associated layout box, falling back to the frame rect for box-bearing
// render objects.
func layoutInfo(ro RenderObject) (x, y, w, h float64) {
	lb := ro.LayoutBox()
	if lb != nil {
		if rv := ro.View(); rv != nil {
			ls := rv.LayoutState()
			if ls != nil {
				g := ls.GeometryForBox(lb)
				return g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			}
		}
	}
	switch v := ro.(type) {
	case *RenderBox:
		fr := v.FrameRect()
		return fr.X, fr.Y, fr.Width, fr.Height
	case *RenderBlockFlow:
		fr := v.FrameRect()
		return fr.X, fr.Y, fr.Width, fr.Height
	case *RenderBlock:
		fr := v.FrameRect()
		return fr.X, fr.Y, fr.Width, fr.Height
	case *RenderView:
		return 0, 0, v.viewWidth, v.viewHeight
	}
	return 0, 0, 0, 0
}

// walkAllLayoutBoxes logs the geometry of every render object that has an
// associated layout box, for test diagnostics.
func walkAllLayoutBoxes(ro RenderObject, t *testing.T) {
	if ro == nil {
		return
	}
	lb := ro.LayoutBox()
	if lb != nil {
		var lx, ly, lw, lh float64
		if rv := ro.View(); rv != nil {
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(lb)
				lx, ly, lw, lh = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			}
		}
		cs := ro.Style()
		tag := ""
		if el, ok := ro.Node().(*dom.Element); ok {
			tag = el.LocalName()
			if id := el.GetId(); id != "" {
				tag += "#" + id
			}
			if cls := el.GetClassName(); cls != "" {
				tag += "." + cls
			}
		}
		bg := ""
		cw := ""
		ch := ""
		if cs != nil {
			bg = fmt.Sprintf("#%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
			cw = cs.Width.String()
			ch = cs.Height.String()
		}
		t.Logf("  <%-30s> rect=(%6.0f,%6.0f) %4.0fx%-4.0f w=%s h=%s bg=%s",
			tag, lx, ly, lw, lh, cw, ch, bg)
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		walkAllLayoutBoxes(c, t)
	}
}

// findByClass returns the first render object whose DOM element has the given
// class name, or nil.
func findByClass(root RenderObject, className string) RenderObject {
	var result RenderObject
	var walk func(ro RenderObject)
	walk = func(ro RenderObject) {
		if result != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok {
			if el.GetClassName() == className {
				result = ro
				return
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(root)
	return result
}

// dumpLayoutTree logs the render tree structure with geometry, for test
// diagnostics.
func dumpLayoutTree(ro RenderObject, depth int, t *testing.T) {
	t.Helper()
	if ro == nil {
		return
	}
	pad := ""
	for i := 0; i < depth; i++ {
		pad += "  "
	}
	name := ro.RenderName()
	nodeInfo := ""
	if el, ok := ro.Node().(*dom.Element); ok {
		tag := el.LocalName()
		id := el.GetId()
		cls := el.GetClassName()
		nodeInfo = fmt.Sprintf("<%s", tag)
		if id != "" {
			nodeInfo += "#" + id
		}
		if cls != "" {
			nodeInfo += "." + cls
		}
		nodeInfo += ">"
	}
	t.Logf("%s%s %s", pad, name, nodeInfo)
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpLayoutTree(c, depth+1, t)
	}
}
