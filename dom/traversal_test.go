package dom

import "testing"

// localName returns the local (lowercase for HTML elements) name of n. For non-element
// nodes it returns NodeName (e.g. "#text"). Used by traversal tests to compare against
// lowercase expectations.
func localName(n Node) string {
	if el, ok := n.(*Element); ok {
		return el.LocalName()
	}
	return n.NodeName()
}

// buildTraversalTree builds a document with the structure:
//
//	root
//	├─ a (element)
//	│  ├─ a1 (element)
//	│  └─ a2 (text "x")
//	├─ b (element)
//	└─ c (element)
//	   └─ c1 (element)
//
// and returns the document, root and the named nodes.
func buildTraversalTree(t *testing.T) (*Document, *Element, *Element, *Element, *Element, *Element, *Element) {
	t.Helper()
	d := NewDocument()
	root := d.CreateElement("root")
	_ = d.AppendChild(root)
	a := d.CreateElement("a")
	_ = root.AppendChild(a)
	a1 := d.CreateElement("a1")
	_ = a.AppendChild(a1)
	a2 := d.CreateTextNode("x")
	_ = a.AppendChild(a2)
	b := d.CreateElement("b")
	_ = root.AppendChild(b)
	c := d.CreateElement("c")
	_ = root.AppendChild(c)
	c1 := d.CreateElement("c1")
	_ = c.AppendChild(c1)
	return d, root, a, a1, b, c, c1
}

// TestTreeWalkerRoot covers the basic accessor fields of a TreeWalker.
func TestTreeWalkerRoot(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	if w.Root() != root {
		t.Errorf("Root = %v, want root", w.Root())
	}
	if w.CurrentNode() != root {
		t.Errorf("CurrentNode = %v, want root", w.CurrentNode())
	}
	if w.WhatToShow() != ShowAll {
		t.Errorf("WhatToShow = %#x, want %#x", w.WhatToShow(), ShowAll)
	}
}

// TestTreeWalkerFirstChild covers FirstChild moving into the first child.
func TestTreeWalkerFirstChild(t *testing.T) {
	_, root, a, _, _, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	if got := w.FirstChild(); got != a {
		t.Errorf("FirstChild = %v, want a", got)
	}
	if w.CurrentNode() != a {
		t.Errorf("CurrentNode after FirstChild = %v, want a", w.CurrentNode())
	}
	// FirstChild again should descend into a1.
	if got := w.FirstChild(); got == nil || localName(got) != "a1" {
		t.Errorf("second FirstChild = %v, want a1", got)
	}
}

// TestTreeWalkerLastChild covers LastChild moving into the last child.
func TestTreeWalkerLastChild(t *testing.T) {
	_, root, _, _, _, c, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	if got := w.LastChild(); got != c {
		t.Errorf("LastChild = %v, want c", got)
	}
}

// TestTreeWalkerNextSibling covers NextSibling moving across siblings.
func TestTreeWalkerNextSibling(t *testing.T) {
	_, root, _, _, b, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	_ = w.FirstChild()
	if got := w.NextSibling(); got != b {
		t.Errorf("NextSibling from a = %v, want b", got)
	}
}

// TestTreeWalkerPreviousSibling covers PreviousSibling moving back across siblings.
func TestTreeWalkerPreviousSibling(t *testing.T) {
	_, root, a, _, b, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	w.SetCurrentNode(b)
	if got := w.PreviousSibling(); got != a {
		t.Errorf("PreviousSibling from b = %v, want a", got)
	}
}

// TestTreeWalkerParentNode covers ParentNode moving up to a visible ancestor.
func TestTreeWalkerParentNode(t *testing.T) {
	_, root, a, a1, _, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowAll, nil)
	w.SetCurrentNode(a1)
	if got := w.ParentNode(); got != a {
		t.Errorf("ParentNode from a1 = %v, want a", got)
	}
	// ParentNode from root should return nil (cannot walk above root).
	w.SetCurrentNode(root)
	if got := w.ParentNode(); got != nil {
		t.Errorf("ParentNode from root = %v, want nil", got)
	}
}

// TestTreeWalkerNextNode covers the forward document-order traversal.
func TestTreeWalkerNextNode(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowElement, nil)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"a", "a1", "b", "c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestTreeWalkerPreviousNode covers the backward document-order traversal.
func TestTreeWalkerPreviousNode(t *testing.T) {
	_, root, _, _, _, _, c1 := buildTraversalTree(t)
	w := NewTreeWalker(root, ShowElement, nil)
	w.SetCurrentNode(c1)
	var visited []string
	for {
		n := w.PreviousNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"c", "b", "a1", "a", "root"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestTreeWalkerWhatToShow verifies the whatToShow mask filters nodes by type.
func TestTreeWalkerWhatToShow(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	// Show only text nodes.
	w := NewTreeWalker(root, ShowText, nil)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, n.NodeName())
	}
	// Only a2 (text "x") is a text node in the tree.
	if len(visited) != 1 {
		t.Fatalf("visited %d text nodes, want 1 (%v)", len(visited), visited)
	}
	if visited[0] != "#text" {
		t.Errorf("visited[0] = %q, want %q", visited[0], "#text")
	}
}

