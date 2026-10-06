package bindings

// DOM 标准 Element 级遍历 API 的绑定测试：
//
//	Element.prototype.firstElementChild / lastElementChild
//	Element.prototype.nextElementSibling / previousElementSibling
//
// 语义要点（DOM §4.4 ParentNode / NonDocumentTypeChildNode）：
//   - 只认 Element：Text（含**纯空白**文本节点）与 Comment 一律跳过；
//   - 无匹配则返回 null（不是 undefined，且属性必须存在——'in' 为 true）；
//   - 必须是 **live** 属性（每次读取重新遍历，不缓存），否则
//     `while (el.firstElementChild)` 类搬运循环（Vue insertStaticContent 同款）
//     会永远看到同一个节点 → 死循环。
//
// 同时覆盖 DocumentFragment / ShadowRoot 的同族 API（同一标准族）。

import (
	"testing"

	"wb-ui/engine/dom"
)

// runElementProbe 在独立解释器里执行一段探测 JS，结果必须是 "PASS"。
func runElementProbe(t *testing.T, src string) {
	t.Helper()
	rt, _, _ := newRuntimeWithDoc(t)
	v, err := rt.RunJS(src)
	if err != nil {
		t.Fatalf("RunJS error: %v", err)
	}
	if got := v.ToString(); got != "PASS" {
		t.Fatalf("probe failed: %s", got)
	}
}

// 空元素：4 个 accessor 全为 null；属性必须存在（'in' 为 true）。
func TestElementTraversalEmpty(t *testing.T) {
	runElementProbe(t, `(function(){
		var el = document.createElement('div');
		var names = ['firstElementChild','lastElementChild','nextElementSibling','previousElementSibling'];
		for (var i = 0; i < names.length; i++) {
			if (!(names[i] in el)) return names[i] + ' missing (in=false)';
			if (el[names[i]] !== null) return names[i] + '=' + el[names[i]];
		}
		if (el.childElementCount !== 0) return 'childElementCount=' + el.childElementCount;
		if (el.children.length !== 0) return 'children=' + el.children.length;
		return 'PASS';
	})()`)
}

// 仅注释子节点：不是 Element ⇒ first/lastElementChild 均 null。
func TestElementTraversalCommentOnly(t *testing.T) {
	runElementProbe(t, `(function(){
		var el = document.createElement('div');
		el.appendChild(document.createComment('only comment'));
		if (el.firstElementChild !== null) return 'firstElementChild=' + el.firstElementChild.nodeName;
		if (el.lastElementChild !== null) return 'lastElementChild=' + el.lastElementChild.nodeName;
		if (el.childNodes.length !== 1) return 'childNodes=' + el.childNodes.length;
		if (el.children.length !== 0) return 'children=' + el.children.length;
		return 'PASS';
	})()`)
}

// 混合子节点序列：首尾为空白文本/注释时跳过，4 个 accessor 各自正确，
// 且与 children/childElementCount 口径一致。
func TestElementTraversalMixedChildren(t *testing.T) {
	runElementProbe(t, `(function(){
		function mk(tag, id) { var e = document.createElement(tag); e.id = id; return e; }
		function id(n) { return n === null ? 'null' : (n.id || n.nodeName); }
		var root = document.createElement('div');
		var ws = document.createTextNode('   \n\t  ');   // 纯空白文本
		var a = mk('div', 'a');
		var b = mk('span', 'b');
		root.appendChild(ws);
		root.appendChild(document.createComment('c0'));
		root.appendChild(a);
		root.appendChild(document.createTextNode('x'));  // 非空文本
		root.appendChild(b);
		root.appendChild(document.createComment('c2'));

		// ① 第一个 Element：跳过纯空白文本 + 注释
		if (root.firstElementChild !== a) return 'firstElementChild=' + id(root.firstElementChild);
		// ② 最后一个 Element：跳过尾注释
		if (root.lastElementChild !== b) return 'lastElementChild=' + id(root.lastElementChild);
		// ③ a 之后跳过文本 x 找到 b
		if (a.nextElementSibling !== b) return 'a.nextElementSibling=' + id(a.nextElementSibling);
		// ④ b 之前跳过文本 x 找到 a
		if (b.previousElementSibling !== a) return 'b.previousElementSibling=' + id(b.previousElementSibling);
		// ⑤ 边界：a 之前只剩空白文本+注释、b 之后只剩注释 ⇒ null
		if (a.previousElementSibling !== null) return 'a.previousElementSibling=' + id(a.previousElementSibling);
		if (b.nextElementSibling !== null) return 'b.nextElementSibling=' + id(b.nextElementSibling);
		// ⑥ children / childElementCount 同口径
		if (root.children.length !== 2 || root.children[0] !== a || root.children[1] !== b) return 'children mismatch';
		if (root.childElementCount !== 2) return 'childElementCount=' + root.childElementCount;
		return 'PASS';
	})()`)
}

