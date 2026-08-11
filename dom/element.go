// Translation of: Source/WebCore/dom/Element.h
//                  Source/WebCore/dom/Element.cpp
// Completeness: 80%
// Simplifications:
//   - Element embeds nodeBase (no separate ContainerNode layer); attribute storage is a
//     Go map plus an insertion-ordered name slice instead of WebKit's ElementData
//   - qualified names / namespaces are collapsed to a plain local tag name
//   - shadow DOM, custom elements, animation/ARIA hooks are omitted
//   - setInnerHTML uses a small stack-based HTML fragment parser (see documentfragment.go)
//   - getInnerHTML serialises a conservative HTML form (always closing tags, text escaped)

package dom

import (
	"strings"
)

// Element is the Go translation of WebCore::Element. It embeds nodeBase for the tree
// and EventTarget machinery and adds a tag name plus an insertion-ordered attribute
// set. Element creation goes through Document.CreateElement so that owner documents are
// tracked; NewElement is the lower-level constructor used by the document factory and
// by fragment parsing.
type Element struct {
	nodeBase
	tag       string
	attrOrder []string
	attrs     map[string]string

	// Dynamic pseudo-class state, set by the embedding application (e.g. from
	// mouse/keyboard event handlers). These mirror the interactive pseudo-classes
	// :hover / :focus / :active in the CSS selector model.
	hovered         bool
	focused         bool
	focusByKeyboard bool // true when focus came from keyboard (Tab) — drives :focus-visible
	active          bool

	// attrVersion 随属性变更递增，样式解析器据此失效 per-element 缓存：
	// class/type/checked 等影响 CSS 选择器匹配的属性变化后必须重算样式
	// （浏览器 attribute 变化触发 style recalc）。此前 RebuildRenderTree 的
	// style 指纹缓存跳过 ClearCache → class 切换样式残留（选中高亮多个并存）。
	attrVersion uint64
}

// NewElement creates an Element owned by doc with the given (original-case) tag name.
// The tag is stored verbatim; TagName returns the uppercased form for HTML and
// LocalName returns the lowercased form, mirroring nodeName/localName.
func NewElement(doc *Document, tagName string) *Element {
	e := &Element{
		tag:   tagName,
		attrs: map[string]string{},
	}
	e.initNodeBase(e, doc, NodeElement)
	return e
}

// NodeName returns the tag name in canonical form. For HTML elements this is the
// uppercased local name, mirroring Element::nodeName()/tagName().
func (e *Element) NodeName() string { return strings.ToUpper(e.tag) }

// TagName returns the tag name, mirroring Element::tagName() (alias of NodeName).
func (e *Element) TagName() string { return e.NodeName() }

// LocalName returns the lowercased local name, mirroring Element::localName().
func (e *Element) LocalName() string { return strings.ToLower(e.tag) }

// NodeValue for an Element is always the empty string, mirroring Node::nodeValue().
func (e *Element) NodeValue() string { return "" }

// SetNodeValue has no effect on an Element, mirroring Node::setNodeValue().
func (e *Element) SetNodeValue(string) error { return nil }

// cloneShallow produces an empty copy of the element with the same tag and attributes.
func (e *Element) cloneShallow(doc *Document) Node {
	c := NewElement(doc, e.tag)
	for _, name := range e.attrOrder {
		if v, ok := e.attrs[name]; ok {
			c.attrs[name] = v
			c.attrOrder = append(c.attrOrder, name)
		}
	}
	return c
}

// HasAttribute reports whether a named attribute is present, mirroring
// Element::hasAttribute(name).
func (e *Element) HasAttribute(name string) bool {
	_, ok := e.attrs[strings.ToLower(name)]
	return ok
}

// GetAttribute returns the attribute value or the empty string when absent, mirroring
// Element::getAttribute(name). Attribute names are matched case-insensitively for HTML.
func (e *Element) GetAttribute(name string) string {
	return e.attrs[strings.ToLower(name)]
}

// SetAttribute sets an attribute value, mirroring Element::setAttribute(name, value).
// A new attribute is appended to the insertion order.
func (e *Element) SetAttribute(name, value string) {
	key := strings.ToLower(name)
	oldValue, existed := e.attrs[key]
	if _, exists := e.attrs[key]; !exists {
		e.attrOrder = append(e.attrOrder, key)
	}
	e.attrs[key] = value
	// MutationObserver: notify attributes
	if !existed || oldValue != value {
		e.attrVersion++
		NotifyAttributes(e, key, oldValue)
	}
}

