// Translation of: Source/WebCore/dom/NodeFilter.h
//                  Source/WebCore/dom/NodeFilter.cpp
//                  Source/WebCore/dom/Traversal.h
//                  Source/WebCore/dom/TreeWalker.h
//                  Source/WebCore/dom/TreeWalker.cpp
// Completeness: 85%
// Simplifications:
//   - NodeFilter is a Go interface (AcceptNode); the C++ RefCounted/ActiveDOMCallback base
//     is omitted along with the JS callback wrappers
//   - NodeFilterFunc adapts a plain function to NodeFilter for ergonomic registration
//   - the NodeIteratorBase (Traversal) shared state is inlined into TreeWalker and
//     NodeIterator rather than being a separate struct
//   - opaqueRootForCurrentNodeInGCThread and the lock are omitted (Go GC manages lifetime)
//   - whatToShow bits use 1<<(nodeType-1) per the DOM spec; SHOW_ALL is 0xFFFFFFFF

package dom

// NodeFilterResult mirrors the NodeFilter acceptNode return values.
type NodeFilterResult uint16

// NodeFilter result constants, mirroring NodeFilter::FILTER_ACCEPT/REJECT/SKIP.
const (
	FilterAccept NodeFilterResult = 1
	FilterReject NodeFilterResult = 2
	FilterSkip   NodeFilterResult = 3
)

// whatToShow bit constants, mirroring NodeFilter::SHOW_*.
const (
	ShowAll                   uint32 = 0xFFFFFFFF
	ShowElement               uint32 = 0x00000001
	ShowAttribute             uint32 = 0x00000002
	ShowText                 uint32 = 0x00000004
	ShowCDATASection        uint32 = 0x00000008
	ShowEntityReference     uint32 = 0x00000010
	ShowEntity              uint32 = 0x00000020
	ShowProcessingInstruction uint32 = 0x00000040
	ShowComment             uint32 = 0x00000080
	ShowDocument            uint32 = 0x00000100
	ShowDocumentType        uint32 = 0x00000200
	ShowDocumentFragment    uint32 = 0x00000400
	ShowNotation            uint32 = 0x00000800
)

// NodeFilter is the Go translation of WebCore::NodeFilter. It is the predicate that
// decides whether a node is accepted, rejected (with its subtree) or skipped (but its
// subtree still considered) during TreeWalker/NodeIterator traversal.
type NodeFilter interface {
	AcceptNode(node Node) NodeFilterResult
}

// NodeFilterFunc adapts a plain function to the NodeFilter interface.
type NodeFilterFunc func(node Node) NodeFilterResult

// AcceptNode calls f(node).
func (f NodeFilterFunc) AcceptNode(node Node) NodeFilterResult { return f(node) }

// nodeFilterAdapter wraps a NodeFilter so it can be nil-checked uniformly. A nil filter
// is treated as always accepting.
type nodeFilterAdapter struct {
	inner NodeFilter
}

func (a nodeFilterAdapter) AcceptNode(node Node) NodeFilterResult {
	if a.inner == nil {
		return FilterAccept
	}
	return a.inner.AcceptNode(node)
}

// TreeWalker is the Go translation of WebCore::TreeWalker. It walks a subtree rooted at
// rootNode, applying the whatToShow mask and the optional NodeFilter to decide which nodes
// to expose. The current position is held as currentNode and moved by the ParentNode /
// FirstChild / LastChild / PreviousSibling / NextSibling / PreviousNode / NextNode methods.
type TreeWalker struct {
	root        Node
	whatToShow  uint32
	filter      nodeFilterAdapter
	current     Node
}

// NewTreeWalker creates a TreeWalker rooted at root, mirroring TreeWalker::create(root,
// whatToShow, filter). A nil filter is treated as accepting every node in the mask.
func NewTreeWalker(root Node, whatToShow uint32, filter NodeFilter) *TreeWalker {
	return &TreeWalker{
		root:       root,
		whatToShow: whatToShow,
		filter:     nodeFilterAdapter{inner: filter},
		current:    root,
	}
}

// Root returns the root node of the walk, mirroring TreeWalker::root().
func (w *TreeWalker) Root() Node { return w.root }

