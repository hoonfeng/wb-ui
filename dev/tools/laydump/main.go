// laydump：布局诊断工具。把一份 HTML 跑过 wb-ui 全管线（html.Parse →
// style.Resolver → RenderTreeBuilder → RenderView.Layout），按选择器打印
// 匹配元素及其后代的几何，用于与浏览器（Edge headless 自上报）逐元素对照。
//
// 用法：
//
//	go run ./dev/tools/laydump -html cmd/mediastudio/web/index.html -sel ".ctrl-body,.panel"
//
// 与 consistency 套件的区别：套件做「双端自动比对」，本工具只输出 wb-ui 侧
// 的完整几何树（含深度缩进），便于看清「某个元素的宽/高是怎么被算出来的、
// 它的子元素各自占多少」——排查单个字段差异时的第一手数据。
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

type matcher struct{ tag, id, class string }

func parseMatchers(spec string) []matcher {
	var out []matcher
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var m matcher
		switch {
		case strings.HasPrefix(part, "."):
			m.class = part[1:]
		case strings.HasPrefix(part, "#"):
			m.id = part[1:]
		default:
			m.tag = part
		}
		out = append(out, m)
	}
	return out
}

func matches(ms []matcher, e *dom.Element) bool {
	if e == nil {
		return false
	}
	for _, m := range ms {
		if m.id != "" {
			if e.GetAttribute("id") == m.id {
				return true
			}
			continue
		}
		if m.class != "" {
			for _, c := range strings.Fields(e.GetAttribute("class")) {
				if c == m.class {
					return true
				}
			}
			continue
		}
		if e.LocalName() == m.tag {
			return true
		}
	}
	return false
}

func esc(s string) string {
	if s == "" {
		return ""
	}
	return " " + s
}

func label(e *dom.Element) string {
	s := e.LocalName()
	if id := e.GetAttribute("id"); id != "" {
		s += "#" + id
	}
	if cls := e.GetAttribute("class"); cls != "" {
		s += "." + strings.Join(strings.Fields(cls), ".")
	}
	return s
}

func main() {
	htmlPath := flag.String("html", "", "HTML 文件路径（必填）")
	sel := flag.String("sel", "body", "选择器：tag / #id / .class，逗号分隔（也接受祖先 root）")
	vw := flag.Int("w", 1280, "视口宽")
	vh := flag.Int("h", 860, "视口高")
	maxDepth := flag.Int("depth", 4, "子代打印深度")
	flag.Parse()
	if *htmlPath == "" {
		fmt.Fprintln(os.Stderr, "usage: laydump -html <file> -sel <selectors> [-w 1280 -h 860 -depth 4]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*htmlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read html: %v\n", err)
		os.Exit(1)
	}

	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	doc, err := html.Parse(string(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, "html parse: %v\n", err)
		os.Exit(1)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(float64(*vw), float64(*vh))
	state := layout.NewLayoutState(float64(*vw), float64(*vh))
	rv.Layout(state)

	ms := parseMatchers(*sel)
	fmt.Printf("viewport %dx%d\n", *vw, *vh)
	found := 0
	// rel = 距「最近一个匹配元素」的相对深度；未匹配到任何元素时为 -1。
	// 只按相对深度裁剪，否则匹配元素往往在 DOM 深处（.ctrl-body 在第 9 层），
	// 绝对深度一上来就超过 -depth，子元素根本打不出来。
	var walk func(ro rendering.RenderObject, rel int)
	walk = func(ro rendering.RenderObject, rel int) {
		if ro == nil {
			return
		}
		var e *dom.Element
		if n := ro.Node(); n != nil {
			e, _ = n.(*dom.Element)
		}
		cur := rel
		if cur < 0 && e != nil && matches(ms, e) {
			cur = 0
			found++
		} else if cur >= 0 {
			cur++
		}
		if cur >= 0 && e != nil {
			x, y, w, h, ok := rendering.BoxGeometry(ro)
			tag := label(e)
			if !ok {
				// inline 元素无盒：退回首个文本段的几何，便于看行盒。
				if ix, iy, iw, ih, iok := inlineGeo(ro); iok {
					x, y, w, h, ok = ix, iy, iw, ih, iok
					tag += " (inline)"
				}
			}
			if ok && (w > 0 || h > 0 || cur == 0) {
				fmt.Printf("%s%s  x=%s y=%s w=%s h=%s\n",
					strings.Repeat("  ", cur), tag,
					trim(x), trim(y), trim(w), trim(h))
			}
		}
		if cur >= *maxDepth {
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, cur)
		}
	}
	walk(rv, -1)
	if found == 0 {
		fmt.Fprintf(os.Stderr, "no element matched %q\n", *sel)
	}
}

func trim(f float64) string {
	if math.Abs(f-math.Round(f)) < 0.05 {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}

// inlineGeo：inline 元素无自有盒，取首个后代文本段的几何。
func inlineGeo(ro rendering.RenderObject) (x, y, w, h float64, ok bool) {
	var found *rendering.RenderText
	var find func(o rendering.RenderObject)
	find = func(o rendering.RenderObject) {
		if found != nil || o == nil {
			return
		}
		if o.IsRenderText() {
			found = o.(*rendering.RenderText)
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(ro)
	if found == nil {
		return 0, 0, 0, 0, false
	}
	segs := found.Segments()
	if len(segs) == 0 {
		return rendering.BoxGeometry(found)
	}
	s := segs[0]
	return s.X, s.Y, s.Width, s.Height, true
}

// mergeCSSFromDOM：把 <style> 块喂给解析器（与套件的 wbui.go 同源做法）。
func mergeCSSFromDOM(doc *dom.Document, resolver *style.Resolver) {
	var styles []string
	var collect func(n dom.Node)
	collect = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					styles = append(styles, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c)
		}
	}
	collect(doc)
	for _, s := range styles {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(s)
		p.SetOrigin(css.OriginAuthor)
		for _, r := range p.ParseStyleSheet() {
			sheet.AppendRule(r)
		}
		resolver.AddStyleSheet(sheet)
	}
}
