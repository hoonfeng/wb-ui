// 命中测试 Pass1 剪枝的正确性证据（对应 PERF_DISPATCH_BREAKDOWN §7 的
// 「命中测试剪枝」项）：
//
//   1. 逐点等价：剪枝开 / 剪枝关 两种模式在网格点上结果完全一致；
//   2. 探测正确：MayHaveFixedDescendant 与夹具事实（是否含 position:fixed）一致；
//   3. 失效正确：渲染树就地插入 fixed 元素后探测缓存必须失效（否则剪枝会
//      漏掉 fixed 子树 → 点击弹窗穿透到下层元素）；
//   4. 语义保留：fixed 子树优先（遮罩内按钮胜过坐标重叠的下层元素）。
package rendering_test

import (
	"fmt"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

// pruneFixture 是一个剪枝测试夹具：hasFixed 声明夹具是否含 position:fixed
// 元素（用于校验探测结果，避免测试自身依赖被测逻辑）。
type pruneFixture struct {
	name     string
	body     string
	hasFixed bool
}

func pruneFixtures() []pruneFixture {
	return []pruneFixture{
		{
			name:     "flat-no-fixed",
			body:     flatBody(60),
			hasFixed: false,
		},
		{
			name:     "scroller-no-fixed",
			body:     editorLikeBody(60),
			hasFixed: false,
		},
		{
			name: "fixed-overlay-over-button",
			body: `<div class="page" style="width:700px;height:400px">
  <button class="under" style="width:200px;height:100px;margin-left:100px;margin-top:100px">under</button>
</div>
<div class="overlay" style="position:fixed;left:0;top:0;width:700px;height:400px">
  <button class="ok" style="width:80px;height:30px;margin-left:120px;margin-top:120px">ok</button>
</div>`,
			hasFixed: true,
		},
		{
			name: "fixed-stacked-two-overlays",
			body: `<div class="page" style="width:700px;height:400px"></div>
<div class="ov1" style="position:fixed;left:0;top:0;width:700px;height:400px"></div>
<div class="ov2" style="position:fixed;left:50px;top:50px;width:400px;height:300px">
  <button class="btn2" style="width:60px;height:24px;margin-left:20px;margin-top:20px">b</button>
</div>`,
			hasFixed: true,
		},
	}
}

// elemDesc 输出元素的可读标识（失败信息用）。
func elemDesc(el *dom.Element) string {
	if el == nil {
		return "<nil>"
	}
	return fmt.Sprintf("<%s class=%q>", el.LocalName(), el.GetAttribute("class"))
}

// TestHitTestFixedPruneEquivalence 在网格点上逐点比较剪枝开/关的命中结果。
// 剪枝跳过的只是「树内无 fixed 时对结果零贡献的 Pass1」，两者必须逐点一致。
func TestHitTestFixedPruneEquivalence(t *testing.T) {
	xs := []float64{2, 30, 120, 150, 350, 690}
	ys := []float64{2, 40, 140, 300, 390}
	for _, fx := range pruneFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			_, rv, _, _ := mkRuntime(t, fx.body)
			for _, attr := range []string{"", "onclick"} {
				for _, x := range xs {
					for _, y := range ys {
						rendering.HitTestPruneFixedDisabled = false
						got := rendering.HitTest(rv, x, y, attr)
						rendering.HitTestPruneFixedDisabled = true
						want := rendering.HitTest(rv, x, y, attr)
						if got != want {
							rendering.HitTestPruneFixedDisabled = false
							t.Fatalf("点(%v,%v) attr=%q：剪枝=%v，未剪枝=%v（必须一致）",
								x, y, attr, elemDesc(got), elemDesc(want))
						}
					}
				}
			}
			rendering.HitTestPruneFixedDisabled = false
		})
	}
}

// TestFixedProbeMatchesFixtureFact 探测结果必须与夹具声明的事实一致
// （两侧都要覆盖：无 fixed 走剪枝、有 fixed 不剪枝）。
func TestFixedProbeMatchesFixtureFact(t *testing.T) {
	for _, fx := range pruneFixtures() {
		t.Run(fx.name, func(t *testing.T) {
			_, rv, _, _ := mkRuntime(t, fx.body)
			if got := rv.MayHaveFixedDescendant(); got != fx.hasFixed {
				t.Fatalf("MayHaveFixedDescendant() = %v, want %v", got, fx.hasFixed)
			}
		})
	}
}

