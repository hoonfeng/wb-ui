// Translation of: Source/WebCore/dom/Node.h
//                  Source/WebCore/dom/Node.cpp
//                  Source/WebCore/dom/NodeType.h
// Completeness: 80%
// Simplifications:
//   - C++ multiple inheritance (Node : EventTarget) replaced with a Go interface
//     that embeds EventTarget plus a shared nodeBase struct embedded by every concrete node
//   - ContainerNode is folded into nodeBase: leaf nodes (Text/Comment) carry an empty
//     child list and reject AppendChild rather than using a separate ContainerNode layer
//   - NodeType bitfield flags replaced with a NodeType enum plus bool predicates
//   - intrusive ref()/deref() reference counting is omitted; Go GC manages lifetime
//   - ScriptExecutionContext is omitted; ownerDocument alone tracks the owning document
//   - shadow tree / mutation observers / style recalc / rendering hooks are omitted
//   - nodeBase.self holds the outer Node so dispatch can traverse ancestors
//   - wtf.String/AtomString are available but native Go strings are used for ergonomics
//   - NodeList is returned as a plain []Node slice

package dom

import (
	"errors"
	"strings"
)

// NodeType mirrors WebCore::NodeType.
type NodeType uint8

// Node type constants, mirroring NodeType.h.
const (
	NodeElement               NodeType = 1
	NodeAttribute             NodeType = 2
	NodeText                  NodeType = 3
	NodeCDATASection          NodeType = 4
	NodeProcessingInstruction NodeType = 7
	NodeComment               NodeType = 8
	NodeDocument              NodeType = 9
	NodeDocumentType          NodeType = 10
	NodeDocumentFragment      NodeType = 11
)

// LastNodeType is the historical upper bound on node types (NOTATION_NODE).
const LastNodeType NodeType = 12

// DocumentPosition mirrors Node::DocumentPosition.
const (
	DocumentPositionEquivalent             = 0x00
	DocumentPositionDisconnected           = 0x01
	DocumentPositionPreceding              = 0x02
	DocumentPositionFollowing              = 0x04
	DocumentPositionContains               = 0x08
	DocumentPositionContainedBy            = 0x10
	DocumentPositionImplementationSpecific = 0x20
)

// Sentinel errors mirroring WebCore::ExceptionCode values used by the DOM mutation API.
var (
	ErrHierarchyRequest = errors.New("dom: HierarchyRequestError")
	ErrNotFound         = errors.New("dom: NotFoundError")
	ErrInvalidNodeType  = errors.New("dom: InvalidNodeTypeError")
	ErrIndexSize        = errors.New("dom: IndexSizeError")
	ErrNotSupported     = errors.New("dom: NotSupportedError")
)

// Node is the Go translation of WebCore::Node. In WebKit Node is the abstract base of
// the DOM tree and also derives from EventTarget. The Go port models this as an
// interface that embeds EventTarget: every concrete node type (Element, Document, Text,
// Comment, DocumentFragment) satisfies Node by embedding the nodeBase struct, which
// carries the shared parent/sibling/child/owner-document state and the EventTarget
// implementation. Tree mutations (AppendChild/RemoveChild/InsertBefore/ReplaceChild)
// follow the DOM specification's hierarchy and pre-insert checks.
type Node interface {
	EventTarget

	// Core attributes.
	NodeType() NodeType
	NodeName() string
	NodeValue() string
	SetNodeValue(string) error
	OwnerDocument() *Document
	IsConnected() bool

	// Tree navigation.
	ParentNode() Node
	ParentElement() *Element
	PreviousSibling() Node
	NextSibling() Node
	FirstChild() Node
	LastChild() Node
	HasChildNodes() bool
	ChildNodes() []Node

	// Tree mutation.
	AppendChild(Node) error
	RemoveChild(Node) error
	InsertBefore(newChild, refChild Node) error
	ReplaceChild(newChild, oldChild Node) error

	// Queries.
	Contains(Node) bool
	IsDescendantOf(Node) bool
	TextContent() string
	SetTextContent(string) error
	CloneNode(bool) Node
	IsSameNode(Node) bool
	IsEqualNode(Node) bool
	Normalize()
	CompareDocumentPosition(Node) uint16

	// Type predicates mirroring Node::is*().
	IsElementNode() bool
	IsTextNode() bool
	IsContainerNode() bool
	IsCharacterDataNode() bool
	IsDocumentNode() bool
	IsDocumentFragment() bool
}

