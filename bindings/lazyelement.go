// bindings/lazyelement.go — 惰性元素包装器（性能优化）。
//
// wrapElement 此前为每个新建元素立即安装 ~90 个自有属性（函数闭包 +
// accessor），每元素 ~60µs（jsc.SetAccessor/newNativeFunc + GC）——CM6 文件
// 打开重绘、Vue 文件树、xterm 行重建的 DOM 构建慢主因（8000 节点 570ms）。
// 现在元素包装器是 goja DynamicObject：属性在首次访问时经
// installElementProperty 物化（data 值缓存，accessor 每次求值），
// createElement 只付对象 + Internal 成本。
//
// 语义保持：所有属性闭包与 wrapElement 原实现逐字一致（捕获 el），
// data 属性（classList/style/dataset）按元素缓存保证同一性
// （el.classList === el.classList），accessor 保持 live 求值，
// JS 可覆写方法（expando 遮蔽）。
package bindings

import (
	"bytes"
	"encoding/base64"
	"math"
	"strings"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/rendering"
)

// elemAccessor 表示一个 accessor 属性（getter-only 时 set 为 nil）。
type elemAccessor struct {
	get func() jsc.JSValue
	set func(v jsc.JSValue) // nil = getter-only（赋值静默忽略）
}

// lazyElemProps 实现 jsc.LazyPropSet：按属性名惰性物化元素包装器属性。
type lazyElemProps struct {
	el      *dom.Element
	interp  *jsc.Interpreter
	cached  map[string]jsc.JSValue // 已物化的 data 属性（含覆写前的原始值）
	expando map[string]jsc.JSValue // JS 侧自定义属性/方法覆写

	// onclickFn 是用户通过 el.onclick = fn 设置的处理器（IDL 事件处理器
	// 属性，与内联 onclick attribute 联动：getter 优先返回它，其次返回
	// 基于 attribute 代码的包装函数）。
	onclickFn jsc.JSValue
}

// elemAccessorProps：accessor（活值）属性名集合——每次读取重新求值，
// 适配器层不得缓存（缓存的 firstChild 会让 Vue insertStaticContent 的
// while(wrapper.firstChild) 循环永远看到同一个节点 → 死循环/挂死）。
var elemAccessorProps = map[string]bool{
	"parentNode": true, "parentElement": true, "nextSibling": true, "previousSibling": true,
	"firstChild": true, "lastChild": true, "childElementCount": true, "children": true, "childNodes": true,
	"ownerDocument": true,
	"scrollTop": true, "scrollLeft": true, "scrollHeight": true, "scrollWidth": true,
	"clientHeight": true, "clientWidth": true, "offsetHeight": true, "offsetWidth": true,
	"offsetTop": true, "offsetLeft": true,
	"value": true, "checked": true, "type": true, "disabled": true,
	"selectionStart": true, "selectionEnd": true,
	"multiple": true, "selectedIndex": true, "options": true, "selectedOptions": true, "selected": true,
	"tagName": true, "nodeName": true, "nodeType": true, "shadowRoot": true, "nodeValue": true,
	"id": true, "className": true, "title": true, "src": true,
	"attributes": true, "innerHTML": true, "outerHTML": true, "textContent": true, "content": true,
	"onclick": true,
	"track":   true, "textTracks": true,
	// <dialog> / <details> 的 live 状态：open 与 returnValue 会随
	// show/showModal/close 变化，缓存住就会读到过期值（:modal / :open 匹配
	// 与事件回调里读 d.open 都会错）。
	"open": true, "returnValue": true,
}

// Live 实现 jsc.LazyLiveProps：accessor 属性每次读取重新求值。
// 媒体属性（<video>/<audio>）也是 live——duration/paused/readyState 会随
// 状态机变化，缓存住就会读到过期值。
func (p *lazyElemProps) Live(key string) bool {
	return elemAccessorProps[key] || hasMediaElementProp(p.el, key)
}

func (p *lazyElemProps) Get(key string) jsc.JSValue {
	if key == "" {
		return jsc.Undefined()
	}
	// onclick：IDL 事件处理器属性（live 求值，先于 expando）。
	if key == "onclick" {
		return p.getOnClick()
	}
	if v, ok := p.expando[key]; ok {
		return v
	}
	if v, ok := p.cached[key]; ok {
		return v
	}
	val, acc, ok := installElementProperty(p.interp, p.el, key)
	if !ok {
		return jsc.Undefined()
	}
	if acc != nil {
		if acc.get != nil {
			return acc.get()
		}
		return jsc.Undefined()
	}
	p.cached[key] = val
	return val
}

func (p *lazyElemProps) Set(key string, v jsc.JSValue) bool {
	if key == "" {
		return false
	}
	// onclick：IDL 事件处理器属性（live 求值，先于 expando）。
	if key == "onclick" {
		p.setOnClick(v)
		return true
	}
	// accessor 属性走 setter；getter-only 静默忽略（浏览器语义）。
	if _, acc, ok := installElementProperty(p.interp, p.el, key); ok && acc != nil {
		if acc.set != nil {
			acc.set(v)
		}
		return true
	}
	// data 属性/未知属性：expando 覆写（el.appendChild = fn 与浏览器一致）。
	delete(p.cached, key)
	p.expando[key] = v
	return true
}

func (p *lazyElemProps) Has(key string) bool {
	if _, ok := elemKnownProps[key]; ok {
		return true
	}
	if hasMediaElementProp(p.el, key) {
		return true
	}
	_, ok := p.expando[key]
	return ok
}

