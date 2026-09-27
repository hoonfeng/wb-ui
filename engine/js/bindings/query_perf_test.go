// 选择器查询 / 命中测试的基准与被测成本分解（性能回归）。
//
// 背景：docs/PERF_DISPATCH_BREAKDOWN.md §6 在**真实编辑器 DOM**（allNodes=1774、
// cmLines=94）上测出四个退化的只读 API：
//
//	querySelector    1e4 → ~2024ms  ≈ 200µs/次
//	querySelectorAll 1e3 → ~380ms   ≈ 380µs/次
//	elementFromPoint 1e3 → ~165ms   ≈ 165µs/次
//	getComputedStyle 1e3 → ~47ms    ≈  47µs/次
//
// 它们不在滚动主路径上（CM6 的 scroll handler 零调用），但 CM6 / Vue 其它路径
// 大量使用（例如 Vue 的 patch 会对每个元素查选择器、CM6 更新测量用 elementFromPoint），
// 相对浏览器有 10~100× 差距，故单独收口。
//
// 本文件的两类用途：
//  1. Benchmark*：把「每次调用的成本」钉在**分解口径**上——解析选择器 / 匹配单元素 /
//     全子树遍历，哪一段占大头，优化就必须落在哪一段（禁止凭印象猜）。
//  2. TestSelectorQuerySemantics*：无论怎么优化，语义不得漂移（首个匹配、文档序、
//     不匹配宿主自身、无效选择器抛/返回空、:is/:not 等复杂选择器一致）。
package bindings

import (
	"strconv"
	"strings"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
)

// buildEditorishDoc 造一棵形状贴近 CM6 编辑器 DOM 的树：外层 cm-editor > cm-scroller，
// 前半是 gutter（cm-gutterElement > span），后半是内容行（cm-line > span.tok）。
//
// 两个保真度要点（都影响测量结论，缺一会低估成本）：
//  1. `.cm-line` 的第一个实例**不在**开头（前面有 gutter 子树），因此 querySelector
//     必须真的走一段树，而不是第一个孩子就命中。
//  2. 每个元素带 3~4 个属性（class + style + dir/role/tabindex + data-*），并且每行
//     都含文本节点 —— 与真实 CM6 DOM 一致。只有 class 一个属性的 DOM 会让
//     attrs map 退化成 1 项（swiss map 的单元素快路径），从而**低估**
//     GetAttribute 的真实成本：真实编辑器 DOM 上 profile 显示属性读取链路占
//     querySelector 窗口 ~48% cum 样本。
func buildEditorishDoc(tb testing.TB, lines int) *dom.Document {
	tb.Helper()
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body><div class="cm-editor" style="position:relative" dir="ltr" data-testid="editor"><div class="cm-scroller" style="overflow:auto" tabindex="0"><div class="cm-gutters" style="left:0" aria-hidden="true">`)
	for i := 0; i < lines; i++ {
		b.WriteString(`<div class="cm-gutterElement" style="height:18px" data-line="`)
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(`"><span class="ln" role="presentation">`)
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(`</span></div>`)
	}
	b.WriteString(`</div><div class="cm-content" contenteditable="true" role="textbox" aria-multiline="true">`)
	for i := 0; i < lines; i++ {
		b.WriteString(`<div class="cm-line" style="padding:0 2px" dir="ltr" role="presentation">line `)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(` <span class="tok" data-kind="kw">text</span></div>`)
	}
	b.WriteString(`</div></div></div></body></html>`)
	doc, err := html.Parse(b.String())
	if err != nil {
		tb.Fatalf("html.Parse: %v", err)
	}
	return doc
}

// BenchmarkSelectorParse 只测「解析选择器字符串」这一段的成本。
func BenchmarkSelectorParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if parseSelectorList(".cm-line") == nil {
			b.Fatal("parse failed")
		}
	}
}

// BenchmarkSelectorMatchOne 只测「已解析选择器对一个元素的匹配」成本（无遍历）。
func BenchmarkSelectorMatchOne(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	el := doc.GetElementById("nonexistent")
	if el == nil {
		el = DocumentQuerySelector(doc, ".cm-line")
	}
	if el == nil {
		b.Fatal("no .cm-line")
	}
	selList := parseSelectorList(".cm-line")
	if selList == nil || len(selList.Selectors) != 1 {
		b.Fatal("bad selector list")
	}
	sel := selList.Selectors[0]
	checker := css.NewSelectorChecker()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !checker.Match(sel, el) {
			b.Fatal("expected match")
		}
	}
}

// BenchmarkDocumentQuerySelector 端到端：解析 + 全子树遍历（首个命中即停）。
func BenchmarkDocumentQuerySelector(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if DocumentQuerySelector(doc, ".cm-line") == nil {
			b.Fatal("no match")
		}
	}
}

// BenchmarkDocumentQuerySelectorAll 端到端：解析 + 全子树遍历（收集全部）。
func BenchmarkDocumentQuerySelectorAll(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := len(DocumentQuerySelectorAll(doc, ".cm-line")); n != 400 {
			b.Fatalf("got %d matches, want 400", n)
		}
	}
}
