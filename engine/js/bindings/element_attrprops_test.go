package bindings

// Element 侧属性收口（第 16 次监督轮）的绑定测试：
//
//	批 A（Node/Element 核心 15 项）：localName / namespaceURI / prefix / tabIndex /
//	  innerText / outerText / lang / dir / draggable / spellcheck / translate /
//	  accessKey / nonce / inert / autofocus
//	批 B（几何 / Shadow 相关 8 项）：offsetParent / clientTop / clientLeft / part /
//	  slot / assignedSlot / contentEditable / isContentEditable
//
// 覆盖两类硬要求：
//  1. `'x' in el === true` —— 全部登记进 elemKnownPropNames（Has 只看那张表）；
//  2. 值语义与 Edge 逐项一致：反射属性（含 setter 写回属性）、默认值、live
//     语义（读一次后再改属性/树 → 立刻反映）、无 shadow 分配时 part 为空表且
//     assignedSlot 为 null（不得以「不适用」跳过）。
//
// 双侧（wbui vs Edge）证据：dev/fixtures/webshot/element_attrs.html（85/85 行
// IDENTICAL）与 element_geom.html（61/61 行 IDENTICAL），经 dev/tools/gprobe_cmp.sh 产出。

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// elemAttrPropNames 是本轮收口的 23 个属性名（探针 elProps 的缺失清单）。
var elemAttrPropNames = []string{
	"localName", "namespaceURI", "prefix", "tabIndex", "innerText", "outerText",
	"lang", "dir", "draggable", "spellcheck", "translate", "accessKey",
	"nonce", "inert", "autofocus",
	"offsetParent", "clientTop", "clientLeft", "part", "slot", "assignedSlot",
	"contentEditable", "isContentEditable",
}

// 23 项全部存在：`'x' in el === true`（登记进 elemKnownPropNames）。
func TestElementAttrPropsExist(t *testing.T) {
	runElementProbe(t, `(function(){
		var names = `+jsStringArray(elemAttrPropNames)+`;
		var el = document.createElement('div');
		var miss = [];
		for (var i = 0; i < names.length; i++) {
			if (!(names[i] in el)) { miss.push(names[i]); }
		}
		return miss.length ? 'missing: ' + miss.join(',') : 'PASS';
	})()`)
}

// jsStringArray 把 Go 字符串切片渲染成 JS 字面量数组。
func jsStringArray(names []string) string {
	out := "["
	for i, n := range names {
		if i > 0 {
			out += ","
		}
		out += "'" + n + "'"
	}
	return out + "]"
}

// runElementProbeWithBody 在**带 <html><body>** 的解释器里执行探测 JS（结果必须为
// "PASS"）。浏览器语义的断言（innerText 的「已渲染」路径、offsetParent 回退到
// body）要求 document.body 存在，而裸 dom.NewDocument() 没有 html/body —— 测试侧
// 显式搭出这两层（只搭测试环境，不改引擎）。
func runElementProbeWithBody(t *testing.T, src string) {
	t.Helper()
	rt, doc, _ := newRuntimeWithDoc(t)
	html := doc.CreateElement("html")
	body := doc.CreateElement("body")
	if err := html.AppendChild(body); err != nil {
		t.Fatalf("append body: %v", err)
	}
	if err := doc.AppendChild(html); err != nil {
		t.Fatalf("append html: %v", err)
	}
	v, err := rt.RunJS(src)
	if err != nil {
		t.Fatalf("RunJS error: %v", err)
	}
	if got := v.ToString(); got != "PASS" {
		t.Fatalf("probe failed: %s", got)
	}
}

