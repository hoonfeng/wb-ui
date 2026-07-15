// Translation of: Source/WebCore/dom/Range.h
//                  Source/WebCore/dom/Range.cpp
//                  Source/WebCore/dom/AbstractRange.h
//                  Source/WebCore/dom/RangeBoundaryPoint.h
// Completeness: 70%
// Simplifications:
//   - Range embeds no AbstractRange base; the start/end boundary state is held directly
//   - boundary offsets count Unicode runes rather than UTF-16 code units (consistent with
//     the characterDataBase simplification)
//   - getClientRects / getBoundingClientRect / createContextualFragment / expand are
//     omitted (no layout or fragment-parser integration in this port)
//   - mutation observer integration (nodeWillBeRemoved / textNodesMerged / ...) is omitted;
//     the Range does not auto-adjust when the DOM is mutated outside the Range API
//   - the document-owner plumbing is reduced to storing the document passed at creation

package dom

import "strings"

// CompareHow mirrors WebCore::Range::CompareHow.
type CompareHow uint16

// CompareBoundaryPoints how constants, mirroring Range::START_TO_START/...
const (
	StartToStart CompareHow = 0
	StartToEnd   CompareHow = 1
	EndToEnd     CompareHow = 2
	EndToStart   CompareHow = 3
)

// CompareResult mirrors WebCore::Range::CompareResults.
type CompareResult uint8

// Node comparison results, mirroring Range::NODE_BEFORE/...
const (
	NodeBefore           CompareResult = 0
	NodeAfter            CompareResult = 1
	NodeBeforeAndAfter   CompareResult = 2
	NodeInside           CompareResult = 3
)

// Range is the Go translation of WebCore::Range. It represents a contiguous span of DOM
// content delimited by a (startContainer, startOffset) / (endContainer, endOffset) pair
// of boundary points. Offsets count children for container nodes and runes for
// character-data nodes, matching the DOM specification.
type Range struct {
	ownerDoc       *Document
	startContainer Node
	startOffset    int
	endContainer   Node
	endOffset      int
}

// NewRange creates a Range anchored at the document root, mirroring Range::create(doc).
// Both boundary points start at (document, 0) so the range is collapsed.
func NewRange(doc *Document) *Range {
	return &Range{
		ownerDoc:       doc,
		startContainer: doc,
		startOffset:    0,
		endContainer:   doc,
		endOffset:      0,
	}
}

// StartContainer returns the node containing the start boundary point, mirroring
// Range::startContainer().
func (r *Range) StartContainer() Node { return r.startContainer }

// StartOffset returns the offset of the start boundary point, mirroring
// Range::startOffset().
func (r *Range) StartOffset() int { return r.startOffset }

// EndContainer returns the node containing the end boundary point, mirroring
// Range::endContainer().
func (r *Range) EndContainer() Node { return r.endContainer }

// EndOffset returns the offset of the end boundary point, mirroring Range::endOffset().
func (r *Range) EndOffset() int { return r.endOffset }

// Collapsed reports whether the range is empty (start == end), mirroring
// Range::collapsed().
func (r *Range) Collapsed() bool {
	return r.startContainer == r.endContainer && r.startOffset == r.endOffset
}

// CommonAncestorContainer returns the nearest inclusive ancestor of both boundary
// containers, mirroring Range::commonAncestorContainer().
func (r *Range) CommonAncestorContainer() Node {
	return commonAncestor(r.startContainer, r.endContainer)
}

// SetStart sets the start boundary point, mirroring Range::setStart(node, offset).
// If the new start is after the existing end, the end is moved to the new start so the
// range stays well-formed.
func (r *Range) SetStart(node Node, offset int) error {
	if err := checkNodeOffsetPair(node, offset); err != nil {
		return err
	}
	r.startContainer = node
	r.startOffset = offset
	if compareBoundaryPoints(r.startContainer, r.startOffset, r.endContainer, r.endOffset) > 0 {
		r.endContainer = node
		r.endOffset = offset
	}
	return nil
}

