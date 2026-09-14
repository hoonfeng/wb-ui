// Translation of: Source/WebCore/dom/Element.h
//                  Source/WebCore/dom/Element.cpp
// Completeness: 80%
// Simplifications:
//   - Element embeds nodeBase (no separate ContainerNode layer); attribute storage is a
//     Go map plus an insertion-ordered name slice instead of WebKit's ElementData
//   - qualified names / namespaces are collapsed to a plain local tag name
//   - shadow DOM is implemented (ShadowRoot + slot projection, see shadowroot.go);
//     custom elements and animation/ARIA hooks are omitted
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

	// modal 是 HTML 的「模态状态」（in the modal state）：<dialog> 经
	// showModal() 打开、或将来模态 popover 显示时由宿主置位。它为 true 时
	// :modal 伪类匹配（HTML 渲染规范 + Selectors-4），渲染层可据此把元素
	// 画在最上层。由宿主（html5/bindings）设置，dom 包本身不改变它。
	modal bool

	// indeterminate 是 <input type="checkbox"> 的「不确定状态」：由
	// HTMLInputElement.indeterminate IDL 属性设置（**不是** HTML 内容属性，
	// 因此不参与属性反射、不进属性序列化）。:indeterminate 伪类的 checkbox
	// 分支读它。由宿主（bindings）设置，dom 包本身不改变它。
	indeterminate bool

	// attrVersion 随属性变更递增，样式解析器据此失效 per-element 缓存：
	// class/type/checked 等影响 CSS 选择器匹配的属性变化后必须重算样式
	// （浏览器 attribute 变化触发 style recalc）。此前 RebuildRenderTree 的
	// style 指纹缓存跳过 ClearCache → class 切换样式残留（选中高亮多个并存）。
	attrVersion uint64

	// shadowRoot holds the shadow tree attached via AttachShadow (nil when absent).
	// It is NOT part of the child list: the shadow tree replaces the light-DOM
	// children for rendering/layout (see FirstComposedChild).
	shadowRoot *ShadowRoot

	// onClickJSListener is the listener installed by el.onclick = fn (bindings
	// layer wraps the JS function as an EventListener and registers it for
	// "click"). Stored here so a later assignment can remove the old one —
	// dom.RemoveEventListener matches by Go listener identity.
	onClickJSListener EventListener

	// canvasSurface holds the engine-side backing draw surface for a <canvas>
	// element (an *graphics.Canvas created by the bindings layer's getContext;
	// the rendering pipeline blits it during paint). Stored as any so the dom
	// package stays free of graphics/rendering imports — the same internal
	// bridge pattern as onClickJSListener. Nil for non-canvas elements.
	canvasSurface any
}

// SetOnClickJSListener / GetOnClickJSListener store the listener installed by the
// el.onclick IDL attribute (see bindings/lazyelement.go setOnClick). Not part of the
// DOM spec's Element interface — an internal bridge for the JS wrapper.
func (e *Element) SetOnClickJSListener(l EventListener) { e.onClickJSListener = l }
func (e *Element) GetOnClickJSListener() EventListener  { return e.onClickJSListener }

// SetCanvasSurface / CanvasSurface store the engine-side offscreen draw surface
// for a <canvas> element (bindings writes, rendering reads at paint time).
// The surface type is opaque (any) to keep the dom package dependency-free.
func (e *Element) SetCanvasSurface(s any) { e.canvasSurface = s }
func (e *Element) CanvasSurface() any     { return e.canvasSurface }

// SetModalState / ModalState store the element's HTML "modal state"
// (in the modal state), set by the embedding application when it opens a
// <dialog> through showModal() (html5.HTMLDialogElement.ShowModal). The CSS
// selector engine reads it for the :modal pseudo-class.
func (e *Element) SetModalState(v bool) { e.modal = v }
func (e *Element) ModalState() bool     { return e.modal }

// SetIndeterminate / IsIndeterminate store the <input type="checkbox"> IDL
// "indeterminate" state (HTML §4.16.3). It is an in-memory state only — there is
// no HTML content attribute for it — set by the bindings layer
// (input.indeterminate = true) and read by the CSS selector engine for the
// :indeterminate pseudo-class.
func (e *Element) SetIndeterminate(v bool) { e.indeterminate = v }
func (e *Element) IsIndeterminate() bool   { return e.indeterminate }

