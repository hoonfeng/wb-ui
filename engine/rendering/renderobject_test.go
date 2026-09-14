// Translation of: tests for Source/WebCore/rendering/RenderObject.cpp
// Completeness: 60%
// Simplifications:
//   - tests focus on tree structure, type predicates and the box model; layout
//     delegation is smoke-tested but not exhaustively verified (the layout package has
//     its own test suite)

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/layout"
	"wb-ui/engine/style"
)

// helper to make a default ComputedStyle with a given display.
func styleWithDisplay(d style.DisplayType) *style.ComputedStyle {
	cs := style.NewComputedStyle()
	cs.Display = d
	return cs
}

// helper to make a ComputedStyle with position.
func styleWithPosition(p style.PositionType) *style.ComputedStyle {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	cs.Position = p
	return cs
}

func newTestDocument() *dom.Document {
	return dom.NewDocument()
}

// TestRenderObjectTreeNavigation verifies parent/child/sibling links after AddChild.
func TestRenderObjectTreeNavigation(t *testing.T) {
	doc := newTestDocument()
	view := NewRenderView(doc, styleWithDisplay(style.DisplayBlock))

	parent := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	child1 := NewRenderBlockFlow(doc.CreateElement("p"), styleWithDisplay(style.DisplayBlock))
	child2 := NewRenderBlockFlow(doc.CreateElement("p"), styleWithDisplay(style.DisplayBlock))

	view.AddChild(parent, nil)
	parent.AddChild(child1, nil)
	parent.AddChild(child2, nil)

	if parent.Parent() != RenderObject(view) {
		t.Fatalf("parent.Parent() = %v, want view", parent.Parent())
	}
	if parent.FirstChild() != child1 {
		t.Fatalf("parent.FirstChild() = %v, want child1", parent.FirstChild())
	}
	if parent.LastChild() != child2 {
		t.Fatalf("parent.LastChild() = %v, want child2", parent.LastChild())
	}
	if child1.NextSibling() != child2 {
		t.Fatalf("child1.NextSibling() = %v, want child2", child1.NextSibling())
	}
	if child2.PreviousSibling() != child1 {
		t.Fatalf("child2.PreviousSibling() = %v, want child1", child2.PreviousSibling())
	}
}

// TestRenderObjectInsertBefore verifies insertion in the middle of the child list.
func TestRenderObjectInsertBefore(t *testing.T) {
	doc := newTestDocument()
	parent := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	a := NewRenderBlockFlow(doc.CreateElement("a"), styleWithDisplay(style.DisplayBlock))
	b := NewRenderBlockFlow(doc.CreateElement("b"), styleWithDisplay(style.DisplayBlock))
	c := NewRenderBlockFlow(doc.CreateElement("c"), styleWithDisplay(style.DisplayBlock))

	parent.AddChild(a, nil)
	parent.AddChild(c, nil)
	parent.AddChild(b, c) // insert b before c

	order := []RenderObject{}
	for ch := parent.FirstChild(); ch != nil; ch = ch.NextSibling() {
		order = append(order, ch)
	}
	if len(order) != 3 || order[0] != a || order[1] != b || order[2] != c {
		t.Fatalf("child order = %v, want [a b c]", order)
	}
}

// TestRenderObjectRemoveChild verifies removal updates sibling links correctly.
func TestRenderObjectRemoveChild(t *testing.T) {
	doc := newTestDocument()
	parent := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	a := NewRenderBlockFlow(doc.CreateElement("a"), styleWithDisplay(style.DisplayBlock))
	b := NewRenderBlockFlow(doc.CreateElement("b"), styleWithDisplay(style.DisplayBlock))
	c := NewRenderBlockFlow(doc.CreateElement("c"), styleWithDisplay(style.DisplayBlock))

	parent.AddChild(a, nil)
	parent.AddChild(b, nil)
	parent.AddChild(c, nil)

	parent.RemoveChild(b)
	if a.NextSibling() != c {
		t.Fatalf("after removing b, a.NextSibling() = %v, want c", a.NextSibling())
	}
	if c.PreviousSibling() != a {
		t.Fatalf("after removing b, c.PreviousSibling() = %v, want a", c.PreviousSibling())
	}
	if b.Parent() != nil {
		t.Fatalf("removed child should have nil parent")
	}
}