// 批 A · localName / namespaceURI / prefix（DOM §4.3.1）。
func TestElementAttrNodeCore(t *testing.T) {
	runElementProbe(t, `(function(){
		var d = document.createElement('div');
		if (d.localName !== 'div') { return 'localName=' + d.localName; }
		if (d.localName === d.tagName) { return 'localName 应与大写 tagName 不同'; }
		if (d.namespaceURI !== 'http://www.w3.org/1999/xhtml') { return 'namespaceURI=' + d.namespaceURI; }
		if (d.prefix !== null) { return 'prefix=' + d.prefix; }
		var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
		if (svg.localName !== 'svg') { return 'svg.localName=' + svg.localName; }
		if (svg.namespaceURI !== 'http://www.w3.org/2000/svg') { return 'svg.namespaceURI=' + svg.namespaceURI; }
		var rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
		svg.appendChild(rect);
		if (rect.namespaceURI !== 'http://www.w3.org/2000/svg') { return 'rect.namespaceURI=' + rect.namespaceURI; }
		return 'PASS';
	})()`)
}

// 批 A · tabIndex：反射 tabindex + 无属性时的默认值（Edge 实测对齐）。
func TestElementAttrTabIndex(t *testing.T) {
	runElementProbe(t, `(function(){
		var d = document.createElement('div');
		if (d.tabIndex !== -1) { return 'div.tabIndex=' + d.tabIndex; }
		if (document.createElement('button').tabIndex !== 0) { return 'button'; }
		if (document.createElement('input').tabIndex !== 0) { return 'input'; }
		if (document.createElement('span').tabIndex !== -1) { return 'span'; }
		if (document.createElement('a').tabIndex !== 0) { return 'a(no href)'; }
		if (document.createElement('summary').tabIndex !== -1) { return 'detached summary 应为 -1'; }
		var det = document.createElement('details');
		var sm = document.createElement('summary');
		det.appendChild(sm);
		if (sm.tabIndex !== 0) { return 'details 首个 summary 应为 0'; }

		var t3 = document.createElement('div');
		t3.setAttribute('tabindex', '3');
		if (t3.tabIndex !== 3) { return 'tabindex=3 → ' + t3.tabIndex; }
		t3.setAttribute('tabindex', 'abc');
		if (t3.tabIndex !== -1) { return 'tabindex=abc → ' + t3.tabIndex; }
		var set5 = document.createElement('div');
		set5.tabIndex = 5;
		if (set5.getAttribute('tabindex') !== '5') { return 'setter attr=' + set5.getAttribute('tabindex'); }
		if (set5.tabIndex !== 5) { return 'setter value=' + set5.tabIndex; }
		return 'PASS';
	})()`)
}

