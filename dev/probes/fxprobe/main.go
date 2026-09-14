// Command fxprobe dumps a fixture's render tree together with the paint-order
// inputs (style Display / Float and RenderBox.IsFloated) so a "why is this box
// painted after that one" question can be answered without guessing.
//
// Usage: go run _temp/fxprobe/main.go <file.html>
package main

import (
	"fmt"
	"os"
	"strings"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

// boxish is the subset of RenderBox's API the probe needs. RenderBox is embedded
// in RenderBlock / RenderBlockFlow, so a concrete-type assertion would miss them
// and every box would print as a plain node.
type boxish interface {
	Style() *style.ComputedStyle
	ParentBox() *rendering.RenderBox
	IsFloated() bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: fxprobe <file.html>")
		return
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	doc, err := html.Parse(string(raw))
	if err != nil {
		panic(err)
	}
	var sheets []string
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					sheets = append(sheets, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc)

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	for _, s := range sheets {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(s)
		p.SetOrigin(css.OriginAuthor)
		for _, rule := range p.ParseStyleSheet() {
			sheet.AppendRule(rule)
		}
		resolver.AddStyleSheet(sheet)
	}
	resolver.SetViewportSize(900, 1000)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		panic("build failed")
	}
	rv.SetResolver(resolver)
	rv.SetViewportSize(900, 1000)
	rv.Layout(layout.NewLayoutState(900, 1000))

	var dump func(o rendering.RenderObject, d int)
	dump = func(o rendering.RenderObject, d int) {
		if o == nil {
			return
		}
		indent := strings.Repeat("  ", d)
		label := "(text)"
		if el, ok := o.Node().(*dom.Element); ok {
			label = el.LocalName()
			if id := el.GetAttribute("id"); id != "" {
				label += "#" + id
			}
			if cls := strings.TrimSpace(el.GetAttribute("class")); cls != "" {
				label += "." + strings.ReplaceAll(cls, " ", ".")
			}
		}
		if rb, ok := o.(boxish); ok {
			st := rb.Style()
			x, y, w, h, ok2 := rendering.BoxGeometry(o)
			geo := ""
			if ok2 {
				geo = fmt.Sprintf("%.0fx%.0f@(%.0f,%.0f)", w, h, x, y)
			}
			disp, flt := "", ""
			if st != nil {
				disp = fmt.Sprintf("%v", st.Display)
				flt = st.Float
			}
			parentDisp := "(none)"
			if pb := rb.ParentBox(); pb != nil && pb.Style() != nil {
				parentDisp = fmt.Sprintf("%v", pb.Style().Display)
			}
			fmt.Printf("%s%-34s %-18s float=%-5q disp=%-12s parentDisp=%-12s IsFloated=%v\n",
				indent, label, geo, flt, disp, parentDisp, rb.IsFloated())
		} else {
			fmt.Printf("%s%-34s %s\n", indent, label, "")
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			dump(c, d+1)
		}
	}
	dump(rv, 0)
}
