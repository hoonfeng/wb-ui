// engine/js/bindings/elemattr.go — Element 侧属性收口（第 16 次监督轮）。
//
// 背景：dev/probes/webplatform 的 elProps（65 项）在 `document.createElement('div')`
// 上有 23 项缺失。本轮只动绑定层，把 Node/Element 核心属性（批 A 15 项：
// localName / namespaceURI / prefix / tabIndex / innerText / outerText / lang /
// dir / draggable / spellcheck / translate / accessKey / nonce / inert /
// autofocus）与几何/Shadow 相关属性（批 B 8 项）一次补齐。集中在本文件，
// 由 installElementProperty 委派（同 <video>/popover 的做法），不把 23 个
// case 散进那个大 switch。
//
// 三条硬约束（监督指令）：
//  1. `'x' in el === true` —— 全部登记进 elemKnownPropNames（Has 只看那张表）；
//  2. 反射属性登记进 elemAccessorProps（活值）—— 适配器层缓存住 getter 结果
//     会让 `el.setAttribute('lang','en')` 之后仍读到旧值；
//  3. 值语义与 Edge 逐项一致 —— 证据是 dev/fixtures/webshot/element_attrs.html
//     （批 A）与 element_geom.html（批 B）经 dev/tools/gprobe_cmp.sh 双侧比对
//     IDENTICAL（台账贴 wbui/Edge 行数）。
//
// 与布局无关：本文件不读渲染树；几何相关项只读 computed style（CSSOM 语义），
// engine/layout 零改动。
package bindings

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// ── 命名空间（DOM §4.3 Element）───────────────────────────
//
// DOM 层的 Element 只保留本地标签名（engine/dom/element.go 文件头：qualified
// names / namespaces are collapsed to a plain local tag name），因此绑定层补一张
// **最小**的命名空间表：
//   - createElementNS 的显式记录（最权威，覆盖 detached 元素）；
//   - 最近的 <svg> / <math> 祖先（HTML 解析器把 <svg>/<math> 子树标成对应
//     命名空间；<foreignObject> 之下回到 HTML）。
const (
	xhtmlNamespaceURI  = "http://www.w3.org/1999/xhtml"
	svgNamespaceURI    = "http://www.w3.org/2000/svg"
	mathMLNamespaceURI = "http://www.w3.org/1998/Math/MathML"
)

// elemNamespaceOverrides 记录经 document.createElementNS(ns, tag) 创建、且
// 命名空间不是 HTML 的元素。createElement / HTML 解析走文档默认命名空间，
// 无需记录。写入量限于显式的非 HTML 命名空间元素（页面里通常是个位数的
// SVG/MathML 根），不会随普通 DOM 构建增长。
var elemNamespaceOverrides = map[*dom.Element]string{}

// recordElementNamespace 由 document.createElementNS 调用。
func recordElementNamespace(el *dom.Element, ns string) {
	if el == nil || ns == "" || ns == xhtmlNamespaceURI {
		return
	}
	elemNamespaceOverrides[el] = ns
}

// elementNamespaceURI 返回元素的命名空间 URI（判定顺序见文件头说明）。
func elementNamespaceURI(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if ns, ok := elemNamespaceOverrides[el]; ok {
		return ns
	}
	for n := dom.Node(el); !isNilNode(n); n = n.ParentNode() {
		e, ok := n.(*dom.Element)
		if !ok || e == nil {
			continue
		}
		switch e.LocalName() {
		case "svg":
			return svgNamespaceURI
		case "math":
			return mathMLNamespaceURI
		case "foreignObject":
			return xhtmlNamespaceURI
		}
	}
	return xhtmlNamespaceURI
}

// ── 属性值解析（HTML 的「rules for parsing integers」）──────

