// 结构索引（dom_query_index.go）的正确性证据（对应 PERF_DISPATCH_BREAKDOWN §7
// 的「选择器匹配索引化」项）：
//
//  1. **等价性**：索引路径与遍历路径（DisableQueryIndex=true）在多种夹具 ×
//     多种选择器下逐项一致（元素指针、数量、顺序都要相同）；
//  2. **失效**：DOM 变更（属性 / 增删节点 / id）后索引必须失效——失效缺失会
//     返回陈旧元素（比性能问题严重得多）；
//  3. **量级**：索引命中与遍历的基准对照（同窗口紧邻测量）。
package bindings

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
)

func parseDoc(tb testing.TB, src string) *dom.Document {
	tb.Helper()
	doc, err := html.Parse(src)
	if err != nil {
		tb.Fatalf("html.Parse: %v", err)
	}
	return doc
}

// indexFixtures 覆盖：编辑器形状宽树、浅树（重复 class / id）、空 body。
func indexFixtures(tb testing.TB) map[string]*dom.Document {
	return map[string]*dom.Document{
		"editorish": buildEditorishDoc(tb, 20),
		"shallow": parseDoc(tb, `<html><body><div id="a" class="x y"><span class="x">t</span></div>`+
			`<div class="y"><span class="x">u</span></div><p id="b" class="z">p</p></body></html>`),
		"empty": parseDoc(tb, `<html><body></body></html>`),
	}
}

// indexTestSelectors 覆盖四种简单形态（class / id / tag / 通配）+ 无匹配。
var indexTestSelectors = []string{
	".cm-line", ".cm-gutterElement", ".x", ".y", ".z",
	"#a", "#b", "#nonexistent",
	"div", "span", "p", "body", "html", "*",
}

func sameElements(a, b []*dom.Element) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestQueryIndexEquivalentToWalk：索引路径与遍历路径必须逐项一致。
func TestQueryIndexEquivalentToWalk(t *testing.T) {
	for fxName, doc := range indexFixtures(t) {
		t.Run(fxName, func(t *testing.T) {
			for _, sel := range indexTestSelectors {
				DisableQueryIndex = true
				wantEl := DocumentQuerySelector(doc, sel)
				wantAll := DocumentQuerySelectorAll(doc, sel)
				DisableQueryIndex = false
				gotEl := DocumentQuerySelector(doc, sel)
				gotAll := DocumentQuerySelectorAll(doc, sel)

				if gotEl != wantEl {
					t.Fatalf("querySelector(%q)：索引=%v 遍历=%v", sel, elemName(gotEl), elemName(wantEl))
				}
				if !sameElements(gotAll, wantAll) {
					t.Fatalf("querySelectorAll(%q)：索引 %d 项 != 遍历 %d 项",
						sel, len(gotAll), len(wantAll))
				}
			}
		})
	}
	DisableQueryIndex = false
}

// elemName 输出元素可读标识（失败信息用）。
func elemName(el *dom.Element) string {
	if el == nil {
		return "<nil>"
	}
	return "<" + el.LocalName() + " id=" + el.GetId() + " class=" + el.GetAttribute("class") + ">"
}