// IsModalDialog reports whether the element is a <dialog> in the modal state
// (HTML §4.11.6). The modal state is set by showModal() and cleared by close()/
// the removals steps — NOT by removing the open attribute (the spec keeps such a
// dialog modal, which is why it recommends close() over removing the attribute).
// This is the single predicate behind the :modal pseudo-class
// (css/selectorchecker.go) and the ::backdrop box generation (layout/rendering),
// so all three agree.
func (e *Element) IsModalDialog() bool {
	return e != nil && strings.EqualFold(e.tag, "dialog") && e.modal
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

// AttachShadow attaches a new shadow root to this element, mirroring
// Element::attachShadow(init). An element may host at most one shadow root; a second
// call returns ErrNotSupported. The mode is "open" or "closed" ("open" exposes the
// shadow root via ShadowRoot(), "closed" hides it).
func (e *Element) AttachShadow(mode string) (*ShadowRoot, error) {
	if e.shadowRoot != nil {
		return nil, ErrNotSupported
	}
	sr := NewShadowRoot(e.OwnerDocument(), e, mode)
	e.shadowRoot = sr
	return sr, nil
}

// ShadowRoot returns the element's shadow root, mirroring Element::shadowRoot. It
// returns nil when there is no shadow root or when the root is closed (mode
// "closed"), matching the DOM spec where closed shadow roots are inaccessible.
func (e *Element) ShadowRoot() *ShadowRoot {
	if e.shadowRoot != nil && e.shadowRoot.mode == "closed" {
		return nil
	}
	return e.shadowRoot
}

// HasShadowRoot reports whether the element hosts a shadow tree (open or closed).
// Unlike ShadowRoot() it does NOT hide closed roots — it is the internal predicate
// used by CSS :host / :host-context matching and slot assignment, which must work
// for closed shadow roots too (the host always knows its own shadow tree).
func (e *Element) HasShadowRoot() bool { return e.shadowRoot != nil }

// AssignedSlot returns the <slot> element (inside this element's host's shadow tree)
// that assigns this element, or nil when this element is not a light-DOM child of a
// shadow host (or no matching slot exists). Mirrors Element::assignedSlot(). A
// light-DOM child with a `slot="name"` attribute is assigned to the named slot; one
// without is assigned to the default (unnamed) slot.
func (e *Element) AssignedSlot() *Element {
	parent := e.ParentNode()
	if parent == nil {
		return nil
	}
	host, ok := parent.(*Element)
	if !ok || !host.HasShadowRoot() {
		return nil
	}
	slotName := e.GetAttribute("slot")
	var found *Element
	var walk func(n Node)
	walk = func(n Node) {
		if found != nil {
			return
		}
		if el, ok := n.(*Element); ok && el.LocalName() == "slot" {
			name := el.GetAttribute("name")
			if slotName == "" {
				if name == "" {
					found = el
					return
				}
			} else if name == slotName {
				found = el
				return
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for c := host.shadowRoot.FirstChild(); c != nil; c = c.NextSibling() {
		walk(c)
	}
	return found
}

// PartNames returns the whitespace-separated list of part names from the element's
// `part` attribute, mirroring Element::partNames(). Used by the ::part() pseudo-element.
func (e *Element) PartNames() []string {
	return strings.Fields(e.GetAttribute("part"))
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
		notifyDynamicPseudoChanged(e)
	}
}

// DynamicPseudoStateChanged 是动态伪类（:hover/:focus/:active）状态变化
// 回调（bindings 注册）：除了 dom 层自身信息（attrVersion→resolver 缓存
// 失效），computedStyleFor 的结果缓存（bindings/csscache.go）只按全局
// 样式版本失效——hover/focus 状态变化（不影响样式表与属性）也必须失效，
// 否则 getComputedStyle 返回陈旧值（「鼠标移开 :hover 样式不恢复」根因）。
var DynamicPseudoStateChanged func(el *Element)

func notifyDynamicPseudoChanged(e *Element) {
	if DynamicPseudoStateChanged != nil {
		DynamicPseudoStateChanged(e)
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
		notifyDynamicPseudoChanged(e)
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
		notifyDynamicPseudoChanged(e)
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
		notifyDynamicPseudoChanged(e)
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