// parseHTMLInteger 按 HTML 的整数解析规则解析 s：跳过前导空白，可选 +/-
// 符号，后跟一位以上 ASCII 数字；解析失败返回 ok=false（调用方用默认值）。
func parseHTMLInteger(s string) (int, bool) {
	t := strings.TrimLeft(s, " \t\n\f\r")
	if t == "" {
		return 0, false
	}
	i := 0
	sign := 1
	if t[0] == '+' || t[0] == '-' {
		if t[0] == '-' {
			sign = -1
		}
		i = 1
	}
	start := i
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	n, err := strconv.Atoi(t[start:i])
	if err != nil {
		return 0, false
	}
	return sign * n, true
}

// idlLong 按 WebIDL long 的规则把 JS 值转成 Go int（ToNumber → ToInt32 的
// modulo 2^32 回绕；NaN/Infinity → 0）。禁止直接用 int(float) —— Go 对大数
// 的 float→int 转换结果与实现相关。
func idlLong(v jsc.JSValue) int {
	f := v.ToNumber()
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(f), 4294967296)
	if m < 0 {
		m += 4294967296
	}
	n := int64(m)
	if n >= 2147483648 {
		n -= 4294967296
	}
	return int(n)
}

// elementReflectedBool 读 HTML 布尔属性（存在即 true）。
func elementReflectedBool(el *dom.Element, attr string) bool {
	return el.HasAttribute(attr)
}

// setElementReflectedBool 写 HTML 布尔属性（true → 空值属性，false → 移除）。
func setElementReflectedBool(el *dom.Element, attr string, v jsc.JSValue) {
	if v.ToBoolean() {
		el.SetAttribute(attr, "")
		return
	}
	el.RemoveAttribute(attr)
}

// elementEnumeratedBool 读「true/false 枚举 + 默认值」属性（HTML §6.7.4 的
// enumerated attribute 规则）："true"/"" → true，"false" → false，其它
// （含缺失）→ def；invalid value default = def。
func elementEnumeratedBool(el *dom.Element, attr string, def bool) bool {
	if !el.HasAttribute(attr) {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(el.GetAttribute(attr))) {
	case "true", "":
		return true
	case "false":
		return false
	}
	return def
}

// ── tabIndex（HTML §6.6.3）───────────────────────────────

// renderedLineBreak 是渲染文本流里的「硬换行」哨兵：只有 <br> 与块级盒子
// 边界会写入它。文本节点里的换行/制表符属于**可折叠空白**（CSS
// white-space:normal 把它们当空格处理），两者绝不能混为一谈 —— 混淆会让
// `textContent = "a\nb"` 这类文本被 innerText 报成两行（Edge 实测是 "a b"）。
const renderedLineBreak = '\x00'

// defaultTabIndex 返回**无 tabindex 内容属性**时的默认值：可交互内容 /
// focusable area 为 0，其余为 -1。集合与 Edge 逐项对齐（evidence：
// element_attrs.html 的 tabIndex.default.* 行）。
func defaultTabIndex(el *dom.Element) int {
	switch el.LocalName() {
	case "input", "button", "select", "textarea", "iframe", "object", "embed":
		return 0
	case "a", "area":
		// ★ Edge 实测：<a> 无论有没有 href 都是 0（夹具 tabIndex.aNoHref=0 与
		//   tabIndex.aHref=0 两侧一致）—— 不按「a[href] 才可聚焦」收窄。
		return 0
	case "audio", "video":
		if el.HasAttribute("controls") {
			return 0
		}
	case "summary":
		// ★ 只有 details 的**第一个 summary 子元素**才是 focusable area
		//   （Edge 实测：detached 的 summary → -1）。
		if p, ok := el.ParentNode().(*dom.Element); ok && p != nil && p.LocalName() == "details" {
			for c := p.FirstChild(); !isNilNode(c); c = c.NextSibling() {
				e, ok := c.(*dom.Element)
				if !ok || e == nil {
					continue
				}
				if e.LocalName() == "summary" {
					if e == el {
						return 0
					}
					break
				}
			}
		}
	}
	return -1
}

// elementTabIndex 返回 tabIndex 的当前值：有 tabindex 属性且可解析 →
// 解析值；属性缺失或解析失败 → defaultTabIndex。
func elementTabIndex(el *dom.Element) int {
	if el.HasAttribute("tabindex") {
		if n, ok := parseHTMLInteger(el.GetAttribute("tabindex")); ok {
			return n
		}
	}
	return defaultTabIndex(el)
}

