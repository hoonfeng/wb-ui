// engine/js/bindings/elemdomapi.go — 类别① 第一批标准 DOM 方法（第 18 次监督轮）
//
// 本轮补齐的接口面（全部是 DOM 标准 API，不依赖任何平台子系统）：
//
//	ParentNode（Element / Document 共用）append / prepend / replaceChildren
//	ChildNode（Element）before / after / replaceWith（remove 已有）
//	Element 查询  getElementsByTagName / getElementsByClassName（**live** 集合）
//	Element 插入  insertAdjacentElement / insertAdjacentText
//	Element 比较  isEqualNode / isSameNode / webkitMatchesSelector
//	命名空间属性族 setAttributeNS / getAttributeNS / hasAttributeNS /
//	                removeAttributeNS / getAttributeNames
//	Document      append / prepend / replaceChildren / importNode / adoptNode /
//	                getElementsByName / createAttribute /
//	                createProcessingInstruction / createCDATASection
//
// 设计取舍：
//   - 方法装在 **Element.prototype**（与 dom.go 里既有的 getAttribute/setAttribute
//     族一致，浏览器里它们同样在原型上），因此所有元素经原型链共享，且可被
//     框架/探针在 Element.prototype 上 hook。
//   - 插入类方法统一走 insertNodesBefore：Node 参数按原样插入（DocumentFragment
//     展开为其子节点），字符串参数转 Text 节点，一次调用内的插入顺序与参数顺序
//     一致（DOM §4.2.5 "convert nodes into a node" + pre-insert）。
//   - 查询类方法返回 **live** 集合（makeLiveElementCollection）：length / 索引 /
//     item / namedItem 每次读取都重新求值，保留引用后也能观察到增删，与浏览器
//     HTMLCollection 语义一致。
package bindings

import (
	"errors"
	"strconv"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// ─── live 集合（HTMLCollection / NodeList 语义）──────────────────────────────

// liveCollectionIndexLimit 是 live 集合预注册的数字索引上限。索引属性本身在
// goja 里必须显式注册才能被 `c[0]` 命中，而集合长度随 DOM 变化，无法在构建时
// 精确枚举，故预注册一段区间：区间内每次读取重新求值（真 live），越界返回
// undefined —— 浏览器 HTMLCollection 索引越界同样是 undefined。真实页面里单个
// 集合超过 64 项且需要按下标访问的场景极少，item(i) 不受此上限影响。
const liveCollectionIndexLimit = 64

// makeLiveElementCollection 构造 live 元素集合对象（HTMLCollection / NodeList）。
// build() 在每次读取 length / 索引 / item / namedItem 时被调用，所以调用方保留
// 集合引用后仍能观察到 DOM 增删（`const c = el.getElementsByClassName("x");
// el.append(div); c.length` 会 +1）。
func makeLiveElementCollection(in *jsc.Interpreter, className string, build func() []*dom.Element) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName(className)
	for i := 0; i < liveCollectionIndexLimit; i++ {
		idx := i
		obj.SetAccessor(strconv.Itoa(idx), getter(func(in *jsc.Interpreter) jsc.JSValue {
			els := build()
			if idx >= len(els) || isNilEl(els[idx]) {
				return jsc.Undefined()
			}
			return jsc.ObjectValue(wrapElement(in, els[idx]))
		}), nil)
	}
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(build())))
	}), nil)
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			els := build()
			i := int(args[0].ToNumber())
			if i < 0 || i >= len(els) || isNilEl(els[i]) {
				return jsc.Null()
			}
			return jsc.ObjectValue(wrapElement(in, els[i]))
		}, 1)))
	// namedItem：按 id 或 name 内容属性匹配（DOM §4.2.2 HTMLCollection.namedItem）。
	obj.Set("namedItem", jsc.FunctionValue(jsc.NewNativeFunction("namedItem",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			name := args[0].ToString()
			if name == "" {
				return jsc.Null()
			}
			for _, el := range build() {
				if isNilEl(el) {
					continue
				}
				if el.GetId() == name || el.GetAttribute("name") == name {
					return jsc.ObjectValue(wrapElement(in, el))
				}
			}
			return jsc.Null()
		}, 1)))
	return obj
}

