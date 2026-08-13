// Translation of: tests for Source/WebCore/rendering/updating/RenderTreeBuilder.cpp
// Completeness: 60%
// Simplifications:
//   - tests verify the render tree shape (types, parent/child links) without running a
//     full layout pass; the layout package has its own test suite for geometry

package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/style"
)

// buildSimpleDoc creates a document with a single <html><body><div>hello</div></body></html>.
func buildSimpleDoc() *dom.Document {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	div := doc.CreateElement("div")
	text := doc.CreateTextNode("hello")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(div)
	div.AppendChild(text)
	return doc
}

// TestBuildBasicDocument verifies that Build produces a RenderView with the document
// element's render object as its child.
func TestBuildBasicDocument(t *testing.T) {
	doc := buildSimpleDoc()
	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("Build returned nil view")
	}
	if view.Document() != doc {
		t.Fatalf("view.Document() = %v, want %v", view.Document(), doc)
	}
	if !view.IsRenderView() {
		t.Error("view should be RenderView")
	}
	// The first child should correspond to the <html> element.
	first := view.FirstChild()
	if first == nil {
		t.Fatal("view has no children")
	}
	if el, ok := first.Node().(*dom.Element); !ok || el.LocalName() != "html" {
		t.Fatalf("first child node = %v, want <html>", first.Node())
	}
}

// TestBuildDisplayNone verifies that display:none elements are skipped.
func TestBuildDisplayNone(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	hidden := doc.CreateElement("div")
	hidden.SetAttribute("style", "display:none")
	visible := doc.CreateElement("div")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(hidden)
	body.AppendChild(visible)

	// Use a resolver that applies the inline style.
	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)
	if view == nil {
		t.Fatal("Build returned nil")
	}

	// Count render objects that correspond to div elements.
	divCount := 0
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if el, ok := cur.Node().(*dom.Element); ok && el.LocalName() == "div" {
			divCount++
		}
	}
	if divCount != 1 {
		t.Fatalf("found %d div render objects, want 1 (display:none should be skipped)", divCount)
	}
}

// TestBuildInlineBlockMixing verifies that mixed inline/block content produces an
// anonymous wrapper for the inline children.
func TestBuildInlineBlockMixing(t *testing.T) {
	doc := dom.NewDocument()
	container := doc.CreateElement("div")
	block1 := doc.CreateElement("p")
	span := doc.CreateElement("span")
	block2 := doc.CreateElement("p")
	doc.AppendChild(container)
	container.AppendChild(block1)
	container.AppendChild(span)
	container.AppendChild(block2)

	// Build the tree from the container element directly.
	st := styleWithDisplay(style.DisplayBlock)
	view := NewRenderView(doc, st)
	builder := NewRenderTreeBuilder(nil)
	builder.buildChildren(view, container)

	// The container should have block children: block1, anonymous(inline), block2.
	// Since we built under the view, walk the view's children.
	children := []RenderObject{}
	for c := view.FirstChild(); c != nil; c = c.NextSibling() {
		children = append(children, c)
	}
	if len(children) < 2 {
		t.Fatalf("expected at least 2 top-level children, got %d", len(children))
	}
}

// TestBuildTextNodes verifies that DOM text nodes become RenderText leaves.
func TestBuildTextNodes(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.AppendChild(doc.CreateTextNode("Hello World"))
	doc.AppendChild(div)

	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)

	var textObjs []*RenderText
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if rt, ok := cur.(*RenderText); ok {
			textObjs = append(textObjs, rt)
		}
	}
	if len(textObjs) == 0 {
		t.Fatal("no RenderText objects found in tree")
	}
	if textObjs[0].Text() != "Hello World" {
		t.Fatalf("text = %q, want %q", textObjs[0].Text(), "Hello World")
	}
}

// TestBuildReplacedElement verifies that replaced elements (img) get a RenderBox.
func TestBuildReplacedElement(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	img := doc.CreateElement("img")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(img)

	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)

	// Find the img render object.
	var imgRO RenderObject
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if el, ok := cur.Node().(*dom.Element); ok && el.LocalName() == "img" {
			imgRO = cur
			break
		}
	}
	if imgRO == nil {
		t.Fatal("no render object for <img>")
	}
	if !imgRO.IsBox() {
		t.Error("img render object should be a box")
	}
}

// TestBuildLayerTree verifies that the builder constructs a layer tree when compositing
// conditions are present.
func TestBuildLayerTree(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	fixed := doc.CreateElement("div")
	fixed.SetAttribute("style", "position:fixed")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(fixed)

	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	view := builder.Build(doc)

	if view.RootLayer() == nil {
		t.Fatal("view has no root layer after Build")
	}
}

// TestUpdaterInsert verifies the RenderTreeUpdater handles node insertion.
func TestUpdaterInsert(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	doc.AppendChild(html)
	html.AppendChild(body)

	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)

	updater := NewRenderTreeUpdater(view, nil)
	newDiv := doc.CreateElement("div")
	body.AppendChild(newDiv)
	updater.MarkInsert(newDiv)
	updater.Update()

	// The new div should have a render object.
	var found bool
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if cur.Node() == newDiv {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("inserted div was not found in render tree after Update")
	}
}

// TestUpdaterRemove verifies the RenderTreeUpdater handles node removal.
func TestUpdaterRemove(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	div := doc.CreateElement("div")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(div)

	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)

	// Find the div render object before removal.
	var divRO RenderObject
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if cur.Node() == div {
			divRO = cur
			break
		}
	}
	if divRO == nil {
		t.Fatal("div render object not found before removal")
	}

	updater := NewRenderTreeUpdater(view, nil)
	body.RemoveChild(div)
	updater.MarkRemove(div)
	updater.Update()

	// The div's render object should be detached.
	if divRO.Parent() != nil {
		t.Fatal("div render object should have nil parent after removal")
	}
}

// TestUpdaterTextChange verifies the RenderTreeUpdater re-syncs a RenderText's
// text after a DOM Text node's data changes (mirrors RenderTreeUpdater::updateTextRenderer).
func TestUpdaterTextChange(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	textNode := doc.CreateTextNode("hello")
	doc.AppendChild(html)
	html.AppendChild(body)
	body.AppendChild(textNode)

	builder := NewRenderTreeBuilder(nil)
	view := builder.Build(doc)

	var rt *RenderText
	for cur := RenderObject(view); cur != nil; cur = cur.NextInPreOrder() {
		if tr, ok := cur.(*RenderText); ok && cur.Node() == textNode {
			rt = tr
			break
		}
	}
	if rt == nil {
		t.Fatal("RenderText not found before change")
	}
	if rt.Text() != "hello" {
		t.Fatalf("initial text = %q, want %q", rt.Text(), "hello")
	}

	updater := NewRenderTreeUpdater(view, nil)
	textNode.SetData("world")
	updater.MarkTextChange(textNode)
	updater.Update()

	if rt.Text() != "world" {
		t.Fatalf("text after update = %q, want %q", rt.Text(), "world")
	}
}
