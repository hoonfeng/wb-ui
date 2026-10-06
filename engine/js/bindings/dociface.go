// dociface.go — Document 包装器的 Node/ParentNode/Document 接口族，以及 DocumentType
// 节点（DOM §4.4 Node / §4.5 Document / §4.9 DocumentType）。
//
// ★ 第 17 次监督轮新增。此前 wrapDocument 只装了业务上「被库实际读到」的部分属性
// （title/body/head/documentElement/URL/baseURI/compatMode/readyState/styleSheets/
// scripts/cookie/activeElement/defaultView/currentScript/fullscreen*…），文档级的
// Node/ParentNode 接口与 Document 接口的其余成员整体缺失 —— 探针 documentProps
// 18/56，缺 38 项。
//
// 本轮补齐这一族。实现约定（与元素侧既有做法一致）：
//   - 标量属性一律**活值**（每次读取重新求值，不缓存快照）；
//   - 集合类（children/childNodes/forms/images/links/…）每次读取按当前 DOM 重新
//     构建，因此是 live 语义（appendChild/remove 之后立刻反映）；
//   - 单例对象（location/fonts/implementation/timeline）首次读取时缓存，同一文档
//     内多次读取返回**同一身份**（库用 === 做单例判断）。
package bindings

import (
	"net/url"
	"strconv"
	"sync"
	"time"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// domDocumentTypeProto 是 DocumentType 构造器的 prototype（RegisterDOMBindings 装配，
// 见 dom.go 的 DOM Constructors 段）。使 `doctype instanceof DocumentType` 成立。
var domDocumentTypeProto *jsc.JSObject

// designModeByDoc 保存 document.designMode 的每文档状态（HTML §7.5.4）。
// 该标志是纯文档级状态，引擎的编辑/渲染路径尚不消费它，故落在绑定层，
// 不进 engine/dom（本轮 engine/dom 只新增 DocumentType 节点类型）。
var designModeByDoc sync.Map // map[*dom.Document]string

// ─── DocumentType（DOM §4.9）─────────────────────────────

// wrapDocumentType 把 dom.DocumentType 暴露成 JS 的 DocumentType 对象：name/
// publicId/systemId + Node 接口的 nodeType(10)/nodeName/nodeValue(null)/
// ownerDocument/parentNode/兄弟指针，以及 remove()/cloneNode()/hasChildNodes()。
//
// ★ 包装对象进 nodeWrapperCache：`document.firstChild === document.doctype` 在浏览器
// 里为 true（同一个 reflector），且库的兄弟/子节点遍历依赖引用相等。
func wrapDocumentType(rt *jsc.Interpreter, dt *dom.DocumentType) *jsc.JSObject {
	if cached, ok := nodeWrapperCache[dt]; ok {
		return cached
	}
	proto := rt.ObjectPrototype()
	if domDocumentTypeProto != nil {
		proto = domDocumentTypeProto
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("DocumentType")
	obj.SetInternal(dt)

	// DocumentType 专有（DOM §4.9.1）。
	obj.SetAccessor("name", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(dt.Name())
	}), nil)
	obj.SetAccessor("publicId", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(dt.PublicID())
	}), nil)
	obj.SetAccessor("systemId", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(dt.SystemID())
	}), nil)

	// Node 接口（DOM §4.4）。nodeValue/textContent 恒 null（DocumentType 既没有数据
	// 也没有子树 —— 引擎侧返回空串，这里按规范映射成 null）。
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(dom.NodeDocumentType))
	}), nil)
	obj.SetAccessor("nodeName", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(dt.NodeName())
	}), nil)
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null()
	}), nil)
	obj.SetAccessor("textContent", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null()
	}), nil)
	obj.SetAccessor("ownerDocument", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return in.GlobalObject().GetOrZero("document")
	}), nil)
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return dt.ParentNode() }), nil)
	obj.SetAccessor("parentElement", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null() // doctype 的父节点只可能是 Document，不是 Element
	}), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return dt.PreviousSibling() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return dt.NextSibling() }), nil)
	obj.SetAccessor("firstChild", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("lastChild", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
	}), nil)
	obj.SetAccessor("isConnected", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(dt.ParentNode() != nil)
	}), nil)

	obj.Set("hasChildNodes", jsc.FunctionValue(jsc.NewNativeFunction("hasChildNodes",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(false)
		}, 0)))
	// DocumentType.remove()（DOM §4.9.1）：把 doctype 从文档上摘掉。
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := dt.ParentNode(); p != nil {
				_ = p.RemoveChild(dt)
			}
			return jsc.Undefined()
		}, 0)))
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapDocumentType(in,
				dom.NewDocumentType(dt.OwnerDocument(), dt.Name(), dt.PublicID(), dt.SystemID())))
		}, 1)))

	nodeWrapperCache[dt] = obj
	return obj
}

