package bindings

import (
	"strings"
	"sync"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// ── HTML*Element 接口构造器体系（HTML 规范 §4）─────────────────────────
//
// 浏览器里每个 HTML 元素接口都有一个全局构造器，元素实例位于对应原型链上：
//
//	div instanceof HTMLDivElement   // true
//	div instanceof HTMLElement      // true（HTMLDivElement.prototype.__proto__ = HTMLElement.prototype）
//	div instanceof Element          // true
//	div instanceof Node             // true
//	div.constructor === HTMLDivElement
//
// 此前引擎只注册了 Node/Element/HTMLElement/SVGElement 四个构造器，具体元素接口
// 全缺。后果不只是「instanceof 返回 false」——**当右操作数是 undefined 时，
// `x instanceof undefined` 直接抛 TypeError**（不是返回 false）：
//
//	TypeError: Value is not an object: undefined
//	  at getActiveElementDeep (react-dom-client.production.js)
//	  at commitBeforeMutationEffects / commitRoot
//
// React 19 的 commit 阶段正走 `element instanceof containerInfo.HTMLIFrameElement`，
// 于是整个应用在 commit 阶段中断、页面永久空白且无任何 console 输出（异常发生在
// 调度器异步回调里）。Vue/Lit 等库的元素类型检测、constructor.name 分支同样失效。
//
// 本文件按 WebIDL 建立全部 HTML 元素接口构造器，并由 htmlElementPrototypeFor 提供
// tag → prototype 分派，使 wrapElement 产出的元素包装器直接落在具体接口原型上。

// htmlElementInterface 描述一个 HTML 元素接口（HTML 规范 §4.x）。
type htmlElementInterface struct {
	// name 全局构造器名（HTMLDivElement 等）。
	name string
	// tags 该接口对应的标签名（小写）。一个接口可对应多个标签
	//（HTMLHeadingElement → h1..h6；HTMLTableSectionElement → thead/tbody/tfoot）。
	tags []string
}

// htmlElementInterfaces 是 HTML 规范定义的 HTML*Element 接口全集。
// 不含 HTMLMediaElement/HTMLVideoElement/HTMLAudioElement —— 它们由
// media_element.go 的 registerMediaElementTypes 注册（video/audio 元素已有
// 专用原型与方法），重复注册会覆盖并破坏媒体元素行为。
//
// 表内标签不重复（分派 map 以先注册者为准，重复会静默丢弃后者）。
var htmlElementInterfaces = []htmlElementInterface{
	// 文档结构
	{name: "HTMLHtmlElement", tags: []string{"html"}},
	{name: "HTMLHeadElement", tags: []string{"head"}},
	{name: "HTMLBodyElement", tags: []string{"body"}},
	{name: "HTMLTitleElement", tags: []string{"title"}},
	{name: "HTMLMetaElement", tags: []string{"meta"}},
	{name: "HTMLLinkElement", tags: []string{"link"}},
	{name: "HTMLStyleElement", tags: []string{"style"}},
	{name: "HTMLBaseElement", tags: []string{"base"}},
	{name: "HTMLScriptElement", tags: []string{"script"}},
	{name: "HTMLTemplateElement", tags: []string{"template"}},
	{name: "HTMLSlotElement", tags: []string{"slot"}},

	// 分组与文本
	{name: "HTMLDivElement", tags: []string{"div"}},
	{name: "HTMLSpanElement", tags: []string{"span"}},
	{name: "HTMLParagraphElement", tags: []string{"p"}},
	{name: "HTMLHeadingElement", tags: []string{"h1", "h2", "h3", "h4", "h5", "h6"}},
	{name: "HTMLPreElement", tags: []string{"pre"}},
	{name: "HTMLQuoteElement", tags: []string{"blockquote", "q"}},
	{name: "HTMLModElement", tags: []string{"ins", "del"}},
	{name: "HTMLHRElement", tags: []string{"hr"}},
	{name: "HTMLBRElement", tags: []string{"br"}},
	{name: "HTMLTimeElement", tags: []string{"time"}},
	{name: "HTMLDataElement", tags: []string{"data"}},
	{name: "HTMLFontElement", tags: []string{"font"}},
	{name: "HTMLDirectoryElement", tags: []string{"dir"}},
	{name: "HTMLMarqueeElement", tags: []string{"marquee"}},

	// 列表
	{name: "HTMLUListElement", tags: []string{"ul"}},
	{name: "HTMLOListElement", tags: []string{"ol"}},
	{name: "HTMLLIElement", tags: []string{"li"}},
	{name: "HTMLDListElement", tags: []string{"dl"}},
	{name: "HTMLMenuElement", tags: []string{"menu"}},

	// 链接与嵌入
	{name: "HTMLAnchorElement", tags: []string{"a"}},
	{name: "HTMLAreaElement", tags: []string{"area"}},
	{name: "HTMLImageElement", tags: []string{"img"}},
	{name: "HTMLMapElement", tags: []string{"map"}},
	{name: "HTMLObjectElement", tags: []string{"object"}},
	{name: "HTMLEmbedElement", tags: []string{"embed"}},
	{name: "HTMLParamElement", tags: []string{"param"}},
	{name: "HTMLIFrameElement", tags: []string{"iframe"}},
	{name: "HTMLFrameElement", tags: []string{"frame"}},
	{name: "HTMLFrameSetElement", tags: []string{"frameset"}},
	{name: "HTMLPictureElement", tags: []string{"picture"}},
	{name: "HTMLSourceElement", tags: []string{"source"}},
	{name: "HTMLTrackElement", tags: []string{"track"}},

	// 表单
	{name: "HTMLFormElement", tags: []string{"form"}},
	{name: "HTMLInputElement", tags: []string{"input"}},
	{name: "HTMLTextAreaElement", tags: []string{"textarea"}},
	{name: "HTMLSelectElement", tags: []string{"select"}},
	{name: "HTMLOptionElement", tags: []string{"option"}},
	{name: "HTMLOptGroupElement", tags: []string{"optgroup"}},
	{name: "HTMLDataListElement", tags: []string{"datalist"}},
	{name: "HTMLOutputElement", tags: []string{"output"}},
	{name: "HTMLProgressElement", tags: []string{"progress"}},
	{name: "HTMLMeterElement", tags: []string{"meter"}},
	{name: "HTMLLabelElement", tags: []string{"label"}},
	{name: "HTMLButtonElement", tags: []string{"button"}},
	{name: "HTMLFieldSetElement", tags: []string{"fieldset"}},
	{name: "HTMLLegendElement", tags: []string{"legend"}},

	// 表格
	{name: "HTMLTableElement", tags: []string{"table"}},
	{name: "HTMLTableCaptionElement", tags: []string{"caption"}},
	{name: "HTMLTableColElement", tags: []string{"col", "colgroup"}},
	{name: "HTMLTableSectionElement", tags: []string{"thead", "tbody", "tfoot"}},
	{name: "HTMLTableRowElement", tags: []string{"tr"}},
	{name: "HTMLTableCellElement", tags: []string{"td", "th"}},

	// 画布与交互
	{name: "HTMLCanvasElement", tags: []string{"canvas"}},
	{name: "HTMLDetailsElement", tags: []string{"details"}},
	{name: "HTMLDialogElement", tags: []string{"dialog"}},

	// 兜底：无对应接口的未知标签（<foo>）。自定义元素（名称含 '-'）在浏览器里
	// 落到 HTMLElement 而非 HTMLUnknownElement，由 htmlElementPrototypeFor 区分。
	{name: "HTMLUnknownElement", tags: nil},
}

// svgOwnedTags 是 SVG 命名空间专属标签名。
//
// 引擎的 dom.Element 不保存命名空间（engine/dom/element.go 顶部注释：qualified
// names / namespaces are collapsed to a plain local tag name），且 LocalName() 会
// 把标签名小写化（SVG 的 foreignObject/clipPath/linearGradient →
// foreignobject/clippath/lineargradient），因此这里用**小写标签名启发式**分派：
// 落在本表的标签使用 SVGElement.prototype，其余按 HTML 接口分派。
//
// 重名标签（<a>/<title>/<style>/<script>/<font>）以 HTML 为准 —— 这是缺命名空间
// 信息的既有局限；dom 层将来若补命名空间，应改为按命名空间分派。
var svgOwnedTags = []string{
	"svg", "circle", "ellipse", "line", "path", "polygon", "polyline", "rect",
	"g", "defs", "desc", "metadata", "symbol", "use", "view", "switch",
	"foreignobject", "tspan", "textpath", "marker", "pattern", "mask", "clippath",
	"lineargradient", "radialgradient", "stop", "filter",
	"animate", "animatemotion", "animatetransform", "mpath",
	"feblend", "fecolormatrix", "fecomponenttransfer", "fecomposite",
	"feconvolvematrix", "fediffuselighting", "fedisplacementmap", "fedistantlight",
	"fedropshadow", "feflood", "fefunca", "fefuncb", "fefuncg", "fefuncr",
	"fegaussianblur", "feimage", "femerge", "femergenode", "femorphology",
	"feoffset", "fepointlight", "fespecularlighting", "fespotlight", "fetile",
	"feturbulence",
}

// tag → prototype 分派表（注册后只读；用 RWMutex 兼顾初始化期并发安全）。
var (
	htmlElemProtoMu    sync.RWMutex
	htmlProtoByTag     map[string]*jsc.JSObject
	htmlProtoDefault   *jsc.JSObject // HTMLElement.prototype
	htmlProtoUnknown   *jsc.JSObject // HTMLUnknownElement.prototype
	htmlProtoSVG       *jsc.JSObject // SVGElement.prototype
	htmlElementTypesOn bool
)

// registerHTMLElementTypes 注册全部 HTML*Element 构造器并建立 tag → prototype 分派表。
//
// htmlElementProto = HTMLElement.prototype（所有具体元素接口的父），
// svgElementProto = SVGElement.prototype（SVG 专属标签兜底）。
// 必须在 dom.go 接好 HTMLElement.prototype.__proto__ = Element.prototype 之后调用。
func registerHTMLElementTypes(rt *jsc.Interpreter, g *jsc.JSObject, htmlElementProto, svgElementProto *jsc.JSObject) {
	if rt == nil || g == nil || htmlElementProto == nil {
		return
	}
	// newCtor 建一个元素接口构造器。
	//
	// 浏览器里这些接口不可构造（new HTMLDivElement() 抛 TypeError: Illegal
	// constructor），此处采用宽容实现：返回 this（或新建对象）而不抛错 ——
	// 与既有 Node/Element/HTMLElement 的 emptyCtor 一致，避免破坏线上脚本。
	newCtor := func(name string, parent *jsc.JSObject) *jsc.JSObject {
		ctor := rt.NewConstructor(name, func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			if o := this.AsObject(); o != nil {
				return o
			}
			return jsc.NewObject(nil)
		})
		fn := jsc.FunctionValue(ctor)
		g.Set(name, fn)
		proto := fn.AsObject().GetStr("prototype").AsObject()
		if proto == nil {
			return nil
		}
		if parent != nil {
			// 接原型链：XxxElement.prototype.__proto__ = HTMLElement.prototype
			proto.Set("__proto__", jsc.ObjectValue(parent))
		}
		// 显式回填 constructor：库常读 el.constructor / el.constructor.name 做分支。
		proto.Set("constructor", fn)
		return proto
	}

	byTag := make(map[string]*jsc.JSObject, len(htmlElementInterfaces)+len(svgOwnedTags))
	var unknownProto *jsc.JSObject
	for _, itf := range htmlElementInterfaces {
		proto := newCtor(itf.name, htmlElementProto)
		if itf.name == "HTMLUnknownElement" {
			unknownProto = proto
			continue
		}
		for _, tag := range itf.tags {
			if _, dup := byTag[tag]; !dup {
				byTag[tag] = proto
			}
		}
	}
	// SVG 专属标签 → SVGElement.prototype（不与 HTML 接口争抢已占用的标签名）。
	if svgElementProto != nil {
		for _, tag := range svgOwnedTags {
			if _, dup := byTag[tag]; !dup {
				byTag[tag] = svgElementProto
			}
		}
	}

	htmlElemProtoMu.Lock()
	htmlProtoByTag = byTag
	htmlProtoDefault = htmlElementProto
	htmlProtoUnknown = unknownProto
	htmlProtoSVG = svgElementProto
	htmlElementTypesOn = true
	htmlElemProtoMu.Unlock()
}

// htmlElementPrototypeFor 返回元素包装器应使用的原型对象。
//
// 返回 nil 表示无可用分派（调用方回退到 Element.prototype）。
// 分派顺序：具体接口 → SVG 专属标签 → 自定义元素（HTMLElement）→ 未知标签
//（HTMLUnknownElement）。<video>/<audio> 由 mediaElementPrototypeFor 处理，
// 本函数不涉及（表内已排除，避免覆盖媒体元素方法）。
func htmlElementPrototypeFor(el *dom.Element) *jsc.JSObject {
	if el == nil {
		return nil
	}
	htmlElemProtoMu.RLock()
	defer htmlElemProtoMu.RUnlock()
	if !htmlElementTypesOn {
		return nil
	}
	tag := el.LocalName()
	if p := htmlProtoByTag[tag]; p != nil {
		return p
	}
	def := htmlProtoDefault
	if def == nil {
		return nil
	}
	// 自定义元素（名称含 '-'，如 <my-widget>）：浏览器返回 HTMLElement。
	if strings.IndexByte(tag, '-') >= 0 {
		return def
	}
	if htmlProtoUnknown != nil {
		return htmlProtoUnknown
	}
	return def
}