// collectElementsByClassName 收集 root 后代中同时带全部 class token 的元素。
// 规范的 getElementsByClassName 接受以空白分隔的多个 token（必须全部命中），
// 而 dom.Element.GetElementsByClassName 只做单 token，故在此按 Fields 拆分。
func collectElementsByClassName(root *dom.Element, classNames string) []*dom.Element {
	tokens := strings.Fields(classNames)
	if len(tokens) == 0 {
		return nil
	}
	var out []*dom.Element
	root.WalkDescendantElements(func(e *dom.Element) bool {
		// DOM §4.9：只算后代，不含元素自身（WalkDescendantElements 从 root 自身开始遍历）。
		if e == root {
			return true
		}
		for _, t := range tokens {
			if !e.HasClassName(t) {
				return true
			}
		}
		out = append(out, e)
		return true
	})
	return out
}

// ─── ParentNode / ChildNode 插入 ────────────────────────────────────────────

// insertNodesBefore 实现 ParentNode.append/prepend 与 ChildNode.before/after 的
// 共享插入步骤：
//   - Node 参数直接采用；DocumentFragment 先展开为其子节点（DOM 规范：片段内容
//     被移动而不是片段本身）
//   - 其余值（字符串/数字/...）转成 Text 节点
//   - before 为 nil 表示追加到末尾
//
// 插入失败（层级检查）时回退为 AppendChild，与既有 appendChild/insertBefore 的
// 容错口径一致（Vue/React 的 vnode 与真实 DOM 短暂不一致时不让节点消失）。
func insertNodesBefore(parent dom.Node, in *jsc.Interpreter, args []jsc.JSValue, before dom.Node) {
	if isNilNode(parent) {
		return
	}
	doc := parent.OwnerDocument()
	if doc == nil {
		if d, ok := parent.(*dom.Document); ok {
			doc = d
		}
	}
	var nodes []dom.Node
	for _, a := range args {
		if a.IsObject() || a.IsCallable() {
			if n := unwrapNode(a); n != nil {
				if frag, ok := n.(*dom.DocumentFragment); ok {
					for c := frag.FirstChild(); !isNilNode(c); c = frag.FirstChild() {
						_ = frag.RemoveChild(c)
						nodes = append(nodes, c)
					}
					continue
				}
				nodes = append(nodes, n)
				continue
			}
		}
		if doc == nil {
			continue
		}
		nodes = append(nodes, dom.NewText(doc, a.ToString()))
	}
	for _, n := range nodes {
		var err error
		if isNilNode(before) {
			err = parent.AppendChild(n)
		} else {
			err = parent.InsertBefore(n, before)
		}
		if err != nil {
			_ = parent.AppendChild(n)
		}
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
}

// replaceChildrenOf 实现 ParentNode.replaceChildren：清空全部子节点后按参数顺序插入。
func replaceChildrenOf(parent dom.Node, in *jsc.Interpreter, args []jsc.JSValue) {
	if isNilNode(parent) {
		return
	}
	for c := parent.FirstChild(); !isNilNode(c); c = parent.FirstChild() {
		_ = parent.RemoveChild(c)
		if OnNodeRemoved != nil {
			OnNodeRemoved(c)
		}
	}
	insertNodesBefore(parent, in, args, nil)
}

// insertAdjacentNode 按 insertAdjacent{Element,Text} 的位置语义插入单个节点，
// 返回是否插入成功。位置字符串的合法性由调用方先行校验（见
// validAdjacentPosition）：规范在位置非法时抛 SyntaxError，而 beforebegin/afterend
// 遇到无父元素时按规范返回 null（不中断脚本）。
func insertAdjacentNode(el *dom.Element, position string, node dom.Node) bool {
	if isNilEl(el) || isNilNode(node) {
		return false
	}
	switch position {
	case "beforebegin":
		p := el.ParentNode()
		if isNilNode(p) {
			return false
		}
		if err := p.InsertBefore(node, el); err != nil {
			return false
		}
	case "afterbegin":
		if ref := el.FirstChild(); isNilNode(ref) {
			if err := el.AppendChild(node); err != nil {
				return false
			}
		} else if err := el.InsertBefore(node, ref); err != nil {
			return false
		}
	case "beforeend":
		if err := el.AppendChild(node); err != nil {
			return false
		}
	case "afterend":
		p := el.ParentNode()
		if isNilNode(p) {
			return false
		}
		if ref := el.NextSibling(); isNilNode(ref) {
			if err := p.AppendChild(node); err != nil {
				return false
			}
		} else if err := p.InsertBefore(node, ref); err != nil {
			return false
		}
	default:
		return false
	}
	if OnNodeInserted != nil {
		OnNodeInserted(node)
	}
	if isStyleElement(node) {
		BumpStyleVersion()
		if OnStyleNodeAdded != nil {
			OnStyleNodeAdded(node)
		}
	}
	return true
}

// validAdjacentPosition 报告位置字符串是否是四个合法值之一（DOM §4.2.6：
// insertAdjacentElement/insertAdjacentText/insertAdjacentHTML 共用同一位置词汇表，
// 非法位置抛 SyntaxError —— Edge 实测一致）。
func validAdjacentPosition(position string) bool {
	switch position {
	case "beforebegin", "afterbegin", "beforeend", "afterend":
		return true
	}
	return false
}

// throwSyntaxError 抛 SyntaxError（本引擎没有 DOMException 构造器，以 Go 错误上抛，
// goja 转为 JS 异常；脚本能 catch 到，只是 name 为 "Error"）。
func throwSyntaxError(in *jsc.Interpreter) {
	panic(in.VM().NewGoError(errors.New(
		"SyntaxError: The provided position is not one of 'beforebegin', 'afterbegin', 'beforeend', 'afterend'")))
}

// ─── Element.prototype：类别① 第一批方法 ────────────────────────────────────

// protoMethod 在给定原型对象上注册一个 Element 方法：调用时从 this 取回
// *dom.Element（与 dom.go 里 protoAttr 的做法相同），fn 拿到的 el 保证非 nil。
func protoMethod(proto *jsc.JSObject, name string, argc int, fn func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue) {
	proto.Set(name, jsc.FunctionValue(jsc.NewNativeFunction(name,
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if !this.IsObject() {
				return jsc.Undefined()
			}
			el, _ := this.AsObject().Internal().(*dom.Element)
			if el == nil {
				return jsc.Undefined()
			}
			return fn(in, el, args)
		}, argc)))
}

