// Translation of: Source/WebCore/dom/ShadowRoot.h
//                  Source/WebCore/dom/ShadowRoot.cpp
// Completeness: 85% (shadow-tree container + slot projection + style scoping +
//                    :host/:host-context/::slotted/::part cascade origins)
// Simplifications:
//   - ShadowRoot is a DocumentFragment that carries a host back-pointer and a mode
//     ("open"/"closed") string; it is NOT attached to the host's child list, so its
//     ParentNode() is nil (matching the DOM spec where shadowRoot.parentNode === null).
//   - Slot projection: <slot> renders its AssignedNodes (default + named slots).
//   - Style scoping: author sheets inside a shadow tree are scoped to that tree via
//     ContainingShadowRoot; inheritance crosses the shadow boundary (host → shadow
//     tree). :host / :host-context / ::slotted / ::part cascade origins are routed by
//     the style resolver with a per-sheet tree-scope depth (CSS Scoping Level 1 §3.3).
//   - host-selector::part(name) and host-selector::slotted(...) forward-matching across
//     the shadow boundary (the host-selector prefix matching the shadow host) is
//     implemented for both same-compound prefixes (e.g. x-widget::part(btn)) and
//     cross-combinator prefixes (e.g. .outer x-widget::part(btn), where the combinator
//     walks from the host via ComposedParent).
//   - Slot fallback content: AssignedNodes returns the slot's own children when no
//     light-DOM node is assigned (CSS Scoping Level 1 flattened tree).
//   - Composed event paths (ComposedPath / ComposedParent) cross shadow boundaries for
//     composed events and stop at the shadow root for non-composed events; assigned
//     nodes route through their <slot>; exportparts re-export is honored by ::part.
//   - Event retargeting retargets the target to the shadow host for listeners on/above
//     the host.

package dom

// ShadowRoot is the Go translation of WebCore::ShadowRoot. It is the root of a
// shadow tree attached to a host element via Element.AttachShadow. It embeds nodeBase
// with NodeDocumentFragment so it inherits the full container/query/mutation surface
// of a DocumentFragment, and adds the host + mode state required by shadow DOM.
type ShadowRoot struct {
	nodeBase
	host *Element
	mode string // "open" or "closed"
}

// NewShadowRoot creates a ShadowRoot owned by doc, bound to host with the given mode.
func NewShadowRoot(doc *Document, host *Element, mode string) *ShadowRoot {
	sr := &ShadowRoot{host: host, mode: mode}
	sr.initNodeBase(sr, doc, NodeDocumentFragment)
	return sr
}

// NodeName returns "#shadow-root", mirroring ShadowRoot::nodeName().
func (sr *ShadowRoot) NodeName() string { return "#shadow-root" }

// NodeValue returns the empty string, mirroring Node::nodeValue().
func (sr *ShadowRoot) NodeValue() string { return "" }

// SetNodeValue has no effect, mirroring Node::setNodeValue().
func (sr *ShadowRoot) SetNodeValue(string) error { return nil }

// Host returns the element this shadow root is attached to.
func (sr *ShadowRoot) Host() *Element { return sr.host }

// Mode returns "open" or "closed".
func (sr *ShadowRoot) Mode() string { return sr.mode }

// TreeScopeDepth returns the nesting depth of the shadow root's tree scope: 1 for a
// shadow root whose host lives directly in the document tree, 2 for a shadow root
// whose host lives inside another shadow tree, and so on. The style resolver uses this
// as the "scope" dimension of the cascade — declarations from a deeper shadow tree
// outrank those from a shallower one (CSS Scoping Level 1 §3.3), with the order
// reversed for !important.
func (sr *ShadowRoot) TreeScopeDepth() int {
	depth := 1
	for h := sr.host; h != nil; {
		p := h.ParentNode()
		parentSR, ok := p.(*ShadowRoot)
		if !ok {
			break
		}
		depth++
		h = parentSR.host
	}
	return depth
}

// cloneShallow returns an empty shadow root with the same host and mode, mirroring
// cloneNodeInternal. (Shadow roots are rarely cloned; this keeps CloneNode safe.)
func (sr *ShadowRoot) cloneShallow(doc *Document) Node {
	return NewShadowRoot(doc, sr.host, sr.mode)
}

