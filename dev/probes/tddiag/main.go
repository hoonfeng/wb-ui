// tddiag 打印一个 HTML 文件里所有元素的 computed display / 声明的 width 与布局
// 后的 border box 几何，用于定位「表格单元格宽度被内容撑开」这类问题——渲染树的
// 尺寸看不出是哪一层写坏了，把 display 与实际几何并排打印即可分辨。
//
//	go run ./dev/probes/tddiag -file dev/suites/cssprobe/fixtures/fixed-table-layout.html
package main

import (
	"flag"
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

const viewportW, viewportH = 900, 1000

func main() {
	file := flag.String("file", "", "HTML file to diagnose")
	depth := flag.Int("depth", 6, "max tree depth to print")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "tddiag: -file is required")
		os.Exit(2)
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tddiag: %v\n", err)
		os.Exit(1)
	}

	layout.MeasureTextFunc = func(family string, size float64, weight int, st, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: st}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, st string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: st}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	layout.XHeightFunc = func(family string, size float64, weight int, st string) float64 {
		return graphics.GlobalFontXHeight(graphics.Font{Family: family, Size: size, Weight: weight, Style: st})
	}

	doc, err := html.Parse(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tddiag: parse: %v\n", err)
		os.Exit(1)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	applyDocumentCSS(doc, resolver)
	// 预先解析全部 computed style（表内元素的 display/width 是诊断重点）。
	styles := resolver.ResolveDocument(doc)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		fmt.Fprintln(os.Stderr, "tddiag: render tree build failed")
		os.Exit(1)
	}
	rv.SetResolver(resolver)
	rv.SetViewportSize(viewportW, viewportH)
	state := layout.NewLayoutState(viewportW, viewportH)
	rv.Layout(state)

	dump(rv, styles, 0, *depth)
}

func dump(ro rendering.RenderObject, styles map[*dom.Element]*style.ComputedStyle, depth, maxDepth int) {
	if ro == nil || depth > maxDepth {
		return
	}
	label := "node"
	var el *dom.Element
	if n := ro.Node(); n != nil {
		switch v := n.(type) {
		case *dom.Element:
			el = v
			label = v.LocalName()
			if id := v.GetAttribute("id"); id != "" {
				label += "#" + id
			}
			if cls := v.GetAttribute("class"); cls != "" {
				label += "." + cls
			}
		case *dom.Text:
			label = "#text"
		}
	}
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	desc := ""
	if el != nil {
		if cs := styles[el]; cs != nil {
			desc = fmt.Sprintf("display=%s cssW=%v%v cssH=%v%v",
				cs.Display, cs.Width.Value, cs.Width.Unit, cs.Height.Value, cs.Height.Unit)
		}
	}
	if x, y, w, h, ok := rendering.BoxGeometry(ro); ok {
		fmt.Printf("%s%s  %.1fx%.1f @(%.1f,%.1f)  %s\n", indent, label, w, h, x, y, desc)
	} else {
		fmt.Printf("%s%s  (no box)  %s\n", indent, label, desc)
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dump(c, styles, depth+1, maxDepth)
	}
}

func applyDocumentCSS(doc *dom.Document, resolver *style.Resolver) {
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
	for _, text := range sheets {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(text)
		p.SetOrigin(css.OriginAuthor)
		for _, rule := range p.ParseStyleSheet() {
			sheet.AppendRule(rule)
		}
		resolver.AddStyleSheet(sheet)
	}
}