// SetEnd sets the end boundary point, mirroring Range::setEnd(node, offset).
func (r *Range) SetEnd(node Node, offset int) error {
	if err := checkNodeOffsetPair(node, offset); err != nil {
		return err
	}
	r.endContainer = node
	r.endOffset = offset
	if compareBoundaryPoints(r.startContainer, r.startOffset, r.endContainer, r.endOffset) > 0 {
		r.startContainer = node
		r.startOffset = offset
	}
	return nil
}

// SetStartBefore places the start boundary immediately before node, mirroring
// Range::setStartBefore(node).
func (r *Range) SetStartBefore(node Node) error {
	parent := node.ParentNode()
	if parent == nil {
		return ErrInvalidNodeType
	}
	return r.SetStart(parent, indexOfChild(parent, node))
}

// SetStartAfter places the start boundary immediately after node, mirroring
// Range::setStartAfter(node).
func (r *Range) SetStartAfter(node Node) error {
	parent := node.ParentNode()
	if parent == nil {
		return ErrInvalidNodeType
	}
	return r.SetStart(parent, indexOfChild(parent, node)+1)
}

// SetEndBefore places the end boundary immediately before node, mirroring
// Range::setEndBefore(node).
func (r *Range) SetEndBefore(node Node) error {
	parent := node.ParentNode()
	if parent == nil {
		return ErrInvalidNodeType
	}
	return r.SetEnd(parent, indexOfChild(parent, node))
}

// SetEndAfter places the end boundary immediately after node, mirroring
// Range::setEndAfter(node).
func (r *Range) SetEndAfter(node Node) error {
	parent := node.ParentNode()
	if parent == nil {
		return ErrInvalidNodeType
	}
	return r.SetEnd(parent, indexOfChild(parent, node)+1)
}

// Collapse moves the boundary points to a single position, mirroring Range::collapse().
// When toStart is true the end is moved to the start; otherwise the start is moved to
// the end.
func (r *Range) Collapse(toStart bool) {
	if toStart {
		r.endContainer = r.startContainer
		r.endOffset = r.startOffset
	} else {
		r.startContainer = r.endContainer
		r.startOffset = r.endOffset
	}
}

// SelectNode sets the range to cover exactly node, mirroring Range::selectNode(node).
func (r *Range) SelectNode(node Node) error {
	parent := node.ParentNode()
	if parent == nil {
		return ErrInvalidNodeType
	}
	idx := indexOfChild(parent, node)
	r.startContainer = parent
	r.startOffset = idx
	r.endContainer = parent
	r.endOffset = idx + 1
	return nil
}

// SelectNodeContents sets the range to cover the contents of node, mirroring
// Range::selectNodeContents(node).
func (r *Range) SelectNodeContents(node Node) error {
	if !nodeBaseOf(node).canHaveChildren() {
		return ErrInvalidNodeType
	}
	r.startContainer = node
	r.startOffset = 0
	r.endContainer = node
	r.endOffset = nodeBaseOf(node).countChildren()
	return nil
}

// CompareBoundaryPoints compares a boundary point of this range with one of sourceRange,
// mirroring Range::compareBoundaryPoints(how, sourceRange). Returns -1, 0 or +1.
func (r *Range) CompareBoundaryPoints(how CompareHow, source *Range) (int, error) {
	var thisC Node
	var thisO int
	var otherC Node
	var otherO int
	switch how {
	case StartToStart:
		thisC, thisO = r.startContainer, r.startOffset
		otherC, otherO = source.startContainer, source.startOffset
	case StartToEnd:
		thisC, thisO = r.startContainer, r.startOffset
		otherC, otherO = source.endContainer, source.endOffset
	case EndToEnd:
		thisC, thisO = r.endContainer, r.endOffset
		otherC, otherO = source.endContainer, source.endOffset
	case EndToStart:
		thisC, thisO = r.endContainer, r.endOffset
		otherC, otherO = source.startContainer, source.startOffset
	default:
		return 0, ErrNotSupported
	}
	cmp := compareBoundaryPoints(thisC, thisO, otherC, otherO)
	switch {
	case cmp < 0:
		return -1, nil
	case cmp > 0:
		return 1, nil
	default:
		return 0, nil
	}
}

