package main

import (
	"fmt"
	"os"
	"strings"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/style"
)

func main() {
	html := `<!DOCTYPE html><html><head><style>
		.parent { display: flex; flex-direction: row; width: 500px; height: 400px; }
		.sidebar { width: 80px; background: red; }
		.content { display: flex; flex-direction: column; background: green; }
		.item { height: 50px; background: blue; }
	</style></head><body><div class="parent">
		<div class="sidebar">S</div>
		<div class="content">
			<div class="item">A</div>
			<div class="item">B</div>
		</div>
	</div></body></html>`

	doc, err := html5.Parse(strings.NewReader(html))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		os.Exit(1)
	}

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())

	for _, el := range doc.GetElementsByTagName("style") {
		if text := el.TextContent(); text != "" {
			sheet := style.NewCSSStyleSheet()
			sheet.Parse(text)
			resolver.AddStyleSheet(sheet)
		}
	}

	root := doc.DocumentElement()
	layoutRoot := layout.BuildLayoutTree(root, resolver)
	if layoutRoot == nil {
		fmt.Fprintln(os.Stderr, "layout build failed")
		os.Exit(1)
	}

	state := layout.NewLayoutState(500, 400)
	layoutRoot.Rect = layout.Rect{X: 0, Y: 0, Width: 500, Height: 400}
	layout.LayoutRoot(layoutRoot, state)

	find := func(box *layout.LayoutBox, class string) *layout.LayoutBox {
		if box.Element != nil {
			if cls, _ := box.Element.GetAttribute("class"); cls == class {
				return box
			}
		}
		for _, c := range box.Children {
			if found := find(c, class); found != nil {
				return found
			}
		}
		return nil
	}

	content := find(layoutRoot, "content")
	if content == nil {
		fmt.Fprintln(os.Stderr, "content div not found")
		os.Exit(1)
	}
	fmt.Printf("content: x=%.0f y=%.0f w=%.0f h=%.0f\n",
		content.Rect.X, content.Rect.Y, content.Rect.Width, content.Rect.Height)

	if content.Rect.Width > 0 {
		fmt.Println("PASS: content width > 0")
	} else {
		fmt.Println("FAIL: content width = 0")
		os.Exit(1)
	}
}
