// ScrollRange 是「元素能否滚动」的判据（CSSOM View：max = 内容总长 - 可视长），
// 与滚动条几何（VerticalScrollbarMetrics.OK =「该不该画滚动条」）是两件事：
// 容器小到放不下箭头按钮时滚动条不画，但元素依然可以滚动。把滚动条几何当
// 「可否滚动」用，会让小尺寸滚动容器上的 scrollTop 赋值被静默丢弃——
// dev/suites/cssprobe 的 modern-hydration-contracts 正是这种容器（10×10 +
// overflow:scroll + 100×100 内容，脚本随后读回 scrollLeft/scrollTop === 15）。
//
// 本测试同时断言两者在小容器上的分歧：滚动范围成立（canY/canX = true、上限 90），
// 而滚动条几何 OK=false。若将来有人把 setElementScrollOffset 的判定改回滚动条
// 几何，本测试的对照断言会立刻暴露语义回退。
package rendering_test

import (
	"testing"

	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

// scrollRangeHTML：10×10 的 overflow:scroll 容器，内容 100×100 ⇒ 两轴滚动上限
// 均为 90。容器高 10px 远小于滚动条箭头尺寸下限（2*12 + 2*5 = 34px），滚动条
// 因此不绘制（这是既有的绘制行为，本测试只要求它不再影响可滚动性判断）。
const scrollRangeHTML = `<!doctype html>
<style>
  html, body { margin: 0; }
  #scroller { position: absolute; left: 0; top: 0; width: 10px; height: 10px; overflow: scroll; }
  #scroller div { width: 100px; height: 100px; }
</style>
<div id="scroller"><div></div></div>`

func TestScrollRangeSmallOverflowScrollContainer(t *testing.T) {
	doc, err := html.Parse(scrollRangeHTML)
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	if root := doc.DocumentElement(); root != nil {
		extractStylesTest(root, resolver)
	}
	rv := rendering.NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("render tree build failed")
	}
	rv.SetResolver(resolver)
	rv.SetViewportSize(900, 1000)
	rv.Layout(layout.NewLayoutState(900, 1000))

	el := doc.GetElementById("scroller")
	if el == nil {
		t.Fatal("fixture element #scroller missing")
	}
	box := rv.FindRenderBoxForNode(el)
	if box == nil {
		t.Fatal("no render box for #scroller")
	}

	maxX, maxY, canX, canY := rendering.ScrollRange(rv, box)
	if !canY {
		t.Errorf("canY = false, want true (10px viewport, 100px content, overflow:scroll)")
	}
	if !canX {
		t.Errorf("canX = false, want true (10px viewport, 100px content, overflow:scroll)")
	}
	if maxY != 90 {
		t.Errorf("maxY = %v, want 90", maxY)
	}
	if maxX != 90 {
		t.Errorf("maxX = %v, want 90", maxX)
	}

	// 对照：同一容器上滚动条几何不可用（不绘制）。两者分歧正是本修复的语义
	// 要点——滚动条画不下 ≠ 元素不可滚动。
	vm := rendering.VerticalScrollbarMetrics(rv, box)
	if vm.OK {
		t.Errorf("VerticalScrollbarMetrics.OK = true on a 10px-high container; " +
			"the contrast this test documents (drawing judgement vs. scrollability) no longer holds")
	}
}