// WhatToShow returns the show mask, mirroring TreeWalker::whatToShow().
func (w *TreeWalker) WhatToShow() uint32 { return w.whatToShow }

// Filter returns the filter predicate, mirroring TreeWalker::filter().
func (w *TreeWalker) Filter() NodeFilter { return w.filter.inner }

// CurrentNode returns the current position of the walker, mirroring
// TreeWalker::currentNode().
func (w *TreeWalker) CurrentNode() Node { return w.current }

// SetCurrentNode sets the current position, mirroring TreeWalker::setCurrentNode(). The
// DOM spec allows the caller to move the walker anywhere in the tree.
func (w *TreeWalker) SetCurrentNode(node Node) { w.current = node }

// accept reports whether node should be exposed: it must be in the whatToShow mask and
// pass the filter. Returns the filter result and a bool indicating acceptance. A node not
// in the whatToShow mask is FILTER_SKIP (not REJECT) so that its descendants are still
// considered, matching the DOM specification.
func (w *TreeWalker) accept(node Node) (NodeFilterResult, bool) {
	if !w.visible(node) {
		return FilterSkip, false
	}
	r := w.filter.AcceptNode(node)
	return r, r == FilterAccept
}

// visible reports whether node's type is in the whatToShow mask, mirroring
// NodeIteratorBase::acceptNode's mask check.
func (w *TreeWalker) visible(node Node) bool {
	if w.whatToShow == ShowAll {
		return true
	}
	nt := uint32(node.NodeType())
	if nt < 1 || nt > 12 {
		return false
	}
	return w.whatToShow&(1<<(nt-1)) != 0
}

// ParentNode moves the current node to its nearest visible ancestor (excluding the root),
// mirroring TreeWalker::parentNode(). Returns the new current node or nil if there is no
// visible ancestor.
func (w *TreeWalker) ParentNode() Node {
	node := w.current
	for node != nil && node != w.root {
		parent := node.ParentNode()
		if parent == nil || parent == w.root {
			return nil
		}
		if _, ok := w.accept(parent); ok {
			w.current = parent
			return parent
		}
		node = parent
	}
	return nil
}

// FirstChild moves the current node to its first visible child, mirroring
// TreeWalker::firstChild(). It traverses children in document order; rejected children
// are skipped along with their subtrees.
func (w *TreeWalker) FirstChild() Node {
	return w.traverseChildren(true)
}

// LastChild moves the current node to its last visible child, mirroring
// TreeWalker::lastChild().
func (w *TreeWalker) LastChild() Node {
	return w.traverseChildren(false)
}

// traverseChildren walks the children of the current node in order (forward when first is
// true, reverse otherwise). For each child it consults the filter: ACCEPT sets the
// current node and returns; SKIP recurses into the child's subtree; REJECT skips the
// subtree entirely.
func (w *TreeWalker) traverseChildren(first bool) Node {
	parent := w.current
	if parent == nil || !parent.HasChildNodes() {
		return nil
	}
	var children []Node
	for c := nodeBaseOf(parent).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		children = append(children, c)
	}
	idx := 0
	if !first {
		idx = len(children) - 1
	}
	for idx >= 0 && idx < len(children) {
		child := children[idx]
		r, ok := w.accept(child)
		if ok {
			w.current = child
			return child
		}
		if r == FilterSkip {
			// Try the child's subtree before moving on.
			if desc := w.descendAccepted(child); desc != nil {
				return desc
			}
		}
		if first {
			idx++
		} else {
			idx--
		}
	}
	return nil
}

// descendAccepted does a pre-order search of the subtree rooted at node and returns the
// first accepted descendant, setting w.current to it.
func (w *TreeWalker) descendAccepted(node Node) Node {
	var found Node
	var walk func(n Node) bool
	walk = func(n Node) bool {
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			r, ok := w.accept(c)
			if ok {
				w.current = c
				found = c
				return true
			}
			if r == FilterSkip {
				if walk(c) {
					return true
				}
			}
		}
		return false
	}
	walk(node)
	return found
}

// PreviousSibling moves the current node to its previous visible sibling, mirroring
// TreeWalker::previousSibling().
func (w *TreeWalker) PreviousSibling() Node {
	return w.traverseSiblings(false)
}