// CloneRange returns a copy of the range, mirroring Range::cloneRange().
func (r *Range) CloneRange() *Range {
	cp := *r
	return &cp
}

// Detach is a no-op as per the DOM specification, mirroring Range::detach().
func (r *Range) Detach() {}

// IsPointInRange reports whether (node, offset) lies within the range, mirroring
// Range::isPointInRange(node, offset).
func (r *Range) IsPointInRange(node Node, offset int) (bool, error) {
	if err := checkNodeOffsetPair(node, offset); err != nil {
		return false, err
	}
	if compareBoundaryPoints(node, offset, r.startContainer, r.startOffset) < 0 {
		return false, nil
	}
	if compareBoundaryPoints(node, offset, r.endContainer, r.endOffset) > 0 {
		return false, nil
	}
	return true, nil
}

// ComparePoint reports the position of (node, offset) relative to the range, mirroring
// Range::comparePoint(node, offset). Returns -1 (before), 0 (inside) or +1 (after).
func (r *Range) ComparePoint(node Node, offset int) (int, error) {
	if err := checkNodeOffsetPair(node, offset); err != nil {
		return 0, err
	}
	if compareBoundaryPoints(node, offset, r.startContainer, r.startOffset) < 0 {
		return -1, nil
	}
	if compareBoundaryPoints(node, offset, r.endContainer, r.endOffset) > 0 {
		return 1, nil
	}
	return 0, nil
}

// IntersectsNode reports whether the range intersects any part of node's subtree,
// mirroring Range::intersectsNode(node).
func (r *Range) IntersectsNode(node Node) bool {
	parent := node.ParentNode()
	if parent == nil {
		return false
	}
	idx := indexOfChild(parent, node)
	startCmp := compareBoundaryPoints(parent, idx, r.endContainer, r.endOffset)
	endCmp := compareBoundaryPoints(parent, idx+1, r.startContainer, r.startOffset)
	return startCmp < 0 && endCmp > 0
}

// CompareNode reports the position of node relative to the range, mirroring
// Range::compareNode(node).
func (r *Range) CompareNode(node Node) (CompareResult, error) {
	parent := node.ParentNode()
	if parent == nil {
		return NodeBeforeAndAfter, ErrInvalidNodeType
	}
	idx := indexOfChild(parent, node)
	startCmp := compareBoundaryPoints(parent, idx, r.endContainer, r.endOffset)
	endCmp := compareBoundaryPoints(parent, idx+1, r.startContainer, r.startOffset)
	switch {
	case startCmp >= 0:
		return NodeAfter, nil
	case endCmp <= 0:
		return NodeBefore, nil
	case startCmp < 0 && endCmp > 0:
		return NodeInside, nil
	}
	return NodeBeforeAndAfter, nil
}

// DeleteContents removes the range's contents from the document, mirroring
// Range::deleteContents().
func (r *Range) DeleteContents() error {
	return r.processContents(actionDelete, nil)
}

// ExtractContents removes the range's contents and returns them in a DocumentFragment,
// mirroring Range::extractContents().
func (r *Range) ExtractContents() (*DocumentFragment, error) {
	frag := NewDocumentFragment(r.ownerDoc)
	if err := r.processContents(actionExtract, frag); err != nil {
		return nil, err
	}
	return frag, nil
}

// CloneContents returns a DocumentFragment containing clones of the range's contents,
// mirroring Range::cloneContents().
func (r *Range) CloneContents() (*DocumentFragment, error) {
	frag := NewDocumentFragment(r.ownerDoc)
	if err := r.processContents(actionClone, frag); err != nil {
		return nil, err
	}
	return frag, nil
}

// InsertNode inserts node at the start of the range, mirroring Range::insertNode(node).
// For a character-data start container the node's data is spliced at the start offset;
// otherwise the node is inserted as a child of the start container at the start offset.
func (r *Range) InsertNode(node Node) error {
	if r.startContainer.IsCharacterDataNode() {
		cd := r.startContainer.(characterDataLike)
		cd.InsertData(r.startOffset, node.NodeValue())
		if r.endContainer == r.startContainer {
			r.endOffset += len([]rune(node.NodeValue()))
		}
		return nil
	}
	container := r.startContainer
	if !nodeBaseOf(container).canHaveChildren() {
		return ErrHierarchyRequest
	}
	child := nodeBaseOf(container).childAt(r.startOffset)
	if child == nil {
		return container.AppendChild(node)
	}
	return container.InsertBefore(node, child)
}