// nodeBase is the shared implementation embedded by every concrete node type. It holds
// the structural fields (parent, siblings, first/last child), the owner document, the
// node type tag and the EventTarget listener storage. Methods defined on *nodeBase are
// promoted to every embedding type, so Element/Document/Text/... all share one
// implementation of the tree and EventTarget APIs.
//
// The self field is a back-pointer to the outer Node (set by initNodeBase) so that
// nodeBase methods can recover the concrete node value when building event dispatch
// paths or reporting the target.
type nodeBase struct {
	self        Node
	nodeType    NodeType
	ownerDoc    *Document
	parentNode  Node
	prevSibling Node
	nextSibling Node
	firstChild  Node
	lastChild   Node
	listeners   map[string][]*registeredListener
}

// nodeBaseProvider is the internal interface used to recover the *nodeBase from any
// Node value. Every concrete node embeds nodeBase by value and therefore promotes the
// asNodeBase method (pointer receiver) onto its pointer type.
type nodeBaseProvider interface {
	asNodeBase() *nodeBase
}

// asNodeBase lets a *nodeBase satisfy nodeBaseProvider. Because concrete nodes embed
// nodeBase by value, taking the address of the embedded field yields the same pointer
// that owns the structural fields, so mutations through it persist on the node.
func (b *nodeBase) asNodeBase() *nodeBase { return b }

// nodeBaseOf returns the *nodeBase backing n, or nil for a nil node.
func nodeBaseOf(n Node) *nodeBase {
	if n == nil {
		return nil
	}
	if p, ok := n.(nodeBaseProvider); ok {
		return p.asNodeBase()
	}
	return nil
}

// initNodeBase populates the shared node state. It is called by every concrete node
// constructor with self pointing at the freshly allocated outer node so that nodeBase
// methods can later recover the concrete value.
func (b *nodeBase) initNodeBase(self Node, doc *Document, nt NodeType) {
	b.self = self
	b.ownerDoc = doc
	b.nodeType = nt
}

// NodeType returns the node's type tag, mirroring Node::nodeType().
func (b *nodeBase) NodeType() NodeType { return b.nodeType }

// OwnerDocument returns the document that owns this node, mirroring
// Node::ownerDocument(). A Document node reports a nil owner document, matching the
// DOM spec where document.ownerDocument is null.
func (b *nodeBase) OwnerDocument() *Document {
	if b.nodeType == NodeDocument {
		return nil
	}
	return b.ownerDoc
}

// IsConnected reports whether this node is in a document tree, mirroring
// Node::isConnected(). It walks up the parent chain and returns true when an ancestor
// (or this node itself) is a Document.
func (b *nodeBase) IsConnected() bool {
	for n := b.self; n != nil; n = n.ParentNode() {
		if n.NodeType() == NodeDocument {
			return true
		}
	}
	return false
}

// isInDocumentTree reports whether the node is rooted at a Document. It is the
// internal counterpart of IsConnected used to decide owner-document adoption on
// insertion.
func (b *nodeBase) isInDocumentTree() bool {
	return b.IsConnected()
}

// ParentNode returns the parent node, mirroring Node::parentNode().
func (b *nodeBase) ParentNode() Node { return b.parentNode }

// ParentElement returns the parent if it is an Element, mirroring Node::parentElement().
func (b *nodeBase) ParentElement() *Element {
	if b.parentNode == nil {
		return nil
	}
	if e, ok := b.parentNode.(*Element); ok {
		return e
	}
	return nil
}

// PreviousSibling / NextSibling mirror Node::previousSibling()/nextSibling().
func (b *nodeBase) PreviousSibling() Node { return b.prevSibling }
func (b *nodeBase) NextSibling() Node     { return b.nextSibling }

// FirstChild / LastChild mirror Node::firstChild()/lastChild().
func (b *nodeBase) FirstChild() Node { return b.firstChild }
func (b *nodeBase) LastChild() Node  { return b.lastChild }

// HasChildNodes reports whether the node has any children, mirroring
// Node::hasChildNodes().
func (b *nodeBase) HasChildNodes() bool { return b.firstChild != nil }

// ChildNodes returns a snapshot of the child nodes in tree order, mirroring
// Node::childNodes(). The slice is freshly allocated so callers cannot mutate the
// node's internal child list through it.
func (b *nodeBase) ChildNodes() []Node {
	var out []Node
	for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		out = append(out, c)
	}
	return out
}