// ── draggable（HTML §6.7.4）──────────────────────────────

// elementDraggable 返回 draggable 的当前值：枚举 true/false 优先；
// 属性缺失或非法值 → auto（<img> 与带 href 的 <a> 为 true，其余 false）。
func elementDraggable(el *dom.Element) bool {
	if el.HasAttribute("draggable") {
		switch strings.ToLower(strings.TrimSpace(el.GetAttribute("draggable"))) {
		case "true":
			return true
		case "false":
			return false
		}
	}
	switch el.LocalName() {
	case "img":
		return true
	case "a":
		return el.HasAttribute("href")
	}
	return false
}

// ── innerText / outerText（HTML §3.2.7）──────────────────
//
// getter 返回「渲染文本」：
//   - 元素未被渲染（未连接文档，或自身/祖先 display:none）→ 规范要求
//     回退为 textContent（一次字符都不折叠）；
//   - 已渲染：display:none 子树整体跳过、visibility:hidden 跳过、<br> 与块级
//     盒子边界产生换行、行内空白折叠为单空格、行首尾空白裁剪。
//
// setter 用「rendered text fragment」替换内容：按 \n 拆分、以 <br> 连接
// （`el.innerText = "a\nb"` ⇒ innerHTML `a<br>b`，与 Edge 一致）。

// elementInnerText 返回元素的渲染文本。
func elementInnerText(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if !elementIsBeingRendered(el) {
		return el.TextContent()
	}
	var b strings.Builder
	collectRenderedText(dom.Node(el), &b, 0)
	return collapseRenderedText(b.String())
}

// elementIsBeingRendered 报告元素是否处于渲染状态（未连接或 display:none
// 子树内 → 规范要求 innerText 回退 textContent）。
func elementIsBeingRendered(el *dom.Element) bool {
	if el == nil || !el.IsConnected() {
		return false
	}
	for n := dom.Node(el); !isNilNode(n); n = n.ParentNode() {
		e, ok := n.(*dom.Element)
		if !ok || e == nil {
			continue
		}
		if elemComputedProp(e, "display") == "none" {
			return false
		}
		if tag := e.LocalName(); tag == "body" || tag == "html" {
			break
		}
	}
	return true
}

// collectRenderedText 按渲染顺序收集 n 子树的文本（块级边界与 <br> 产生
// 换行；display:none / visibility:hidden 的子树不产出字符）。
func collectRenderedText(n dom.Node, b *strings.Builder, depth int) {
	if isNilNode(n) || depth > 64 {
		return
	}
	for c := n.FirstChild(); !isNilNode(c); c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Text:
			if v != nil {
				b.WriteString(v.TextContent())
			}
		case *dom.Element:
			if v == nil {
				continue
			}
			if elemComputedProp(v, "display") == "none" {
				continue
			}
			if vis := elemComputedProp(v, "visibility"); vis == "hidden" || vis == "collapse" {
				continue
			}
			if v.LocalName() == "br" {
				b.WriteByte(renderedLineBreak)
				continue
			}
			block := isBlockLevelDisplay(elemComputedProp(v, "display"))
			if block {
				b.WriteByte(renderedLineBreak)
			}
			collectRenderedText(v, b, depth+1)
			if block {
				b.WriteByte(renderedLineBreak)
			}
		}
	}
}

// collapseRenderedText 做 CSS 空白处理（white-space:normal 的语义）：
//   - 文本节点里的空白（空格/制表/换行）→ 折叠为一个空格；
//   - renderedLineBreak（<br> 与块级边界）→ 保留为一个换行，连续多个合并；
//     行首、换行之后的前导空白与整串首尾的换行一律丢弃。
func collapseRenderedText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pendingSpace := false
	last := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == renderedLineBreak {
			pendingSpace = false
			if last != 0 && last != '\n' {
				b.WriteByte('\n')
				last = '\n'
			}
			continue
		}
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			pendingSpace = true
			continue
		}
		if pendingSpace && last != 0 && last != '\n' {
			b.WriteByte(' ')
			last = ' '
		}
		pendingSpace = false
		b.WriteByte(c)
		last = c
	}
	return strings.TrimRight(b.String(), "\n ")
}