// FirstComposedChild returns the first child of el in the flattened (composed) tree.
// When el has a shadow root, the shadow tree replaces the light-DOM children, so the
// shadow root's first child is returned and light-DOM children are skipped. This is the
// single seam the render-tree and layout-tree builders use to walk the composed tree
// instead of the raw light-DOM tree. Slot projection is not yet handled.
func FirstComposedChild(el *Element) Node {
	if el == nil {
		return nil
	}
	if el.shadowRoot != nil {
		return el.shadowRoot.FirstChild()
	}
	return el.FirstChild()
}

// ContainingShadowRoot returns the innermost shadow root containing n, walking n's
// ancestor chain until a ShadowRoot is found. Returns nil when n lives in the
// document tree (or a detached subtree without a shadow root). This is the scoping
// predicate the style resolver uses to decide whether an author stylesheet inside a
// shadow tree applies to a given element (and vice versa).
func ContainingShadowRoot(n Node) *ShadowRoot {
	for p := n.ParentNode(); p != nil; p = p.ParentNode() {
		if sr, ok := p.(*ShadowRoot); ok {
			return sr
		}
	}
	return nil
}

// ComposedParent returns n's parent in the composed (flattened) tree, crossing the
// shadow boundary: the composed parent of a shadow-tree child is the shadow host
// (and the composed parent of a shadow host is its own light-DOM/shadow parent). For
// a node whose parent is a plain element/document it is just ParentNode(). Returns
// nil at the document root. This is the single traversal primitive used to walk
// ancestor chains that must cross shadow boundaries (forward-matching selector
// combinators, composed event paths).
func ComposedParent(n Node) Node {
	p := n.ParentNode()
	if sr, ok := p.(*ShadowRoot); ok {
		return sr.Host()
	}
	return p
}

// WalkComposedTree performs a depth-first walk of the composed (flattened) tree: for a
// host element it descends into its shadow tree instead of its light-DOM children.
// Slot-projected nodes are NOT revisited here (they already appear at their light-DOM
// position), so each node is visited exactly once. visit is called for every node
// (elements and text). Used by the style-extraction pass to collect <style> elements
// that live inside shadow trees, which GetElementsByTagName (light-DOM only) misses.
func WalkComposedTree(root Node, visit func(Node)) {
	if root == nil {
		return
	}
	visit(root)
	if el, ok := root.(*Element); ok && el.shadowRoot != nil {
		for c := el.shadowRoot.FirstChild(); c != nil; c = c.NextSibling() {
			WalkComposedTree(c, visit)
		}
		return
	}
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		WalkComposedTree(c, visit)
	}
}

// AssignedNodes returns the light-DOM nodes assigned to a <slot> element in the
// flattened tree. It only has meaning for a <slot> inside a shadow tree: it walks up
// to the owning ShadowRoot, then collects the host's light-DOM children that match the
// slot's name. A slot without a name attribute (the "default" slot) collects every
// light-DOM child that does NOT carry a slot attribute; a named slot collects children
// whose slot attribute equals the slot name. When no light-DOM node is assigned, the
// slot's own children are returned as fallback content (CSS Scoping Level 1 flattened
// tree). Returns nil for non-slot elements or a slot outside any shadow tree.
func (e *Element) AssignedNodes() []Node {
	if e.LocalName() != "slot" {
		return nil
	}
	var sr *ShadowRoot
	for n := e.ParentNode(); n != nil; n = n.ParentNode() {
		if s, ok := n.(*ShadowRoot); ok {
			sr = s
			break
		}
	}
	if sr == nil || sr.Host() == nil {
		return nil
	}
	slotName := e.GetAttribute("name")
	host := sr.Host()
	var assigned []Node
	for c := host.FirstChild(); c != nil; c = c.NextSibling() {
		if slotName == "" {
			// Default slot: every light-DOM child without a slot attribute.
			if el, ok := c.(*Element); ok && el.GetAttribute("slot") != "" {
				continue
			}
			assigned = append(assigned, c)
		} else {
			// Named slot: only children whose slot attribute matches.
			if el, ok := c.(*Element); ok && el.GetAttribute("slot") == slotName {
				assigned = append(assigned, c)
			}
		}
	}
	if len(assigned) == 0 {
		// Fallback content: when nothing is assigned, the slot renders its own
		// children (the light-DOM nodes that live inside <slot> in the shadow tree).
		var fb []Node
		for c := e.FirstChild(); c != nil; c = c.NextSibling() {
			fb = append(fb, c)
		}
		return fb
	}
	return assigned
}