// canHaveChildren reports whether this node may legally hold children. Leaf node types
// (Text/Comment/CDATA/ProcessingInstruction) return false, matching WebKit's
// ContainerNode split where only Element/Document/DocumentFragment are containers.
func (b *nodeBase) canHaveChildren() bool {
	switch b.nodeType {
	case NodeElement, NodeDocument, NodeDocumentFragment:
		return true
	default:
		return false
	}
}

// isAncestorOf reports whether self is an ancestor of other (inclusive). It is used by
// the hierarchy checks in AppendChild/InsertBefore/ReplaceChild.
func (b *nodeBase) isAncestorOf(other Node) bool {
	for n := other; n != nil; n = n.ParentNode() {
		if n == b.self {
			return true
		}
	}
	return false
}

// Contains reports whether other is this node or a descendant, mirroring
// Node::contains().
func (b *nodeBase) Contains(other Node) bool {
	if other == nil {
		return false
	}
	return b.isAncestorOf(other)
}

// IsDescendantOf reports whether this node is a descendant of other, mirroring
// Node::isDescendantOf().
func (b *nodeBase) IsDescendantOf(other Node) bool {
	if other == nil {
		return false
	}
	for n := b.parentNode; n != nil; n = n.ParentNode() {
		if n == other {
			return true
		}
	}
	return false
}

// removeIfPresent detaches newChild from its current parent, mirroring the "remove"
// step of the pre-insert algorithm. Errors from the old parent are ignored because the
// DOM spec treats removal as best-effort during adoption.
func removeIfPresent(child Node) {
	if child == nil {
		return
	}
	if p := child.ParentNode(); p != nil {
		_ = p.RemoveChild(child)
	}
}

// AppendChild inserts newChild as the last child, mirroring Node::appendChild(). It
// performs the DOM hierarchy checks: newChild must not be an ancestor of this node, and
// this node must be a container. newChild is first removed from any existing parent.
func (b *nodeBase) AppendChild(newChild Node) error {
	if newChild == nil {
		return ErrInvalidNodeType
	}
	if !b.canHaveChildren() {
		return ErrHierarchyRequest
	}
	if newChild == b.self {
		return ErrHierarchyRequest
	}
	if newChild.Contains(b.self) {
		return ErrHierarchyRequest
	}
	removeIfPresent(newChild)
	cb := nodeBaseOf(newChild)
	cb.parentNode = b.self
	cb.prevSibling = b.lastChild
	cb.nextSibling = nil
	if b.lastChild != nil {
		nodeBaseOf(b.lastChild).nextSibling = newChild
	} else {
		b.firstChild = newChild
	}
	b.lastChild = newChild
	if b.isInDocumentTree() {
		b.adoptSubtree(newChild, b.documentForAdoption())
	}
	return nil
}

// documentForAdoption returns the document that inserted nodes should be adopted into.
// For a Document it is the document itself; for other nodes it is the owner document.
func (b *nodeBase) documentForAdoption() *Document {
	if b.nodeType == NodeDocument {
		return b.self.(*Document)
	}
	return b.ownerDoc
}

// InsertBefore inserts newChild before refChild, mirroring Node::insertBefore(). A nil
// refChild appends. refChild must be a child of this node.
func (b *nodeBase) InsertBefore(newChild, refChild Node) error {
	if newChild == nil {
		return ErrInvalidNodeType
	}
	if !b.canHaveChildren() {
		return ErrHierarchyRequest
	}
	if refChild == nil {
		return b.AppendChild(newChild)
	}
	if nodeBaseOf(refChild).parentNode != b.self {
		return ErrNotFound
	}
	if newChild == b.self {
		return ErrHierarchyRequest
	}
	if newChild.Contains(b.self) {
		return ErrHierarchyRequest
	}
	removeIfPresent(newChild)
	prev := nodeBaseOf(refChild).prevSibling
	next := refChild
	cb := nodeBaseOf(newChild)
	cb.parentNode = b.self
	cb.prevSibling = prev
	cb.nextSibling = next
	if prev != nil {
		nodeBaseOf(prev).nextSibling = newChild
	} else {
		b.firstChild = newChild
	}
	nodeBaseOf(next).prevSibling = newChild
	if b.isInDocumentTree() {
		b.adoptSubtree(newChild, b.documentForAdoption())
	}
	return nil
}