// ─── Document 接口族（DOM §4.4 / §4.5）───────────────────

// installDocumentIfaceProps 在 Document 包装对象（wrapDocument 建的 obj）上安装
// Node/ParentNode/Document 接口族。分四组：
//
//	① Node/ParentNode 导航：nodeType/nodeName/ownerDocument/firstChild/lastChild/
//	   childNodes/children/firstElementChild/lastElementChild/childElementCount；
//	② Document 标量成员：characterSet/charset/inputEncoding/contentType/
//	   documentURI/referrer/implementation/dir/domain/location；
//	③ 集合成员（live HTMLCollection）：forms/images/links/embeds/plugins/anchors/
//	   applets/all；
//	④ 其余：adoptedStyleSheets/fonts/hidden/visibilityState/pointerLockElement/
//	   pictureInPictureElement/designMode/scrollingElement/timeline，以及清单外的
//	   doctype（本轮 DocumentType 建模的自然配套）。
func installDocumentIfaceProps(rt *jsc.Interpreter, obj *jsc.JSObject, doc *dom.Document) {
	// ── ① Node / ParentNode ──
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(dom.NodeDocument))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(doc.NodeName()), nil) // "#document"
	obj.SetAccessor("nodeValue", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("textContent", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("ownerDocument", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("parentNode", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("parentElement", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("previousSibling", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("nextSibling", getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.Null() }), nil)
	obj.SetAccessor("isConnected", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(true)
	}), nil)
	// document.doctype：文档的 DOCTYPE 节点（无则 null）。
	obj.SetAccessor("doctype", nodeAccFn(rt, func() dom.Node {
		if dt := doc.Doctype(); dt != nil {
			return dt
		}
		return nil
	}), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return doc.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return doc.LastChild() }), nil)
	// childNodes 沿用元素侧既有的数组语义（arrNode）；每次读取重新求值 → live。
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, doc.ChildNodes())
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrElemAs(in, documentElementChildren(doc), "HTMLCollection")
	}), nil)
	obj.SetAccessor("firstElementChild", nodeAccFn(rt, func() dom.Node { return firstElementChildNode(doc) }), nil)
	obj.SetAccessor("lastElementChild", nodeAccFn(rt, func() dom.Node { return lastElementChildNode(doc) }), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(documentElementChildren(doc))))
	}), nil)

	// ── ② Document 标量成员 ──
	// characterSet/charset/inputEncoding（HTML §3.1.4 encoder spec）：本引擎的
	// tokenizer 按 UTF-8 解码文档字节流、JS 字符串亦为 UTF-8 语义，故此处恒
	// "UTF-8"（charset/inputEncoding 是规范保留的历史别名，与 characterSet 同值）。
	obj.SetAccessor("characterSet", strAcc("UTF-8"), nil)
	obj.SetAccessor("charset", strAcc("UTF-8"), nil)
	obj.SetAccessor("inputEncoding", strAcc("UTF-8"), nil)
	obj.SetAccessor("contentType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(doc.ContentType())
	}), nil)
	obj.SetAccessor("documentURI", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(doc.URL())
	}), nil)
	// referrer：本引擎不发送导航请求，无 Referer 头 → "".（规范允许空串）
	obj.SetAccessor("referrer", strAcc(""), nil)
	// document.dir：反射 documentElement 的 dir 内容属性（HTML §3.2.6.1）。
	obj.SetAccessor("dir",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			if de := doc.DocumentElement(); de != nil {
				return jsc.StringValue(de.GetAttribute("dir"))
			}
			return jsc.StringValue("")
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			if de := doc.DocumentElement(); de != nil {
				de.SetAttribute("dir", v.ToString())
			}
		})
	// document.domain：文档 URL 的主机名（无端口）。
	obj.SetAccessor("domain", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(hostOfURL(doc.URL()))
	}), nil)
	// document.designMode（HTML §7.5.4）："off"/"on"，默认 "off"。
	obj.SetAccessor("designMode",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			if v, ok := designModeByDoc.Load(doc); ok {
				return jsc.StringValue(v.(string))
			}
			return jsc.StringValue("off")
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			s := v.ToString()
			if s != "on" {
				s = "off"
			}
			designModeByDoc.Store(doc, s)
		})
	// document.location：与 document.URL 同源；首次读取时构造并缓存（同一文档内
	// 多次读取必须同一身份）。★ 本引擎的 window.location 尚无包装（globals 类别，
	// 另轮处理），因此这里为该文档单独构造 location 对象。
	var locObj *jsc.JSObject
	obj.SetAccessor("location", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if locObj == nil {
			locObj = makeLocationObject(in, doc.URL())
		}
		return jsc.ObjectValue(locObj)
	}), nil)
	// document.implementation（DOM §4.5.1 DOMImplementation）：单例。
	var implObj *jsc.JSObject
	obj.SetAccessor("implementation", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if implObj == nil {
			implObj = makeDOMImplementation(in, doc)
		}
		return jsc.ObjectValue(implObj)
	}), nil)

	// ── ③ 集合成员（live HTMLCollection）──
	// 每次读取按当前 DOM 重新求值并重建集合（与元素侧 children/childNodes 的数组
	// 语义一致）：既能反映后续 appendChild/remove，也不会留下陈旧快照。
	installCollection := func(name, className string, resolve func() []*dom.Element) {
		obj.SetAccessor(name, getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(wrapElementCollection(in, className, resolve()))
		}), nil)
	}
	installCollection("forms", "HTMLCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("form")
	})
	installCollection("images", "HTMLCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("img")
	})
	// embeds 与 plugins 是同一集合的两个名字（HTML §3.1.4，plugins 为历史别名）。
	installCollection("embeds", "HTMLCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("embed")
	})
	installCollection("plugins", "HTMLCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("embed")
	})
	installCollection("applets", "HTMLCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("applet")
	})
	// links = 带 href 的 <a>/<area>；anchors = 带 name 的 <a>（HTML §3.1.4）。
	installCollection("links", "HTMLCollection", func() []*dom.Element {
		return documentLinkElements(doc)
	})
	installCollection("anchors", "HTMLCollection", func() []*dom.Element {
		return documentAnchorElements(doc)
	})
	// document.all（HTML §3.1.4）：全部元素（HTMLAllCollection；本引擎不实现它的
	// 可调用形态，只提供集合语义与 namedItem）。
	installCollection("all", "HTMLAllCollection", func() []*dom.Element {
		return doc.GetElementsByTagName("*")
	})

	// ── ④ 其余 Document 成员 ──
	// adoptedStyleSheets（CSSOM §document.adoptedStyleSheets）：本引擎的构造式样式表
	// （new CSSStyleSheet）尚未移植，故恒为空数组；赋值被接受但不保留（比 undefined
	// 安全——库的 `document.adoptedStyleSheets.length` 与 `?? []` 分支都能成立）。
	obj.SetAccessor("adoptedStyleSheets",
		getter(func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ jsc.JSValue) {})
	// document.fonts（CSS Font Loading §document.fonts）：FontFaceSet 单例。
	var fontsObj *jsc.JSObject
	obj.SetAccessor("fonts", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if fontsObj == nil {
			fontsObj = makeFontFaceSet(in)
		}
		return jsc.ObjectValue(fontsObj)
	}), nil)
	// hidden/visibilityState（Page Visibility §4）：本引擎无页面可见性信号（不会
	// 最小化/切换标签页），故恒为"可见"。
	obj.SetAccessor("hidden", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(false)
	}), nil)
	obj.SetAccessor("visibilityState", strAcc("visible"), nil)
	// pointerLockElement / pictureInPictureElement：本引擎未实现指针锁定与画中画，
	// 恒 null（规范值域含 null）。
	obj.SetAccessor("pointerLockElement", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null()
	}), nil)
	obj.SetAccessor("pictureInPictureElement", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.Null()
	}), nil)
	// scrollingElement（CSSOM View §3.1）：standards 模式返回 documentElement，
	// quirks 模式返回 body。
	obj.SetAccessor("scrollingElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if doc.Quirks() {
			if b := doc.Body(); b != nil {
				return jsc.ObjectValue(wrapElement(in, b))
			}
		}
		if de := doc.DocumentElement(); de != nil {
			return jsc.ObjectValue(wrapElement(in, de))
		}
		return jsc.Null()
	}), nil)
	// document.timeline（Web Animations §document.timeline）：DocumentTimeline 单例。
	var timelineObj *jsc.JSObject
	obj.SetAccessor("timeline", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if timelineObj == nil {
			timelineObj = makeDocumentTimeline(in)
		}
		return jsc.ObjectValue(timelineObj)
	}), nil)
}