// isBlockLevelDisplay 报告某个 display 值是否产生块级边界（innerText 的
// 换行来源）。table-* 系列在浏览器里用制表符/换行混合表达，这里保守地
// 只把真正的块级/流式容器算作换行边界。
func isBlockLevelDisplay(d string) bool {
	switch d {
	case "block", "flow-root", "flex", "grid", "list-item":
		return true
	}
	return false
}

// setElementInnerText 用渲染文本片段替换元素内容（见文件内说明）。
func setElementInnerText(el *dom.Element, s string) {
	doc := el.OwnerDocument()
	if doc == nil {
		return
	}
	for c := el.FirstChild(); !isNilNode(c); c = el.FirstChild() {
		_ = el.RemoveChild(c)
	}
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			_ = el.AppendChild(doc.CreateElement("br"))
		}
		if line != "" {
			_ = el.AppendChild(dom.NewText(doc, line))
		}
	}
	if OnNodeInserted != nil && el.IsConnected() {
		OnNodeInserted(el)
	}
}

// setElementOuterText 用渲染文本片段替换元素**自身**（父节点里逐个插入
// 文本/<br>，再移除元素）。
func setElementOuterText(el *dom.Element, s string) {
	parent := el.ParentNode()
	doc := el.OwnerDocument()
	if isNilNode(parent) || doc == nil {
		return
	}
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			_ = parent.InsertBefore(doc.CreateElement("br"), el)
		}
		if line != "" {
			_ = parent.InsertBefore(dom.NewText(doc, line), el)
		}
	}
	_ = parent.RemoveChild(el)
	if OnNodeInserted != nil {
		OnNodeInserted(parent)
	}
}

// elemComputedProp 读元素的一个计算样式属性（display/visibility/position
// 等）：级联只收录**声明过**的属性 → 未声明时按 UA 语义回退（display 走
// withDisplayFallback 的单一真相源表）。★ 复制一份 map 再回退，避免把 UA
// 默认值写进 computedStyleFor 的缓存 map。
func elemComputedProp(el *dom.Element, prop string) string {
	if el == nil {
		return ""
	}
	cs := computedStyleFor(el)
	if v := strings.ToLower(strings.TrimSpace(cs[prop])); v != "" {
		return v
	}
	if prop == "display" {
		cp := make(map[string]string, len(cs)+1)
		for k, v := range cs {
			cp[k] = v
		}
		return strings.ToLower(strings.TrimSpace(withDisplayFallback(cp, el)["display"]))
	}
	return ""
}

// ── 批 A 的属性装配 ──────────────────────────────────────