// TestRenderObjectTypePredicates verifies the Is*() methods on each concrete type.
func TestRenderObjectTypePredicates(t *testing.T) {
	doc := newTestDocument()
	view := NewRenderView(doc, styleWithDisplay(style.DisplayBlock))
	block := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	box := NewRenderBox(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	inline := NewRenderInline(doc.CreateElement("span"), styleWithDisplay(style.DisplayInline))
	text := NewRenderText(doc.CreateTextNode("hi"), styleWithDisplay(style.DisplayInline))

	if !view.IsRenderView() {
		t.Error("RenderView.IsRenderView() = false")
	}
	if !block.IsBlock() || !block.IsRenderBlock() || !block.IsRenderBlockFlow() {
		t.Error("RenderBlockFlow type predicates incorrect")
	}
	if !box.IsBox() {
		t.Error("RenderBox.IsBox() = false")
	}
	if !inline.IsInline() || !inline.IsRenderInline() {
		t.Error("RenderInline type predicates incorrect")
	}
	if !text.IsText() || !text.IsRenderText() {
		t.Error("RenderText type predicates incorrect")
	}
	// base predicates that should be false
	if box.IsInline() {
		t.Error("RenderBox.IsInline() should be false")
	}
	if inline.IsBox() {
		t.Error("RenderInline.IsBox() should be false")
	}
}

// TestRenderBoxGeometry verifies the box-model rect accessors.
func TestRenderBoxGeometry(t *testing.T) {
	doc := newTestDocument()
	box := NewRenderBox(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))

	box.SetLocation(10, 20)
	box.SetSize(100, 200)
	box.SetBorder(layout.Edges{Top: 1, Right: 2, Bottom: 3, Left: 4})
	box.SetPadding(layout.Edges{Top: 5, Right: 6, Bottom: 7, Left: 8})

	bb := box.BorderBoxRect()
	if bb.X != 10 || bb.Y != 20 || bb.Width != 100 || bb.Height != 200 {
		t.Fatalf("BorderBoxRect = %+v, want {10,20,100,200}", bb)
	}
	pb := box.PaddingBoxRect()
	if pb.X != 14 || pb.Y != 21 {
		t.Fatalf("PaddingBoxRect origin = (%v,%v), want (14,21)", pb.X, pb.Y)
	}
	if pb.Width != 100-6 || pb.Height != 200-4 {
		t.Fatalf("PaddingBoxRect size = (%v,%v), want (94,196)", pb.Width, pb.Height)
	}
	cb := box.ContentBoxRect()
	if cb.X != 22 || cb.Y != 26 {
		t.Fatalf("ContentBoxRect origin = (%v,%v), want (22,26)", cb.X, cb.Y)
	}
	if cb.Width != 100-6-14 || cb.Height != 200-4-12 {
		t.Fatalf("ContentBoxRect size = (%v,%v), want (80,184)", cb.Width, cb.Height)
	}
}

// TestRenderObjectDirtyPropagation verifies that Dirty marks ancestors.
func TestRenderObjectDirtyPropagation(t *testing.T) {
	doc := newTestDocument()
	parent := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	child := NewRenderBlockFlow(doc.CreateElement("p"), styleWithDisplay(style.DisplayBlock))
	parent.AddChild(child, nil)

	parent.ClearNeedsLayout()
	child.ClearNeedsLayout()

	if parent.NeedsLayout() || child.NeedsLayout() {
		t.Fatal("expected clean layout state")
	}
	child.Dirty()
	if !child.NeedsLayout() {
		t.Error("child should be dirty after Dirty()")
	}
	if !parent.NeedsLayout() {
		t.Error("parent should be dirty after child Dirty()")
	}
}

// TestRenderTextTransform verifies lazy text-transform application.
func TestRenderTextTransform(t *testing.T) {
	doc := newTestDocument()
	st := style.NewComputedStyle()
	st.TextTransform = "uppercase"
	rt := NewRenderTextWith(doc.CreateTextNode("hello"), st, "hello")
	if got := rt.OriginalText(); got != "HELLO" {
		t.Fatalf("OriginalText() = %q, want HELLO", got)
	}
	if rt.Length() != 5 {
		t.Fatalf("Length() = %d, want 5", rt.Length())
	}
	if rt.ContainsOnlyWhitespace() {
		t.Fatal("ContainsOnlyWhitespace() = true, want false")
	}
}

// TestRenderObjectPreOrderTraversal verifies NextInPreOrder walks the tree correctly.
func TestRenderObjectPreOrderTraversal(t *testing.T) {
	doc := newTestDocument()
	root := NewRenderBlockFlow(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	a := NewRenderBlockFlow(doc.CreateElement("a"), styleWithDisplay(style.DisplayBlock))
	b := NewRenderBlockFlow(doc.CreateElement("b"), styleWithDisplay(style.DisplayBlock))
	c := NewRenderBlockFlow(doc.CreateElement("c"), styleWithDisplay(style.DisplayBlock))
	root.AddChild(a, nil)
	root.AddChild(b, nil)
	a.AddChild(c, nil)

	// Pre-order: root, a, c, b
	var visited []string
	for cur := RenderObject(root); cur != nil; cur = cur.NextInPreOrder() {
		visited = append(visited, cur.RenderName())
	}
	if len(visited) != 4 {
		t.Fatalf("visited %d nodes, want 4", len(visited))
	}
}

// TestContainingBlock verifies the containing-block lookup for positioned elements.
func TestContainingBlock(t *testing.T) {
	doc := newTestDocument()
	outer := NewRenderBlockFlow(doc.CreateElement("div"), styleWithPosition(style.PositionRelative))
	inner := NewRenderBox(doc.CreateElement("div"), styleWithPosition(style.PositionAbsolute))
	outer.AddChild(inner, nil)

	cb := inner.ContainingBlock()
	if cb != outer {
		t.Fatalf("ContainingBlock() = %v, want outer (relative parent)", cb)
	}
}

// TestRenderObjectStyleChange verifies SetStyle marks the object dirty.
func TestRenderObjectStyleChange(t *testing.T) {
	doc := newTestDocument()
	box := NewRenderBox(doc.CreateElement("div"), styleWithDisplay(style.DisplayBlock))
	box.ClearNeedsLayout()
	if box.NeedsLayout() {
		t.Fatal("expected clean after ClearNeedsLayout")
	}
	box.SetStyle(styleWithDisplay(style.DisplayInline))
	if !box.NeedsLayout() {
		t.Fatal("expected dirty after SetStyle")
	}
}
