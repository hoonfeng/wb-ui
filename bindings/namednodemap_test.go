package bindings

import (
	"testing"

	"wb-ui/dom"
)

// TestDocumentCurrentScript 覆盖 document.currentScript 的语义：脚本执行期间
// 指向该 <script> 元素，其余时刻为 null（HTML §4.12.1）。page.Frame 在每次
// 脚本执行前后设置/恢复 bindings.CurrentScriptElement。
func TestDocumentCurrentScript(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	scriptEl := doc.CreateElement("script")
	doc.AppendChild(scriptEl)

	prev := CurrentScriptElement
	defer func() { CurrentScriptElement = prev }()

	CurrentScriptElement = nil
	mustRun(t, rt, `
		if (document.currentScript !== null) {
			throw new Error("currentScript outside script execution = " + document.currentScript);
		}
	`)

	CurrentScriptElement = scriptEl
	mustRun(t, rt, `
		var s = document.currentScript;
		if (!s || s !== document.getElementsByTagName("script")[0]) {
			throw new Error("currentScript should be the executing script element");
		}
	`)
}

// TestNamedNodeMapIsLiveCollection 覆盖 el.attributes 的 NamedNodeMap 契约：
// 同一元素返回同一实例，length 与数字索引随属性增删变化，removeAttributeNode
// 逐项移除并返回该 Attr。React 19 用
// `while (map.length) el.removeAttributeNode(map[0])` 清空属性——循环条件与取出
// 的条目都必须反映每次删除后的状态。
func TestNamedNodeMapIsLiveCollection(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("probe")
	el.SetAttribute("id", "probe")
	el.SetAttribute("class", "card")
	el.SetAttribute("data-state", "server")
	doc.AppendChild(el)

	mustRun(t, rt, `
		var el = document.getElementById("probe");
		var map = el.attributes;
		if (!(map instanceof NamedNodeMap)) throw new Error("attributes is not a NamedNodeMap");
		if (el.attributes !== map) throw new Error("el.attributes must be a single instance");
		if (map.length !== 3) throw new Error("initial length = " + map.length);
		if (map[0].name !== "id" || map[0].value !== "probe") throw new Error("entry 0 = " + map[0].name + "=" + map[0].value);
		if (!el.hasAttributes()) throw new Error("hasAttributes() should be true");

		var removed = 0;
		while (map.length && removed < 10) {
			el.removeAttributeNode(map[0]);
			removed++;
		}
		if (removed !== 3) throw new Error("removed " + removed + " attributes, want 3");
		if (map.length !== 0) throw new Error("length after removal = " + map.length);
		if (map[0] !== undefined) throw new Error("stale entry 0 survived removal");
		if (el.hasAttributes()) throw new Error("hasAttributes() should be false once empty");
	`)

	// 属性写入同样反映到已持有的集合对象上（live 语义）。
	// 上一段把 id 也移除了（清空循环删光了属性），故此处按标签取元素。
	mustRun(t, rt, `
		var el = document.getElementsByTagName("div")[0];
		var map = el.attributes;
		el.setAttribute("data-x", "1");
		if (map.length !== 1) throw new Error("live length after setAttribute = " + map.length);
		if (map[0].name !== "data-x" || map[0].value !== "1") throw new Error("live entry after setAttribute");
		el.removeAttribute("data-x");
		if (map.length !== 0) throw new Error("live length after removeAttribute = " + map.length);
	`)
}

// TestElementScrollToAndScrollBy 覆盖 Element.prototype.scrollTo/scrollBy：既支持
// 对象参数（{left, top, behavior}，现代框架 ref 回调用法），也支持位置参数；
// scrollBy 相对当前位置累加。滚动读写经 Get/SetElementScrollOffset 注入点
// （webkit 在真实环境接入渲染树）。
func TestElementScrollToAndScrollBy(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	el := doc.CreateElement("div")
	el.SetId("scroller")
	doc.AppendChild(el)

	prevGet, prevSet := GetElementScrollOffset, SetElementScrollOffset
	defer func() { GetElementScrollOffset, SetElementScrollOffset = prevGet, prevSet }()
	var sx, sy float64
	GetElementScrollOffset = func(*dom.Element) (float64, float64) { return sx, sy }
	SetElementScrollOffset = func(_ *dom.Element, x, y float64) { sx, sy = x, y }

	mustRun(t, rt, `
		var el = document.getElementById("scroller");
		if (typeof el.scrollTo !== "function") throw new Error("scrollTo is not a function");
		if (typeof el.scrollBy !== "function") throw new Error("scrollBy is not a function");

		el.scrollTo({left: 12, top: 20, behavior: "auto"});
		if (el.scrollLeft !== 12 || el.scrollTop !== 20) {
			throw new Error("scrollTo(object): " + el.scrollLeft + "," + el.scrollTop);
		}
		el.scrollBy(3, -5);
		if (el.scrollLeft !== 15 || el.scrollTop !== 15) {
			throw new Error("scrollBy(3,-5): " + el.scrollLeft + "," + el.scrollTop);
		}
		el.scrollTo(1, 2);
		if (el.scrollLeft !== 1 || el.scrollTop !== 2) {
			throw new Error("scrollTo(x,y): " + el.scrollLeft + "," + el.scrollTop);
		}
	`)
}