// 批 A · innerText / outerText（HTML §3.2.7）。
func TestElementAttrInnerText(t *testing.T) {
	runElementProbeWithBody(t, `(function(){
		var LF = String.fromCharCode(10);

		// 未渲染（detached）→ 回退 textContent，不做空白折叠
		var det = document.createElement('div');
		det.textContent = 'a  b';
		if (det.innerText !== 'a  b') { return 'detached=' + det.innerText; }
		if (det.outerText !== 'a  b') { return 'detached outerText=' + det.outerText; }

		// 已渲染：行内/换行/块级/<br>/display:none/visibility:hidden
		var c = document.createElement('div');
		document.body.appendChild(c);
		c.innerHTML = 'Hello <span>World</span>';
		if (c.innerText !== 'Hello World') { return 'inline=' + c.innerText; }
		if (c.outerText !== c.innerText) { return 'outerText 应与 innerText 同值'; }

		var br = document.createElement('div');
		document.body.appendChild(br);
		br.innerHTML = 'a<br>b';
		if (br.innerText !== 'a' + LF + 'b') { return 'br=' + br.innerText; }

		var blocks = document.createElement('div');
		document.body.appendChild(blocks);
		blocks.innerHTML = '<div>x</div><div>y</div>';
		if (blocks.innerText !== 'x' + LF + 'y') { return 'blocks=' + blocks.innerText; }

		var dn = document.createElement('div');
		document.body.appendChild(dn);
		dn.innerHTML = 'a<span style="display:none">z</span>b';
		if (dn.innerText !== 'ab') { return 'displayNoneChild=' + dn.innerText; }

		var vh = document.createElement('div');
		document.body.appendChild(vh);
		vh.innerHTML = 'a<span style="visibility:hidden">z</span>b';
		if (vh.innerText !== 'ab') { return 'visibilityHidden=' + vh.innerText; }

		// 文本节点里的换行是**可折叠空白**（不是换行）
		var ws = document.createElement('div');
		document.body.appendChild(ws);
		ws.textContent = 'a' + LF + '   b';
		if (ws.innerText !== 'a b') { return 'whitespace=' + ws.innerText; }

		// 祖先 display:none → 整体回退 textContent
		var hid = document.createElement('div');
		hid.style.display = 'none';
		var hidc = document.createElement('span');
		hidc.textContent = 'a  b';
		hid.appendChild(hidc);
		document.body.appendChild(hid);
		if (hidc.innerText !== 'a  b') { return 'underDisplayNone=' + hidc.innerText; }

		// setter：按行拆分、<br> 连接（结构断言，避免依赖 innerHTML 的空元素序列化）
		var t7 = document.createElement('div');
		document.body.appendChild(t7);
		t7.innerText = 'a' + LF + 'b';
		if (t7.childNodes.length !== 3) { return 'set.childNodes=' + t7.childNodes.length; }
		if (t7.firstChild.nodeValue !== 'a') { return 'set.first=' + t7.firstChild.nodeValue; }
		if (t7.childNodes[1].nodeName !== 'BR') { return 'set.mid=' + t7.childNodes[1].nodeName; }
		if (t7.lastChild.nodeValue !== 'b') { return 'set.last=' + t7.lastChild.nodeValue; }
		if (t7.innerText !== 'a' + LF + 'b') { return 'set.readback=' + t7.innerText; }

		// outerText setter：替换元素**自身**
		var w = document.createElement('div');
		document.body.appendChild(w);
		var s = document.createElement('span');
		s.textContent = 'old';
		w.appendChild(s);
		s.outerText = 'new';
		if (w.childNodes.length !== 1) { return 'outerText.childNodes=' + w.childNodes.length; }
		if (w.firstChild.nodeValue !== 'new') { return 'outerText.child=' + w.firstChild.nodeValue; }
		return 'PASS';
	})()`)
}

// 批 A · 反射属性：lang / dir / draggable / spellcheck / translate / accessKey /
// nonce / inert / autofocus（含 setter 写回内容属性）。
func TestElementAttrReflected(t *testing.T) {
	runElementProbe(t, `(function(){
		var d = document.createElement('div');
		if (d.lang !== '') { return 'lang=' + d.lang; }
		d.lang = 'en';
		if (d.getAttribute('lang') !== 'en' || d.lang !== 'en') { return 'lang setter'; }
		if (d.dir !== '') { return 'dir=' + d.dir; }
		d.dir = 'rtl';
		if (d.dir !== 'rtl') { return 'dir setter'; }

		if (d.draggable !== false) { return 'div.draggable=' + d.draggable; }
		if (document.createElement('img').draggable !== true) { return 'img.draggable 应为 true'; }
		var a = document.createElement('a');
		a.setAttribute('href', '#');
		if (a.draggable !== true) { return 'a[href].draggable 应为 true'; }
		var dbl = document.createElement('div');
		dbl.draggable = true;
		if (dbl.getAttribute('draggable') !== 'true') { return 'draggable setter'; }

		if (d.spellcheck !== true) { return 'spellcheck=' + d.spellcheck; }
		var spf = document.createElement('div');
		spf.setAttribute('spellcheck', 'false');
		if (spf.spellcheck !== false) { return 'spellcheck=false'; }
		if (d.translate !== true) { return 'translate=' + d.translate; }
		var trn = document.createElement('div');
		trn.setAttribute('translate', 'no');
		if (trn.translate !== false) { return 'translate=no'; }

		if (d.accessKey !== '') { return 'accessKey=' + d.accessKey; }
		d.accessKey = 'q';
		if (d.getAttribute('accesskey') !== 'q') { return 'accessKey setter'; }
		if (d.nonce !== '') { return 'nonce=' + d.nonce; }
		d.nonce = 'n1';
		if (d.getAttribute('nonce') !== 'n1') { return 'nonce setter'; }

		if (d.inert !== false) { return 'inert=' + d.inert; }
		d.inert = true;
		if (!d.hasAttribute('inert')) { return 'inert setter 未写属性'; }
		d.inert = false;
		if (d.hasAttribute('inert')) { return 'inert setter 未移除属性'; }
		if (d.autofocus !== false) { return 'autofocus=' + d.autofocus; }
		d.autofocus = true;
		if (!d.hasAttribute('autofocus')) { return 'autofocus setter'; }
		return 'PASS';
	})()`)
}