// SurroundContents moves the range's contents into node and replaces the range with node,
// mirroring Range::surroundContents(node). The range must not partially contain a
// non-Text node.
func (r *Range) SurroundContents(node Node) error {
	if r.startContainer != r.endContainer && r.startContainer != r.CommonAncestorContainer() {
		return ErrInvalidNodeType
	}
	frag, err := r.ExtractContents()
	if err != nil {
		return err
	}
	if err := node.AppendChild(frag); err != nil {
		return err
	}
	if err := r.InsertNode(node); err != nil {
		return err
	}
	return nil
}

// ToString returns the concatenation of text nodes inside the range, mirroring
// Range::toString().
func (r *Range) ToString() string {
	var sb strings.Builder
	r.walkContainedText(func(text Node, start, end int) {
		t := text.(*Text)
		sb.WriteString(t.SubstringData(start, end-start))
	})
	return sb.String()
}

// characterDataLike is the internal interface used by InsertNode to splice data into a
// character-data start container without type-asserting to a concrete type.
type characterDataLike interface {
	InsertData(offset int, s string)
}

// rangeAction mirrors Range::ActionType.
type rangeAction uint8

const (
	actionDelete rangeAction = iota
	actionExtract
	actionClone
)

// walkContainedText invokes fn for each Text node wholly or partially contained by the
// range, with the [start, end) rune range that lies inside the range. It is the
// text-collection helper used by ToString.
func (r *Range) walkContainedText(fn func(text Node, start, end int)) {
	walkSubtreeBetween(r, func(n Node) {
		if n.NodeType() != NodeText {
			return
		}
		t := n.(*Text)
		start := 0
		end := t.Length()
		if n == r.startContainer {
			start = r.startOffset
		}
		if n == r.endContainer {
			end = r.endOffset
		}
		if start >= end {
			return
		}
		fn(n, start, end)
	})
}

// walkSubtreeBetween walks the subtree between the boundary points of r and invokes fn
// for each visited node, mirroring the partially-contained-node iteration in
// Range::processContents.
func walkSubtreeBetween(r *Range, fn func(Node)) {
	// Find the common ancestor and walk only the relevant slice of its subtree.
	common := r.CommonAncestorContainer()
	if common == nil {
		return
	}
	// Walk in document order; for each node decide whether it is inside the range using
	// compareBoundaryPoints.
	var walk func(n Node)
	walk = func(n Node) {
		// Determine containment of n.
		if n != common {
			if isNodeBeforeBoundary(n, r.startContainer, r.startOffset) {
				// n lies entirely before the range start: skip the subtree.
				return
			}
			if isNodeAfterBoundary(n, r.endContainer, r.endOffset) {
				// n lies entirely after the range end: skip the subtree.
				return
			}
		}
		fn(n)
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			walk(c)
		}
	}
	walk(common)
}

// isNodeBeforeBoundary reports whether node n lies entirely before the boundary point
// (boundaryC, boundaryO) in document order.
func isNodeBeforeBoundary(n, boundaryC Node, boundaryO int) bool {
	parent := n.ParentNode()
	if parent == nil {
		return false
	}
	idx := indexOfChild(parent, n)
	cmp := compareBoundaryPoints(parent, idx+1, boundaryC, boundaryO)
	return cmp <= 0
}

// isNodeAfterBoundary reports whether node n lies entirely after the boundary point.
func isNodeAfterBoundary(n, boundaryC Node, boundaryO int) bool {
	parent := n.ParentNode()
	if parent == nil {
		return false
	}
	idx := indexOfChild(parent, n)
	cmp := compareBoundaryPoints(parent, idx, boundaryC, boundaryO)
	return cmp >= 0
}

