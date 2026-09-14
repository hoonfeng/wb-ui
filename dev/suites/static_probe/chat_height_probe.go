//go:build ignore

// Command chat_height_probe 复现「对话高度/对话列表高度计算不正确」：
// 模仿 RightPanel.vue 的 chat-area 结构（column flex + flex:1 消息列表 +
// overflow-y:auto + 大量 msg-item），测量 chat-messages 的 clientHeight/
// scrollHeight 与 chat-input-area 的可见性，对比真实浏览器预期。
package main

import (
	"fmt"
	"os"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

func main() {
	n := 2000
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &n)
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><style>
* { box-sizing: border-box; margin: 0; padding: 0; }
html, body { height: 100%; }
body { font-family: sans-serif; font-size: 13px; }
.right-panel { height: 800px; display: flex; flex-direction: column; background: #1e1e1e; color: #ccc; }
.rp-header { height: 40px; flex-shrink: 0; padding: 8px; }
.rp-body { flex: 1; display: flex; flex-direction: row; overflow: hidden; min-height: 0; }
.chat-area { flex: 1; display: flex; flex-direction: column; min-width: 0; overflow: hidden; max-width: 100%; }
.chat-messages { flex: 1; overflow-y: auto; padding: 8px 12px; min-height: 0; position: relative; }
.msg-list-wrap { display: flex; flex-direction: column; gap: 12px; min-height: 100%; }
.msg-item { display: flex; gap: 8px; align-items: flex-start; }
.msg-avatar { width: 28px; height: 28px; flex-shrink: 0; background: #333; border-radius: 50%; }
.msg-bubble { flex: 1; min-width: 0; font-size: 13px; line-height: 1.6; word-break: break-word; }
.chat-input-area { display: flex; flex-direction: column; flex-shrink: 0; padding: 0 8px 8px 8px; }
.input-wrapper { border: 1px solid #333; border-radius: 8px; }
.chat-input { display: block; width: 100%; min-height: 80px; }
.conv-sidebar { width: 240px; flex-shrink: 0; border-left: 1px solid #333; padding: 8px; }
</style></head><body>
<div class="right-panel">
  <div class="rp-header">对话</div>
  <div class="rp-body">
    <div class="chat-area">
      <div class="chat-messages">
        <div class="msg-list-wrap">`)

	for i := 0; i < n; i++ {
		b.WriteString(`<div class="msg-item"><div class="msg-avatar"></div><div class="msg-bubble">消息 `)
		b.WriteString(fmt.Sprintf("%d", i))
		b.WriteString(`：这是一段测试消息内容，用于模拟对话列表中的历史消息。包含多行内容以便产生合理的高度。第二行内容。第三行内容。</div></div>`)
	}

	b.WriteString(`
        </div>
      </div>
      <div class="chat-input-area">
        <div class="input-wrapper"><textarea class="chat-input"></textarea></div>
      </div>
    </div>
    <div class="conv-sidebar">会话列表</div>
  </div>
</div>
</body></html>`)

	htmlStr := b.String()

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

	doc, err := html.Parse(htmlStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	// 内联 <style> 解析（与 static_probe/dump.go 的 mergeCSSFromDOM 一致）
	var collect func(n dom.Node, styles2 *[]string)
	collect = func(n dom.Node, styles2 *[]string) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					*styles2 = append(*styles2, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c, styles2)
		}
	}
	var styles2 []string
	collect(doc, &styles2)
	for _, s := range styles2 {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(s)
		p.SetOrigin(css.OriginAuthor)
		for _, r := range p.ParseStyleSheet() {
			sheet.AppendRule(r)
		}
		resolver.AddStyleSheet(sheet)
	}

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	if rv == nil {
		fmt.Println("RenderView nil")
		os.Exit(1)
	}
	rv.SetViewportSize(1280, 800)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	fmt.Printf("=== chat_height_probe: %d 条消息 ===\n", n)
	var walk func(ro rendering.RenderObject)
	walk = func(ro rendering.RenderObject) {
		if n0 := ro.Node(); n0 != nil {
			if el, ok := n0.(*dom.Element); ok {
				cls := el.GetAttribute("class")
				switch cls {
				case "right-panel", "rp-body", "chat-area", "chat-messages", "msg-list-wrap", "chat-input-area", "conv-sidebar":
					rb := rv.FindRenderBoxForNode(el)
					lb := ro.LayoutBox()
					var top, h, clientH, scrollH, scrollW float64
					if lb != nil {
						g := state.GeometryForBox(lb)
						top, h = g.Top(), g.BorderBoxHeight()
					}
					if rb != nil {
						pb := rb.PaddingBoxRect()
						clientH = pb.Height
						scrollW, scrollH = rv.BoxContentSize(rb)
					}
					fmt.Printf("%-16s top=%.0f h=%.0f clientH=%.0f scrollH=%.0f scrollW=%.0f\n",
						cls, top, h, clientH, scrollH, scrollW)
				}
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	fmt.Println("=== done ===")
}