// installElementProtoDOMMethods 在 Element.prototype 上安装类别① 的全部元素侧方法。
// 由 RegisterDOMBindings 在建立起元素原型链之后调用一次。
func installElementProtoDOMMethods(proto *jsc.JSObject) {
	// ── ParentNode：append / prepend / replaceChildren ──
	protoMethod(proto, "append", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		insertNodesBefore(el, in, args, nil)
		return jsc.Undefined()
	})
	protoMethod(proto, "prepend", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		insertNodesBefore(el, in, args, el.FirstChild())
		return jsc.Undefined()
	})
	protoMethod(proto, "replaceChildren", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		replaceChildrenOf(el, in, args)
		return jsc.Undefined()
	})

	// ── ChildNode：before / after / replaceWith ──
	protoMethod(proto, "before", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if p := el.ParentNode(); !isNilNode(p) {
			insertNodesBefore(p, in, args, el)
		}
		return jsc.Undefined()
	})
	protoMethod(proto, "after", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if p := el.ParentNode(); !isNilNode(p) {
			insertNodesBefore(p, in, args, el.NextSibling())
		}
		return jsc.Undefined()
	})
	protoMethod(proto, "replaceWith", 0, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if p := el.ParentNode(); !isNilNode(p) {
			insertNodesBefore(p, in, args, el)
			_ = p.RemoveChild(el)
			if OnNodeRemoved != nil {
				OnNodeRemoved(el)
			}
		}
		return jsc.Undefined()
	})

	// ── Element 查询（live 集合）──
	protoMethod(proto, "getElementsByTagName", 1, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		tag := ""
		if len(args) > 0 {
			tag = args[0].ToString()
		}
		return jsc.ObjectValue(makeLiveElementCollection(in, "HTMLCollection", func() []*dom.Element {
			return el.GetElementsByTagName(tag)
		}))
	})
	protoMethod(proto, "getElementsByClassName", 1, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		names := ""
		if len(args) > 0 {
			names = args[0].ToString()
		}
		return jsc.ObjectValue(makeLiveElementCollection(in, "HTMLCollection", func() []*dom.Element {
			return collectElementsByClassName(el, names)
		}))
	})

	// ── 相邻插入 ──
	protoMethod(proto, "insertAdjacentElement", 2, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Null()
		}
		pos := args[0].ToString()
		if !validAdjacentPosition(pos) {
			throwSyntaxError(in)
		}
		n := unwrapNode(args[1])
		if n == nil {
			return jsc.Null()
		}
		if !insertAdjacentNode(el, pos, n) {
			return jsc.Null()
		}
		return nodeToJS(in, n)
	})
	protoMethod(proto, "insertAdjacentText", 2, func(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		pos := args[0].ToString()
		if !validAdjacentPosition(pos) {
			throwSyntaxError(in)
		}
		doc := el.OwnerDocument()
		if doc == nil {
			return jsc.Undefined()
		}
		insertAdjacentNode(el, pos, dom.NewText(doc, args[1].ToString()))
		return jsc.Undefined()
	})

	// ── 节点比较 ──
	protoMethod(proto, "isEqualNode", 1, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.BooleanValue(false)
		}
		other := unwrapNode(args[0])
		if other == nil {
			return jsc.BooleanValue(false)
		}
		return jsc.BooleanValue(el.IsEqualNode(other))
	})
	protoMethod(proto, "isSameNode", 1, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.BooleanValue(false)
		}
		other := unwrapNode(args[0])
		if other == nil {
			return jsc.BooleanValue(false)
		}
		return jsc.BooleanValue(el.IsSameNode(other))
	})
	// webkitMatchesSelector：matches 的 WebKit 前缀别名（标准语义完全一致，
	// 大量既有库按 webkit 名调用）。
	protoMethod(proto, "webkitMatchesSelector", 1, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.BooleanValue(false)
		}
		return jsc.BooleanValue(ElementMatches(el, args[0].ToString()))
	})

	// ── 命名空间属性族（DOM §4.9）──
	nsArg := func(v jsc.JSValue) string {
		if v.IsNull() || v.IsUndefined() {
			return ""
		}
		return v.ToString()
	}
	protoMethod(proto, "setAttributeNS", 3, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 3 {
			return jsc.Undefined()
		}
		name := args[1].ToString()
		el.SetAttributeNS(nsArg(args[0]), name, args[2].ToString())
		invalidateNamedNodeMap(el)
		invalidateAttrChange(el, name)
		return jsc.Undefined()
	})
	protoMethod(proto, "getAttributeNS", 2, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Null()
		}
		ns, local := nsArg(args[0]), args[1].ToString()
		// DOM §4.9.21：属性不存在时 getAttributeNS 返回 **null**（不是空串）。
		if !el.HasAttributeNS(ns, local) {
			return jsc.Null()
		}
		return jsc.StringValue(el.GetAttributeNS(ns, local))
	})
	protoMethod(proto, "hasAttributeNS", 2, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.BooleanValue(false)
		}
		return jsc.BooleanValue(el.HasAttributeNS(nsArg(args[0]), args[1].ToString()))
	})
	protoMethod(proto, "removeAttributeNS", 2, func(_ *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 {
			return jsc.Undefined()
		}
		local := args[1].ToString()
		if el.RemoveAttributeNS(nsArg(args[0]), local) {
			invalidateNamedNodeMap(el)
			invalidateAttrChange(el, local)
		}
		return jsc.Undefined()
	})
	protoMethod(proto, "getAttributeNames", 0, func(in *jsc.Interpreter, el *dom.Element, _ []jsc.JSValue) jsc.JSValue {
		names := el.AttributeNames()
		items := make([]jsc.JSValue, len(names))
		for i, n := range names {
			items[i] = jsc.StringValue(n)
		}
		return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
	})
}