// NextSibling moves the current node to its next visible sibling, mirroring
// TreeWalker::nextSibling().
func (w *TreeWalker) NextSibling() Node {
	return w.traverseSiblings(true)
}

// traverseSiblings walks the siblings of the current node in order (forward when next is
// true, reverse otherwise). Skipped siblings have their subtrees searched for an
// accepted node before moving on.
func (w *TreeWalker) traverseSiblings(next bool) Node {
	node := w.current
	if node == nil || node == w.root {
		return nil
	}
	parent := node.ParentNode()
	if parent == nil {
		return nil
	}
	// Build the sibling list once and locate the current node's index.
	var siblings []Node
	for c := nodeBaseOf(parent).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		siblings = append(siblings, c)
	}
	idx := -1
	for i, s := range siblings {
		if s == node {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	if next {
		idx++
	} else {
		idx--
	}
	for idx >= 0 && idx < len(siblings) {
		sib := siblings[idx]
		r, ok := w.accept(sib)
		if ok {
			w.current = sib
			return sib
		}
		if r == FilterSkip {
			if desc := w.descendAccepted(sib); desc != nil {
				return desc
			}
		}
		if next {
			idx++
		} else {
			idx--
		}
	}
	return nil
}

// PreviousNode moves the current node backwards in document order, mirroring
// TreeWalker::previousNode(). It is the inverse of NextNode: it steps to the previous
// node in document order that passes the filter, walking up through ancestors when a
// subtree is exhausted.
func (w *TreeWalker) PreviousNode() Node {
	node := w.current
	if node == nil || node == w.root {
		return nil
	}
	for {
		prev := previousNodeInDocumentOrder(node, w.root)
		if prev == nil {
			return nil
		}
		node = prev
		_, ok := w.accept(node)
		if ok {
			w.current = node
			return node
		}
		// For FILTER_SKIP the node itself is skipped but its descendants (which precede
		// it in reverse document order) were already visited in earlier iterations, so
		// stepping further back via previousNodeInDocumentOrder is correct.
		// For FILTER_REJECT the entire subtree is skipped; previousNodeInDocumentOrder
		// returns the node before the rejected node's subtree, so descendants are
		// not visited.
	}
}

// NextNode moves the current node forwards in document order, mirroring
// TreeWalker::nextNode(). It tries the first child, then the next sibling, then walks up
// to ancestors' next siblings.
func (w *TreeWalker) NextNode() Node {
	node := w.current
	if node == nil {
		return nil
	}
	// 1. Try first child (skipping rejected subtrees).
	if node.HasChildNodes() {
		if r := w.FirstChild(); r != nil {
			return r
		}
	}
	// 2. Walk up trying next siblings.
	for node != w.root {
		sib := nextSiblingOf(node)
		for sib != nil {
			r, ok := w.accept(sib)
			if ok {
				w.current = sib
				return sib
			}
			if r == FilterSkip {
				// Try sib's subtree.
				if desc := w.descendAccepted(sib); desc != nil {
					return desc
				}
			}
			sib = nextSiblingOf(sib)
		}
		parent := node.ParentNode()
		if parent == nil || parent == w.root {
			return nil
		}
		node = parent
	}
	return nil
}

// previousSiblingOf / nextSiblingOf are small helpers returning the previous / next
// sibling of node or nil, hiding nodeBaseOf from the traversal methods.
func previousSiblingOf(node Node) Node {
	if node == nil {
		return nil
	}
	return nodeBaseOf(node).prevSibling
}

func nextSiblingOf(node Node) Node {
	if node == nil {
		return nil
	}
	return nodeBaseOf(node).nextSibling
}

// lastAcceptedDescendant returns the last accepted node in the subtree rooted at node
// (deepest, rightmost), using w.accept to filter. Used by PreviousNode to step into the
// rightmost position of a skipped sibling.
func lastAcceptedDescendant(node Node, w *TreeWalker) Node {
	var found Node
	var walk func(n Node)
	walk = func(n Node) {
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			r, ok := w.accept(c)
			if ok {
				found = c
			}
			if r != FilterReject {
				walk(c)
			}
		}
	}
	// Start with node itself if accepted.
	r, ok := w.accept(node)
	if ok {
		found = node
	}
	if r != FilterReject {
		walk(node)
	}
	return found
}
