// 常驻滚动条预留（classic scrollbar gutter）的端到端回归测试。
//
// 对应缺陷：overflow-y:auto 的滚动容器给子元素的可用宽度没有扣除常驻滚动条，
// 于是 width:auto 的块级子元素（连同 border-bottom）一路铺到容器最右缘、压在
// 滚动条上把它切成几段（gou-ide 插件面板 `.pp-item` 分隔线盖住滚动条）。
// 浏览器（Chrome/Windows）的 classic 滚动条占位：子元素宽度 = clientWidth
// = padding box 宽 - 滚动条宽。
//
// ★ 本测试的关键在于容器形态：`.pp-list` 是 flex 容器的 item（flex:1），它的
// 高度由**父 flex 布局**分配，而不是自己的 CSS height。曾经把判定放在布局
// （BFC）收尾处读取 g.PaddingBoxHeight()，那一刻 flex 还没把高度压到分配值
// （读到的是内容撑开的高度 1287 vs 视口 1295）→ 恒判「不溢出」→ 预留永不
// 生效。因此本测试必须用 flex 分配高度的形态，才能覆盖该回归。
package rendering_test

import (
	"testing"

	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

// scrollbarReserveFlexHTML：300x200 的 flex 列容器，头部 30px，
// #list 占余下 170px 且 overflow-y:auto；内容 10×40 = 400px > 170px → 溢出。
const scrollbarReserveFlexHTML = `<!doctype html>
<style>
  html, body { margin: 0; }
  #flex { display: flex; flex-direction: column; width: 300px; height: 200px; }
  #list { flex: 1; overflow-y: auto; }
  #list .item { height: 40px; }
</style>
<div id="flex">
  <div style="height:30px"></div>
  <div id="list">
    <div class="item" id="item1">a</div>
    <div class="item">b</div>
    <div class="item">c</div>
    <div class="item">d</div>
    <div class="item">e</div>
    <div class="item">f</div>
    <div class="item">g</div>
    <div class="item">h</div>
    <div class="item">i</div>
    <div class="item">j</div>
  </div>
</div>`

// scrollbarReserveFitHTML：#list 内只有一项，内容不溢出 → 不得预留。
const scrollbarReserveFitHTML = `<!doctype html>
<style>
  html, body { margin: 0; }
  #flex { display: flex; flex-direction: column; width: 300px; height: 200px; }
  #list { flex: 1; overflow-y: auto; }
  #list .item { height: 40px; }
</style>
<div id="flex">
  <div style="height:30px"></div>
  <div id="list">
    <div class="item" id="item1">a</div>
  </div>
</div>`

// layoutFixture 解析 HTML、装配渲染树并完成布局（含滚动条预留收敛 pass）。
func layoutFixture(t *testing.T, page string, vw, vh float64) (*rendering.RenderView, func(string) *rendering.RenderBox) {
	t.Helper()
	doc, err := html.Parse(page)
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
	rv.SetViewportSize(vw, vh)
	rv.Layout(layout.NewLayoutState(vw, vh))

	find := func(id string) *rendering.RenderBox {
		el := doc.GetElementById(id)
		if el == nil {
			t.Fatalf("fixture element #%s missing", id)
		}
		box := rv.FindRenderBoxForNode(el)
		if box == nil {
			t.Fatalf("no render box for #%s", id)
		}
		return box
	}
	return rv, find
}

// TestScrollbarReserve_FlexItemOverflowReservesWidth：flex 分配高度下溢出时，
// #list 的可用内容宽必须扣除滚动条宽（15px = scrollbar-color 自绘滚动条，有头
// Edge 实测），子元素宽度 = 300-15 = 285。
func TestScrollbarReserve_FlexItemOverflowReservesWidth(t *testing.T) {
	rv, find := layoutFixture(t, scrollbarReserveFlexHTML, 900, 1000)

	listBox := find("list")
	// 先确认前提：flex 确实把 #list 压到了剩余高度（200-30=170），否则本用例
	// 退化成了「容器被内容撑开」，测不到 flex 分配高度这条路径。
	if h := listBox.PaddingBoxRect().Height; h < 169 || h > 171 {
		t.Fatalf("#list padding box height = %v, want 170 (flex-distributed)", h)
	}
	// 且内容确实溢出（否则不应该预留）。
	_, totalH := rv.BoxContentSize(listBox)
	viewH := listBox.PaddingBoxRect().Height
	if totalH <= viewH {
		t.Fatalf("#list content height %v <= viewport %v: fixture no longer overflows", totalH, viewH)
	}

	itemBox := find("item1")
	got := itemBox.FrameRect().Width
	if want := 300.0 - 15.0; got != want {
		t.Errorf(".item width = %v, want %v (container width 300 - scrollbar 15)", got, want)
	}
}

// TestScrollbarReserve_NoOverflowNoReserve：内容未溢出时不预留，子元素仍占满
// 容器宽（否则容器右侧会凭空多出一条空白）。
func TestScrollbarReserve_NoOverflowNoReserve(t *testing.T) {
	_, find := layoutFixture(t, scrollbarReserveFitHTML, 900, 1000)
	got := find("item1").FrameRect().Width
	if got != 300 {
		t.Errorf(".item width = %v, want 300 (no reserve when content fits)", got)
	}
}
