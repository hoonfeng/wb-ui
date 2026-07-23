package main

import (
	"fmt"
	"os"

	"wb-ui/dom"
	"wb-ui/layout"
)

func buildTestDocument() *dom.Document {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	doc.AppendChild(html)
	head := doc.CreateElement("head")
	html.AppendChild(head)
	body := doc.CreateElement("body")
	html.AppendChild(body)

	parent := doc.CreateElement("div")
	parent.SetAttribute("class", "parent")
	body.AppendChild(parent)

	sidebar := doc.CreateElement("div")
	sidebar.SetAttribute("class", "sidebar")
	sidebar.AppendChild(doc.CreateTextNode("S"))
	parent.AppendChild(sidebar)

	content := doc.CreateElement("div")
	content.SetAttribute("class", "content")
	parent.AppendChild(content)

	item1 := doc.CreateElement("div")
	item1.SetAttribute("class", "item")
	item1.AppendChild(doc.CreateTextNode("A"))
	content.AppendChild(item1)

	item2 := doc.CreateElement("div")
	item2.SetAttribute("class", "item")
	item2.AppendChild(doc.CreateTextNode("B"))
	content.AppendChild(item2)

	return doc
}

func main() {
	doc := buildTestDocument()
	root := doc.DocumentElement()
	layoutRoot := layout.BuildLayoutTree(root, nil)
	if layoutRoot == nil {
		fmt.Fprintln(os.Stderr, "layout build failed")
		os.Exit(1)
	}

	rootEb, ok := layoutRoot.(*layout.ElementBox)
	if !ok {
		fmt.Fprintln(os.Stderr, "root is not an ElementBox")
		os.Exit(1)
	}

	state := layout.NewLayoutState(500, 400)
	rootG := state.GeometryForBox(rootEb)
	rootG.SetTopLeft(0, 0)
	rootG.SetContentWidth(500)
	rootG.SetContentHeight(400)
	layout.LayoutRoot(rootEb, state)

	var find func(*layout.ElementBox, string) *layout.ElementBox
	find = func(box *layout.ElementBox, class string) *layout.ElementBox {
		if box.Element() != nil {
			if cls := box.Element().GetAttribute("class"); cls == class {
				return box
			}
		}
		for _, c := range box.Children() {
			if childEb, ok := c.(*layout.ElementBox); ok {
				if found := find(childEb, class); found != nil {
					return found
				}
			}
		}
		return nil
	}

	content := find(rootEb, "content")
	if content == nil {
		fmt.Fprintln(os.Stderr, "content div not found")
		os.Exit(1)
	}
	cg := state.GeometryForBox(content)
	fmt.Printf("content: x=%.0f y=%.0f w=%.0f h=%.0f\n",
		cg.Left(), cg.Top(), cg.BorderBoxWidth(), cg.BorderBoxHeight())

	if cg.BorderBoxWidth() > 0 {
		fmt.Println("PASS: content width > 0")
	} else {
		fmt.Println("FAIL: content width = 0")
		os.Exit(1)
	}
}
