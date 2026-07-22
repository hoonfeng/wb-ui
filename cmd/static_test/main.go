package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

func main() {
	cssText := `* { margin:0; padding:0; box-sizing:border-box; }
body { background:#0d1117; color:#e6edf3; font-family:sans-serif; font-size:13px; }
.header { background:#161b22; height:48px; display:flex; align-items:center; padding:0 16px; border-bottom:1px solid #30363d; font-size:14px; font-weight:bold; }
.layout { display:flex; }
.sidebar { width:200px; background:#161b22; border-right:1px solid #30363d; padding:12px; display:flex; flex-direction:column; }
.sidebar .item { padding:6px 8px; color:#e6edf3; font-size:13px; }
.sidebar .item.active { background:#1f6feb; color:#fff; border-radius:6px; }
.main { flex:1; display:flex; flex-direction:column; }
.toolbar { background:#161b22; height:36px; display:flex; align-items:center; padding:0 12px; border-bottom:1px solid #30363d; }
.toolbar .tab { padding:4px 12px; border-radius:4px; font-size:12px; color:#8b949e; }
.toolbar .tab.active { background:#1f6feb; color:#fff; }
.content { flex:1; background:#0d1117; padding:16px; font-family:monospace; font-size:13px; line-height:1.6; }
.content .keyword { color:#ff7b72; }
.content .string { color:#a5d6ff; }
.content .comment { color:#8b949e; }
.statusbar { background:#161b22; height:24px; display:flex; align-items:center; padding:0 12px; border-top:1px solid #30363d; font-size:11px; color:#8b949e; }
.badge { padding:2px 6px; border-radius:3px; font-size:10px; font-weight:bold; display:inline-block; }
.badge.red { background:#da3633; color:#fff; }
.badge.green { background:#238636; color:#fff; }
.badge.blue { background:#1f6feb; color:#fff; }
`
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(cssText)
	htmlEl.AppendChild(styleEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	h := dom.NewElement(doc, "div")
	h.SetClassName("header")
	h.AppendChild(doc.CreateTextNode("IDE Header"))
	bodyEl.AppendChild(h)

	layoutEl := dom.NewElement(doc, "div")
	layoutEl.SetClassName("layout")
	bodyEl.AppendChild(layoutEl)

	sb := dom.NewElement(doc, "div")
	sb.SetClassName("sidebar")
	layoutEl.AppendChild(sb)
	for _, it := range []struct{ t, c string }{
		{"Files", "item active"}, {"Search", "item"}, {"Debug", "item"}, {"Settings", "item"},
	} {
		el := dom.NewElement(doc, "div")
		el.SetClassName(it.c)
		el.AppendChild(doc.CreateTextNode(it.t))
		sb.AppendChild(el)
	}

	mainEl := dom.NewElement(doc, "div")
	mainEl.SetClassName("main")
	layoutEl.AppendChild(mainEl)

	tb := dom.NewElement(doc, "div")
	tb.SetClassName("toolbar")
	mainEl.AppendChild(tb)
	for _, it := range []struct{ t, c string }{
		{"main.go", "tab active"}, {"app.tsx", "tab"}, {"style.css", "tab"},
	} {
		el := dom.NewElement(doc, "div")
		el.SetClassName(it.c)
		el.AppendChild(doc.CreateTextNode(it.t))
		tb.AppendChild(el)
	}

	ct := dom.NewElement(doc, "div")
	ct.SetClassName("content")
	mainEl.AppendChild(ct)
	for _, l := range []struct{ cls, text string }{
		{"keyword", "package"},
		{"comment", "// Hello"},
		{"", "fmt.Println("},
		{"string", "\"Hello!\""},
		{"", ")"},
	} {
		line := dom.NewElement(doc, "div")
		span := dom.NewElement(doc, "span")
		if l.cls != "" {
			span.SetClassName(l.cls)
		}
		span.AppendChild(doc.CreateTextNode(l.text))
		line.AppendChild(span)
		ct.AppendChild(line)
	}

	st := dom.NewElement(doc, "div")
	st.SetClassName("statusbar")
	mainEl.AppendChild(st)
	for _, it := range []struct{ t, c string }{
		{"Go 1.26", ""}, {"UTF-8", ""}, {"OK", "badge green"}, {"3 files", "badge blue"}, {"0 err", "badge red"},
	} {
		el := dom.NewElement(doc, "span")
		if it.c != "" {
			el.SetClassName(it.c)
		}
		el.AppendChild(doc.CreateTextNode(it.t))
		st.AppendChild(el)
	}

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := rendering.NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		fmt.Println("ERROR: RenderView is nil")
		return
	}

	lb := rv.LayoutBox()
	fmt.Println("=== LAYOUT TREE (pre-layout) ===")
	dumpLB(lb, 0)

	rv.SetViewportSize(800, 600)
	rv.Layout(nil)

	fmt.Println("\n=== LAYOUT TREE (post-layout) ===")
	dumpLB(lb, 0)

	fmt.Println("\n=== RENDER TREE (post-layout) ===")
	dumpRO(rv, 0)

	canvas := graphics.NewCanvas(800, 600)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 800, Height: 600})

	outDir := filepath.Join("dev", "output")
	os.MkdirAll(outDir, 0755)
	outPath := filepath.Join(outDir, "static_test.png")
	savePNG(canvas, outPath)
	fmt.Printf("\nPNG: %s\n", outPath)

	fmt.Println("\n=== PIXEL GRID (7x7) ===")
	cw, ch := canvas.Width(), canvas.Height()
	for sy := 0; sy < 7; sy++ {
		y := sy * ch / 7
		line := ""
		for sx := 0; sx < 7; sx++ {
			x := sx * cw / 7
			p := canvas.PixelAt(x, y)
			line += fmt.Sprintf(" (%3d,%3d)#%02x%02x%02x", x, y, p.R, p.G, p.B)
		}
		fmt.Println(line)
	}
}