// installElementAttrProperty 物化批 A 的 15 个 Element 属性；非本批属性返回
// ok=false，让 installElementProperty 继续走自己的大 switch。
func installElementAttrProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	if el == nil {
		return jsc.JSValue{}, nil, false
	}
	switch key {
	case "localName":
		// DOM §4.3.1：只读，只读一次即可（这里返回 data 值 → 包装器缓存，
		// 与浏览器「localName 不随 DOM 操作变化」一致）。HTML 元素为小写。
		return jsc.StringValue(el.LocalName()), nil, true

	case "namespaceURI":
		// DOM §4.3.1：只读，判定见文件头（createElementNS 记录 → svg/math
		// 祖先 → HTML）。
		return jsc.StringValue(elementNamespaceURI(el)), nil, true

	case "prefix":
		// DOM §4.3.1：本引擎不建模限定名前缀（createElementNS 的 prefix 参数
		// 被忽略）→ 对任何元素都是 null，与 Edge 上无前缀元素的实测一致。
		return jsc.Null(), nil, true

	case "tabIndex":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(float64(elementTabIndex(el))) },
			set: func(v jsc.JSValue) { el.SetAttribute("tabindex", strconv.Itoa(idlLong(v))) }}, true

	case "innerText":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(elementInnerText(el)) },
			set: func(v jsc.JSValue) { setElementInnerText(el, v.ToString()) }}, true

	case "outerText":
		// HTML §3.2.7：outerText 的 getter 与 innerText 同值；setter 替换元素
		// 自身（渲染文本片段）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(elementInnerText(el)) },
			set: func(v jsc.JSValue) { setElementOuterText(el, v.ToString()) }}, true

	case "lang":
		// HTML §3.2.6.2：反射 lang 内容属性（无属性 → ""；不做祖先继承——
		// Edge 实测 <div lang="en"><span></span></div> 时 span.lang === ""）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("lang")) },
			set: func(v jsc.JSValue) { el.SetAttribute("lang", v.ToString()) }}, true

	case "dir":
		// HTML §3.2.6.1：反射 dir 内容属性（无属性 → ""，不返回 UA 默认值）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("dir")) },
			set: func(v jsc.JSValue) { el.SetAttribute("dir", v.ToString()) }}, true

	case "draggable":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(elementDraggable(el)) },
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("draggable", "true")
					return
				}
				el.SetAttribute("draggable", "false")
			}}, true

	case "spellcheck":
		// HTML §6.7.4：枚举 true/false，missing value default = true。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.BooleanValue(elementEnumeratedBool(el, "spellcheck", true))
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("spellcheck", "true")
					return
				}
				el.SetAttribute("spellcheck", "false")
			}}, true

	case "translate":
		// HTML §3.2.6.3：枚举 yes/no，missing value default = true
		//（invalid value default 同样是 yes）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if el.HasAttribute("translate") {
					return jsc.BooleanValue(strings.ToLower(strings.TrimSpace(el.GetAttribute("translate"))) != "no")
				}
				return jsc.BooleanValue(true)
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("translate", "yes")
					return
				}
				el.SetAttribute("translate", "no")
			}}, true

	case "accessKey":
		// HTML §6.6.3：反射 accesskey 内容属性（无 → ""）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("accesskey")) },
			set: func(v jsc.JSValue) { el.SetAttribute("accesskey", v.ToString()) }}, true

	case "nonce":
		// HTML §3.2.6.4：nonce 反射 nonce 内容属性（无 → ""）。CSP 相关行为
		// （有 CSP 时 getter 恒为 ""）不在本轮范围。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("nonce")) },
			set: func(v jsc.JSValue) { el.SetAttribute("nonce", v.ToString()) }}, true

	case "inert":
		// HTML §6.7.4：布尔属性反射（无 → false）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(elementReflectedBool(el, "inert")) },
			set: func(v jsc.JSValue) { setElementReflectedBool(el, "inert", v) }}, true

	case "autofocus":
		// HTML §4.12.5：布尔属性反射（无 → false）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(elementReflectedBool(el, "autofocus")) },
			set: func(v jsc.JSValue) { setElementReflectedBool(el, "autofocus", v) }}, true

	// ── 批 B：几何 / Shadow 相关（8 项）──────────────────

	case "part":
		// DOM §4.5：part 是 DOMTokenList（cached attribute value —— 同一元素
		// 每次读取返回**同一实例**）。getter-only：赋值静默忽略。
		// ★ 不登记进 elemAccessorProps：这里正是要靠适配器层的缓存保证同一性；
		//   值本身仍是活的（对象内部每次实时读 part 属性）。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.ObjectValue(makePartTokenList(rt, el))
		}}, true

	case "slot":
		// DOM §4.8.5：slot 反射 slot 内容属性（无属性 → ""；无 shadow 分配时
		// 同样是 ""，不是 null）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("slot")) },
			set: func(v jsc.JSValue) { el.SetAttribute("slot", v.ToString()) }}, true

	case "assignedSlot":
		// DOM §4.8.6：无 shadow 分配（不是 shadow host 的 light-DOM 子节点，
		// 或没有匹配的 slot）→ null。**属性必须存在且值为 null，不得以
		// 「不适用」为由跳过**。live：分配结果随 slot 属性与 shadow 树变化。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node {
				if s := el.AssignedSlot(); s != nil {
					return s
				}
				return nil
			})(rt, jsc.JSValue{})
		}}, true

	case "contentEditable":
		// HTML §6.7.4：枚举 true/false/plaintext-only/inherit —— invalid 与
		// missing 都落到 inherit（getter 返回 "inherit"，不是 ""）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(elementContentEditable(el)) },
			set: func(v jsc.JSValue) { setElementContentEditable(el, v) }}, true

	case "isContentEditable":
		// HTML §6.7.4：由元素自身或最近的可编辑祖先（contenteditable 非
		// false）决定；都没有 → false。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(elementIsContentEditable(el)) }}, true

	case "offsetParent":
		// CSSOM View §7.2：display:none / position:fixed / body / 未连接文档
		// → null；否则最近的定位祖先，找不到则 body（td/th/table 用最近的
		// table 系列祖先）。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return nodeAccFn(rt, func() dom.Node {
				if op := offsetParentOf(el); op != nil {
					return op
				}
				return nil
			})(rt, jsc.JSValue{})
		}}, true

	case "clientTop":
		// CSSOM View §7.1：上边框宽度（取整）；无 layout box（未连接）→ 0。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.NumberValue(borderWidthPxForSide(el, "top"))
		}}, true

	case "clientLeft":
		// CSSOM View §7.1：左边框宽度（取整）；无 layout box（未连接）→ 0。
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.NumberValue(borderWidthPxForSide(el, "left"))
		}}, true
	}
	return jsc.JSValue{}, nil, false
}

