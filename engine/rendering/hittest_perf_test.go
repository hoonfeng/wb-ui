// 命中测试热路径基准（elementFromPoint 的算法成本取证）。
//
// 场景与真实编辑器一致：cm-scroller（overflow:auto，层化）内多行 .cm-line，
// 树规模千级节点、无 position:fixed 元素。用于量化 HitTest 的
// 「固定开销」部分（Pass1 fixed 预扫描 / 每节点滚动偏移查询 / 层内 skip 表），
// 并在优化后做前后对照（同窗口、紧邻测量）。
package rendering_test

import (
	"strings"
	"testing"

	"wb-ui/engine/rendering"
)

// editorLikeBody 生成编辑器式 DOM：cm-scroller > cm-content > N × cm-line
// （每行含 3 个 span token）。无 fixed 元素、有 1 个滚动容器。
func editorLikeBody(rows int) string {
	var b strings.Builder
	b.WriteString(`<div class="cm-scroller" style="width:760px;height:560px;overflow:auto;position:relative">`)
	b.WriteString(`<div class="cm-content" style="width:740px">`)
	for i := 0; i < rows; i++ {
		b.WriteString(`<div class="cm-line" style="height:19px;font-family:Consolas;font-size:14px">`)
		b.WriteString(`<span class="tok-kw">func</span> <span class="tok-fn">main</span>() {</span>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// flatBody 生成无滚动容器的扁平 DOM（对照组：无 per-box 滚动偏移）。
func flatBody(rows int) string {
	var b strings.Builder
	b.WriteString(`<div class="flat-root" style="width:760px">`)
	for i := 0; i < rows; i++ {
		b.WriteString(`<div class="flat-row" style="height:19px;font-size:14px">text row</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

const (
	benchHitX = 40.0
	benchHitY = 200.0
)

// BenchmarkHitTestEditorDOM：编辑器式树（层化 + 滚动容器），最深元素命中。
func BenchmarkHitTestEditorDOM(b *testing.B) {
	_, rv, _, _ := mkRuntime(b, editorLikeBody(300))
	if el := rendering.HitTest(rv, benchHitX, benchHitY, ""); el == nil {
		b.Fatal("HitTest returned nil (fixture broken)")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rendering.HitTest(rv, benchHitX, benchHitY, "")
	}
}

// BenchmarkHitTestEditorDOMAttr：带属性过滤（attrName 路径，事件分发用）。
func BenchmarkHitTestEditorDOMAttr(b *testing.B) {
	_, rv, _, _ := mkRuntime(b, editorLikeBody(300))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rendering.HitTest(rv, benchHitX, benchHitY, "onclick")
	}
}

// BenchmarkHitTestFlatDOM：无滚动容器对照（验证滚动偏移短路收益）。
func BenchmarkHitTestFlatDOM(b *testing.B) {
	_, rv, _, _ := mkRuntime(b, flatBody(300))
	if el := rendering.HitTest(rv, benchHitX, benchHitY, ""); el == nil {
		b.Fatal("HitTest returned nil (fixture broken)")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rendering.HitTest(rv, benchHitX, benchHitY, "")
	}
}

// BenchmarkHitTestMissPoint：点在所有元素之外（走满三趟的最大成本路径）。
func BenchmarkHitTestMissPoint(b *testing.B) {
	_, rv, _, _ := mkRuntime(b, editorLikeBody(300))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rendering.HitTest(rv, -500, -500, "")
	}
}