// ReplaceChild replaces oldChild with newChild in the child list, mirroring
// Node::replaceChild().
func (b *nodeBase) ReplaceChild(newChild, oldChild Node) error {
	if newChild == nil || oldChild == nil {
		return ErrInvalidNodeType
	}
	if !b.canHaveChildren() {
		return ErrHierarchyRequest
	}
	if nodeBaseOf(oldChild).parentNode != b.self {
		return ErrNotFound
	}
	if newChild != oldChild {
		if newChild == b.self {
			return ErrHierarchyRequest
		}
		if newChild.Contains(b.self) {
			return ErrHierarchyRequest
		}
		removeIfPresent(newChild)
	}
	// Splice newChild into oldChild's slot.
	prev := nodeBaseOf(oldChild).prevSibling
	next := nodeBaseOf(oldChild).nextSibling
	cb := nodeBaseOf(newChild)
	cb.parentNode = b.self
	cb.prevSibling = prev
	cb.nextSibling = next
	if prev != nil {
		nodeBaseOf(prev).nextSibling = newChild
	} else {
		b.firstChild = newChild
	}
	if next != nil {
		nodeBaseOf(next).prevSibling = newChild
	} else {
		b.lastChild = newChild
	}
	nodeBaseOf(oldChild).parentNode = nil
	nodeBaseOf(oldChild).prevSibling = nil
	nodeBaseOf(oldChild).nextSibling = nil
	if b.isInDocumentTree() {
		b.adoptSubtree(newChild, b.documentForAdoption())
	}
	return nil
}

// RemoveChild detaches child, mirroring Node::removeChild(). child must be a child of
// this node.
func (b *nodeBase) RemoveChild(child Node) error {
	if child == nil {
		return ErrNotFound
	}
	cb := nodeBaseOf(child)
	if cb.parentNode != b.self {
		return ErrNotFound
	}
	prev := cb.prevSibling
	next := cb.nextSibling
	if prev != nil {
		nodeBaseOf(prev).nextSibling = next
	} else {
		b.firstChild = next
	}
	if next != nil {
		nodeBaseOf(next).prevSibling = prev
	} else {
		b.lastChild = prev
	}
	cb.parentNode = nil
	cb.prevSibling = nil
	cb.nextSibling = nil
	return nil
}

// adoptSubtree recursively assigns doc as the owner document of node and its
// descendants, mirroring Document::adoptNode / the move-to-new-document step that runs
// when a node is inserted into a document tree.
func (b *nodeBase) adoptSubtree(node Node, doc *Document) {
	cb := nodeBaseOf(node)
	if cb == nil {
		return
	}
	cb.ownerDoc = doc
	for c := cb.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		b.adoptSubtree(c, doc)
	}
}

// TextContent returns the concatenation of all descendant text, mirroring
// Node::textContent(). For character-data nodes (Text/Comment/...) it returns the
// node's data; for containers it walks the subtree in document order.
func (b *nodeBase) TextContent() string {
	switch b.nodeType {
	case NodeText, NodeComment, NodeCDATASection, NodeProcessingInstruction:
		return b.self.NodeValue()
	default:
		var sb strings.Builder
		for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			sb.WriteString(c.TextContent())
		}
		return sb.String()
	}
}

// SetTextContent replaces the node's text content, mirroring Node::setTextContent().
// For character-data nodes it sets the data; for containers it removes all children and
// appends a single Text node holding the string (when non-empty).
func (b *nodeBase) SetTextContent(s string) error {
	switch b.nodeType {
	case NodeText, NodeComment, NodeCDATASection, NodeProcessingInstruction:
		return b.self.SetNodeValue(s)
	default:
		for c := b.firstChild; c != nil; {
			next := nodeBaseOf(c).nextSibling
			_ = b.RemoveChild(c)
			c = next
		}
		if s != "" {
			return b.AppendChild(NewText(b.ownerDoc, s))
		}
		return nil
	}
}

// cloneable is the internal interface used by the generic CloneNode to obtain an empty
// shallow copy of the concrete node type. Every concrete node implements cloneShallow.
type cloneable interface {
	cloneShallow(doc *Document) Node
}

// CloneNode returns a copy of the node, mirroring Node::cloneNode(deep). When deep is
// true the subtree is cloned recursively via AppendChild, which preserves tree order.
func (b *nodeBase) CloneNode(deep bool) Node {
	cl, ok := b.self.(cloneable)
	if !ok {
		return nil
	}
	doc := b.ownerDoc
	if b.nodeType == NodeDocument {
		doc = nil
	}
	copy := cl.cloneShallow(doc)
	if deep && copy != nil {
		for c := b.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			_ = copy.AppendChild(c.CloneNode(true))
		}
	}
	return copy
}