// getOnClick 返回 onclick 处理器（浏览器 IDL 事件处理器属性语义）：
//   - 用户通过 el.onclick = fn 设置过 → 返回该函数（可 el.onclick() 调用）
//   - 否则元素带内联 onclick attribute → 返回基于属性代码的包装函数
//     （调用时经 RunJS 执行属性代码，模拟浏览器「onclick 属性代码即处理器」）
//   - 都没有 → null
func (p *lazyElemProps) getOnClick() jsc.JSValue {
	if !p.onclickFn.IsUndefined() {
		return p.onclickFn
	}
	code := p.el.GetAttribute("onclick")
	if code == "" {
		return jsc.Null()
	}
	interp := p.interp
	return jsc.FunctionValue(jsc.NewNativeFunction("onclick", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		if interp == nil {
			return jsc.Undefined()
		}
		v, err := interp.RunJS(code)
		if err != nil {
			return jsc.Undefined()
		}
		return v
	}, 0))
}

// setOnClick 设置 onclick 处理器（浏览器 IDL 语义）：
//   - 函数 → 注册为 click 监听器（替换旧的 el.onclick 处理器）
//   - null / undefined → 移除已注册的处理器
//   - 其他值 → 字符串反射为 onclick attribute（host.go/configwin 的
//     HitTest("onclick") 读 attribute 执行，保持两种方式一致）
func (p *lazyElemProps) setOnClick(v jsc.JSValue) {
	if old := p.el.GetOnClickJSListener(); old != nil {
		p.el.RemoveEventListener("click", old, false)
		p.el.SetOnClickJSListener(nil)
	}
	p.onclickFn = jsc.JSValue{}
	if v.IsNull() || v.IsUndefined() {
		return
	}
	if v.IsFunction() {
		p.onclickFn = v
		l := &jsListener{interp: p.interp, fn: v}
		p.el.SetOnClickJSListener(l)
		p.el.AddEventListener("click", l, false)
		return
	}
	p.el.SetAttribute("onclick", v.ToString())
}

func (p *lazyElemProps) Delete(key string) bool {
	delete(p.expando, key)
	delete(p.cached, key)
	return true
}

func (p *lazyElemProps) Keys() []string {
	out := make([]string, 0, len(elemKnownPropNames)+len(p.expando))
	out = append(out, elemKnownPropNames...)
	// 媒体元素额外列出 HTMLMediaElement 的属性（按标签判定——div 等不含）。
	if p.el != nil {
		switch p.el.LocalName() {
		case "video", "audio":
			out = append(out, mediaElementPropList...)
		}
	}
	for k := range p.expando {
		out = append(out, k)
	}
	return out
}

// elemKnownProps / elemKnownPropNames：元素包装器的全部已知属性名
// （installElementProperty 的 case 全集 + constructor）。
var (
	elemKnownProps     = map[string]bool{}
	elemKnownPropNames = []string{
		"constructor",
		"appendChild", "removeChild", "insertBefore", "replaceChild", "replaceChildren",
		"contains", "cloneNode", "hasChildNodes", "isConnected",
		"matches", "closest", "querySelector", "querySelectorAll", "insertAdjacentHTML",
		"dataset", "classList", "style",
		"addEventListener", "removeEventListener", "dispatchEvent",
		"parentNode", "parentElement", "nextSibling", "previousSibling",
		"firstChild", "lastChild", "childElementCount", "children", "childNodes",
		"ownerDocument",
		"scrollTop", "scrollLeft", "scrollHeight", "scrollWidth",
		"clientHeight", "clientWidth", "offsetHeight", "offsetWidth", "offsetTop", "offsetLeft",
		"onclick",
		"getBoundingClientRect", "getClientRects", "scrollIntoView",
		"remove", "focus", "blur",
		"value", "checked", "type", "disabled",
		// indeterminate 是 input 的 IDL 状态（:indeterminate 读它），必须
		// 登记，否则 `"indeterminate" in input` 为 false（Has 只看这张表）。
		"indeterminate",
		"selectionStart", "selectionEnd", "setSelectionRange",
		"multiple", "selectedIndex", "options", "selectedOptions", "selected",
		"tagName", "nodeName", "nodeType", "getRootNode", "attachShadow", "shadowRoot",
		"compareDocumentPosition", "nodeValue",
		"id", "className", "title", "src",
		"attributes", "innerHTML", "outerHTML", "textContent", "content",
		"getContext", "width", "height", "toDataURL",
		"track", "textTracks",
		"requestFullscreen", "exitFullscreen", "fullscreenElement", "fullscreenEnabled",
		"open", "show", "showModal", "close", "returnValue",
	}
)

func init() {
	for _, k := range elemKnownPropNames {
		elemKnownProps[k] = true
	}
}