// TestQueryIndexInvalidatedByMutations：属性/增删/id 变更后索引必须失效。
// 每个子用例都先「预热」索引（让索引缓存该 key），再做变更，最后断言结果正确。
func TestQueryIndexInvalidatedByMutations(t *testing.T) {
	// 1) 属性变更（class 新增）
	t.Run("attr-class-added", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="x">a</div></body></html>`)
		if got := DocumentQuerySelector(doc, ".added"); got != nil {
			t.Fatalf("前置条件失败：预热查询返回 %v", elemName(got))
		}
		el := DocumentQuerySelector(doc, ".x")
		if el == nil {
			t.Fatal("no .x")
		}
		el.SetAttribute("class", "x added")
		if got := DocumentQuerySelector(doc, ".added"); got != el {
			t.Fatalf("属性变更后索引未失效：got %v want %v", elemName(got), elemName(el))
		}
	})

	// 2) 属性变更（id 新增）
	t.Run("attr-id-added", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="x">a</div></body></html>`)
		if got := DocumentQuerySelector(doc, "#late"); got != nil {
			t.Fatalf("前置条件失败：预热查询返回 %v", elemName(got))
		}
		el := DocumentQuerySelector(doc, ".x")
		el.SetAttribute("id", "late")
		if got := DocumentQuerySelector(doc, "#late"); got != el {
			t.Fatalf("id 变更后索引未失效：got %v want %v", elemName(got), elemName(el))
		}
	})

	// 3) 属性移除（class 删除后不应再命中）
	t.Run("attr-class-removed", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="x goner">a</div></body></html>`)
		if got := DocumentQuerySelector(doc, ".goner"); got == nil {
			t.Fatal("前置条件失败：预热查询应命中")
		}
		el := DocumentQuerySelector(doc, ".goner")
		el.RemoveAttribute("class")
		if got := DocumentQuerySelector(doc, ".goner"); got != nil {
			t.Fatalf("class 移除后索引未失效：got %v", elemName(got))
		}
	})

	// 4) 新增节点（appendChild）
	t.Run("node-appended", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="host"></div></body></html>`)
		if got := DocumentQuerySelector(doc, ".fresh"); got != nil {
			t.Fatalf("前置条件失败：预热查询返回 %v", elemName(got))
		}
		host := DocumentQuerySelector(doc, ".host")
		fresh := doc.CreateElement("div")
		fresh.SetAttribute("class", "fresh")
		host.AppendChild(fresh)
		if got := DocumentQuerySelector(doc, ".fresh"); got != fresh {
			t.Fatalf("新增节点后索引未失效：got %v want %v", elemName(got), elemName(fresh))
		}
		if n := len(DocumentQuerySelectorAll(doc, ".fresh")); n != 1 {
			t.Fatalf("新增节点后 querySelectorAll 得到 %d 项，want 1", n)
		}
	})

	// 5) 删除节点
	t.Run("node-removed", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="host"><span class="gone">g</span></div></body></html>`)
		if got := DocumentQuerySelector(doc, ".gone"); got == nil {
			t.Fatal("前置条件失败：预热查询应命中")
		}
		host := DocumentQuerySelector(doc, ".host")
		var gone *dom.Element
		for _, s := range doc.GetElementsByTagName("span") {
			gone = s
		}
		if err := host.RemoveChild(gone); err != nil {
			t.Fatalf("RemoveChild: %v", err)
		}
		if got := DocumentQuerySelector(doc, ".gone"); got != nil {
			t.Fatalf("删除节点后索引未失效：got %v", elemName(got))
		}
	})

	// 6) 索引缓存命中后再变更，仍必须失效（跨多次查询）
	t.Run("repeat-query-then-mutate", func(t *testing.T) {
		doc := parseDoc(t, `<html><body><div class="x">a</div></body></html>`)
		for i := 0; i < 3; i++ {
			_ = DocumentQuerySelectorAll(doc, ".x") // 反复命中索引
		}
		el := doc.CreateElement("p")
		el.SetAttribute("class", "x")
		DocumentQuerySelector(doc, "body").AppendChild(el)
		if n := len(DocumentQuerySelectorAll(doc, ".x")); n != 2 {
			t.Fatalf("变更后查询得到 %d 项，want 2（索引缓存未失效）", n)
		}
	})
}

// TestQueryIndexReturnsCopy：querySelectorAll 必须返回副本——调用方 append
// 不得改写索引中缓存的切片（否则后续查询会串数据）。
func TestQueryIndexReturnsCopy(t *testing.T) {
	doc := parseDoc(t, `<html><body><div class="x">a</div><div class="x">b</div></body></html>`)
	first := DocumentQuerySelectorAll(doc, ".x")
	if len(first) != 2 {
		t.Fatalf("got %d, want 2", len(first))
	}
	poisoned := append(first, nil) // 若 first 与索引共享底层数组，这会污染索引
	_ = poisoned
	second := DocumentQuerySelectorAll(doc, ".x")
	if len(second) != 2 {
		t.Fatalf("索引被调用方 append 污染：第二次得到 %d 项，want 2", len(second))
	}
	for i := range second {
		if second[i] == nil {
			t.Fatalf("索引第 %d 项被污染为 nil", i)
		}
	}
}

// ── 基准：索引命中 vs 强制遍历 ──────────────────────────────────────────────

// BenchmarkQuerySelectorIndexed：重复查询同一选择器（索引命中）。
func BenchmarkQuerySelectorIndexed(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	DisableQueryIndex = false
	if DocumentQuerySelector(doc, ".cm-line") == nil {
		b.Fatal("no match")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if DocumentQuerySelector(doc, ".cm-line") == nil {
			b.Fatal("no match")
		}
	}
}

// BenchmarkQuerySelectorWalk：同一查询的遍历路径对照。
func BenchmarkQuerySelectorWalk(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	DisableQueryIndex = true
	defer func() { DisableQueryIndex = false }()
	if DocumentQuerySelector(doc, ".cm-line") == nil {
		b.Fatal("no match")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if DocumentQuerySelector(doc, ".cm-line") == nil {
			b.Fatal("no match")
		}
	}
}

// BenchmarkQuerySelectorAllIndexed：重复查询同一选择器的全量结果（索引命中）
// ——注意每次调用都返回副本，成本 O(结果数)。
func BenchmarkQuerySelectorAllIndexed(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	DisableQueryIndex = false
	if n := len(DocumentQuerySelectorAll(doc, ".cm-line")); n != 400 {
		b.Fatalf("got %d, want 400", n)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := len(DocumentQuerySelectorAll(doc, ".cm-line")); n != 400 {
			b.Fatalf("got %d, want 400", n)
		}
	}
}

// BenchmarkQuerySelectorAllWalk：同一查询的遍历路径对照。
func BenchmarkQuerySelectorAllWalk(b *testing.B) {
	doc := buildEditorishDoc(b, 400)
	DisableQueryIndex = true
	defer func() { DisableQueryIndex = false }()
	if n := len(DocumentQuerySelectorAll(doc, ".cm-line")); n != 400 {
		b.Fatalf("got %d, want 400", n)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if n := len(DocumentQuerySelectorAll(doc, ".cm-line")); n != 400 {
			b.Fatalf("got %d, want 400", n)
		}
	}
}
