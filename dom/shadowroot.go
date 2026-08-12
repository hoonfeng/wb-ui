// Translation of: Source/WebCore/dom/ShadowRoot.h
//                  Source/WebCore/dom/ShadowRoot.cpp
// Completeness: 40% (minimum viable shadow-tree container; slot assignment /
//                  style scoping / :host cascade are not yet implemented)
// Simplifications:
//   - ShadowRoot is a DocumentFragment that carries a host back-pointer and a mode
//     ("open"/"closed") string; it is NOT attached to the host's child list, so its
//     ParentNode() is nil (matching the DOM spec where shadowRoot.parentNode === null).
//   - No slot assignment / flattened-tree composition yet: light-DOM children are
//     hidden once a shadow root exists (FirstComposedChild returns the shadow root's
//     children), until <slot> projection lands.
//   - No style scoping: styles inside the shadow tree still participate in the
//     document-wide cascade (no :host / ::slotted source).

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

// AssignedNodes returns the light-DOM nodes assigned to a <slot> element in the
// flattened tree. It only has meaning for a <slot> inside a shadow tree: it walks up
// to the owning ShadowRoot, then collects the host's light-DOM children that match the
// slot's name. A slot without a name attribute (the "default" slot) collects every
// light-DOM child that does NOT carry a slot attribute; a named slot collects children
// whose slot attribute equals the slot name. Returns nil for non-slot elements or a
// slot outside any shadow tree.
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
	return assigned
}