// ─── Document：ParentNode + 工厂 + 导入 ────────────────────────────────────

// docProtoMethod 在 document 包装对象上注册一个方法（this 无关，闭包直接持 doc）。
func docProtoMethod(obj *jsc.JSObject, name string, argc int, fn func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue) {
	obj.Set(name, jsc.FunctionValue(jsc.NewNativeFunction(name,
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return fn(in, args)
		}, argc)))
}

// installDocumentDOMMethods 在 document 包装对象上安装类别① 的文档侧方法。
func installDocumentDOMMethods(rt *jsc.Interpreter, obj *jsc.JSObject, doc *dom.Document) {
	// ── ParentNode ──
	docProtoMethod(obj, "append", 0, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		insertNodesBefore(doc, in, args, nil)
		return jsc.Undefined()
	})
	docProtoMethod(obj, "prepend", 0, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		insertNodesBefore(doc, in, args, doc.FirstChild())
		return jsc.Undefined()
	})
	docProtoMethod(obj, "replaceChildren", 0, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		replaceChildrenOf(doc, in, args)
		return jsc.Undefined()
	})

	// ── 导入 / 收养 ──
	docProtoMethod(obj, "importNode", 2, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Null()
		}
		n := unwrapNode(args[0])
		if n == nil {
			return jsc.Null()
		}
		deep := len(args) > 1 && args[1].ToBoolean()
		copied, err := doc.ImportNode(n, deep)
		if err != nil {
			panic(in.VM().NewGoError(err))
		}
		return nodeToJS(in, copied)
	})
	docProtoMethod(obj, "adoptNode", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Null()
		}
		n := unwrapNode(args[0])
		if n == nil {
			return jsc.Null()
		}
		adopted, err := doc.AdoptNode(n)
		if err != nil {
			panic(in.VM().NewGoError(err))
		}
		return nodeToJS(in, adopted)
	})

	// ── 按 name 查询（规范为 live NodeList）──
	docProtoMethod(obj, "getElementsByName", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		name := ""
		if len(args) > 0 {
			name = args[0].ToString()
		}
		return jsc.ObjectValue(makeLiveElementCollection(in, "NodeList", func() []*dom.Element {
			return doc.GetElementsByName(name)
		}))
	})

	// ── 工厂：createAttribute / createProcessingInstruction / createCDATASection ──
	docProtoMethod(obj, "createAttribute", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		name := ""
		if len(args) > 0 {
			name = args[0].ToString()
		}
		return jsc.ObjectValue(wrapAttr(in, doc.CreateAttribute(name)))
	})
	docProtoMethod(obj, "createProcessingInstruction", 2, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		target, data := "", ""
		if len(args) > 0 {
			target = args[0].ToString()
		}
		if len(args) > 1 {
			data = args[1].ToString()
		}
		pi, err := doc.CreateProcessingInstruction(target, data)
		if err != nil {
			// DOM §4.9.6：非法 target（非 XML Name 或 "xml"）与含 "?>" 的 data
			// 都抛 InvalidCharacterError。本引擎没有 DOMException 构造器，
			// 这里以 Go 错误上抛（goja 转为 Error），夹具只断言「抛出了」。
			panic(in.VM().NewGoError(err))
		}
		return jsc.ObjectValue(wrapProcessingInstruction(in, pi))
	})
	docProtoMethod(obj, "createCDATASection", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		data := ""
		if len(args) > 0 {
			data = args[0].ToString()
		}
		// ★ DOM §4.9.5：HTML 文档上 createCDATASection 必须抛 NotSupportedError
		//（HTML 没有 CDATA section 概念）。本引擎的文档都是 text/html，因此这条
		// 路径总是抛错——与 Edge 行为一致；CDATASection 节点类型本身已建模
		//（engine/dom/processinginstruction.go），供 XML 文档使用。
		ct := strings.ToLower(strings.TrimSpace(doc.ContentType()))
		if ct == "" || strings.Contains(ct, "html") {
			panic(in.VM().NewGoError(errors.New(
				"NotSupportedError: This operation is not supported for HTML documents")))
		}
		return jsc.ObjectValue(wrapCDATASection(in, doc.CreateCDATASection(data)))
	})
}