// nextSibling 指向 Text 时 nextElementSibling 必须继续往后找（不因相邻兄弟
// 是 Text 就返回 null）；previousElementSibling 向前同理。
func TestElementSiblingSkipsAdjacentText(t *testing.T) {
	runElementProbe(t, `(function(){
		function mk(id) { var e = document.createElement('div'); e.id = id; return e; }
		var root = document.createElement('div');
		var a = mk('a'), b = mk('b'), c = mk('c');
		root.appendChild(a);
		root.appendChild(document.createTextNode('  '));      // 纯空白
		root.appendChild(document.createTextNode('between')); // 非空
		root.appendChild(b);
		root.appendChild(document.createTextNode('after'));
		root.appendChild(c);

		// 相邻兄弟确实是 Text（确认走的是穿透路径，而不是恰好相邻）
		if (a.nextSibling === null || a.nextSibling.nodeType !== 3) return 'a.nextSibling not Text';
		if (c.previousSibling === null || c.previousSibling.nodeType !== 3) return 'c.previousSibling not Text';
		if (a.nextElementSibling !== b) return 'a.nextElementSibling wrong';
		if (b.previousElementSibling !== a) return 'b.previousElementSibling wrong';
		if (c.previousElementSibling !== b) return 'c.previousElementSibling wrong';
		if (b.nextElementSibling !== c) return 'b.nextElementSibling wrong';
		if (c.nextElementSibling !== null) return 'c.nextElementSibling not null';
		if (a.previousElementSibling !== null) return 'a.previousElementSibling not null';
		return 'PASS';
	})()`)
}

// live（不缓存）：先读一次，再做插入/删除，结果必须立刻变化；
// 且 while (el.firstElementChild) 搬运循环必须有限步终止（Vue insertStaticContent）。
func TestElementTraversalIsLive(t *testing.T) {
	runElementProbe(t, `(function(){
		var root = document.createElement('div');
		var a = document.createElement('div');
		root.appendChild(a);
		root.firstElementChild;              // 先读一次：若被缓存，下面的断言会失败
		root.lastElementChild;
		var b = document.createElement('span');
		root.appendChild(b);
		if (root.lastElementChild !== b) return 'lastElementChild stale after append';
		if (root.firstElementChild !== a) return 'firstElementChild wrong after append';
		root.removeChild(a);
		if (root.firstElementChild !== b) return 'firstElementChild stale after removal';
		if (root.lastElementChild !== b) return 'lastElementChild wrong after removal';
		var n = 0;
		while (root.firstElementChild) {
			root.removeChild(root.firstElementChild);
			if (++n > 5) return 'while(firstElementChild) did not terminate';
		}
		if (n !== 1) return 'moved=' + n;
		if (root.firstElementChild !== null) return 'firstElementChild not null after drain';
		if (root.lastElementChild !== null) return 'lastElementChild not null after drain';
		return 'PASS';
	})()`)
}

