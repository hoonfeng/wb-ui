package main

import (
	"fmt"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

func main() {
	htmlSrc := `<!DOCTYPE html>
<html><body>
<div id="app" style="display:flex;flex-direction:row;width:1280px;height:800px">
  <div id="toolbar" style="width:48px;background:#21262d">T</div>
  <div id="sidebar" style="width:280px;background:#161b22">Files</div>
  <div id="editor" style="flex:1;background:#0d1117">Editor</div>
  <div id="right" style="width:250px;background:#21262d">Right</div>
</div>
</body></html>`

	doc, _ := html.Parse(htmlSrc)

	resolver := style.NewResolver()

	// Resolve all
	walk := func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok {
			cs := resolver.ResolveElement(el)
			if id := el.GetAttribute("id"); id != "" {
				w := cs.Width
				fmt.Printf("  %-10s style=%q Width.Val=%.0f Unit=%d bg=#%02x%02x%02x\n",
					id,
					el.GetAttribute("style"),
					w.Value, w.Unit,
					cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B,
				)
			}
		}
	}
	walkNode(doc, walk)
}

func walkNode(n dom.Node, fn func(dom.Node)) {
	fn(n)
	switch v := n.(type) {
	case *dom.Document:
		if el := v.DocumentElement(); el != nil {
			walkNode(el, fn)
		}
		if b := v.Head(); b != nil {
			walkNode(b, fn)
		}
		if b := v.Body(); b != nil {
			walkNode(b, fn)
		}
	case *dom.Element:
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			walkNode(c, fn)
		}
	}
}