// ── 批 B 的辅助实现 ──────────────────────────────────────

// offsetParentOf 实现 Element.offsetParent（CSSOM View §7.2）。
func offsetParentOf(el *dom.Element) *dom.Element {
	if el == nil {
		return nil
	}
	// ① 未渲染（display:none）→ null
	if elemComputedProp(el, "display") == "none" {
		return nil
	}
	// ② position:fixed → null
	if elemComputedProp(el, "position") == "fixed" {
		return nil
	}
	// ③ body → null
	if el.LocalName() == "body" {
		return nil
	}
	// ④ 未连接文档（没有 layout box）→ null
	if !el.IsConnected() {
		return nil
	}
	var body *dom.Element
	if doc := el.OwnerDocument(); doc != nil {
		body = doc.Body()
	}
	// ⑤ td/th/table：offsetParent 是最近的 table/td/th 祖先，找不到则 body
	switch el.LocalName() {
	case "td", "th", "table":
		for a := el.ParentElement(); a != nil; a = a.ParentElement() {
			switch a.LocalName() {
			case "table", "td", "th":
				return a
			}
		}
		return body
	}
	// ⑥ 最近的定位祖先（position != static）；到 body/html 为止
	for a := el.ParentElement(); a != nil; a = a.ParentElement() {
		if p := elemComputedProp(a, "position"); p != "" && p != "static" {
			return a
		}
		if tag := a.LocalName(); tag == "body" || tag == "html" {
			break
		}
	}
	return body
}

// borderWidthPxForSide 返回元素某一边的边框宽度（整数 CSS px）——
// clientTop/clientLeft 的取值来源。级联 map 可能只有 border 简写或
// border-width（1~4 值）→ 这里做回退解析（与 getComputedStyle 的
// borderWidthLonghandProps 展开同一口径）。
func borderWidthPxForSide(el *dom.Element, side string) float64 {
	if el == nil {
		return 0
	}
	cs := computedStyleFor(el)
	// ★ CSS 语义：border-style 为 none/hidden 时该边的**计算宽度恒为 0**，
	//   即使 border-width 声明了非零值（实测 Edge：`#tw{border-width:4px}`
	//   无 border-style → 计算 border-width = 0px、clientTop = 0）。
	if s := borderStyleForSide(cs, side); s == "" || s == "none" || s == "hidden" {
		return 0
	}
	// ★ 级联 map 会保留**简写**键（`border-top:1px solid #000` 存的是
	//   border-top，而不是 border-top-width），未声明的 longhand 走兜底时
	//   还可能读到 "0px"（实测 getPropertyValue('border-top-width') = 0px 而
	//   border-top = 1px solid #000）→ 因此取**首个非零**候选，而不是首个
	//   存在的候选，否则单边简写一律读成 0。
	for _, v := range []string{
		cs["border-"+side+"-width"],
		cssShorthandEdge(cs["border-width"], side),
		cssShorthandEdge(cs["border-"+side], side),
		cssShorthandEdge(cs["border"], side),
	} {
		if v == "" {
			continue
		}
		if px := parseCSSLengthPx(v); px > 0 {
			return math.Round(px)
		}
	}
	return 0
}

