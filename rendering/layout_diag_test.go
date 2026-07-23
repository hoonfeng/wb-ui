package rendering

import (
	"fmt"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

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
#app { background-color: var(--bg-primary); }
`

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

func TestLayout_ViewportFillsFullArea(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	htmlEl.SetClassName("theme-dark")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	bodyEl.SetClassName("theme-dark")
	htmlEl.AppendChild(bodyEl)
	appEl := dom.NewElement(doc, "div")
	appEl.SetId("app")
	bodyEl.AppendChild(appEl)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(indexHTMLCSS).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView nil")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	t.Log("=== LAYOUT DIAGNOSTIC ===")
	if htmlRO := rv.FirstChild(); htmlRO != nil {
		lb := htmlRO.LayoutBox()
		if lb != nil {
			t.Logf("DEBUG: html layout box children=%d", len(lb.Children()))
			for i, c := range lb.Children() {
				t.Logf("  child[%d]: nodeType=%d", i, c.NodeType())
			}
		}
	}
	// Debug: check body's layout box
	var walk func(ro RenderObject)
	walk = func(ro RenderObject) {
		if el, ok := ro.Node().(*dom.Element); ok && el.TagName() == "BODY" {
			lb := ro.LayoutBox()
			if lb != nil {
				ls := rv.LayoutState()
				if ls != nil {
					g := ls.GeometryForBox(lb)
					t.Logf("DEBUG BODY geometry: content=(%.0f,%.0f) %.0fx%.0f  left=%.0f top=%.0f bw=%.0f bh=%.0f",
						g.ContentBoxLeft(), g.ContentBoxTop(),
						g.ContentWidth(), g.ContentHeight(),
						g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
				}
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)

	t.Log("=== LAYOUT DIAGNOSTIC ===")
	t.Logf("Viewport: 1280 x 800")
	dumpLayoutTree(rv, 0, t)

	htmlRO := rv.FirstChild()
	if htmlRO == nil {
		t.Fatal("html not found")
	}
	x, y, w, h := layoutInfo(htmlRO)
	t.Logf("")
	t.Logf("html  layout: (%.0f,%.0f) %.0fx%.0f (expect ~1280x800)", x, y, w, h)
	if w < 100 {
		t.Errorf("html width=%.0f", w)
	}

	bodyRO := htmlRO.FirstChild()
	if bodyRO == nil {
		t.Fatal("body not found")
	}
	x, y, w, h = layoutInfo(bodyRO)
	t.Logf("body  layout: (%.0f,%.0f) %.0fx%.0f (expect ~1280x800)", x, y, w, h)
	if w < 100 {
		t.Errorf("body width=%.0f", w)
	}

	appRO := bodyRO.FirstChild()
	if appRO == nil {
		t.Fatal("#app not found")
	}
	x, y, w, h = layoutInfo(appRO)
	t.Logf("#app  layout: (%.0f,%.0f) %.0fx%.0f (expect ~1280x800)", x, y, w, h)
	if w < 100 {
		t.Errorf("#app width=%.0f", w)
	}
}

func TestLayout_ActivityBarAndSidebar(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	htmlEl.SetClassName("theme-dark")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	bodyEl.SetClassName("theme-dark")
	htmlEl.AppendChild(bodyEl)
	appEl := dom.NewElement(doc, "div")
	appEl.SetId("app")
	bodyEl.AppendChild(appEl)

	menubar := dom.NewElement(doc, "div")
	menubar.SetClassName("menubar")
	appEl.AppendChild(menubar)

	mainContent := dom.NewElement(doc, "div")
	mainContent.SetAttribute("style", "display:flex; flex-direction:row; flex:1; overflow:hidden;")
	appEl.AppendChild(mainContent)

	activityBar := dom.NewElement(doc, "div")
	activityBar.SetClassName("activity-bar")
	activityBar.SetAttribute("style", "width:48px; display:flex; flex-direction:column; background:var(--activity-bar-bg);")
	mainContent.AppendChild(activityBar)

	sidebar := dom.NewElement(doc, "div")
	sidebar.SetClassName("sidebar")
	sidebar.SetAttribute("style", "width:260px; display:flex; flex-direction:column; background:var(--sidebar-bg); border-right:1px solid var(--border-color);")
	mainContent.AppendChild(sidebar)

	editorArea := dom.NewElement(doc, "div")
	editorArea.SetAttribute("style", "flex:1; display:flex; flex-direction:column; overflow:hidden; background:var(--bg-primary);")
	mainContent.AppendChild(editorArea)

	fullCSS := indexHTMLCSS + `
.menubar {
  display: flex; flex-direction: row; align-items: center;
  height: 32px; background: var(--bg-secondary);
  color: var(--text-secondary); font-size: 13px;
  padding: 0 12px; border-bottom: 1px solid var(--border-color);
}
`

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(fullCSS).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView nil")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})
	savePNG(canvas, "F:\\syproject\\gou-ide\\screenshots\\wbui_layout.png")

	t.Log("=== LAYOUT TREE ===")
	dumpLayoutTree(rv, 0, t)

	mb := findByClass(rv, "menubar")
	ab := findByClass(rv, "activity-bar")
	sb := findByClass(rv, "sidebar")

	t.Log("")
	t.Log("┌──────────────────────────────────────────────────┐")
	t.Log("│ BROWSER REFERENCE (web_debug)                     │")
	t.Log("│ menubar:      (0,0)    1280x32                    │")
	t.Log("│ activity-bar: (0,32)   48x768                     │")
	t.Log("│ sidebar:      (48,32)  260x768                    │")
	t.Log("│ editor:       fills rest                           │")
	t.Log("└──────────────────────────────────────────────────┘")

	if mb != nil {
		x, y, w, h := layoutInfo(mb)
		t.Logf("WB-UI menubar:      (%.0f,%.0f) %.0fx%.0f", x, y, w, h)
		if w < 1200 {
			t.Errorf("  W=%.0f expect 1280", w)
		}
		if h < 25 || h > 40 {
			t.Errorf("  H=%.0f expect 32", h)
		}
	}
	if ab != nil {
		x, y, w, h := layoutInfo(ab)
		t.Logf("WB-UI activity-bar: (%.0f,%.0f) %.0fx%.0f", x, y, w, h)
		if w < 40 || w > 56 {
			t.Errorf("  W=%.0f expect 48", w)
		}
	}
	if sb != nil {
		x, y, w, h := layoutInfo(sb)
		t.Logf("WB-UI sidebar:      (%.0f,%.0f) %.0fx%.0f", x, y, w, h)
		if w < 250 || w > 270 {
			t.Errorf("  W=%.0f expect 260", w)
		}
	}

	t.Log("")
	t.Log("=== ALL LAYOUT BOXES ===")
	walkAllLayoutBoxes(rv, t)

	t.Log("")
	t.Log("=== PAINT GRID ===")
	cw, ch := canvas.Width(), canvas.Height()
	for sy := 0; sy < 8; sy++ {
		cy := sy * ch / 8
		line := ""
		for sx := 0; sx < 8; sx++ {
			cx := sx * cw / 8
			p := canvas.PixelAt(cx, cy)
			line += fmt.Sprintf(" (%3d,%3d)=#%02x%02x%02x", cx, cy, p.R, p.G, p.B)
		}
		t.Log(line)
	}
}

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

	styleInfo := ""
	x, y, w, h := layoutInfo(ro)
	cs := ro.Style()
	if cs != nil {
		styleInfo = fmt.Sprintf(" (%.0f,%.0f) %.0fx%.0f CSS:w=%s h=%s disp=%d bg=#%02x%02x%02x",
			x, y, w, h,
			cs.Width, cs.Height, cs.Display,
			cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
	} else {
		styleInfo = fmt.Sprintf(" (%.0f,%.0f) %.0fx%.0f", x, y, w, h)
	}

	t.Logf("%s%s %s %s", pad, name, nodeInfo, styleInfo)

	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpLayoutTree(c, depth+1, t)
	}
}
