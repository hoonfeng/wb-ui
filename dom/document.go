// Translation of: Source/WebCore/dom/Document.h
//                  Source/WebCore/dom/Document.cpp
// Completeness: 80%
// Simplifications:
//   - Document embeds nodeBase (no separate ContainerNode/TreeScope layer)
//   - DOMImplementation, parser integration, style recalc, scripting and security
//     origin machinery are omitted
//   - head/body/title are resolved lazily by querying the document element rather than
//     being maintained as mutation-tracked pointers
//   - createEvent supports the event types implemented in this port

package dom

import "strings"

// Document is the Go translation of WebCore::Document. It is the root of a DOM tree and
// owns the node-creation factories. Nodes created through a Document record that
// document as their owner so that IsConnected and event dispatch work correctly.
type Document struct {
	nodeBase
	title        string
	contentType  string
	url          string
	quirks       bool

	// focusedEl 缓存当前 focused 元素（SetFocused 维护）。bindings 的
	// document.hasFocus()/activeElement 用它 O(1) 查询——此前每次遍历
	// 全文档元素（O(N)，CM6 每 measure 调 hasFocus → 事件响应慢贡献者）。
	focusedEl *Element

	// onTreeChange 是 DOM 结构变更（appendChild/insertBefore/removeChild/
	// setTextContent/setNodeValue 等）通知回调。宿主（app.Host）注册它来
	// MarkRenderTreeDirty——否则动态 DOM 更新（如 xterm 每字符 appendChild
	// span 到 rows）不触发渲染树重建，内容永远不显示。
	// ★ 回调在 DOM 操作线程同步调用；宿主侧只置标志（下帧重建），无重入。
	// ★ 回调携带触发变更的节点，为后续增量渲染树更新（RenderTreeUpdater）
	//   提供精确的变更定位（此前无参数，宿主只能全量重建）。
	onTreeChange func(Node)

	// fullscreenElement 是当前处于全屏的元素（HTML §4.11.6 全屏 API 的文档
	// 状态）。本引擎不接管窗口/顶层的真实全屏渲染——这里维护规范要求的状态，
	// 供 CSS `:fullscreen` 与 fullscreenchange 事件消费；是否真的把宿主窗口切到
	// 全屏由宿主（bindings 的使用方）决定。
	fullscreenElement *Element

	// popover 是文档上的 popover 子系统状态（HTML §6.12）：top layer 中显示中
	// 的 popover 序列、hint 栈父元素、light dismiss 的 pointerdown 记忆与重入
	// 标记。字段定义见 popoverstate.go；由 wb-ui/popover 包维护，dom 包本身
	// 不改变它。
	popover PopoverDocumentState
}

// FullscreenElement 返回当前处于全屏的元素（无则 nil）。
func (d *Document) FullscreenElement() *Element { return d.fullscreenElement }

// SetFullscreenElement 设置/清除全屏元素（全屏 API 的实现方调用）。
func (d *Document) SetFullscreenElement(e *Element) { d.fullscreenElement = e }

// FocusedElement 返回当前 focused 元素（无则 nil），O(1)。
func (d *Document) FocusedElement() *Element { return d.focusedEl }

// SetFocusedElement 记录 focused 元素（Element.SetFocused 维护，嵌合方
// 不需要直接调用）。
func (d *Document) SetFocusedElement(e *Element) { d.focusedEl = e }

// SetTreeChangeCallback 注册 DOM 结构变更回调（宿主在 LoadHTML 后调用）。
func (d *Document) SetTreeChangeCallback(fn func(Node)) {
	d.onTreeChange = fn
}

// notifyTreeChange 在文档树结构变更时通知宿主（仅当节点已挂入文档树）。
func (b *nodeBase) notifyTreeChange() {
	if !b.isInDocumentTree() {
		return
	}
	d := b.documentForAdoption()
	if d != nil && d.onTreeChange != nil {
		d.onTreeChange(b.self)
	}
}

