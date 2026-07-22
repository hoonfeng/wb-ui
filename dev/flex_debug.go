package main

import (
	"fmt"

	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/style"
)

func main() {
	src := `<!DOCTYPE html>
<html><body>
<div id="app" style="display:flex;flex-direction:row;width:1280px;height:800px">
  <div id="a" style="width:48px;background:gray">A</div>
  <div id="b" style="width:280px;background:blue">B</div>
  <div id="c" style="flex:1;background:red">C</div>
</div>
</body></html>`
	doc, _ := html.Parse(src)

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())

	root := doc.DocumentElement()
	layoutRoot := layout.BuildLayoutTree(root, resolver)
	if layoutRoot == nil {
		fmt.Println("FAIL: nil layout root")
		return
	}

	// Use the full layout pipeline (stretchRoot + layoutRoot + roundTree)
	layout.Layout(layoutRoot, 1280, 800)

	// Find #app and dump its children
	var appBox *layout.LayoutBox
	var walk func(box *layout.LayoutBox)
	walk = func(box *layout.LayoutBox) {
		if box.Style != nil && box.Element != nil {
			if id := box.Element.GetAttribute("id"); id == "app" {
				appBox = box
			}
		}
		for _, c := range box.Children {
			walk(c)
		}
	}
	walk(layoutRoot)

	if appBox == nil {
		fmt.Println("FAIL: #app not found")
		return
	}

	fmt.Println("=== Flex items after layout.Layout ===")
	for _, c := range appBox.Children {
		id := ""
		if c.Element != nil {
			id = c.Element.GetAttribute("id")
		}
		w := c.Style.Width
		fmt.Printf("  %-4s x=%.0f y=%.0f w=%.0f h=%.0f style.width=%.0f/unit=%q flexGrow=%.0f flexBasis=%.0f/unit=%q\n",
			id, c.Rect.X, c.Rect.Y, c.Rect.Width, c.Rect.Height,
			w.Value, w.Unit,
			c.Style.FlexGrow,
			c.Style.FlexBasis.Value, c.Style.FlexBasis.Unit)
	}
	fmt.Println("\nExpected: a=48px  b=280px  c=952px (flex-grow fills remaining)")
}