// ─── 新节点类型的 JS 包装（Attr / ProcessingInstruction / CDATASection）─────

// wrapAttr 把 *dom.Attr 暴露成 Attr 对象（DOM §4.9.1）：name/value/nodeName/
// nodeValue/localName/prefix/namespaceURI/ownerElement/nodeType。value 可写
// （写穿到所属元素的属性表，样式失效与 MutationObserver 同路径）。
func wrapAttr(in *jsc.Interpreter, a *dom.Attr) *jsc.JSObject {
	if a == nil {
		return nil
	}
	if cached, ok := nodeWrapperCache[a]; ok {
		return cached
	}
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("Attr")
	obj.SetInternal(a)
	obj.SetAccessor("name", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(a.Name())
	}), nil)
	obj.SetAccessor("value",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(a.Value()) }),
		func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			a.SetValue(v.ToString())
			if el := a.OwnerElement(); el != nil {
				invalidateNamedNodeMap(el)
			}
		})
	obj.SetAccessor("nodeName", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(a.NodeName())
	}), nil)
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(a.NodeValue())
	}), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(dom.NodeAttribute))
	}), nil)
	obj.SetAccessor("localName", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(a.LocalName())
	}), nil)
	obj.SetAccessor("prefix", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if p := a.Prefix(); p != "" {
			return jsc.StringValue(p)
		}
		return jsc.Null()
	}), nil)
	obj.SetAccessor("namespaceURI", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		if ns := a.NamespaceURI(); ns != "" {
			return jsc.StringValue(ns)
		}
		return jsc.Null()
	}), nil)
	obj.SetAccessor("specified", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(true)
	}), nil)
	obj.SetAccessor("textContent", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(a.TextContent())
	}), nil)
	obj.SetAccessor("parentNode", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null() // Attr 不在树里（DOM §4.9：属性的 parentNode 恒为 null）
	}), nil)
	obj.SetAccessor("ownerElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if el := a.OwnerElement(); el != nil {
			return jsc.ObjectValue(wrapElement(in, el))
		}
		return jsc.Null()
	}), nil)
	nodeWrapperCache[a] = obj
	return obj
}