// TestTreeWalkerFilterAccept verifies that a NodeFilter accepting specific nodes restricts
// the traversal.
func TestTreeWalkerFilterAccept(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	// Accept only nodes whose name starts with "c".
	filter := NodeFilterFunc(func(n Node) NodeFilterResult {
		name := localName(n)
		if len(name) > 0 && name[0] == 'c' {
			return FilterAccept
		}
		return FilterSkip
	})
	w := NewTreeWalker(root, ShowAll, filter)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestTreeWalkerFilterReject verifies that a NodeFilter rejecting a subtree skips it
// entirely.
func TestTreeWalkerFilterReject(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	// Reject the "a" subtree (so a1/a2 are never visited).
	filter := NodeFilterFunc(func(n Node) NodeFilterResult {
		if localName(n) == "a" {
			return FilterReject
		}
		return FilterAccept
	})
	w := NewTreeWalker(root, ShowAll, filter)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"b", "c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestNodeFilterConstants verifies the NodeFilter result and show constants match the DOM
// specification values.
func TestNodeFilterConstants(t *testing.T) {
	if FilterAccept != 1 {
		t.Errorf("FilterAccept = %d, want 1", FilterAccept)
	}
	if FilterReject != 2 {
		t.Errorf("FilterReject = %d, want 2", FilterReject)
	}
	if FilterSkip != 3 {
		t.Errorf("FilterSkip = %d, want 3", FilterSkip)
	}
	if ShowAll != 0xFFFFFFFF {
		t.Errorf("ShowAll = %#x, want 0xFFFFFFFF", ShowAll)
	}
	if ShowElement != 0x1 {
		t.Errorf("ShowElement = %#x, want 0x1", ShowElement)
	}
	if ShowText != 0x4 {
		t.Errorf("ShowText = %#x, want 0x4", ShowText)
	}
	if ShowComment != 0x80 {
		t.Errorf("ShowComment = %#x, want 0x80", ShowComment)
	}
}

// TestNodeIteratorNextNode covers the forward document-order iteration.
func TestNodeIteratorNextNode(t *testing.T) {
	_, root, _, _, _, _, c1 := buildTraversalTree(t)
	w := NewNodeIterator(root, ShowElement, nil)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"root", "a", "a1", "b", "c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
	// Reference node should be the last visited (c1).
	if w.ReferenceNode() != c1 {
		t.Errorf("ReferenceNode = %v, want c1", w.ReferenceNode())
	}
	if w.PointerBeforeReferenceNode() {
		t.Errorf("after forward iteration, pointer should be after reference")
	}
}

// TestNodeIteratorPreviousNode covers the backward iteration.
func TestNodeIteratorPreviousNode(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewNodeIterator(root, ShowElement, nil)
	// Walk forward to the end first.
	for {
		if w.NextNode() == nil {
			break
		}
	}
	var visited []string
	for {
		n := w.PreviousNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"c1", "c", "b", "a1", "a", "root"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestNodeIteratorWhatToShow verifies the whatToShow mask filters by node type.
func TestNodeIteratorWhatToShow(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewNodeIterator(root, ShowText, nil)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, n.NodeName())
	}
	if len(visited) != 1 {
		t.Fatalf("visited %d text nodes, want 1 (%v)", len(visited), visited)
	}
	if visited[0] != "#text" {
		t.Errorf("visited[0] = %q, want %q", visited[0], "#text")
	}
}

// TestNodeIteratorFilterAccept verifies that a NodeFilter accepting specific nodes
// restricts the iteration.
func TestNodeIteratorFilterAccept(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	filter := NodeFilterFunc(func(n Node) NodeFilterResult {
		name := localName(n)
		if len(name) > 0 && name[0] == 'c' {
			return FilterAccept
		}
		return FilterSkip
	})
	w := NewNodeIterator(root, ShowAll, filter)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestNodeIteratorFilterReject verifies that a rejected node's subtree is skipped
// entirely.
func TestNodeIteratorFilterReject(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	filter := NodeFilterFunc(func(n Node) NodeFilterResult {
		if localName(n) == "a" {
			return FilterReject
		}
		return FilterAccept
	})
	w := NewNodeIterator(root, ShowAll, filter)
	var visited []string
	for {
		n := w.NextNode()
		if n == nil {
			break
		}
		visited = append(visited, localName(n))
	}
	want := []string{"root", "b", "c", "c1"}
	if len(visited) != len(want) {
		t.Fatalf("visited %d nodes, want %d (%v)", len(visited), len(want), visited)
	}
	for i, name := range want {
		if visited[i] != name {
			t.Errorf("visited[%d] = %q, want %q (full: %v)", i, visited[i], name, visited)
		}
	}
}

// TestNodeIteratorDetach verifies that Detach is a no-op and does not break iteration.
func TestNodeIteratorDetach(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewNodeIterator(root, ShowAll, nil)
	w.Detach()
	if w.NextNode() == nil {
		t.Errorf("Detach should not affect iteration")
	}
}

// TestNodeIteratorRoot verifies the basic accessor fields of a NodeIterator.
func TestNodeIteratorRoot(t *testing.T) {
	_, root, _, _, _, _, _ := buildTraversalTree(t)
	w := NewNodeIterator(root, ShowElement, nil)
	if w.Root() != root {
		t.Errorf("Root = %v, want root", w.Root())
	}
	if w.WhatToShow() != ShowElement {
		t.Errorf("WhatToShow = %#x, want %#x", w.WhatToShow(), ShowElement)
	}
	if w.ReferenceNode() != root {
		t.Errorf("ReferenceNode = %v, want root", w.ReferenceNode())
	}
	if !w.PointerBeforeReferenceNode() {
		t.Errorf("fresh iterator should have pointer before reference")
	}
}
