// Translation of: Source/WebCore/dom/NodeIterator.h
//                  Source/WebCore/dom/NodeIterator.cpp
// Completeness: 80%
// Simplifications:
//   - NodeIterator shares no common base with TreeWalker; the whatToShow/filter state is
//     inlined (the WebCore NodeIteratorBase/Traversal base is omitted)
//   - nodeWillBeRemoved is exposed for callers that mutate the DOM so they can keep the
//     iterator's reference valid, but the iterator does not subscribe to mutations itself
//   - the detach() method is a no-op as per the DOM specification
//   - RefCountedAndCanMakeWeakPtr is omitted; Go GC manages lifetime

package dom

// NodeIterator is the Go translation of WebCore::NodeIterator. It walks a subtree rooted
// at rootNode in document order, exposing one node at a time via NextNode/PreviousNode.
// Unlike TreeWalker it does not retain a "current" node: it keeps a reference node plus a
// flag indicating whether the logical pointer is before or after that node. This lets the
// iterator describe a position between two siblings.
type NodeIterator struct {
	root         Node
	whatToShow   uint32
	filter       nodeFilterAdapter
	reference    Node
	beforeRef    bool
}

// NewNodeIterator creates a NodeIterator rooted at root, mirroring NodeIterator::create.
// The reference node starts at root and the pointer is before it, so the first
// NextNode call yields the first accepted descendant (or root itself).
func NewNodeIterator(root Node, whatToShow uint32, filter NodeFilter) *NodeIterator {
	return &NodeIterator{
		root:       root,
		whatToShow: whatToShow,
		filter:     nodeFilterAdapter{inner: filter},
		reference:  root,
		beforeRef:  true,
	}
}

// Root returns the root node, mirroring NodeIterator::root().
func (it *NodeIterator) Root() Node { return it.root }

// WhatToShow returns the show mask, mirroring NodeIterator::whatToShow().
func (it *NodeIterator) WhatToShow() uint32 { return it.whatToShow }

// Filter returns the filter predicate, mirroring NodeIterator::filter().
func (it *NodeIterator) Filter() NodeFilter { return it.filter.inner }

// ReferenceNode returns the iterator's reference node, mirroring
// NodeIterator::referenceNode().
func (it *NodeIterator) ReferenceNode() Node { return it.reference }

// PointerBeforeReferenceNode reports whether the logical pointer is before the reference
// node, mirroring NodeIterator::pointerBeforeReferenceNode().
func (it *NodeIterator) PointerBeforeReferenceNode() bool { return it.beforeRef }

// Detach is a no-op as per the DOM specification, mirroring NodeIterator::detach().
func (it *NodeIterator) Detach() {}

// accept reports whether node should be exposed by the iterator. A nil filter accepts
// every node in the whatToShow mask; a node not in the mask is FILTER_SKIP (not REJECT)
// so that its descendants are still considered, matching the DOM specification. A user
// filter returning FilterReject causes the iterator to skip the entire subtree.
func (it *NodeIterator) accept(node Node) (NodeFilterResult, bool) {
	if !it.visible(node) {
		return FilterSkip, false
	}
	r := it.filter.AcceptNode(node)
	return r, r == FilterAccept
}

// visible reports whether node's type is in the whatToShow mask.
func (it *NodeIterator) visible(node Node) bool {
	if it.whatToShow == ShowAll {
		return true
	}
	nt := uint32(node.NodeType())
	if nt < 1 || nt > 12 {
		return false
	}
	return it.whatToShow&(1<<(nt-1)) != 0
}

// NextNode advances the pointer to the next accepted node in document order and returns
// it, mirroring NodeIterator::nextNode(). Returns nil when the end of the subtree is
// reached.
func (it *NodeIterator) NextNode() Node {
	if it.reference == nil {
		return nil
	}
	node := it.reference
	before := it.beforeRef
	// If the pointer is before the reference node, the reference node itself is the first
	// candidate; otherwise start with the next node in document order.
	if !before {
		node = nextNodeInDocumentOrder(node, it.root)
		if node == nil {
			return nil
		}
	}
	for node != nil {
		r, ok := it.accept(node)
		if ok {
			it.reference = node
			it.beforeRef = false
			return node
		}
		if r == FilterSkip {
			// Skip this node but consider its children.
			node = nextNodeInDocumentOrder(node, it.root)
			continue
		}
		// FilterReject: skip this node and its subtree. Find the next node outside the
		// subtree.
		node = nextNodeOutsideSubtree(node, it.root)
	}
	return nil
}