// TestFixedProbeInvalidatedByInPlaceUpdate 渲染树**就地**插入 fixed 元素后，
// 探测缓存必须失效。若失效缺失，剪枝会让新插入的 fixed 子树不可命中
//（点击弹窗穿透）——这是剪枝方案唯一的正确性风险点。
func TestFixedProbeInvalidatedByInPlaceUpdate(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<div class="host" style="width:400px;height:200px"></div>`)
	if rv.MayHaveFixedDescendant() {
		t.Fatal("夹具不应含 fixed（前置条件失败）")
	}

	// resolver 必须与真实路径一致：增量插入会 resolveStyle（内联 style /
	// 样式表匹配），只有解析出 position:fixed 才会真的在渲染树里产生 fixed
	// 盒子。resolver 为 nil 时 fallback 到 tag 默认样式（position 恒为
	// static），本测试就失去意义（探测返回 false 反而正确）。
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	sheet := css.NewCSSStyleSheet()
	css.NewParser(".late-overlay{position:fixed;left:0;top:0;width:400px;height:200px}").ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	updater := rendering.NewRenderTreeUpdater(rv, resolver)
	overlay := doc.CreateElement("div")
	overlay.SetAttribute("class", "late-overlay")
	var body *dom.Element
	for _, b := range doc.GetElementsByTagName("body") {
		body = b
		break
	}
	if body == nil {
		t.Fatal("fixture has no body")
	}
	body.AppendChild(overlay)
	updater.MarkInsert(overlay)
	updater.Update()

	// 诊断并断言前提：渲染树里确实产生了 position:fixed 的盒子。
	var foundRO bool
	var posDesc = "<none>"
	for cur := rendering.RenderObject(rv); cur != nil; cur = cur.NextInPreOrder() {
		if cur.Node() != overlay {
			continue
		}
		foundRO = true
		if b, ok := cur.(interface{ AsRenderBox() *rendering.RenderBox }); ok {
			if bb := b.AsRenderBox(); bb != nil && bb.Style() != nil {
				posDesc = fmt.Sprintf("%v", bb.Style().Position)
			}
		}
	}
	t.Logf("overlay 渲染对象在树内=%v position=%s", foundRO, posDesc)
	if !foundRO || posDesc != fmt.Sprintf("%v", style.PositionFixed) {
		t.Fatalf("前置条件失败：就地插入未产生 fixed 渲染盒子（inTree=%v position=%s）", foundRO, posDesc)
	}

	if !rv.MayHaveFixedDescendant() {
		t.Fatal("就地插入 position:fixed 元素后探测缓存未失效——剪枝会漏掉 fixed 命中")
	}
}

// TestFixedOverlayBeatsOverlappingUnderlyingElement：fixed 子树优先的语义
// 不得被剪枝削弱——点同时落在下层按钮与 fixed 遮罩内按钮上时，必须命中
// fixed 子树内的元素（Pass1 的核心职责）。
func TestFixedOverlayBeatsOverlappingUnderlyingElement(t *testing.T) {
	_, rv, _, _ := mkRuntime(t, `<div class="page" style="width:700px;height:400px">
  <button class="under" style="width:200px;height:100px;margin-left:100px;margin-top:100px">under</button>
</div>
<div class="overlay" style="position:fixed;left:0;top:0;width:700px;height:400px">
  <button class="ok" style="width:80px;height:30px;margin-left:120px;margin-top:120px">ok</button>
</div>`)
	el := rendering.HitTest(rv, 150, 140, "")
	if el == nil {
		t.Fatal("HitTest nil")
	}
	if cls := el.GetAttribute("class"); cls != "ok" {
		t.Fatalf("命中 %v，期望 fixed 遮罩内的 .ok 按钮（fixed 子树优先）", elemDesc(el))
	}
}