// ─── 集合与对象构造 ──────────────────────────────────────

// wrapElementCollection 构造 HTMLCollection/HTMLAllCollection 语义对象：
// 索引属性 + length + item(i) + namedItem(name)。集合内容是**构建时刻的快照**，
// 由调用方在每次属性读取时按当前 DOM 重建（见 installCollection），因此对使用者
// 表现为 live 集合。
func wrapElementCollection(in *jsc.Interpreter, className string, els []*dom.Element) *jsc.JSObject {
	// ★ 第 19 轮：集合接口原型（HTMLCollection/HTMLAllCollection），
	// 使 `document.forms instanceof HTMLCollection` 成立。
	obj := jsc.NewObject(domIfaceProtoOr(className, in.ObjectPrototype()))
	obj.SetClassName(className)
	objs := make([]*jsc.JSObject, len(els))
	for i, el := range els {
		objs[i] = wrapElement(in, el)
		obj.Set(strconv.Itoa(i), jsc.ObjectValue(objs[i]))
	}
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(objs)))
	}), nil)
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			i := int(args[0].ToNumber())
			if i < 0 || i >= len(objs) {
				return jsc.Null()
			}
			return jsc.ObjectValue(objs[i])
		}, 1)))
	// namedItem：按 id 或 name 内容属性匹配（DOM §4.2.2 HTMLCollection.namedItem）。
	obj.Set("namedItem", jsc.FunctionValue(jsc.NewNativeFunction("namedItem",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			name := args[0].ToString()
			if name == "" {
				return jsc.Null()
			}
			for _, el := range els {
				if el.GetId() == name || el.GetAttribute("name") == name {
					return nodeToJS(in, el)
				}
			}
			return jsc.Null()
		}, 1)))
	return obj
}