// borderStyleForSide 返回某一边的 border-style 计算值（回退链：
// border-<side>-style → border-style（1~4 值）→ border-<side> / border 简写
// 里的样式关键字 → 初始值 "none"）。
func borderStyleForSide(cs map[string]string, side string) string {
	if s := cssBorderStyleToken(cs["border-"+side+"-style"]); s != "" {
		return s
	}
	if s := cssShorthandStyleEdge(cs["border-style"], side); s != "" {
		return s
	}
	if s := cssBorderStyleToken(cs["border-"+side]); s != "" {
		return s
	}
	if s := cssBorderStyleToken(cs["border"]); s != "" {
		return s
	}
	return "none"
}

// cssShorthandStyleEdge 从 1~4 值的 border-style 简写里取出指定边的关键字。
func cssShorthandStyleEdge(v, side string) string {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return ""
	}
	pick := func(i int) string { return strings.ToLower(parts[i]) }
	switch len(parts) {
	case 1:
		return pick(0)
	case 2:
		if side == "top" || side == "bottom" {
			return pick(0)
		}
		return pick(1)
	case 3:
		switch side {
		case "top":
			return pick(0)
		case "bottom":
			return pick(2)
		}
		return pick(1)
	}
	switch side {
	case "top":
		return pick(0)
	case "right":
		return pick(1)
	case "bottom":
		return pick(2)
	}
	return pick(3)
}

// cssShorthandEdge 从 CSS 盒简写（border / border-width，1~4 个长度值）里
// 取出指定边的分量；非长度的 token（solid / #333 等）被跳过。
func cssShorthandEdge(v, side string) string {
	var lens []string
	for _, f := range strings.Fields(v) {
		if isCSSLengthToken(f) {
			lens = append(lens, f)
		}
	}
	switch len(lens) {
	case 0:
		return ""
	case 1:
		return lens[0]
	case 2:
		if side == "top" || side == "bottom" {
			return lens[0]
		}
		return lens[1]
	case 3:
		switch side {
		case "top":
			return lens[0]
		case "bottom":
			return lens[2]
		}
		return lens[1]
	}
	switch side {
	case "top":
		return lens[0]
	case "right":
		return lens[1]
	case "bottom":
		return lens[2]
	}
	return lens[3]
}

// isCSSLengthToken 报告 token 是否以数值开头（1px / 0.5em / -2px / 0）。
func isCSSLengthToken(f string) bool {
	if f == "" {
		return false
	}
	switch c := f[0]; {
	case c >= '0' && c <= '9', c == '.', c == '-', c == '+':
		return true
	}
	return false
}

// parseCSSLengthPx 把 CSS 长度解析成 px 数（em/rem/% 等相对单位不在本轮
// 范围 → 返回 0，与「未声明边框」同值）。
func parseCSSLengthPx(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "px") {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s[:len(s)-2]), 64); err == nil {
			return f
		}
		return 0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return 0
}

// elementContentEditable 返回 contentEditable 的当前值（HTML §6.7.4）。
func elementContentEditable(el *dom.Element) string {
	if !el.HasAttribute("contenteditable") {
		return "inherit"
	}
	switch strings.ToLower(strings.TrimSpace(el.GetAttribute("contenteditable"))) {
	case "true", "":
		return "true"
	case "false":
		return "false"
	case "plaintext-only":
		return "plaintext-only"
	}
	return "inherit"
}