// PreviousNode moves the pointer backwards to the previous accepted node in document
// order and returns it, mirroring NodeIterator::previousNode().
func (it *NodeIterator) PreviousNode() Node {
	if it.reference == nil {
		return nil
	}
	node := it.reference
	// If the pointer is after the reference node, the reference node itself is the first
	// candidate; otherwise start with the previous node in document order.
	if it.beforeRef {
		node = previousNodeInDocumentOrder(node, it.root)
		if node == nil {
			return nil
		}
	}
	for node != nil {
		r, ok := it.accept(node)
		if ok {
			it.reference = node
			it.beforeRef = true
			return node
		}
		if r == FilterSkip {
			node = previousNodeInDocumentOrder(node, it.root)
			continue
		}
		// FilterReject: skip the subtree entirely; step to the previous node outside it.
		node = previousNodeOutsideSubtree(node, it.root)
	}
	return nil
}

// NodeWillBeRemoved adjusts the iterator's reference when a node is removed from the
// document, mirroring NodeIterator::nodeWillBeRemoved. It keeps the logical pointer
// pointing at the same document position by moving it to the previous (or next) node when
// the removed node is the reference.
func (it *NodeIterator) NodeWillBeRemoved(removed Node) {
	if it.reference == nil {
		return
	}
	if !it.reference.Contains(removed) {
		return
	}
	// If the removed node is the reference, move the reference to the previous node and
	// set the pointer to "after" so the next NextNode call yields the node that followed
	// the removed one.
	if it.reference == removed {
		prev := previousNodeInDocumentOrder(removed, it.root)
		if prev != nil {
			it.reference = prev
			it.beforeRef = false
		} else {
			// No previous node: anchor at root before it.
			it.reference = it.root
			it.beforeRef = true
		}
		return
	}
	// The removed node is an ancestor of the reference. Move the reference to the
	// previous node before the removed subtree.
	prev := previousNodeInDocumentOrder(removed, it.root)
	if prev != nil {
		it.reference = prev
		it.beforeRef = false
	} else {
		it.reference = it.root
		it.beforeRef = true
	}
}

// nextNodeInDocumentOrder returns the next node after node in a pre-order traversal of
// the subtree rooted at root, or nil when node is the last node in the subtree. It is the
// pre-order successor: first child, else next sibling, else the next sibling of the
// nearest ancestor (within root).
func nextNodeInDocumentOrder(node, root Node) Node {
	if node == nil {
		return nil
	}
	// First child.
	if fc := nodeBaseOf(node).firstChild; fc != nil {
		return fc
	}
	// Walk up looking for a next sibling.
	for n := node; n != nil && n != root; n = n.ParentNode() {
		if ns := nodeBaseOf(n).nextSibling; ns != nil {
			return ns
		}
	}
	return nil
}

// previousNodeInDocumentOrder returns the previous node before node in a pre-order
// traversal of the subtree rooted at root, or nil when node is the root itself.
func previousNodeInDocumentOrder(node, root Node) Node {
	if node == nil || node == root {
		return nil
	}
	// Previous sibling's last descendant, or the parent.
	ps := nodeBaseOf(node).prevSibling
	if ps == nil {
		return node.ParentNode()
	}
	// Descend to the rightmost deepest descendant of ps.
	for {
		last := nodeBaseOf(ps).lastChild
		if last == nil {
			return ps
		}
		ps = last
	}
}

// nextNodeOutsideSubtree returns the next node after node that is not inside node's
// subtree (the next sibling or an ancestor's next sibling). Used when a FilterReject
// result skips an entire subtree.
func nextNodeOutsideSubtree(node, root Node) Node {
	for n := node; n != nil && n != root; n = n.ParentNode() {
		if ns := nodeBaseOf(n).nextSibling; ns != nil {
			return ns
		}
	}
	return nil
}

// previousNodeOutsideSubtree returns the previous node before node that is not inside
// node's subtree (the parent of the subtree root, then its previous-sibling descendant).
func previousNodeOutsideSubtree(node, root Node) Node {
	if node == nil || node == root {
		return nil
	}
	parent := node.ParentNode()
	if parent == nil || parent == root {
		return nil
	}
	return previousNodeInDocumentOrder(parent, root)
}