// 批 B · offsetParent / clientTop / clientLeft（CSSOM View §7）。
func TestElementAttrGeometryProps(t *testing.T) {
	runElementProbeWithBody(t, `(function(){
		var det = document.createElement('div');
		if (det.offsetParent !== null) { return 'detached offsetParent 应为 null'; }
		if (det.clientTop !== 0) { return 'detached clientTop=' + det.clientTop; }
		if (det.clientLeft !== 0) { return 'detached clientLeft=' + det.clientLeft; }

		var host = document.createElement('div');
		document.body.appendChild(host);
		if (host.offsetParent !== document.body) { return 'plain offsetParent 应为 body'; }

		var rel = document.createElement('div');
		rel.style.position = 'relative';
		host.appendChild(rel);
		var inner = document.createElement('div');
		rel.appendChild(inner);
		if (inner.offsetParent !== rel) { return 'inner.offsetParent 应为定位祖先 rel'; }

		var fx = document.createElement('div');
		fx.style.position = 'fixed';
		host.appendChild(fx);
		if (fx.offsetParent !== null) { return 'fixed offsetParent 应为 null'; }

		var hn = document.createElement('div');
		hn.style.display = 'none';
		host.appendChild(hn);
		if (hn.offsetParent !== null) { return 'display:none offsetParent 应为 null'; }

		if (document.body.offsetParent !== null) { return 'body offsetParent 应为 null'; }

		var bd = document.createElement('div');
		bd.setAttribute('style', 'border:2px solid #000');
		host.appendChild(bd);
		if (bd.clientTop !== 2) { return 'bordered clientTop=' + bd.clientTop; }
		if (bd.clientLeft !== 2) { return 'bordered clientLeft=' + bd.clientLeft; }

		var bt = document.createElement('div');
		bt.setAttribute('style', 'border-top:3px solid #000');
		host.appendChild(bt);
		if (bt.clientTop !== 3) { return 'border-top only clientTop=' + bt.clientTop; }
		if (bt.clientLeft !== 0) { return 'border-top only clientLeft=' + bt.clientLeft; }

		// border-width 声明但 border-style 为 none（初始值）→ 计算宽度为 0
		var noStyle = document.createElement('div');
		noStyle.setAttribute('style', 'border-width:4px');
		host.appendChild(noStyle);
		if (noStyle.clientTop !== 0) { return 'border-width-only clientTop=' + noStyle.clientTop; }
		return 'PASS';
	})()`)
}