// IsSameNode reports identity, mirroring Node::isSameNode().
func (b *nodeBase) IsSameNode(other Node) bool { return other != nil && other == b.self }

// IsEqualNode reports structural equality, mirroring Node::isEqualNode(). Two nodes are
// equal when they share the same type, name, value (for character data), attributes (for
// elements) and equal children in order.
func (b *nodeBase) IsEqualNode(other Node) bool {
	if other == nil || other.NodeType() != b.nodeType {
		return false
	}
	if other.NodeName() != b.self.NodeName() {
		return false
	}
	switch b.nodeType {
	case NodeText, NodeComment, NodeCDATASection, NodeProcessingInstruction:
		return other.NodeValue() == b.self.NodeValue()
	case NodeElement:
		e1, _ := b.self.(*Element)
		e2, _ := other.(*Element)
		if e1 == nil || e2 == nil {
			return e1 == e2
		}
		if !e1.equalAttributes(e2) {
			return false
		}
	}
	a := b.ChildNodes()
	c := other.ChildNodes()
	if len(a) != len(c) {
		return false
	}
	for i := range a {
		if !a[i].IsEqualNode(c[i]) {
			return false
		}
	}
	return true
}

// Normalize merges adjacent text nodes and removes empty ones, mirroring
// Node::normalize().
func (b *nodeBase) Normalize() {
	prev := Node(nil)
	for c := b.firstChild; c != nil; {
		next := nodeBaseOf(c).nextSibling
		if c.NodeType() == NodeText {
			if c.NodeValue() == "" {
				_ = b.RemoveChild(c)
				c = next
				continue
			} else if prev != nil && prev.NodeType() == NodeText {
				pv := prev.NodeValue()
				_ = prev.SetNodeValue(pv + c.NodeValue())
				_ = b.RemoveChild(c)
				c = next
				continue
			}
		}
		if c.NodeType() != NodeText {
			c.Normalize()
			prev = nil
		} else {
			prev = c
		}
		c = next
	}
}

// CompareDocumentPosition returns the document position bitmask of other relative to
// this node, mirroring Node::compareDocumentPosition().
func (b *nodeBase) CompareDocumentPosition(other Node) uint16 {
	if other == nil || other == b.self {
		return DocumentPositionEquivalent
	}
	attr1, attr2 := false, false // attribute containers unsupported in this port
	_ = attr1
	_ = attr2
	if b.isAncestorOf(other) {
		return DocumentPositionContainedBy | DocumentPositionFollowing
	}
	if other.Contains(b.self) {
		return DocumentPositionContains | DocumentPositionPreceding
	}
	// Find common ancestor and compare tree order.
	root := commonAncestor(b.self, other)
	if root == nil {
		return DocumentPositionDisconnected | DocumentPositionImplementationSpecific
	}
	// Compare indices in pre-order traversal of root.
	if rootPrecedes(b.self, other, root) {
		return DocumentPositionPreceding
	}
	return DocumentPositionFollowing
}

// commonAncestor returns the nearest inclusive ancestor shared by a and b.
func commonAncestor(a, b Node) Node {
	seen := map[Node]bool{}
	for n := a; n != nil; n = n.ParentNode() {
		seen[n] = true
	}
	for n := b; n != nil; n = n.ParentNode() {
		if seen[n] {
			return n
		}
	}
	return nil
}

// rootPrecedes reports whether a appears before b in a pre-order traversal of root.
func rootPrecedes(a, b, root Node) bool {
	found := false
	var walk func(n Node) bool
	walk = func(n Node) bool {
		if n == b {
			return true // b reached before a => a does not precede
		}
		if n == a {
			found = true
			return true
		}
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(root)
	return found
}

// Type predicates.
func (b *nodeBase) IsElementNode() bool   { return b.nodeType == NodeElement }
func (b *nodeBase) IsTextNode() bool      { return b.nodeType == NodeText }
func (b *nodeBase) IsContainerNode() bool { return b.canHaveChildren() }
func (b *nodeBase) IsCharacterDataNode() bool {
	switch b.nodeType {
	case NodeText, NodeComment, NodeCDATASection, NodeProcessingInstruction:
		return true
	}
	return false
}
func (b *nodeBase) IsDocumentNode() bool     { return b.nodeType == NodeDocument }
func (b *nodeBase) IsDocumentFragment() bool { return b.nodeType == NodeDocumentFragment }