// processContents implements Delete/Extract/Clone, mirroring
// Range::processContents(action). It walks the contained subtree, deletes/extracts/clones
// the partially-contained boundary nodes and the fully-contained interior nodes.
func (r *Range) processContents(action rangeAction, frag *DocumentFragment) error {
	// Simple cases: collapsed range or a single character-data container.
	if r.Collapsed() {
		return nil
	}
	if r.startContainer == r.endContainer {
		if r.startContainer.IsCharacterDataNode() {
			switch action {
			case actionDelete:
				if t, ok := r.startContainer.(*Text); ok {
					t.DeleteData(r.startOffset, r.endOffset-r.startOffset)
				} else if t, ok := r.startContainer.(*Comment); ok {
					t.DeleteData(r.startOffset, r.endOffset-r.startOffset)
				}
			case actionExtract, actionClone:
				if t, ok := r.startContainer.(*Text); ok {
					frag.AppendChild(NewText(r.ownerDoc, t.SubstringData(r.startOffset, r.endOffset-r.startOffset)))
					if action == actionExtract {
						t.DeleteData(r.startOffset, r.endOffset-r.startOffset)
					}
				}
			}
			return nil
		}
		// Container with children: splice out the children in [startOffset, endOffset).
		children := nodeBaseOf(r.startContainer).childrenInRange(r.startOffset, r.endOffset)
		for _, c := range children {
			switch action {
			case actionDelete:
				_ = r.startContainer.RemoveChild(c)
			case actionExtract:
				frag.AppendChild(c)
			case actionClone:
				frag.AppendChild(c.CloneNode(true))
			}
		}
		return nil
	}
	// General case: walk the contained nodes and apply the action.
	walkSubtreeBetween(r, func(n Node) {
		if n == r.startContainer && r.startContainer.IsCharacterDataNode() {
			if t, ok := n.(*Text); ok {
				switch action {
				case actionDelete:
					t.DeleteData(r.startOffset, t.Length()-r.startOffset)
				case actionExtract:
					frag.AppendChild(NewText(r.ownerDoc, t.SubstringData(r.startOffset, t.Length()-r.startOffset)))
					t.DeleteData(r.startOffset, t.Length()-r.startOffset)
				case actionClone:
					frag.AppendChild(NewText(r.ownerDoc, t.SubstringData(r.startOffset, t.Length()-r.startOffset)))
				}
			}
			return
		}
		if n == r.endContainer && r.endContainer.IsCharacterDataNode() {
			if t, ok := n.(*Text); ok {
				switch action {
				case actionDelete:
					t.DeleteData(0, r.endOffset)
				case actionExtract:
					frag.AppendChild(NewText(r.ownerDoc, t.SubstringData(0, r.endOffset)))
					t.DeleteData(0, r.endOffset)
				case actionClone:
					frag.AppendChild(NewText(r.ownerDoc, t.SubstringData(0, r.endOffset)))
				}
			}
			return
		}
		// Fully contained interior node.
		if isFullyContained(n, r) {
			switch action {
			case actionDelete:
				if p := n.ParentNode(); p != nil {
					_ = p.RemoveChild(n)
				}
			case actionExtract:
				if p := n.ParentNode(); p != nil {
					_ = p.RemoveChild(n)
				}
				frag.AppendChild(n)
			case actionClone:
				frag.AppendChild(n.CloneNode(true))
			}
		}
	})
	return nil
}

// isFullyContained reports whether node n is entirely inside the range, mirroring the
// partially-contained check in Range::processContents.
func isFullyContained(n Node, r *Range) bool {
	if n == r.startContainer || n == r.endContainer {
		return false
	}
	parent := n.ParentNode()
	if parent == nil {
		return false
	}
	idx := indexOfChild(parent, n)
	startCmp := compareBoundaryPoints(parent, idx, r.endContainer, r.endOffset)
	endCmp := compareBoundaryPoints(parent, idx+1, r.startContainer, r.startOffset)
	return startCmp < 0 && endCmp > 0
}

// countChildren returns the number of children of b, mirroring the child-count used by
// Range::selectNodeContents and the boundary-point offset checks. It is defined here on
// *nodeBase because the Range helpers below need it.
func (b *nodeBase) countChildren() int {
	count := 0
	for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		count++
	}
	return count
}