// makeLocationObject 用文档 URL 构造 location 对象（HTML §7.2 Location）：
// href/protocol/host/hostname/port/pathname/search/hash/origin/username/password +
// toString/assign/replace/reload。★ 导航类方法（assign/replace/reload 与 href 赋值）
// 在本引擎里没有宿主导航通道，实现为 no-op（读取语义完整，写入不生效）。
func makeLocationObject(in *jsc.Interpreter, rawURL string) *jsc.JSObject {
	href, protocol, host, hostname, port, pathname, search, hash, origin := rawURL, "", "", "", "", "", "", "", ""
	username, password := "", ""
	if u, err := url.Parse(rawURL); err == nil && u != nil {
		href = u.String()
		protocol = u.Scheme + ":"
		host = u.Host
		hostname = u.Hostname()
		port = u.Port()
		pathname = u.EscapedPath()
		if pathname == "" {
			pathname = "/"
		}
		if u.RawQuery != "" {
			search = "?" + u.RawQuery
		}
		if u.Fragment != "" {
			hash = "#" + u.Fragment
		}
		if u.Scheme != "" || u.Host != "" {
			origin = u.Scheme + "://" + u.Host
		}
		username = u.User.Username()
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("Location")
	obj.SetAccessor("href", strAcc(href), func(_ *jsc.Interpreter, _ jsc.JSValue, _ jsc.JSValue) {})
	obj.SetAccessor("protocol", strAcc(protocol), nil)
	obj.SetAccessor("host", strAcc(host), nil)
	obj.SetAccessor("hostname", strAcc(hostname), nil)
	obj.SetAccessor("port", strAcc(port), nil)
	obj.SetAccessor("pathname", strAcc(pathname), nil)
	obj.SetAccessor("search", strAcc(search), nil)
	obj.SetAccessor("hash", strAcc(hash), nil)
	obj.SetAccessor("origin", strAcc(origin), nil)
	obj.SetAccessor("username", strAcc(username), nil)
	obj.SetAccessor("password", strAcc(password), nil)
	obj.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.StringValue(href)
		}, 0)))
	for _, name := range []string{"assign", "replace", "reload"} {
		obj.Set(name, jsc.FunctionValue(jsc.NewNativeFunction(name,
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.Undefined()
			}, 1)))
	}
	return obj
}