// NewDocument creates an empty Document. It is the root of a tree and is its own owner
// document for adoption purposes (OwnerDocument returns nil for a Document itself).
func NewDocument() *Document {
	d := &Document{contentType: "text/html"}
	d.initNodeBase(d, d, NodeDocument)
	return d
}

// NodeName for a Document is "#document", mirroring Document::nodeName().
func (d *Document) NodeName() string { return "#document" }

// NodeValue for a Document is the empty string, mirroring Node::nodeValue().
func (d *Document) NodeValue() string { return "" }

// SetNodeValue has no effect on a Document, mirroring Node::setNodeValue().
func (d *Document) SetNodeValue(string) error { return nil }

// cloneShallow returns a fresh empty Document, mirroring cloneNodeInternal for Document.
func (d *Document) cloneShallow(_ *Document) Node {
	c := NewDocument()
	c.title = d.title
	c.contentType = d.contentType
	c.url = d.url
	return c
}

// CreateElement returns a new Element owned by this document, mirroring
// Document::createElement(tagName). It first checks the element factory for a
// registered constructor (set via RegisterElement); if none is found, it falls
// back to NewElement. This allows packages like html5 to register constructors
// for <input>, <select>, <textarea>, etc. so that the created element carries
// the correct tag and can be used with the type-safe wrapper functions.
func (d *Document) CreateElement(tagName string) *Element {
	key := strings.ToLower(tagName)
	if ctor, ok := elementFactory[key]; ok {
		return ctor(d, tagName)
	}
	return NewElement(d, tagName)
}

// CreateTextNode returns a new Text node owned by this document, mirroring
// Document::createTextNode(data).
func (d *Document) CreateTextNode(data string) *Text {
	return NewText(d, data)
}

// CreateComment returns a new Comment node owned by this document, mirroring
// Document::createComment(data).
func (d *Document) CreateComment(data string) *Comment {
	return NewComment(d, data)
}

// CreateDocumentFragment returns a new DocumentFragment owned by this document,
// mirroring Document::createDocumentFragment().
func (d *Document) CreateDocumentFragment() *DocumentFragment {
	return NewDocumentFragment(d)
}

// CreateEvent constructs an Event of the given type, mirroring Document::createEvent.
// Supported types: "Event", "Events", "MouseEvent", "KeyboardEvent", "WheelEvent",
// "FocusEvent".
func (d *Document) CreateEvent(eventInterface string) Event {
	switch strings.ToLower(eventInterface) {
	case "event", "events", "htmlevents":
		ev := newBaseEvent("", false, false, false, false)
		return &ev
	case "mouseevent", "mouseevents":
		return NewMouseEvent("", false, false, false)
	case "keyboardevent", "keyboardevents":
		return NewKeyboardEvent("", false, false, false)
	case "wheelevent", "wheelevents":
		return NewWheelEvent("", false, false)
	case "focusevent", "focusevents":
		return NewFocusEvent("", false, false)
	}
	return nil
}

// Quirks returns whether the document is in quirks mode, mirroring
// Document::inQuirksMode(). In standards mode (CSS1Compat) this returns false.
func (d *Document) Quirks() bool { return d.quirks }

// SetQuirks sets the document's quirks mode flag.
func (d *Document) SetQuirks(v bool) { d.quirks = v }

// DocumentElement returns the root element of the document (typically <html>),
// mirroring Document::documentElement(). It is the first Element child of the document.
func (d *Document) DocumentElement() *Element {
	for c := d.firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if el, ok := c.(*Element); ok {
			return el
		}
	}
	return nil
}