// childAt returns the child at index i, or nil when i is out of range. Used by
// Range::insertNode to find the child to insert before.
func (b *nodeBase) childAt(i int) Node {
	idx := 0
	for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if idx == i {
			return c
		}
		idx++
	}
	return nil
}

// childrenInRange returns the children with indices in [start, end), used by
// Range::processContents to splice out a slice of a single container.
func (b *nodeBase) childrenInRange(start, end int) []Node {
	var out []Node
	idx := 0
	for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if idx >= start && idx < end {
			out = append(out, c)
		}
		idx++
	}
	return out
}

// checkNodeOffsetPair validates that offset is a legal index into node, mirroring
// Range::checkNodeOffsetPair.
func checkNodeOffsetPair(node Node, offset int) error {
	if node == nil {
		return ErrNotFound
	}
	if offset < 0 {
		return ErrIndexSize
	}
	if node.IsCharacterDataNode() {
		if t, ok := node.(*Text); ok {
			if offset > t.Length() {
				return ErrIndexSize
			}
			return nil
		}
		if c, ok := node.(*Comment); ok {
			if offset > c.Length() {
				return ErrIndexSize
			}
			return nil
		}
	}
	if !nodeBaseOf(node).canHaveChildren() {
		if offset != 0 {
			return ErrIndexSize
		}
		return nil
	}
	if offset > nodeBaseOf(node).countChildren() {
		return ErrIndexSize
	}
	return nil
}

// indexOfChild returns the index of child in parent's child list, or -1 when not present.
func indexOfChild(parent, child Node) int {
	idx := 0
	for c := nodeBaseOf(parent).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if c == child {
			return idx
		}
		idx++
	}
	return -1
}

// compareBoundaryPoints returns -1/0/+1 comparing the position of (c1, o1) and (c2, o2)
// in document order, mirroring the boundary-point comparison in Range.cpp. A result of -1
// means (c1, o1) comes before (c2, o2); +1 means after; 0 means they are equal.
//
// The algorithm follows the DOM specification: same-container compares offsets directly;
// when one container is an ancestor of the other, the index of the child on the path to
// the deeper container is compared against the ancestor's offset; otherwise the child
// indices under the common ancestor decide the order.
func compareBoundaryPoints(c1 Node, o1 int, c2 Node, o2 int) int {
	if c1 == c2 {
		switch {
		case o1 < o2:
			return -1
		case o1 > o2:
			return 1
		default:
			return 0
		}
	}
	// Case 1: c1 is an ancestor of c2 (c1 contains c2). Find the direct child of c1 on
	// the path to c2 and compare its index against o1.
	if c1.Contains(c2) {
		child := c2
		for nodeBaseOf(child).parentNode != c1 {
			child = child.ParentNode()
		}
		idx := indexOfChild(c1, child)
		switch {
		case idx < o1:
			return 1 // c2 is inside child[idx] which is before boundary point 1
		case idx > o1:
			return -1 // c2 is inside child[idx] which is after boundary point 1
		default:
			return -1 // boundary point 1 is just before child[idx]; c2 is inside it
		}
	}
	// Case 2: c2 is an ancestor of c1 (c2 contains c1). Symmetric to case 1.
	if c2.Contains(c1) {
		child := c1
		for nodeBaseOf(child).parentNode != c2 {
			child = child.ParentNode()
		}
		idx := indexOfChild(c2, child)
		switch {
		case idx < o2:
			return -1
		case idx > o2:
			return 1
		default:
			return 1
		}
	}
	// Case 3: neither is an ancestor of the other. Find the common ancestor and compare
	// the indices of the direct children leading to each container.
	anc := commonAncestor(c1, c2)
	if anc == nil {
		return 0 // disconnected trees
	}
	child1 := c1
	for nodeBaseOf(child1).parentNode != anc {
		child1 = child1.ParentNode()
	}
	child2 := c2
	for nodeBaseOf(child2).parentNode != anc {
		child2 = child2.ParentNode()
	}
	idx1 := indexOfChild(anc, child1)
	idx2 := indexOfChild(anc, child2)
	if idx1 < idx2 {
		return -1
	}
	return 1
}