// makeDOMImplementation 构造 DOMImplementation（DOM §4.5.1）：hasFeature 恒 true
// （规范允许对任何特性返回 true 的历史遗留 API）；createDocumentType /
// createHTMLDocument / createDocument 按规范语义返回新文档或 doctype 节点。
func makeDOMImplementation(rt *jsc.Interpreter, doc *dom.Document) *jsc.JSObject {
	// ★ 第 19 轮：DOMImplementation 接口原型（document.implementation instanceof …）。
	obj := jsc.NewObject(domIfaceProtoOr("DOMImplementation", rt.ObjectPrototype()))
	obj.SetClassName("DOMImplementation")
	obj.Set("hasFeature", jsc.FunctionValue(jsc.NewNativeFunction("hasFeature",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(true)
		}, 2)))
	argStr := func(args []jsc.JSValue, i int) string {
		if i >= len(args) || args[i].IsNull() || args[i].IsUndefined() {
			return ""
		}
		return args[i].ToString()
	}
	obj.Set("createDocumentType", jsc.FunctionValue(jsc.NewNativeFunction("createDocumentType",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapDocumentType(in,
				dom.NewDocumentType(doc, argStr(args, 0), argStr(args, 1), argStr(args, 2))))
		}, 3)))
	// createHTMLDocument(title)：新建一个带 doctype + html/head/body 骨架的文档，
	// title 非空时写入 <title>（DOM §4.5.1）。
	obj.Set("createHTMLDocument", jsc.FunctionValue(jsc.NewNativeFunction("createHTMLDocument",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			nd := dom.NewDocument()
			_ = nd.AppendChild(dom.NewDocumentType(nd, "html", "", ""))
			htmlEl := nd.CreateElement("html")
			head := nd.CreateElement("head")
			bodyEl := nd.CreateElement("body")
			_ = nd.AppendChild(htmlEl)
			_ = htmlEl.AppendChild(head)
			_ = htmlEl.AppendChild(bodyEl)
			if title := argStr(args, 0); title != "" {
				titleEl := nd.CreateElement("title")
				_ = head.AppendChild(titleEl)
				_ = titleEl.AppendChild(nd.CreateTextNode(title))
			}
			return jsc.ObjectValue(wrapDocument(in, nd))
		}, 1)))
	// createDocument(namespace, qualifiedName, doctype)：新建文档；doctype 非空时
	// 先挂 doctype，qualifiedName 非空时再挂根元素（DOM §4.5.1）。
	obj.Set("createDocument", jsc.FunctionValue(jsc.NewNativeFunction("createDocument",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			nd := dom.NewDocument()
			if len(args) > 2 && !args[2].IsNull() && !args[2].IsUndefined() {
				if dt, ok := unwrapNode(args[2]).(*dom.DocumentType); ok && dt != nil {
					_ = nd.AppendChild(dom.NewDocumentType(nd, dt.Name(), dt.PublicID(), dt.SystemID()))
				}
			}
			if qname := argStr(args, 1); qname != "" {
				_ = nd.AppendChild(nd.CreateElement(qname))
			}
			return jsc.ObjectValue(wrapDocument(in, nd))
		}, 3)))
	return obj
}

