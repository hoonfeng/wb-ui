// Package ui 提供 wb-ui 的「UI 库」构建层：用 Go 代码直接构建界面（基础
// 方式 = native），也可以在同一棵界面树里混入 HTML 片段（web 方式）。
//
// 与 webkit 的关系：wb-ui 既能当嵌入浏览器（webkit.NewWebView + 页面 HTML/
// JS），也能当 UI 库（webkit.NewWebViewWithMode(webkit.ModeToolkit) + 本包）。
// 两条路共用同一套 DOM/CSS/布局/渲染/事件管线——本包构建的节点就是引擎文档
// 树里的节点，与页面脚本创建的节点没有区别（可以互相查询、互相嵌套、接收
// 同一套事件）。这就是「某些作为基础、某些作为 web 方式提供」的接线方式。
//
// 基础用法（UI 库模式，宿主不写 HTML）：
//
//	wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
//	wv.Resize(400, 300)
//	view, err := ui.New(wv)
//	view.Style("body{margin:0;font:14px sans-serif} .card{padding:8px}")
//	view.Div().Class("card").
//		Append(
//			view.Text("你好"),
//			view.Button("点我", func(dom.Event) { clicks++ }),
//		)
//	pixels, err := wv.Render()
//
// 混入 web 片段（web 方式；页面脚本、Vue 组件、Markdown 渲染结果都可以）：
//
//	view.Web(`<button class="btn" onclick="go.ping()">web 按钮</button>`)
//
// 组件来源可切换（见 registry.go）：同一个界面里，一部分组件由 Go 构建
// （基础），一部分由 HTML 片段提供（web），宿主按需要在注册表里声明。
package ui

import (
	"errors"
	"strings"

	"wb-ui/dom"
	"wb-ui/webkit"
)

var (
	// ErrNilWebView 表示传入了 nil WebView。
	ErrNilWebView = errors.New("ui: nil WebView")
	// ErrNoDocument 表示 WebView 还没有文档（需要先 LoadHTML / ui.New）。
	ErrNoDocument = errors.New("ui: WebView has no document (LoadHTML first, or use ui.New)")
	// ErrNoBody 表示文档没有 <body>（HTML 不完整，无法作为 UI 宿主）。
	ErrNoBody = errors.New("ui: document has no <body>")
)

// EmptyDocument 是 ui.New 使用的初始文档：最小 HTML5 骨架，宿主不必写 HTML。
const EmptyDocument = `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body></body></html>`

// View 是绑定到某个 WebView 的 UI 构建视图。
type View struct {
	wv   *webkit.WebView
	doc  *dom.Document
	root *Node
	reg  *Registry
}

// Node 是一个 UI 元素的 Go 侧句柄（链式构建/更新）。
type Node struct {
	view *View
	el   *dom.Element
	// handlers 记录本节点上由 On 绑定的监听器（用于「同一事件重复 On 时
	// 替换旧处理器」的确定性语义）。
	handlers map[string]dom.EventListener
}

// ── 建立视图 ─────────────────────────────────────────────

// New 在空文档上建立 UI 视图（UI 库模式的标准入口）。
func New(wv *webkit.WebView) (*View, error) {
	return NewWithHTML(wv, EmptyDocument)
}

// NewWithHTML 在指定初始 HTML 上建立 UI 视图：页面 HTML 为主、Go 局部
// 构建/查询为辅的混合场景用它。
func NewWithHTML(wv *webkit.WebView, html string) (*View, error) {
	if wv == nil {
		return nil, ErrNilWebView
	}
	if err := wv.LoadHTML(html); err != nil {
		return nil, err
	}
	return Attach(wv)
}

// Attach 把 UI 视图挂到已加载文档的 WebView 上（不重新加载文档）。
func Attach(wv *webkit.WebView) (*View, error) {
	if wv == nil {
		return nil, ErrNilWebView
	}
	doc := wv.Document()
	if doc == nil {
		return nil, ErrNoDocument
	}
	body := doc.Body()
	if body == nil {
		return nil, ErrNoBody
	}
	v := &View{wv: wv, doc: doc, reg: NewRegistry()}
	v.root = &Node{view: v, el: body}
	return v, nil
}

// WebView 返回底层 WebView。
func (v *View) WebView() *webkit.WebView { return v.wv }

// Document 返回承载界面的文档。
func (v *View) Document() *dom.Document { return v.doc }

// Root 返回根节点（<body> 的包装）。
func (v *View) Root() *Node { return v.root }

// Registry 返回本视图的组件注册表（见 registry.go）。
func (v *View) Registry() *Registry { return v.reg }

