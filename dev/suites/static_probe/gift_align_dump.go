//go:build ignore

// Command gift_align_dump 复现加班挑战挂件礼物栏（.gifts > .gitem > img+b）
// 的图标/文字垂直对齐问题：dump 每个 box 的几何 + 文本段基线位置。
package main

import (
	"fmt"
	"os"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

const pngURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func main() {
	if graphics.GetFontManager() == nil {
		mgr := graphics.InitFontManager("")
		mgr.LoadSystemFonts()
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	path := "dev/suites/static_probe/ot_real.html"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	fmt.Println("html:", path)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	doc, err := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	var collect func(n dom.Node)
	collect = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					sheet := css.NewCSSStyleSheet()
					sheet.SetOrigin(css.OriginAuthor)
					p := css.NewParser(t.Data())
					p.SetOrigin(css.OriginAuthor)
					for _, r := range p.ParseStyleSheet() {
						sheet.AppendRule(r)
					}
					resolver.AddStyleSheet(sheet)
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c)
		}
	}
	collect(doc)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(500, 320)
	state := layout.NewLayoutState(500, 320)
	rv.Layout(state)

	fmt.Println("=== GIFT ALIGN DUMP ===")
	var walkLayout func(lb *layout.ElementBox, depth int)
	walkLayout = func(lb *layout.ElementBox, depth int) {
		g := state.GeometryForBox(lb)
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		name := "(anon)"
		cls := ""
		if lb.Element() != nil {
			name = lb.Element().LocalName()
			cls = lb.Element().GetAttribute("class")
		}
		if lb.Element() == nil {
			walkChildren(lb, indent, state)
			return
		}
		st := lb.Style()
		cx := g.ContentBoxLeft() + g.ContentWidth()/2
		cy := g.ContentBoxTop() + g.ContentHeight()/2
		fmt.Printf("%s[box] %-14s rect=(%.1f,%.1f w=%.1f h=%.1f) content=(%.1f,%.1f w=%.1f h=%.1f) center=(%.1f,%.1f) disp=%s\n",
			indent, name+"."+cls, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(),
			g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight(),
			cx, cy, st.Display.String())
		walkChildren(lb, indent, state)
	}
	walkLayout(rv.LayoutBox(), 0)
}

func walkChildren(lb *layout.ElementBox, indent string, state *layout.LayoutState) {
	for _, c := range lb.Children() {
		if itb, ok := c.(*layout.InlineTextBox); ok {
			for _, seg := range itb.TextSegments {
				fmt.Printf("%s  [text %q] seg x=%.1f y=%.1f w=%.1f h=%.1f centerY=%.1f\n",
					indent, itb.Text(), seg.X, seg.Y, seg.Width, seg.Height, seg.Y+seg.Height/2)
			}
		}
		if eb, ok := c.(*layout.ElementBox); ok {
			walkElement(eb, indent+"  ", state)
		}
	}
}

func walkElement(eb *layout.ElementBox, indent string, state *layout.LayoutState) {
	g := state.GeometryForBox(eb)
	name := "(anon)"
	cls := ""
	if eb.Element() != nil {
		name = eb.Element().LocalName()
		cls = eb.Element().GetAttribute("class")
	}
	st := eb.Style()
	cx := g.ContentBoxLeft() + g.ContentWidth()/2
	cy := g.ContentBoxTop() + g.ContentHeight()/2
	fmt.Printf("%s[box] %-14s rect=(%.1f,%.1f w=%.1f h=%.1f) center=(%.1f,%.1f) disp=%s\n",
		indent, name+"."+cls, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(),
		cx, cy, st.Display.String())
	for _, c := range eb.Children() {
		if itb, ok := c.(*layout.InlineTextBox); ok {
			for _, seg := range itb.TextSegments {
				fmt.Printf("%s  [text %q] seg x=%.1f y=%.1f w=%.1f h=%.1f centerY=%.1f\n",
					indent, itb.Text(), seg.X, seg.Y, seg.Width, seg.Height, seg.Y+seg.Height/2)
			}
		}
		if ceb, ok := c.(*layout.ElementBox); ok {
			walkElement(ceb, indent+"  ", state)
		}
	}
}