// RemoveAttribute removes an attribute, mirroring Element::removeAttribute(name). It
// returns whether an attribute was removed.
func (e *Element) RemoveAttribute(name string) bool {
	key := strings.ToLower(name)
	oldValue, ok := e.attrs[key]
	if !ok {
		return false
	}
	delete(e.attrs, key)
	// MutationObserver: notify attributes removed
	e.attrVersion++
	NotifyAttributes(e, key, oldValue)
	return true
}

// AttrVersion 返回属性版本号（每次属性增/改/删递增）。
// 样式解析器用它判断 per-element 计算样式缓存是否过期：版本变化意味着
// 可能影响 CSS 选择器匹配（class/id/type/checked/…），必须重算。
func (e *Element) AttrVersion() uint64 { return e.attrVersion }

// AttributeNames returns the attribute names in insertion order, mirroring
// Element::getAttributeNames().
func (e *Element) AttributeNames() []string {
	out := make([]string, 0, len(e.attrs))
	for _, n := range e.attrOrder {
		if _, ok := e.attrs[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// HasAttributes reports whether the element has any attributes set, mirroring
// Element::hasAttributes().
func (e *Element) HasAttributes() bool { return len(e.attrs) > 0 }

// --- Dynamic pseudo-class state --------------------------------------------

// IsHovered reports whether the element is currently in the hover state
// (mouse pointer over the element), mirroring the :hover pseudo-class.
func (e *Element) IsHovered() bool { return e.hovered }

// SetHovered sets the hover state. The embedder calls this from mouse-move
// and mouse-out event handlers.
func (e *Element) SetHovered(h bool) {
	if e.hovered != h {
		e.hovered = h
		e.bumpDynamicPseudoVersion()
	}
}

// bumpDynamicPseudoVersion 使动态伪类（:hover/:focus/:active）状态变化
// 波及的 per-element 样式缓存全部失效。resolver 的缓存 key 是元素自身
// attrVersion，但 :hover 的匹配会跨元素：
//   - `div:hover a` 选择器：后代的缓存依赖**祖先**的 hovered 状态
//   - :hover 冒泡匹配（悬停子元素 → 祖先也匹配 :hover）：祖先的缓存
//     依赖**后代**的 hovered 状态
// 因此状态变化必须 bump 自身 + 全部祖先 + 全部后代（后代树遍历），
// 否则清除 hover 后缓存仍返回旧的 :hover 样式（"无操作时渲染被影响"）。
func (e *Element) bumpDynamicPseudoVersion() {
	e.attrVersion++
	for p := e.ParentElement(); p != nil; p = p.ParentElement() {
		p.attrVersion++
	}
	var walk func(n Node)
	walk = func(n Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if ce, ok := c.(*Element); ok {
				ce.attrVersion++
				walk(ce)
			} else {
				walk(c)
			}
		}
	}
	walk(e)
}

// IsFocused reports whether the element currently has focus, mirroring the
// :focus pseudo-class.
func (e *Element) IsFocused() bool { return e.focused }

// SetFocused sets the focus state. The embedder calls this from focus/blur
// event handlers.
func (e *Element) SetFocused(f bool) {
	if e.focused != f {
		e.focused = f
		// 维护 owner document 的 focused 元素缓存（document.hasFocus()/
		// activeElement O(1) 查询）。
		if doc := e.OwnerDocument(); doc != nil {
			if f {
				doc.SetFocusedElement(e)
			} else if doc.FocusedElement() == e {
				doc.SetFocusedElement(nil)
			}
		}
		e.bumpDynamicPseudoVersion() // :focus/:focus-within 跨元素 → 全链失效
	}
}

// FocusByKeyboard reports whether the current focus was established by the
// keyboard (e.g. Tab), driving the :focus-visible pseudo-class.
func (e *Element) FocusByKeyboard() bool { return e.focusByKeyboard }

// SetFocusByKeyboard records how focus was established. Call SetFocused(true)
// and SetFocusByKeyboard(true) together when Tab moves focus; mouse clicks set
// it false so :focus-visible (UA default outline) does not match.
func (e *Element) SetFocusByKeyboard(b bool) {
	if e.focusByKeyboard != b {
		e.focusByKeyboard = b
		e.attrVersion++ // :focus-visible 只匹配自身 → 仅 bump 自身
	}
}

// IsActive reports whether the element is currently active (being activated
// by the user, e.g. while a mouse button is pressed), mirroring the :active
// pseudo-class.
func (e *Element) IsActive() bool { return e.active }

// SetActive sets the active state. The embedder calls this from mouse-down
// and mouse-up event handlers.
func (e *Element) SetActive(a bool) {
	if e.active != a {
		e.active = a
		e.bumpDynamicPseudoVersion() // :active 冒泡（父按钮因子元素 active 匹配）→ 全链失效
	}
}

// GetId/SetId and GetClassName/SetClassName are convenience accessors for the common
// id and class attributes, mirroring Element::id()/className().
func (e *Element) GetId() string         { return e.GetAttribute("id") }
func (e *Element) SetId(id string)       { e.SetAttribute("id", id) }
func (e *Element) GetClassName() string   { return e.GetAttribute("class") }
func (e *Element) SetClassName(c string) { e.SetAttribute("class", c) }

// ClassName returns the class attribute, mirroring Element::className().
func (e *Element) ClassName() string { return e.GetClassName() }

// HasClassName reports whether the element's class list contains name, mirroring
// Element::hasClassName().
func (e *Element) HasClassName(name string) bool {
	for _, c := range strings.Fields(e.GetClassName()) {
		if c == name {
			return true
		}
	}
	return false
}

// classList returns the space-separated class names.
func (e *Element) classList() []string { return strings.Fields(e.GetClassName()) }

// GetElementById returns the first descendant Element whose id attribute matches id,
// searching in pre-order, mirroring NonElementParentNode::getElementById.
func (e *Element) GetElementById(id string) *Element {
	var found *Element
	e.walkDescendants(func(n Node) bool {
		if el, ok := n.(*Element); ok {
			if el.GetId() == id {
				found = el
				return false
			}
		}
		return true
	})
	return found
}

// GetElementsByTagName returns all descendant elements whose tag name matches tagName
// ("*" matches all), mirroring Element::getElementsByTagName.
func (e *Element) GetElementsByTagName(tagName string) []*Element {
	var out []*Element
	want := strings.ToLower(tagName)
	all := want == "*"
	e.walkDescendants(func(n Node) bool {
		if el, ok := n.(*Element); ok {
			if all || el.LocalName() == want {
				out = append(out, el)
			}
		}
		return true
	})
	return out
}

// GetElementsByClassName returns all descendant elements that have token in their class
// list, mirroring Element::getElementsByClassName.
func (e *Element) GetElementsByClassName(token string) []*Element {
	if token == "" {
		return nil
	}
	var out []*Element
	e.walkDescendants(func(n Node) bool {
		if el, ok := n.(*Element); ok {
			if el.HasClassName(token) {
				out = append(out, el)
			}
		}
		return true
	})
	return out
}

// GetInnerHTML serialises the element's children as HTML, mirroring Element::innerHTML.
func (e *Element) GetInnerHTML() string {
	var sb strings.Builder
	for c := e.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		sb.WriteString(serializeNode(c))
	}
	return sb.String()
}

// SetInnerHTML replaces the element's children by parsing html, mirroring
// Element::setInnerHTML. Parsing uses the small fragment parser in documentfragment.go.
func (e *Element) SetInnerHTML(html string) error {
	for c := e.firstChild; c != nil; {
		next := nodeBaseOf(c).nextSibling
		_ = e.RemoveChild(c)
		c = next
	}
	frag := parseFragment(html, e.ownerDoc)
	for _, c := range frag.ChildNodes() {
		if err := e.AppendChild(c); err != nil {
			return err
		}
	}
	return nil
}

// GetOuterHTML returns the serialised element including its own tags, mirroring
// Element::outerHTML.
func (e *Element) GetOuterHTML() string { return serializeNode(e) }

// InsertAdjacentHTML parses html and inserts the resulting nodes at the given
// position relative to this element, mirroring Element::insertAdjacentHTML().
// Valid positions: "beforebegin", "afterbegin", "beforeend", "afterend".
func (e *Element) InsertAdjacentHTML(position, html string) error {
	frag := parseFragment(html, e.ownerDoc)
	children := frag.ChildNodes()
	switch position {
	case "beforebegin":
		parent := e.ParentNode()
		if parent == nil {
			return ErrHierarchyRequest
		}
		for _, c := range children {
			if err := parent.InsertBefore(c, e); err != nil {
				return err
			}
		}
	case "afterbegin":
		ref := e.FirstChild()
		for _, c := range children {
			if ref == nil {
				if err := e.AppendChild(c); err != nil {
					return err
				}
			} else {
				if err := e.InsertBefore(c, ref); err != nil {
					return err
				}
			}
		}
	case "beforeend":
		for _, c := range children {
			if err := e.AppendChild(c); err != nil {
				return err
			}
		}
	case "afterend":
		parent := e.ParentNode()
		if parent == nil {
			return ErrHierarchyRequest
		}
		ref := e.NextSibling()
		for _, c := range children {
			if ref == nil {
				if err := parent.AppendChild(c); err != nil {
					return err
				}
			} else {
				if err := parent.InsertBefore(c, ref); err != nil {
					return err
				}
			}
		}
	default:
		return ErrNotSupported
	}
	return nil
}

// walkDescendants performs a pre-order traversal of the subtree rooted at e, invoking
// fn for each node. Returning false from fn stops the traversal.
func (e *Element) walkDescendants(fn func(Node) bool) {
	e.WalkDescendants(fn)
}

// WalkDescendants is the exported version of walkDescendants, used by external
// packages (e.g., bindings) to traverse the element's subtree.
func (e *Element) WalkDescendants(fn func(Node) bool) {
	var walk func(n Node) bool
	walk = func(n Node) bool {
		if !fn(n) {
			return false
		}
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(e)
}

// walkDescendantsNode is the Node-based variant used by Document queries.
func walkDescendantsNode(root Node, fn func(Node) bool) {
	var walk func(n Node) bool
	walk = func(n Node) bool {
		if !fn(n) {
			return false
		}
		for c := nodeBaseOf(n).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(root)
}

// equalAttributes reports whether two elements have the same attributes, used by
// Node::isEqualNode.
func (e *Element) equalAttributes(other *Element) bool {
	if len(e.attrs) != len(other.attrs) {
		return false
	}
	for k, v := range e.attrs {
		if ov, ok := other.attrs[k]; !ok || ov != v {
			return false
		}
	}
	return true
}

// serializeNode renders a node as an HTML string. The format is conservative: every
// element emits an open tag, its children and a close tag; text is escaped; comments
// keep their delimiters. Attribute order is the insertion order for fidelity.
func serializeNode(n Node) string {
	switch v := n.(type) {
	case *Element:
		var sb strings.Builder
		sb.WriteByte('<')
		sb.WriteString(v.LocalName())
		// Attributes are emitted in insertion order, mirroring ElementData order.
		for _, name := range v.AttributeNames() {
			sb.WriteByte(' ')
			sb.WriteString(name)
			sb.WriteString("=\"")
			sb.WriteString(escapeAttrValue(v.attrs[name]))
			sb.WriteByte('"')
		}
		sb.WriteByte('>')
		for c := nodeBaseOf(v).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			sb.WriteString(serializeNode(c))
		}
		sb.WriteString("</")
		sb.WriteString(v.LocalName())
		sb.WriteByte('>')
		return sb.String()
	case *Text:
		return escapeText(v.Data())
	case *Comment:
		return "<!--" + v.Data() + "-->"
	case *DocumentFragment:
		var sb strings.Builder
		for c := nodeBaseOf(v).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			sb.WriteString(serializeNode(c))
		}
		return sb.String()
	case *Document:
		var sb strings.Builder
		for c := nodeBaseOf(v).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
			sb.WriteString(serializeNode(c))
		}
		return sb.String()
	}
	return ""
}

// escapeText escapes &, <, > for text content, mirroring MarkupAccumulator text escaping.
func escapeText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// escapeAttrValue escapes &, ", <, > in an attribute value.
func escapeAttrValue(s string) string {
	r := strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}