// makeFontFaceSet 构造 FontFaceSet（CSS Font Loading §3）：本引擎的字体来自系统
// 字体管理器（同步可用，无异步加载阶段），故 status 恒 "loaded"、check() 恒 true、
// size 恒 0（未通过 JS 注册 FontFace）。ready 返回**已履行的 thenable**：
// `document.fonts.ready.then(cb)` 这一最常见调用形态能拿到回调，而无需引入真
// Promise 的内部实现（本引擎的 Promise 由 goja 提供，此处只需调用形态成立）。
func makeFontFaceSet(in *jsc.Interpreter) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("FontFaceSet")
	obj.SetAccessor("size", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(0)
	}), nil)
	obj.SetAccessor("status", strAcc("loaded"), nil)
	obj.SetAccessor("ready", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return makeResolvedThenable(in, jsc.ObjectValue(obj))
	}), nil)
	obj.Set("add", jsc.FunctionValue(jsc.NewNativeFunction("add",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(obj)
		}, 1)))
	obj.Set("delete", jsc.FunctionValue(jsc.NewNativeFunction("delete",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(false)
		}, 1)))
	obj.Set("clear", jsc.FunctionValue(jsc.NewNativeFunction("clear",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	obj.Set("check", jsc.FunctionValue(jsc.NewNativeFunction("check",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.BooleanValue(true)
		}, 2)))
	obj.Set("load", jsc.FunctionValue(jsc.NewNativeFunction("load",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return makeResolvedThenable(in, jsc.ObjectValue(jsc.NewArrayForInterp(in, nil)))
		}, 2)))
	obj.Set("forEach", jsc.FunctionValue(jsc.NewNativeFunction("forEach",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	return obj
}

// makeDocumentTimeline 构造 DocumentTimeline（Web Animations §document.timeline）：
// currentTime 为文档时间线自创建起的毫秒数（单调时钟）；play/pause/cancel 是
// 时间线自身的播放控制，本引擎的动画由宿主帧循环驱动 → 保持骨架实现（返回
// undefined），同时保证属性读取不抛异常。
func makeDocumentTimeline(in *jsc.Interpreter) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("DocumentTimeline")
	t0 := time.Now()
	obj.SetAccessor("currentTime", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(time.Since(t0).Milliseconds()))
	}), nil)
	for _, name := range []string{"play", "pause", "cancel", "reverse", "finish", "persist"} {
		obj.Set(name, jsc.FunctionValue(jsc.NewNativeFunction(name,
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.Undefined()
			}, 0)))
	}
	return obj
}

// makeResolvedThenable 构造一个「已履行」的 thenable：then(cb) 立即以 value 回调
// cb 并返回自身的 thenable（链式 then 继续立即可达），catch 不触发，finally 立即
// 回调。用于表示无需等待的异步结果（document.fonts.ready / load）。
func makeResolvedThenable(in *jsc.Interpreter, value jsc.JSValue) jsc.JSValue {
	obj := jsc.NewObject(in.ObjectPrototype())
	thenFn := jsc.NewNativeFunction("then",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) > 0 && args[0].IsObject() {
				res, _ := in.Call(args[0], jsc.Undefined(), []jsc.JSValue{value})
				return res
			}
			return jsc.Undefined()
		}, 2)
	obj.Set("then", jsc.FunctionValue(thenFn))
	obj.Set("catch", jsc.FunctionValue(jsc.NewNativeFunction("catch",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(obj)
		}, 1)))
	obj.Set("finally", jsc.FunctionValue(jsc.NewNativeFunction("finally",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) > 0 && args[0].IsObject() {
				_, _ = in.Call(args[0], jsc.Undefined(), nil)
			}
			return jsc.ObjectValue(obj)
		}, 1)))
	return jsc.ObjectValue(obj)
}

// ─── 小工具 ─────────────────────────────────────────────

// documentElementChildren 返回文档的 Element 子节点（文档序）。
func documentElementChildren(doc *dom.Document) []*dom.Element {
	var out []*dom.Element
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok {
			out = append(out, el)
		}
	}
	return out
}

// firstElementChildNode / lastElementChildNode 返回任意节点的首/末 Element 子节点
// （ParentNode 接口语义；Text/Comment/DocumentType 子节点被跳过）。
func firstElementChildNode(n dom.Node) dom.Node {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*dom.Element); ok {
			return c
		}
	}
	return nil
}

func lastElementChildNode(n dom.Node) dom.Node {
	var last dom.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*dom.Element); ok {
			last = c
		}
	}
	return last
}

// documentLinkElements 返回带 href 的 <a>/<area>（document.links，文档序）。
func documentLinkElements(doc *dom.Document) []*dom.Element {
	var out []*dom.Element
	for _, el := range doc.GetElementsByTagName("*") {
		switch el.LocalName() {
		case "a", "area":
			if el.HasAttribute("href") {
				out = append(out, el)
			}
		}
	}
	return out
}

// documentAnchorElements 返回带 name 的 <a>（document.anchors，文档序）。
func documentAnchorElements(doc *dom.Document) []*dom.Element {
	var out []*dom.Element
	for _, el := range doc.GetElementsByTagName("a") {
		if el.HasAttribute("name") {
			out = append(out, el)
		}
	}
	return out
}

// hostOfURL 返回 URL 的 hostname（无端口）；解析失败或无 host 时返回 ""。
func hostOfURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return ""
	}
	return u.Hostname()
}