// Render 把界面渲染为 RGBA 像素（宽*高*4 字节，行序自上而下）。宿主拿到
// 后自行上屏（OverlayWindow / 位图拷贝 / 编码成 PNG）。
func (v *View) Render() ([]byte, error) {
	if v == nil || v.wv == nil {
		return nil, ErrNilWebView
	}
	return v.wv.Render()
}

// markDirty 标记渲染树需要重建 + 需要布局：Go 侧 DOM 变更（本包所有写操作
// 都经它）与页面脚本变更走同一条路径——宿主下一次 Render() 时生效。
func (v *View) markDirty() {
	if v == nil || v.wv == nil {
		return
	}
	wf := v.wv.MainFrame()
	if wf == nil {
		return
	}
	fr := wf.Frame()
	if fr == nil {
		return
	}
	fr.MarkRenderTreeDirty()
	fr.SetNeedsLayout(true)
}

// ── 基础方式（native）：Go 构建节点 ─────────────────────

// El 创建一个元素并挂到根节点（顶层元素语义）。需要构造子树时先建父节点
// 再 Append 子节点；Append 与浏览器 appendChild 一样是「移动」语义。
func (v *View) El(tag string) *Node {
	if v == nil || v.doc == nil {
		return nil
	}
	el := v.doc.CreateElement(tag)
	n := &Node{view: v, el: el}
	if v.root != nil && v.root.el != nil && v.root.el != el {
		_ = v.root.el.AppendChild(el)
	}
	v.markDirty()
	return n
}

// Div 创建一个 <div> 并挂到根节点。
func (v *View) Div() *Node { return v.El("div") }

// Span 创建一个 <span> 并挂到根节点。
func (v *View) Span() *Node { return v.El("span") }

// Text 创建一个承载纯文本的 <span> 并挂到根节点。
func (v *View) Text(s string) *Node { return v.El("span").Text(s) }

// Label 创建一个 <label>（可选用 forID 关联表单控件）。
func (v *View) Label(text string, forID string) *Node {
	n := v.El("label").Text(text)
	if forID != "" {
		n.Attr("for", forID)
	}
	return n
}

// Button 创建一个 <button>：标签文本 + 点击回调（回调在引擎的事件分发里
// 同步执行，与页面 JS 的 click 监听器同一条路径）。
func (v *View) Button(label string, onClick func(dom.Event)) *Node {
	n := v.El("button").Text(label)
	if onClick != nil {
		n.On("click", onClick)
	}
	return n
}

// Input 创建一个 <input>（kind 默认 text）。
func (v *View) Input(kind, value string) *Node {
	if strings.TrimSpace(kind) == "" {
		kind = "text"
	}
	n := v.El("input").Attr("type", kind)
	if value != "" {
		n.Attr("value", value)
	}
	return n
}

// Style 注入一张内联样式表（<style>）到 <head>。多次调用每次追加一张，
// 后注入的规则优先（与浏览器相同）。返回 <style> 节点句柄，便于更新：
//
//	st := view.Style(".card{padding:8px}")
//	st.Element().SetTextContent(".card{padding:12px}")  // 更新后再 markDirty
func (v *View) Style(css string) *Node {
	if v == nil || v.doc == nil {
		return nil
	}
	host := v.doc.Head()
	if host == nil {
		host = v.doc.Body()
	}
	if host == nil {
		return nil
	}
	el := v.doc.CreateElement("style")
	el.SetAttribute("data-ui-style", "")
	_ = el.SetTextContent(css)
	_ = host.AppendChild(el)
	// <style> 注入是引擎已知的动态样式路径（onStyleNodeAdded → 样式重扫 +
	// 渲染树重建）；这里再标一次脏，保证宿主直接调用时也生效。
	v.markDirty()
	return &Node{view: v, el: el}
}

// ── web 方式：HTML 片段 ─────────────────────────────────

// Web 以 web 方式在根节点内追加一段 HTML 片段（引擎的片段解析器解析，
// 与 el.innerHTML = … 同一条路径）。
func (v *View) Web(fragment string) *Node {
	if v == nil || v.root == nil {
		return nil
	}
	return v.root.Web(fragment)
}

// Web 在节点内追加一段 HTML 片段，返回承载片段的容器节点（<div
// data-ui-web>）；片段解析出的节点是它的子级，可用 Element() 继续查询/
// 绑定（例如 Element().GetElementById("x")）。解析失败返回 nil。
func (n *Node) Web(fragment string) *Node {
	if n == nil || n.el == nil || n.view == nil {
		return nil
	}
	holder := n.view.doc.CreateElement("div")
	holder.SetAttribute("data-ui-web", "")
	if err := holder.SetInnerHTML(fragment); err != nil {
		return nil
	}
	_ = n.el.AppendChild(holder)
	n.view.markDirty()
	return &Node{view: n.view, el: holder}
}