// 批 B · part / slot / assignedSlot：无 shadow 分配时属性存在、值为空表/null。
func TestElementAttrShadowProps(t *testing.T) {
	runElementProbeWithBody(t, `(function(){
		var d = document.createElement('div');
		// part 是 DOMTokenList：无 part 属性 → 空表（不是 null），且同一实例
		if (typeof d.part !== 'object' || d.part === null) { return 'part 类型/空值错误'; }
		if (d.part.length !== 0) { return 'part.length=' + d.part.length; }
		if (d.part.value !== '') { return 'part.value=' + d.part.value; }
		if (d.part !== d.part) { return 'part 应为同一实例'; }
		if (d.part.contains('x') !== false) { return 'part.contains 空表应为 false'; }
		d.setAttribute('part', 'a b');
		if (d.part.length !== 2) { return 'part.length(a b)=' + d.part.length; }
		if (d.part.value !== 'a b') { return 'part.value=' + d.part.value; }
		if (d.part.item(1) !== 'b') { return 'part.item(1)=' + d.part.item(1); }

		// slot：反射（无属性 → ""）
		if (d.slot !== '') { return 'slot=' + d.slot; }
		d.slot = 'n1';
		if (d.getAttribute('slot') !== 'n1') { return 'slot setter'; }

		// assignedSlot：无 shadow 分配 → null（属性存在）
		if (d.assignedSlot !== null) { return 'assignedSlot 无分配应为 null'; }

		// 真分配：shadow host 的 light-DOM 子节点 → 对应 <slot>
		var host = document.createElement('div');
		document.body.appendChild(host);
		var sr = host.attachShadow({ mode: 'open' });
		var slotEl = document.createElement('slot');
		sr.appendChild(slotEl);
		var light = document.createElement('span');
		host.appendChild(light);
		if (light.assignedSlot !== slotEl) { return 'assignedSlot 应为对应 <slot>'; }

		var named = document.createElement('slot');
		named.setAttribute('name', 'n2');
		sr.appendChild(named);
		var light2 = document.createElement('span');
		light2.setAttribute('slot', 'n2');
		host.appendChild(light2);
		if (light2.assignedSlot !== named) { return 'named assignedSlot 错误'; }
		return 'PASS';
	})()`)
}

// 批 B · contentEditable / isContentEditable（HTML §6.7.4）。
func TestElementAttrContentEditable(t *testing.T) {
	runElementProbe(t, `(function(){
		var d = document.createElement('div');
		if (d.contentEditable !== 'inherit') { return 'default=' + d.contentEditable; }
		if (d.isContentEditable !== false) { return 'default isContentEditable'; }

		var e = document.createElement('div');
		e.setAttribute('contenteditable', 'true');
		if (e.contentEditable !== 'true') { return 'attrTrue=' + e.contentEditable; }
		if (e.isContentEditable !== true) { return 'attrTrue isContentEditable'; }
		var child = document.createElement('span');
		e.appendChild(child);
		if (child.isContentEditable !== true) { return '子元素应继承可编辑'; }
		var off = document.createElement('span');
		off.setAttribute('contenteditable', 'false');
		e.appendChild(off);
		if (off.isContentEditable !== false) { return '显式 false 应为 false'; }

		var empty = document.createElement('div');
		empty.setAttribute('contenteditable', '');
		if (empty.contentEditable !== 'true') { return 'attrEmpty=' + empty.contentEditable; }
		var plain = document.createElement('div');
		plain.setAttribute('contenteditable', 'plaintext-only');
		if (plain.contentEditable !== 'plaintext-only') { return 'plaintext-only=' + plain.contentEditable; }
		var bad = document.createElement('div');
		bad.setAttribute('contenteditable', 'x');
		if (bad.contentEditable !== 'inherit') { return 'invalid=' + bad.contentEditable; }

		e.contentEditable = 'inherit';
		if (e.hasAttribute('contenteditable')) { return 'setter inherit 应移除属性'; }
		if (e.contentEditable !== 'inherit') { return 'setter inherit 回读=' + e.contentEditable; }
		return 'PASS';
	})()`)
}