// GetElementById returns the first element in the document whose id matches, mirroring
// Document::getElementById.
func (d *Document) GetElementById(id string) *Element {
	var found *Element
	walkDescendantsNode(d, func(n Node) bool {
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

// GetElementsByTagName returns all elements in the document whose tag matches,
// mirroring Document::getElementsByTagName.
func (d *Document) GetElementsByTagName(tagName string) []*Element {
	var out []*Element
	want := strings.ToLower(tagName)
	all := want == "*"
	walkDescendantsNode(d, func(n Node) bool {
		if el, ok := n.(*Element); ok {
			if all || el.LocalName() == want {
				out = append(out, el)
			}
		}
		return true
	})
	return out
}

// GetElementsByClassName returns all elements in the document with token in their class
// list, mirroring Document::getElementsByClassName.
func (d *Document) GetElementsByClassName(token string) []*Element {
	if token == "" {
		return nil
	}
	var out []*Element
	walkDescendantsNode(d, func(n Node) bool {
		if el, ok := n.(*Element); ok {
			if el.HasClassName(token) {
				out = append(out, el)
			}
		}
		return true
	})
	return out
}

// Head returns the first <head> element under the document element, mirroring
// Document::head(). It returns nil when no document element or no head exists.
func (d *Document) Head() *Element {
	root := d.DocumentElement()
	if root == nil {
		return nil
	}
	for c := nodeBaseOf(root).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if el, ok := c.(*Element); ok && el.LocalName() == "head" {
			return el
		}
	}
	return nil
}

// Body returns the first <body> element under the document element, mirroring
// Document::body().
func (d *Document) Body() *Element {
	root := d.DocumentElement()
	if root == nil {
		return nil
	}
	for c := nodeBaseOf(root).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if el, ok := c.(*Element); ok && el.LocalName() == "body" {
			return el
		}
	}
	return nil
}

// Title returns the document title (the text content of the first <title> element),
// mirroring Document::title().
func (d *Document) Title() string {
	head := d.Head()
	if head == nil {
		return d.title
	}
	for c := nodeBaseOf(head).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if el, ok := c.(*Element); ok && el.LocalName() == "title" {
			return strings.TrimSpace(el.TextContent())
		}
	}
	return d.title
}

// SetTitle sets the document title, mirroring Document::setTitle(). It updates the
// existing <title> element when present, otherwise stores a fallback value.
func (d *Document) SetTitle(title string) {
	d.title = title
	head := d.Head()
	if head == nil {
		return
	}
	for c := nodeBaseOf(head).firstChild; c != nil; c = nodeBaseOf(c).nextSibling {
		if el, ok := c.(*Element); ok && el.LocalName() == "title" {
			_ = el.SetTextContent(title)
			return
		}
	}
}

// ContentType returns the document's MIME type, mirroring Document::contentType().
func (d *Document) ContentType() string { return d.contentType }

// SetContentType sets the MIME type.
func (d *Document) SetContentType(ct string) { d.contentType = ct }

// URL returns the document URL, mirroring Document::URL().
func (d *Document) URL() string { return d.url }

// SetURL sets the document URL.
func (d *Document) SetURL(u string) { d.url = u }

// --- Element Factory (specialized element registration) ---

// ElementConstructor is a function that creates a specialized *Element for the
// given tag name. Constructors are registered via RegisterElement and are used
// by Document.CreateElement to produce elements with the correct underlying
// representation for a given tag (e.g. <input>, <select>, <textarea>).
// The doc parameter is the owner document; tagName is the original-case tag
// name passed to CreateElement.
type ElementConstructor func(doc *Document, tagName string) *Element

// elementFactory maps lowercase tag names to their registered constructors.
// It is populated by init() functions in packages that call RegisterElement.
var elementFactory = map[string]ElementConstructor{}

// RegisterElement registers a constructor for the given tag name (lowercased
// internally). When Document.CreateElement is called with a matching tag name,
// the registered constructor is invoked instead of the default NewElement.
//
// RegisterElement is intended for use in package init() functions, e.g. from
// the html5 package to register constructors for form-associated elements.
// If a constructor is already registered for the same tag, the last call wins.
func RegisterElement(tagName string, ctor ElementConstructor) {
	elementFactory[strings.ToLower(tagName)] = ctor
}

// RegisteredElementCount returns the number of registered element constructors.
// Useful for testing.
func RegisteredElementCount() int {
	return len(elementFactory)
}

// ClearRegisteredElements removes all registered element constructors. Useful
// for testing isolation.
func ClearRegisteredElements() {
	elementFactory = map[string]ElementConstructor{}
}