// DocumentFragment / ShadowRoot 同族 API（DocumentFragment 缺 firstElementChild
// 会让 <template>.content 的搬运循环静默终止）。
func TestDocumentFragmentAndShadowRootElementTraversal(t *testing.T) {
	runElementProbe(t, `(function(){
		function mk(tag) { return document.createElement(tag); }
		var frag = document.createDocumentFragment();
		var f1 = mk('i'), f2 = mk('b');
		frag.appendChild(document.createTextNode('  '));
		frag.appendChild(f1);
		frag.appendChild(document.createComment('c'));
		frag.appendChild(f2);
		if (frag.firstElementChild !== f1) return 'frag.firstElementChild wrong';
		if (frag.lastElementChild !== f2) return 'frag.lastElementChild wrong';
		if (f1.nextElementSibling !== f2) return 'frag nextElementSibling wrong';
		if (f2.previousElementSibling !== f1) return 'frag previousElementSibling wrong';
		if (frag.children.length !== 2) return 'frag.children=' + frag.children.length;
		if (frag.childElementCount !== 2) return 'frag.childElementCount=' + frag.childElementCount;

		var host = mk('div');
		var sr = host.attachShadow({mode: 'open'});
		if (!sr) return 'attachShadow returned null';
		var s1 = mk('span');
		sr.appendChild(document.createComment('c'));
		sr.appendChild(s1);
		sr.appendChild(document.createTextNode('tail'));
		if (sr.firstElementChild !== s1) return 'sr.firstElementChild wrong';
		if (sr.lastElementChild !== s1) return 'sr.lastElementChild wrong';
		if (sr.children.length !== 1) return 'sr.children=' + sr.children.length;
		if (sr.childElementCount !== 1) return 'sr.childElementCount=' + sr.childElementCount;
		return 'PASS';
	})()`)
}

// ── Go 层直接断言（不经 JS 引擎）──

func assertElemChild(t *testing.T, label string, got dom.Node, want *dom.Element) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s = %v, want nil", label, got)
		}
		return
	}
	e, ok := got.(*dom.Element)
	if !ok || e != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

// TestElementTraversalHelpersGoLevel 直接验证遍历辅助：空容器、纯空白文本与
// 注释跳过、注释-only、兄弟穿透、nil 参数。
func TestElementTraversalHelpersGoLevel(t *testing.T) {
	doc := dom.NewDocument()

	empty := doc.CreateElement("div")
	assertElemChild(t, "empty.firstElementChild", firstElementChildOf(empty), nil)
	assertElemChild(t, "empty.lastElementChild", lastElementChildOf(empty), nil)
	assertElemChild(t, "empty.nextElementSibling", nextElementSiblingOf(empty), nil)
	assertElemChild(t, "empty.previousElementSibling", previousElementSiblingOf(empty), nil)
	if got := elementChildrenOf(empty); len(got) != 0 {
		t.Fatalf("empty elementChildrenOf len = %d, want 0", len(got))
	}

	// 仅注释
	commentOnly := doc.CreateElement("p")
	commentOnly.AppendChild(dom.NewComment(doc, "only"))
	assertElemChild(t, "commentOnly.firstElementChild", firstElementChildOf(commentOnly), nil)
	assertElemChild(t, "commentOnly.lastElementChild", lastElementChildOf(commentOnly), nil)

	// 混合序列：空白文本 / 注释 / 元素 / 文本 / 元素 / 注释
	root := doc.CreateElement("div")
	root.AppendChild(dom.NewText(doc, "  \n\t "))
	root.AppendChild(dom.NewComment(doc, "lead"))
	a := doc.CreateElement("a")
	root.AppendChild(a)
	root.AppendChild(dom.NewText(doc, "mid"))
	b := doc.CreateElement("b")
	root.AppendChild(b)
	root.AppendChild(dom.NewComment(doc, "tail"))

	assertElemChild(t, "root.firstElementChild", firstElementChildOf(root), a)
	assertElemChild(t, "root.lastElementChild", lastElementChildOf(root), b)
	assertElemChild(t, "a.nextElementSibling", nextElementSiblingOf(a), b)
	assertElemChild(t, "b.previousElementSibling", previousElementSiblingOf(b), a)
	assertElemChild(t, "a.previousElementSibling", previousElementSiblingOf(a), nil)
	assertElemChild(t, "b.nextElementSibling", nextElementSiblingOf(b), nil)
	kids := elementChildrenOf(root)
	if len(kids) != 2 || kids[0] != a || kids[1] != b {
		t.Fatalf("elementChildrenOf = %v, want [a b]", kids)
	}

	// typed-nil / 空接口参数（isNilNode 路径）
	var typedNil *dom.Element
	assertElemChild(t, "nil.firstElementChild", firstElementChildOf(typedNil), nil)
	assertElemChild(t, "nil.nextElementSibling", nextElementSiblingOf(typedNil), nil)
}