// setElementContentEditable 实现 contentEditable 的 setter：true/false/
// plaintext-only 写属性；"inherit" 与任何非法值 → 移除属性（getter 回到
// "inherit"，与 Edge 一致）。
func setElementContentEditable(el *dom.Element, v jsc.JSValue) {
	switch strings.ToLower(strings.TrimSpace(v.ToString())) {
	case "true":
		el.SetAttribute("contenteditable", "true")
	case "false":
		el.SetAttribute("contenteditable", "false")
	case "plaintext-only":
		el.SetAttribute("contenteditable", "plaintext-only")
	default:
		el.RemoveAttribute("contenteditable")
	}
	InvalidateComputedStyle(el)
}

// elementIsContentEditable 报告元素是否处于可编辑区域（HTML §6.7.4）：
// 从自身向上找最近带 contenteditable 属性的元素 —— "false" 停止为 false，
// 其余（true / "" / plaintext-only）为 true；到根都没有 → false。
func elementIsContentEditable(el *dom.Element) bool {
	for n := dom.Node(el); !isNilNode(n); n = n.ParentNode() {
		e, ok := n.(*dom.Element)
		if !ok || e == nil || !e.HasAttribute("contenteditable") {
			continue
		}
		if strings.ToLower(strings.TrimSpace(e.GetAttribute("contenteditable"))) == "false" {
			return false
		}
		return true
	}
	return false
}

// makePartTokenList 构造 element.part 的 DOMTokenList（DOM §4.5，作用于
// part 内容属性）。语义与 classList 同族：length/value/item/contains/
// add/remove/toggle 全部实时读属性。
func makePartTokenList(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	tokens := func() []string { return strings.Fields(el.GetAttribute("part")) }
	write := func(list []string) {
		el.SetAttribute("part", strings.Join(list, " "))
		InvalidateComputedStyle(el)
	}
	obj.Set("add", jsc.FunctionValue(jsc.NewNativeFunction("add",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Undefined()
			}
			cur := tokens()
			for _, a := range args {
				if t := a.ToString(); !containsToken(cur, t) {
					cur = append(cur, t)
				}
			}
			write(cur)
			return jsc.Undefined()
		}, 1)))
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Undefined()
			}
			drop := map[string]bool{}
			for _, a := range args {
				drop[a.ToString()] = true
			}
			var keep []string
			for _, t := range tokens() {
				if !drop[t] {
					keep = append(keep, t)
				}
			}
			write(keep)
			return jsc.Undefined()
		}, 1)))
	obj.Set("toggle", jsc.FunctionValue(jsc.NewNativeFunction("toggle",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			name := args[0].ToString()
			cur := tokens()
			for i, t := range cur {
				if t == name {
					if len(args) >= 2 && args[1].ToBoolean() {
						return jsc.BooleanValue(true)
					}
					write(append(append([]string{}, cur[:i]...), cur[i+1:]...))
					return jsc.BooleanValue(false)
				}
			}
			if len(args) >= 2 && !args[1].ToBoolean() {
				return jsc.BooleanValue(false)
			}
			write(append(cur, name))
			return jsc.BooleanValue(true)
		}, 2)))
	obj.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(containsToken(tokens(), args[0].ToString()))
		}, 1)))
	obj.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Null()
			}
			idx := int(args[0].ToNumber())
			cur := tokens()
			if idx < 0 || idx >= len(cur) {
				return jsc.Null()
			}
			return jsc.StringValue(cur[idx])
		}, 1)))
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(tokens())))
	}), nil)
	obj.SetAccessor("value",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(el.GetAttribute("part"))
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			el.SetAttribute("part", v.ToString())
			InvalidateComputedStyle(el)
		})
	return obj
}

// containsToken 报告 list 是否含 token（DOMTokenList 的精确匹配语义）。
func containsToken(list []string, token string) bool {
	for _, t := range list {
		if t == token {
			return true
		}
	}
	return false
}