// 批 A/B 的 Go 层辅助：命名空间判定、边框宽度回退链、offsetParent 判定
// （直接测 Go 函数，避开 JS 层包装）。
func TestElementAttrHelpersGoLevel(t *testing.T) {
	doc := dom.NewDocument()

	div := doc.CreateElement("div")
	if got := elementNamespaceURI(div); got != xhtmlNamespaceURI {
		t.Fatalf("div.namespaceURI = %q, want %q", got, xhtmlNamespaceURI)
	}
	svg := doc.CreateElement("svg")
	if got := elementNamespaceURI(svg); got != svgNamespaceURI {
		t.Fatalf("svg.namespaceURI = %q, want %q", got, svgNamespaceURI)
	}
	mathEl := doc.CreateElement("math")
	if got := elementNamespaceURI(mathEl); got != mathMLNamespaceURI {
		t.Fatalf("math.namespaceURI = %q, want %q", got, mathMLNamespaceURI)
	}
	// 显式 createElementNS 记录优先于祖先启发式
	rec := doc.CreateElement("custom-thing")
	recordElementNamespace(rec, svgNamespaceURI)
	if got := elementNamespaceURI(rec); got != svgNamespaceURI {
		t.Fatalf("recorded ns = %q, want %q", got, svgNamespaceURI)
	}
	delete(elemNamespaceOverrides, rec)

	// tabIndex 默认值与 HTML 整数解析
	btn := doc.CreateElement("button")
	if got := elementTabIndex(btn); got != 0 {
		t.Fatalf("button.tabIndex = %d, want 0", got)
	}
	btn.SetAttribute("tabindex", " 7 ")
	if got := elementTabIndex(btn); got != 7 {
		t.Fatalf("button tabindex=' 7 ' = %d, want 7", got)
	}
	btn.SetAttribute("tabindex", "abc")
	if got := elementTabIndex(btn); got != 0 {
		t.Fatalf("button tabindex=abc = %d, want 0（回退默认值）", got)
	}
	if n, ok := parseHTMLInteger("-12px"); !ok || n != -12 {
		t.Fatalf("parseHTMLInteger(-12px) = %d,%v, want -12,true", n, ok)
	}
	if _, ok := parseHTMLInteger("+"); ok {
		t.Fatalf("parseHTMLInteger(+) 应失败")
	}

	// idlLong：WebIDL long 的 ToInt32 回绕语义
	if got := idlLong(jsc.Undefined()); got != 0 {
		t.Fatalf("idlLong(undefined) = %d, want 0", got)
	}

	// cssShorthandEdge：1~4 值分派
	if got := cssShorthandEdge("1px 2px 3px 4px", "left"); got != "4px" {
		t.Fatalf("cssShorthandEdge(4 值, left) = %q, want 4px", got)
	}
	if got := cssShorthandEdge("1px 2px", "top"); got != "1px" {
		t.Fatalf("cssShorthandEdge(2 值, top) = %q, want 1px", got)
	}
	if got := cssShorthandEdge("1px solid #000", "top"); got != "1px" {
		t.Fatalf("cssShorthandEdge(border 简写) = %q, want 1px", got)
	}
	if got := cssShorthandEdge("solid #000", "top"); got != "" {
		t.Fatalf("cssShorthandEdge(无长度) = %q, want 空", got)
	}
	if got := parseCSSLengthPx("2.5px"); got != 2.5 {
		t.Fatalf("parseCSSLengthPx(2.5px) = %v, want 2.5", got)
	}

	// borderStyleForSide：边框样式回退链（style 为 none 时宽度按 0 处理）
	if got := borderStyleForSide(map[string]string{"border-top": "1px solid #000"}, "top"); got != "solid" {
		t.Fatalf("borderStyleForSide(单边简写) = %q, want solid", got)
	}
	if got := borderStyleForSide(map[string]string{}, "top"); got != "none" {
		t.Fatalf("borderStyleForSide(无声明) = %q, want none", got)
	}
	// CSS 盒简写的 2 值形式是 [上下, 左右]（bottom 与 top 共用第一个值）
	if got := cssShorthandStyleEdge("solid dashed", "bottom"); got != "solid" {
		t.Fatalf("cssShorthandStyleEdge(2 值, bottom) = %q, want solid", got)
	}
	if got := cssShorthandStyleEdge("solid dashed", "left"); got != "dashed" {
		t.Fatalf("cssShorthandStyleEdge(2 值, left) = %q, want dashed", got)
	}

	// borderWidthPxForSide：无声明 → 0；单边简写 → 取宽度
	if got := borderWidthPxForSide(div, "top"); got != 0 {
		t.Fatalf("borderWidthPxForSide(div) = %v, want 0", got)
	}
	// innerText 渲染判定：未连接元素 → 回退 textContent
	det := doc.CreateElement("div")
	det.SetTextContent("a  b")
	if got := elementInnerText(det); got != "a  b" {
		t.Fatalf("elementInnerText(detached) = %q, want %q", got, "a  b")
	}
}