func elName(el *dom.Element) string {
	s := el.LocalName()
	if c := el.ClassName(); c != "" {
		s += "." + c
	}
	return s
}

func dumpLB(lb *layout.LayoutBox, depth int) {
	if lb == nil {
		return
	}
	pref := ""
	for i := 0; i < depth; i++ {
		pref += "  "
	}
	elStr := "(anon)"
	if lb.Element != nil {
		elStr = elName(lb.Element)
	}
	disp := "nil"
	if lb.Style != nil {
		disp = fmt.Sprintf("%v", lb.Style.Display)
	}
	fmt.Printf("%s%-18s %-6s (%5.0f,%5.0f) %5.0fx%5.0f disp=%s\n",
		pref, elStr, lb.Type, lb.Rect.X, lb.Rect.Y, lb.Rect.Width, lb.Rect.Height, disp)
	for _, c := range lb.Children {
		dumpLB(c, depth+1)
	}
}

func dumpRO(ro rendering.RenderObject, depth int) {
	pref := ""
	for i := 0; i < depth; i++ {
		pref += "  "
	}
	n := ro.Node()
	elStr := "(none)"
	if n != nil {
		if el, ok := n.(*dom.Element); ok {
			elStr = elName(el)
		} else if _, ok := n.(*dom.Text); ok {
			elStr = "#text"
		}
	}
	lb := ro.LayoutBox()
	lbStr := "no-lb"
	frStr := ""
	if lb != nil {
		lbStr = fmt.Sprintf("lb=(%5.0f,%5.0f %5.0fx%5.0f)", lb.Rect.X, lb.Rect.Y, lb.Rect.Width, lb.Rect.Height)
	}
	if box, ok := ro.(*rendering.RenderBox); ok {
		fr := box.FrameRect()
		frStr = fmt.Sprintf("  fr=(%5.0f,%5.0f %5.0fx%5.0f) vis=%v", fr.X, fr.Y, fr.Width, fr.Height, box.IsVisible())
	}
	fmt.Printf("%s%-18s %s%s\n", pref, elStr, lbStr, frStr)
	for ch := ro.FirstChild(); ch != nil; ch = ch.NextSibling() {
		dumpRO(ch, depth+1)
	}
}

func savePNG(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