// installElementProperty 按属性名物化元素包装器的一个属性。返回 data 值
// （val）或 accessor（acc），未知属性返回 ok=false。所有实现与 wrapElement
// 原属性定义逐字一致（捕获 el）。
func installElementProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	tag := strings.ToLower(el.LocalName())
	// <video>/<audio> 的 HTMLMediaElement 属性（媒体状态机、事件、Promise 语义）
	// 集中在 media_element.go 实现；这里做一次标签判定后委派，避免在下面的大
	// switch 里散落 30+ 个媒体 case。
	if isMediaElementTag(tag) && isMediaElementProp(key) {
		if v, acc, ok := installMediaElementProperty(rt, el, key); ok {
			return v, acc, true
		}
	}
	switch key {
	case "constructor":
		o := jsc.NewObject(rt.ObjectPrototype())
		o.Set("name", jsc.StringValue("Element"))
		return jsc.ObjectValue(o), nil, true

	// ── Node tree ──
	case "appendChild":
		return funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
			if n == nil {
				return jsc.Null()
			}
			el.AppendChild(n)
			if OnNodeInserted != nil {
				OnNodeInserted(n)
			}
			if OnStyleNodeAdded != nil && isStyleElement(n) {
				OnStyleNodeAdded(n)
			}
			return a
		})), nil, true
	case "removeChild":
		return funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
			if n == nil {
				return jsc.Null()
			}
			el.RemoveChild(n)
			if OnNodeRemoved != nil {
				OnNodeRemoved(n)
			}
			return a
		})), nil, true
	case "insertBefore":
		return funcVal(fn2Node(func(in *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
			if nc == nil {
				return jsc.Null()
			}
			// DocumentFragment: insert all children individually.
			if frag, ok := nc.(*dom.DocumentFragment); ok {
				for c := frag.FirstChild(); c != nil; c = frag.FirstChild() {
					frag.RemoveChild(c)
					if err := el.InsertBefore(c, rc); err != nil {
						// 容错：refChild 不在本节点下时回退为追加，避免 Vue vnode/DOM 不一致
						_ = el.AppendChild(c)
					}
					if OnNodeInserted != nil {
						OnNodeInserted(c)
					}
				}
				return a0
			}
			if err := el.InsertBefore(nc, rc); err != nil {
				// 浏览器对 anchor 不在父下的情况抛 NotFoundError；goja 环境 Vue 的
				// vnode/DOM 可能短暂不一致（anchor detached），静默失败会让元素
				// 永远不进 DOM 但 OnNodeInserted 照常触发 → vnode 认为已插入 →
				// 后续 v-if 关闭/卸载时 unmount 找不到正确 parent，DOM 不移除。
				// 回退追加保证元素真实进入 DOM，Vue 状态一致。
				_ = el.AppendChild(nc)
			}
			if OnNodeInserted != nil {
				OnNodeInserted(nc)
			}
			if isStyleElement(nc) {
				BumpStyleVersion()
			}
			return a0
		})), nil, true
	case "replaceChild":
		return funcVal(fn2Node(func(_ *jsc.Interpreter, nc, oc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
			if nc == nil || oc == nil {
				return jsc.Null()
			}
			el.ReplaceChild(nc, oc)
			if OnNodeRemoved != nil {
				OnNodeRemoved(oc)
			}
			if OnNodeInserted != nil {
				OnNodeInserted(nc)
			}
			return a1
		})), nil, true
	case "replaceChildren":
		// 浏览器标准：清空所有子节点后追加给定节点；字符串/数字参数自动
		// 转为 Text 节点（xterm.js DOM 渲染器用它重建终端行）。
		return jsc.FunctionValue(jsc.NewNativeFunction("replaceChildren",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				for c := el.FirstChild(); c != nil; c = el.FirstChild() {
					el.RemoveChild(c)
					if OnNodeRemoved != nil {
						OnNodeRemoved(c)
					}
				}
				for _, a := range args {
					var n dom.Node
					if a.IsObject() || a.IsCallable() {
						n = unwrapNode(a)
					}
					if n == nil {
						txt := dom.NewText(el.OwnerDocument(), a.ToString())
						el.AppendChild(txt)
						if OnNodeInserted != nil {
							OnNodeInserted(txt)
						}
						continue
					}
					el.AppendChild(n)
					if OnNodeInserted != nil {
						OnNodeInserted(n)
					}
					if isStyleElement(n) {
						BumpStyleVersion()
						if OnStyleNodeAdded != nil {
							OnStyleNodeAdded(n)
						}
					}
				}
				return jsc.Undefined()
			}, 1)), nil, true
	case "contains":
		return funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, _ jsc.JSValue) jsc.JSValue {
			if n == nil {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(el.Contains(n))
		})), nil, true
	case "cloneNode":
		return jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				deep := len(args) > 0 && args[0].ToBoolean()
				switch v := el.CloneNode(deep).(type) {
				case *dom.Element:
					return jsc.ObjectValue(wrapElement(in, v))
				case *dom.Text:
					return jsc.ObjectValue(wrapText(in, v))
				}
				return jsc.Null()
			}, 1)), nil, true
	case "hasChildNodes":
		return funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.BooleanValue(el.HasChildNodes())
		})), nil, true
	case "isConnected":
		return funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.BooleanValue(el.IsConnected())
		})), nil, true

	// ── CSS 选择器匹配 ──
	case "matches":
		return jsc.FunctionValue(jsc.NewNativeFunction("matches",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 {
					return jsc.BooleanValue(false)
				}
				return jsc.BooleanValue(ElementMatches(el, args[0].ToString()))
			}, 1)), nil, true
	case "closest":
		return jsc.FunctionValue(jsc.NewNativeFunction("closest",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 {
					return jsc.Null()
				}
				if found := ElementClosest(el, args[0].ToString()); found != nil {
					return jsc.ObjectValue(wrapElement(in, found))
				}
				return jsc.Null()
			}, 1)), nil, true
	case "querySelector":
		return jsc.FunctionValue(jsc.NewNativeFunction("querySelector",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 {
					return jsc.Null()
				}
				if found := ElementQuerySelector(el, args[0].ToString()); found != nil {
					return jsc.ObjectValue(wrapElement(in, found))
				}
				return jsc.Null()
			}, 1)), nil, true
	case "querySelectorAll":
		return jsc.FunctionValue(jsc.NewNativeFunction("querySelectorAll",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 {
					return arrElem(in, nil)
				}
				return arrElem(in, ElementQuerySelectorAll(el, args[0].ToString()))
			}, 1)), nil, true
	case "insertAdjacentHTML":
		return jsc.FunctionValue(jsc.NewNativeFunction("insertAdjacentHTML",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 2 {
					return jsc.Undefined()
				}
				el.InsertAdjacentHTML(args[0].ToString(), args[1].ToString())
				if OnStyleNodeAdded != nil {
					// Check for newly added <style> elements
					for c := el.FirstChild(); c != nil; c = c.NextSibling() {
						if isStyleElement(c) {
							OnStyleNodeAdded(c)
						}
					}
				}
				return jsc.Undefined()
			}, 2)), nil, true

	// ── dataset / classList / style（data 属性，按元素缓存保证同一性）──
	case "dataset":
		return jsc.ObjectValue(makeDataset(rt, el)), nil, true
	case "classList":
		return jsc.ObjectValue(makeClassList(rt, el)), nil, true
	case "style":
		return jsc.ObjectValue(makeStyleObject(rt, el)), nil, true

	// ── Events ──
	case "addEventListener":
		return jsc.FunctionValue(makeAddEventListener(el)), nil, true
	case "removeEventListener":
		return jsc.FunctionValue(makeRemoveEventListener(el)), nil, true
	case "dispatchEvent":
		return jsc.FunctionValue(makeDispatchEvent(el)), nil, true

	// ── Tree traversal（live accessor）──
	case "parentNode":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.ParentNode() })(rt, jsc.JSValue{})
		}}, true
	case "parentElement":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.ParentElement() })(rt, jsc.JSValue{})
		}}, true
	case "nextSibling":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.NextSibling() })(rt, jsc.JSValue{})
		}}, true
	case "previousSibling":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.PreviousSibling() })(rt, jsc.JSValue{})
		}}, true
	case "firstChild":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.FirstChild() })(rt, jsc.JSValue{})
		}}, true
	case "lastChild":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node { return el.LastChild() })(rt, jsc.JSValue{})
		}}, true
	case "childElementCount":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			n := 0
			for c := el.FirstChild(); c != nil; c = c.NextSibling() {
				if _, ok := c.(*dom.Element); ok {
					n++
				}
			}
			return jsc.NumberValue(float64(n))
		}}, true
	case "children":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				var els []*dom.Element
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if e, ok := c.(*dom.Element); ok {
						els = append(els, e)
					}
				}
				return arrElem(in, els)
			})(rt, jsc.JSValue{})
		}}, true
	case "childNodes":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				return arrNode(in, el.ChildNodes())
			})(rt, jsc.JSValue{})
		}}, true
	case "ownerDocument":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				return in.GlobalObject().GetOrZero("document")
			})(rt, jsc.JSValue{})
		}}, true

	// ── 滚动 / 尺寸 CSSOM 属性（真实几何，经渲染树桥）──
	case "scrollTop":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if GetElementScrollOffset == nil {
					return jsc.NumberValue(0)
				}
				_, y := GetElementScrollOffset(el)
				return jsc.NumberValue(y)
			},
			set: func(v jsc.JSValue) {
				if SetElementScrollOffset == nil {
					return
				}
				x := 0.0
				if GetElementScrollOffset != nil {
					x, _ = GetElementScrollOffset(el)
				}
				SetElementScrollOffset(el, x, v.ToNumber())
			}}, true
	case "scrollLeft":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if GetElementScrollOffset == nil {
					return jsc.NumberValue(0)
				}
				x, _ := GetElementScrollOffset(el)
				return jsc.NumberValue(x)
			},
			set: func(v jsc.JSValue) {
				if SetElementScrollOffset == nil {
					return
				}
				y := 0.0
				if GetElementScrollOffset != nil {
					_, y = GetElementScrollOffset(el)
				}
				SetElementScrollOffset(el, v.ToNumber(), y)
			}}, true
	case "scrollHeight":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementScrollMetrics == nil {
				return jsc.NumberValue(0)
			}
			_, _, _, th, _ := GetElementScrollMetrics(el)
			return jsc.NumberValue(th)
		}}, true
	case "scrollWidth":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementScrollMetrics == nil {
				return jsc.NumberValue(0)
			}
			_, _, tw, _, _ := GetElementScrollMetrics(el)
			return jsc.NumberValue(tw)
		}}, true
	case "clientHeight":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementScrollMetrics == nil {
				return jsc.NumberValue(0)
			}
			_, vh, _, _, _ := GetElementScrollMetrics(el)
			// ★ 浏览器标准：clientHeight 返回整数。
			return jsc.NumberValue(math.Round(vh))
		}}, true
	case "clientWidth":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementScrollMetrics == nil {
				return jsc.NumberValue(0)
			}
			vw, _, _, _, _ := GetElementScrollMetrics(el)
			// ★ 浏览器标准：clientWidth 返回整数。
			return jsc.NumberValue(math.Round(vw))
		}}, true
	case "offsetHeight":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementBoxRect == nil {
				return jsc.NumberValue(0)
			}
			_, _, _, h := GetElementBoxRect(el)
			// ★ 浏览器标准：offsetHeight 返回最接近的整数（四舍五入）。
			return jsc.NumberValue(math.Round(h))
		}}, true
	case "offsetWidth":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementBoxRect == nil {
				return jsc.NumberValue(0)
			}
			_, _, w, _ := GetElementBoxRect(el)
			// ★ 浏览器标准：offsetWidth 返回最接近的整数（四舍五入）。
			return jsc.NumberValue(math.Round(w))
		}}, true
	case "offsetTop":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementBoxRect == nil {
				return jsc.NumberValue(0)
			}
			_, top, _, _ := GetElementBoxRect(el)
			return jsc.NumberValue(math.Round(top))
		}}, true
	case "offsetLeft":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if GetElementBoxRect == nil {
				return jsc.NumberValue(0)
			}
			left, _, _, _ := GetElementBoxRect(el)
			return jsc.NumberValue(math.Round(left))
		}}, true

	// ── Position / dimension ──
	case "getBoundingClientRect":
		return jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				r := jsc.NewObject(in.ObjectPrototype())
				left, top, w, h := 0.0, 0.0, 0.0, 0.0
				if GetElementBoxRect != nil {
					left, top, w, h = GetElementBoxRect(el)
				}
				r.Set("x", jsc.NumberValue(left))
				r.Set("y", jsc.NumberValue(top))
				r.Set("width", jsc.NumberValue(w))
				r.Set("height", jsc.NumberValue(h))
				r.Set("top", jsc.NumberValue(top))
				r.Set("right", jsc.NumberValue(left+w))
				r.Set("bottom", jsc.NumberValue(top+h))
				r.Set("left", jsc.NumberValue(left))
				return jsc.ObjectValue(r)
			}, 0)), nil, true
	case "getClientRects":
		// 浏览器标准返回元素边框矩形的数组（[1 个 rect]）。CM6 的
		// clientRectsFor(元素) 用它取行内 span 的宽度/高度。
		return jsc.FunctionValue(jsc.NewNativeFunction("getClientRects",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				left, top, w, h := 0.0, 0.0, 0.0, 0.0
				if GetElementBoxRect != nil {
					left, top, w, h = GetElementBoxRect(el)
				}
				r := jsc.NewObject(in.ObjectPrototype())
				r.Set("x", jsc.NumberValue(left))
				r.Set("y", jsc.NumberValue(top))
				r.Set("width", jsc.NumberValue(w))
				r.Set("height", jsc.NumberValue(h))
				r.Set("top", jsc.NumberValue(top))
				r.Set("right", jsc.NumberValue(left+w))
				r.Set("bottom", jsc.NumberValue(top+h))
				r.Set("left", jsc.NumberValue(left))
				arr := jsc.NewArray(in.ObjectPrototype(), []jsc.JSValue{jsc.ObjectValue(r)})
				arr.Set("length", jsc.NumberValue(1))
				return jsc.ObjectValue(arr)
			}, 0)), nil, true
	case "scrollIntoView":
		return jsc.FunctionValue(jsc.NewNativeFunction("scrollIntoView",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.Undefined()
			}, 0)), nil, true
	case "remove":
		return jsc.FunctionValue(jsc.NewNativeFunction("remove",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				if p := el.ParentNode(); p != nil {
					p.RemoveChild(el)
				}
				return jsc.Undefined()
			}, 0)), nil, true
	case "focus":
		return jsc.FunctionValue(jsc.NewNativeFunction("focus",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				el.SetFocused(true)
				if FocusBridge != nil {
					FocusBridge(el, true)
				}
				return jsc.Undefined()
			}, 0)), nil, true
	case "blur":
		return jsc.FunctionValue(jsc.NewNativeFunction("blur",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				el.SetFocused(false)
				if FocusBridge != nil {
					FocusBridge(el, false)
				}
				return jsc.Undefined()
			}, 0)), nil, true

	// ── form control：value / checked / disabled / type ──
	case "value":
		if tag != "input" && tag != "select" && tag != "textarea" && tag != "button" && tag != "option" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				// select.value = 选中 option 的 value 属性或文本（浏览器语义）。
				// ★ value="" 显式设置时返回空串（标准）——此前空值回退
				// textContent，「value=""」选项读到的是显示文本（脏数据）。
				if tag == "select" {
					for c := el.FirstChild(); c != nil; c = c.NextSibling() {
						if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
							if opt.HasAttribute("selected") {
								if opt.HasAttribute("value") {
									return jsc.StringValue(opt.GetAttribute("value"))
								}
								return jsc.StringValue(opt.TextContent())
							}
						}
					}
					// HTML 标准：无显式 selected 且非 multiple 时返回第一个
					// 非 disabled option 的 value。
					if !el.HasAttribute("multiple") {
						for c := el.FirstChild(); c != nil; c = c.NextSibling() {
							if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") && !opt.HasAttribute("disabled") {
								if opt.HasAttribute("value") {
									return jsc.StringValue(opt.GetAttribute("value"))
								}
								return jsc.StringValue(opt.TextContent())
							}
						}
					}
					return jsc.StringValue("")
				}
				// textarea.value = 初始文本内容（浏览器语义：value 反射文本）。
				if tag == "textarea" {
					return jsc.StringValue(el.TextContent())
				}
				return jsc.StringValue(el.GetAttribute("value"))
			},
			set: func(v jsc.JSValue) {
				if tag == "select" {
					target := v.ToString()
					idx := -1
					i := 0
					for c := el.FirstChild(); c != nil; c = c.NextSibling() {
						if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
							val := opt.GetAttribute("value")
							// ★ 仅未设置 value 属性时回退文本（显式 value="" 保持空串）
							if !opt.HasAttribute("value") {
								val = opt.TextContent()
							}
							if val == target {
								idx = i
							}
							i++
						}
					}
					i = 0
					for c := el.FirstChild(); c != nil; c = c.NextSibling() {
						if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
							if i == idx {
								opt.SetAttribute("selected", "selected")
							} else {
								opt.RemoveAttribute("selected")
							}
							i++
						}
					}
					return
				}
				// textarea.value = 初始文本内容（浏览器语义：value 反射文本）。
				if tag == "textarea" {
					el.SetTextContent(v.ToString())
					return
				}
				el.SetAttribute("value", v.ToString())
			}}, true
	case "checked":
		if tag != "input" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("checked"))
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("checked", "checked")
				} else {
					el.RemoveAttribute("checked")
				}
			}}, true
	case "type":
		if tag != "input" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if t := el.GetAttribute("type"); t != "" {
					return jsc.StringValue(t)
				}
				// 浏览器默认 input.type = "text"
				return jsc.StringValue("text")
			},
			set: func(v jsc.JSValue) {
				el.SetAttribute("type", v.ToString())
			}}, true
	case "indeterminate":
		// HTMLInputElement.indeterminate：**IDL 状态**而不是内容属性（HTML
		// §4.16.3）——不写 DOM 属性，只存 dom.Element 上的标志（CSS
		// :indeterminate 的 checkbox 分支读它）。规范中它对所有 input 类型都
		// 存在（非 checkbox 无视觉效果），这里同样只要求 tag == "input"。
		if tag != "input" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(el.IsIndeterminate()) },
			set: func(v jsc.JSValue) {
				want := v.ToBoolean()
				if want == el.IsIndeterminate() {
					return
				}
				el.SetIndeterminate(want)
				// 匹配结果变了（:indeterminate）→ 走样式失效链。
				invalidateStateStyle(el)
			}}, true
	case "disabled":
		if tag != "input" && tag != "select" && tag != "textarea" && tag != "button" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("disabled"))
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("disabled", "disabled")
				} else {
					el.RemoveAttribute("disabled")
				}
			}}, true

	// ── Editable control selection (input/textarea) ──
	case "selectionStart", "selectionEnd", "setSelectionRange":
		if tag != "input" && tag != "textarea" {
			return jsc.JSValue{}, nil, false
		}
		selGetter := func() (int, int) {
			if SelectionBridge != nil {
				if s, e := SelectionBridge(el); s >= 0 && e >= 0 {
					return s, e
				}
			}
			// Fallback: caret at end of value.
			val := el.TextContent()
			if tag == "input" {
				val = el.GetAttribute("value")
			}
			n := len([]rune(val))
			return n, n
		}
		switch key {
		case "selectionStart":
			return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
				s, _ := selGetter()
				return jsc.NumberValue(float64(s))
			}}, true
		case "selectionEnd":
			return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
				_, e := selGetter()
				return jsc.NumberValue(float64(e))
			}}, true
		}
		return jsc.FunctionValue(jsc.NewNativeFunction("setSelectionRange",
			func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
				if SetSelectionBridge != nil && len(a) >= 2 {
					SetSelectionBridge(el, int(a[0].ToNumber()), int(a[1].ToNumber()))
				}
				return jsc.Undefined()
			}, 2)), nil, true

	// ── <select> specific ──
	case "multiple":
		if tag != "select" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.BooleanValue(el.HasAttribute("multiple"))
		}}, true
	case "selectedIndex":
		if tag != "select" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				var firstEnabled = -1
				idx := 0
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						if opt.HasAttribute("selected") {
							return jsc.NumberValue(float64(idx))
						}
						if firstEnabled < 0 && !opt.HasAttribute("disabled") {
							firstEnabled = idx
						}
						idx++
					}
				}
				// HTML 标准：无显式 selected 时默认选中第一个非 disabled option。
				if !el.HasAttribute("multiple") && firstEnabled >= 0 {
					return jsc.NumberValue(float64(firstEnabled))
				}
				return jsc.NumberValue(-1)
			},
			set: func(v jsc.JSValue) {
				selIdx := int(v.ToNumber())
				idx := 0
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						if idx == selIdx {
							opt.SetAttribute("selected", "selected")
						} else {
							opt.RemoveAttribute("selected")
						}
						idx++
					}
				}
			}}, true
	case "options":
		if tag != "select" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				var opts []jsc.JSValue
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						opts = append(opts, jsc.ObjectValue(wrapElement(in, opt)))
					}
				}
				arr := jsc.NewArray(in.ObjectPrototype(), opts)
				arr.Set("length", jsc.NumberValue(float64(len(opts))))
				return jsc.ObjectValue(arr)
			})(rt, jsc.JSValue{})
		}}, true
	case "selectedOptions":
		if tag != "select" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				var sel []jsc.JSValue
				var firstEnabled *dom.Element
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if opt, ok := c.(*dom.Element); ok && strings.EqualFold(opt.LocalName(), "option") {
						if opt.HasAttribute("selected") {
							sel = append(sel, jsc.ObjectValue(wrapElement(in, opt)))
						}
						if firstEnabled == nil && !opt.HasAttribute("disabled") {
							firstEnabled = opt
						}
					}
				}
				if len(sel) == 0 && !el.HasAttribute("multiple") && firstEnabled != nil {
					sel = append(sel, jsc.ObjectValue(wrapElement(in, firstEnabled)))
				}
				arr := jsc.NewArray(in.ObjectPrototype(), sel)
				arr.Set("length", jsc.NumberValue(float64(len(sel))))
				return jsc.ObjectValue(arr)
			})(rt, jsc.JSValue{})
		}}, true

	// ── <option> specific ──
	case "selected":
		if tag != "option" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("selected"))
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("selected", "selected")
				} else {
					el.RemoveAttribute("selected")
				}
			}}, true

	// ── <canvas> specific ──
	case "getContext":
		if tag != "canvas" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.FunctionValue(jsc.NewNativeFunction("getContext",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				ctype := "2d"
				if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
					ctype = args[0].ToString()
				}
				if ctype != "2d" {
					// WebGL / bitmaprenderer 尚未支持（v1）：返回 null
					//（浏览器对不支持的类型返回 null，调用方需自理）。
					return jsc.Null()
				}
				return canvas2DGetContext(in, el)
			}, 1)), nil, true
	case "width":
		if tag == "canvas" {
			return jsc.JSValue{}, &elemAccessor{
				get: func() jsc.JSValue {
					w, _ := canvasSizeOf(el)
					return jsc.NumberValue(float64(w))
				},
				set: func(v jsc.JSValue) {
					el.SetAttribute("width", jsc.NumberValue(math.Round(v.ToNumber())).ToString())
					resizeCanvasBitmap(el)
				}}, true
		}
		if tag == "img" {
			// HTMLImageElement.width ≈ 解码图宽度（canvas 2D drawImage
			// 与纹理尺寸查询依赖；未解码时按 src 同步解码）。
			return jsc.JSValue{}, &elemAccessor{
				get: func() jsc.JSValue {
					return jsc.NumberValue(imgPixelDim(el, true))
				}}, true
		}
		return jsc.JSValue{}, nil, false
	case "height":
		if tag == "canvas" {
			return jsc.JSValue{}, &elemAccessor{
				get: func() jsc.JSValue {
					_, h := canvasSizeOf(el)
					return jsc.NumberValue(float64(h))
				},
				set: func(v jsc.JSValue) {
					el.SetAttribute("height", jsc.NumberValue(math.Round(v.ToNumber())).ToString())
					resizeCanvasBitmap(el)
				}}, true
		}
		if tag == "img" {
			return jsc.JSValue{}, &elemAccessor{
				get: func() jsc.JSValue {
					return jsc.NumberValue(imgPixelDim(el, false))
				}}, true
		}
		return jsc.JSValue{}, nil, false
	case "toDataURL":
		if tag != "canvas" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.FunctionValue(jsc.NewNativeFunction("toDataURL",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				if bm, ok := el.CanvasSurface().(*rendering.CanvasBitmap); ok && bm != nil {
					if img := bm.SkiaImage(); img != nil {
						defer img.Release()
						var buf bytes.Buffer
						if _, err := img.EncodeTo(&buf, skia.EncodedFormatPNG, 100); err == nil {
							return jsc.StringValue("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()))
						}
					}
				}
				return jsc.StringValue("")
			}, 0)), nil, true

	// ── 字符串属性 accessor ──
	case "tagName":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return strAcc(el.TagName())(rt, jsc.JSValue{})
		}}, true
	case "nodeName":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return strAcc(el.NodeName())(rt, jsc.JSValue{})
		}}, true
	case "nodeType":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.NumberValue(float64(el.NodeType()))
		}}, true
	case "getRootNode":
		return jsc.FunctionValue(jsc.NewNativeFunction("getRootNode",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				doc := el.OwnerDocument()
				if doc == nil {
					if cached, ok := nodeWrapperCache[el]; ok {
						return jsc.ObjectValue(cached)
					}
					return jsc.ObjectValue(wrapElement(in, el))
				}
				return jsc.ObjectValue(wrapDocument(in, doc))
			}, 0)), nil, true
	case "attachShadow":
		return jsc.FunctionValue(jsc.NewNativeFunction("attachShadow",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				mode := "open"
				if len(args) > 0 {
					if o := args[0].AsObject(); o != nil {
						if mv, ok := o.GetByKey("mode"); ok && !mv.IsUndefined() && !mv.IsNull() {
							mode = mv.ToString()
						}
					}
				}
				sr, err := el.AttachShadow(mode)
				if err != nil || sr == nil {
					return jsc.Null()
				}
				return jsc.ObjectValue(wrapShadowRoot(in, sr))
			}, 1)), nil, true
	case "shadowRoot":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				sr := el.ShadowRoot()
				if sr == nil {
					return jsc.Null()
				}
				return jsc.ObjectValue(wrapShadowRoot(in, sr))
			})(rt, jsc.JSValue{})
		}}, true
	case "compareDocumentPosition":
		return jsc.FunctionValue(jsc.NewNativeFunction("compareDocumentPosition",
			func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
				if len(a) == 0 {
					return jsc.NumberValue(0)
				}
				other := unwrapNode(a[0])
				return jsc.NumberValue(float64(compareDocPosition(el, other)))
			}, 1)), nil, true
	case "nodeValue":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.Null()
		}}, true
	case "id":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetId()) },
			set: func(v jsc.JSValue) { el.SetId(v.ToString()) }}, true
	case "className":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetClassName()) },
			set: func(v jsc.JSValue) {
				el.SetClassName(v.ToString())
				InvalidateComputedStyle(el)
				if OnClassChanged != nil {
					OnClassChanged(el)
				}
			}}, true
	case "title":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("title")) },
			set: func(v jsc.JSValue) { el.SetAttribute("title", v.ToString()) }}, true
	case "src":
		if el.LocalName() != "iframe" && el.LocalName() != "img" && el.LocalName() != "video" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("src")) },
			set: func(v jsc.JSValue) {
				el.SetAttribute("src", v.ToString())
				if IFrameSrcChanged != nil && el.LocalName() == "iframe" {
					IFrameSrcChanged(el, v.ToString())
				}
				// img/video：src 变化后清除旧解码图（下次渲染/绘制按新 src
				// 重新解码）。
				if el.LocalName() == "img" || el.LocalName() == "video" {
					OnImageSrcChanged(el)
				}
			}}, true
	case "attributes":
		// NamedNodeMap（live 集合，DOM §4.9.3）：同一元素返回同一实例
		// （el.attributes === el.attributes），length 与数字索引随属性增删
		// 变化。React 19 的水合契约按 NamedNodeMap 语义遍历并清空属性。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				return jsc.ObjectValue(namedNodeMapFor(in, el))
			})(rt, jsc.JSValue{})
		}}, true
	case "track":
		// HTMLTrackElement.track（HTML §4.8.11.1）：<track> 元素暴露唯一的
		// TextTrack 对象（同一元素始终同一实例），首次访问时按 src 加载
		// WebVTT 并异步派发 load/error。
		if tag != "track" {
			return jsc.Undefined(), nil, true
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if obj := textTrackForElement(rt, el); obj != nil {
				return jsc.ObjectValue(obj)
			}
			return jsc.Undefined()
		}}, true
	case "textTracks":
		// HTMLMediaElement.textTracks（HTML §4.8.11.2）：live 集合，每次读取
		// 按当前子 <track> 元素刷新索引。video.textTracks[i] 与对应 <track>
		// 元素的 .track 是同一对象（框架按对象标识关联视频与字幕）。
		if tag != "video" && tag != "audio" {
			return jsc.Undefined(), nil, true
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if obj := textTracksForMediaElement(rt, el); obj != nil {
				return jsc.ObjectValue(obj)
			}
			return jsc.Undefined()
		}}, true
	case "requestFullscreen":
		// HTML §4.11.6：进入全屏（document.fullscreenElement 指向该元素、
		// CSS :fullscreen 生效、异步派发 fullscreenchange）。真实窗口全屏由
		// 宿主经 bindings.OnFullscreenChanged 决定。
		return funcVal(rt.NewNativeFunction("requestFullscreen",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return requestFullscreenFor(in, el)
			}, 0)), nil, true
	case "innerHTML":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetInnerHTML()) },
			set: func(v jsc.JSValue) {
				el.SetInnerHTML(v.ToString())
				// ★ 声明式更新（v-html / el.innerHTML = html）后触发渲染树
				// 重建标记——否则 DOM 变了但界面不刷新。
				if OnNodeInserted != nil && el.IsConnected() {
					OnNodeInserted(el)
				}
			}}, true
	case "outerHTML":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.StringValue(el.GetOuterHTML())
		}}, true
	case "textContent":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.TextContent()) },
			set: func(v jsc.JSValue) {
				el.SetTextContent(v.ToString())
				if isStyleElement(el) {
					BumpStyleVersion()
				}
				if OnNodeInserted != nil && el.IsConnected() {
					OnNodeInserted(el)
				}
			}}, true

	// ── <template> .content ──
	case "content":
		if !strings.EqualFold(el.LocalName(), "template") {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return getter(func(in *jsc.Interpreter) jsc.JSValue {
				doc := el.OwnerDocument()
				if doc == nil {
					// Fallback: use a detached fragment if no owner document
					return jsc.ObjectValue(wrapDocFrag(in, dom.NewDocumentFragment(nil)))
				}
				frag := doc.CreateDocumentFragment()
				// Move all child nodes into the fragment
				for c := el.FirstChild(); c != nil; c = el.FirstChild() {
					frag.AppendChild(c)
				}
				return jsc.ObjectValue(wrapDocFrag(in, frag))
			})(rt, jsc.JSValue{})
		}}, true

	// ── <dialog> / <details>（open 状态）──
	case "open":
		// open 属性（HTML 反射）：<details> 与 <dialog> 是仅有的两个以属性
		// 表达打开状态的元素（<select> 的 open 是内部状态，不属于属性）。
		// ★ 纯反射：设置/移除属性本身**不**改变 dialog 的模态状态、不派发
		// 事件（规范如此：由 showModal() 打开的 dialog 即使 open 属性被移除
		// 仍处于模态，:modal 与 ::backdrop 都保留）。样式仍需失效：[open]
		// 决定 UA 规则 dialog:not([open]){display:none} 与定位，同时 :open /
		// :closed 的匹配结果也变了。
		if tag != "details" && tag != "dialog" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(el.HasAttribute("open")) },
			set: func(v jsc.JSValue) {
				want := v.ToBoolean()
				if want == el.HasAttribute("open") {
					return
				}
				if want {
					el.SetAttribute("open", "")
				} else {
					el.RemoveAttribute("open")
				}
				invalidateStateStyle(el)
				// <details> 的 open 状态变化要派发 toggle（HTML §4.11.4：
				// 「当 open 属性被切换时排队 details toggle 事件任务」）。
				if tag == "details" {
					queueDialogToggle(rt, el)
				}
			}}, true
	case "show", "showModal":
		if tag != "dialog" {
			return jsc.JSValue{}, nil, false
		}
		method := key
		modal := key == "showModal"
		return funcVal(rt.NewNativeFunction(method,
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				dialogShow(in, el, method, modal)
				return jsc.Undefined()
			}, 0)), nil, true
	case "close":
		if tag != "dialog" {
			return jsc.JSValue{}, nil, false
		}
		return funcVal(rt.NewNativeFunction("close",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				dialogClose(in, el, args)
				return jsc.Undefined()
			}, 1)), nil, true
	case "returnValue":
		// HTMLDialogElement.returnValue：规范是 DOMString 内部状态；本引擎与
		// html5.HTMLDialogElement 一致，用 data-returnvalue 属性承载，Go 侧与
		// JS 侧共享同一份值。
		if tag != "dialog" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.StringValue(el.GetAttribute("data-returnvalue"))
			},
			set: func(v jsc.JSValue) {
				el.SetAttribute("data-returnvalue", v.ToString())
			}}, true
	}
	return jsc.JSValue{}, nil, false
}