// wrapProcessingInstruction 把 *dom.ProcessingInstruction 暴露成 PI 对象
// （nodeType=7）：target/nodeName/data/nodeValue。
func wrapProcessingInstruction(in *jsc.Interpreter, p *dom.ProcessingInstruction) *jsc.JSObject {
	if p == nil {
		return nil
	}
	if cached, ok := nodeWrapperCache[p]; ok {
		return cached
	}
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("ProcessingInstruction")
	obj.SetInternal(p)
	obj.SetAccessor("target", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(p.Target())
	}), nil)
	obj.SetAccessor("nodeName", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(p.NodeName())
	}), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(dom.NodeProcessingInstruction))
	}), nil)
	obj.SetAccessor("data",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(p.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { p.SetData(v.ToString()) })
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(p.NodeValue())
	}), nil)
	obj.SetAccessor("textContent", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(p.TextContent())
	}), nil)
	// 节点遍历属性（PI 可以作为 document 的子节点出现，如 <?xml-stylesheet?>）：
	// 未插入时 parentNode 为 null，插入后返回父节点 —— 与 Node 语义一致。
	obj.SetAccessor("parentNode", nodeAccFn(in, func() dom.Node {
		if p.ParentNode() == nil {
			return nil
		}
		return p.ParentNode()
	}), nil)
	obj.SetAccessor("ownerDocument", nodeAccFn(in, func() dom.Node {
		return p.OwnerDocument()
	}), nil)
	nodeWrapperCache[p] = obj
	return obj
}

// wrapCDATASection 把 *dom.CDATASection 暴露成 CDATA 对象（nodeType=4）。
func wrapCDATASection(in *jsc.Interpreter, c *dom.CDATASection) *jsc.JSObject {
	if c == nil {
		return nil
	}
	if cached, ok := nodeWrapperCache[c]; ok {
		return cached
	}
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("CDATASection")
	obj.SetInternal(c)
	obj.SetAccessor("nodeName", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(c.NodeName())
	}), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(dom.NodeCDATASection))
	}), nil)
	obj.SetAccessor("data",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(c.Data()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(c.NodeValue())
	}), nil)
	obj.SetAccessor("textContent", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(c.TextContent())
	}), nil)
	obj.SetAccessor("parentNode", nodeAccFn(in, func() dom.Node {
		if c.ParentNode() == nil {
			return nil
		}
		return c.ParentNode()
	}), nil)
	obj.SetAccessor("ownerDocument", nodeAccFn(in, func() dom.Node {
		return c.OwnerDocument()
	}), nil)
	nodeWrapperCache[c] = obj
	return obj
}