// ── 节点操作 ─────────────────────────────────────────────

// Element 返回底层 DOM 元素：宿主可用引擎的 dom API 做更细的操作
// （GetElementById/GetElementsByTagName/SetAttribute/…）。
func (n *Node) Element() *dom.Element {
	if n == nil {
		return nil
	}
	return n.el
}

// ID 设置 id 属性。
func (n *Node) ID(id string) *Node { return n.Attr("id", id) }

// Attr 设置属性。
func (n *Node) Attr(name, value string) *Node {
	if n == nil || n.el == nil {
		return n
	}
	n.el.SetAttribute(name, value)
	n.view.markDirty()
	return n
}

// Class 追加 class（已存在则不重复添加）。
func (n *Node) Class(classes ...string) *Node {
	if n == nil || n.el == nil {
		return n
	}
	cur := strings.Fields(n.el.GetClassName())
	have := make(map[string]bool, len(cur))
	for _, c := range cur {
		have[c] = true
	}
	changed := false
	for _, c := range classes {
		c = strings.TrimSpace(c)
		if c == "" || have[c] {
			continue
		}
		cur = append(cur, c)
		have[c] = true
		changed = true
	}
	if changed {
		n.el.SetClassName(strings.Join(cur, " "))
		n.view.markDirty()
	}
	return n
}

// Style 追加一条内联样式（prop:value），已有内联样式保留。
func (n *Node) Style(prop, value string) *Node {
	if n == nil || n.el == nil {
		return n
	}
	prop = strings.TrimSpace(prop)
	if prop == "" {
		return n
	}
	cur := strings.TrimSpace(n.el.GetAttribute("style"))
	add := prop + ":" + strings.TrimSpace(value) + ";"
	if cur != "" {
		add = cur + " " + add
	}
	n.el.SetAttribute("style", add)
	n.view.markDirty()
	return n
}

// Text 把节点内容设置为纯文本（替换原有子节点）。
func (n *Node) Text(s string) *Node {
	if n == nil || n.el == nil {
		return n
	}
	_ = n.el.SetTextContent(s)
	n.view.markDirty()
	return n
}

// Append 把子节点挂到本节点下（浏览器 appendChild 语义：已在别处的节点会
// 被移动过来）。本节点若尚未在文档里，会自动挂到根节点——宿主不必手写
// 「构建完再挂载」这一步。
func (n *Node) Append(children ...*Node) *Node {
	if n == nil || n.el == nil {
		return n
	}
	for _, c := range children {
		if c == nil || c.el == nil {
			continue
		}
		if p := c.el.ParentNode(); p != nil {
			if p == dom.Node(n.el) {
				continue // 已经是本节点的子节点
			}
			_ = p.RemoveChild(c.el)
		}
		_ = n.el.AppendChild(c.el)
	}
	if n.view != nil && n.view.root != nil && n.view.root.el != n.el && n.el.ParentNode() == nil {
		_ = n.view.root.el.AppendChild(n.el)
	}
	if n.view != nil {
		n.view.markDirty()
	}
	return n
}

// On 绑定事件监听器（引擎的事件分发路径：命中测试 → 目标 → 冒泡）。
//
// 幂等语义：同一事件重复调用 On 会替换上一次由 On 绑定的处理器（先移除
// 旧的），因此重新渲染/重新构建时不会累积重复回调。返回本节点便于链式。
func (n *Node) On(event string, fn func(dom.Event)) *Node {
	if n == nil || n.el == nil || fn == nil {
		return n
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return n
	}
	if n.handlers == nil {
		n.handlers = map[string]dom.EventListener{}
	}
	if old, ok := n.handlers[event]; ok && old != nil {
		n.el.RemoveEventListener(event, old, false)
	}
	l := dom.EventListenerFunc(fn)
	n.handlers[event] = l
	n.el.AddEventListener(event, l, false)
	return n
}

// Remove 把本节点从树上摘除（不影响其子节点引用，宿主可再次 Append 回来）。
func (n *Node) Remove() *Node {
	if n == nil || n.el == nil {
		return n
	}
	if p := n.el.ParentNode(); p != nil {
		_ = p.RemoveChild(n.el)
	}
	if n.view != nil {
		n.view.markDirty()
	}
	return n
}
