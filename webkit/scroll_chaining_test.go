package webkit

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/rendering"
)

// TestLayeredOverflowAutoScrollRange 钉死：内层声明 overflow:auto 而内容**并不
// 超出它自己**时，其子内容必须计入**外层**可滚容器的滚动范围。
//
// 复现场景（gou-ide 设置面板）：`.modal-content`（可滚）> `.settings-content`
// （声明 overflow:auto、高度由内容撑开）> 超出内容。此前 `BoxContentSize` 对任何
// overflow 非 visible 的子盒**一律跳过其子树** → 外层内容尺寸塌成可视高 →
// `ScrollRange.maxY = 0`、`VerticalScrollbarMetrics.OK = false`（既不滚动也不绘
// 滚动条），而 DOM 侧 `scrollHeight−clientHeight = 422` 显示内容明确超出
// ——「两套口径不一致」的根因。
func TestLayeredOverflowAutoScrollRange(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="outer" style="width:200px;height:100px;overflow-y:auto">
  <div id="inner" style="overflow-y:auto">
    <div style="height:400px"></div>
  </div>
</div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 为空")
	}
	// 点在 #outer 的空白区（顶层命中 outer，但滚动目标解析应给出 outer 自身）。
	box := rv.HitTestScrollContainer(150, 80)
	if box == nil {
		t.Fatal("未解析到滚动容器：内层 overflow:auto 不可滚时应上溯到 #outer")
	}
	if el, ok := box.Node().(*dom.Element); !ok || el.GetAttribute("id") != "outer" {
		t.Fatalf("滚动目标错误：%v（期望 #outer）", box.Node())
	}
	maxX, maxY, _, vertical := rendering.ScrollRange(rv, box)
	if maxY <= 0 || !vertical {
		t.Fatalf("层化 overflow:auto 容器内容超出但 ScrollRange 未生效：maxX=%v maxY=%v vertical=%v（内容 400 > 可视 100，期望 maxY≈300）", maxX, maxY, vertical)
	}
	if m := rendering.VerticalScrollbarMetrics(rv, box); !m.OK {
		t.Fatal("内容超出但 VerticalScrollbarMetrics.OK=false：滚动条不会被绘制")
	}
}

// TestScrollTargetSkipsNonScrollableAncestor 钉死：命中「声明了 overflow:auto
// 但内容未超出」的盒子时，滚动目标必须**继续向上**找真正可滚的祖先（浏览器
// scroll chaining 语义）。
//
// 复现场景（gou-ide 设置面板）：`.settings-content` 声明 overflow:auto 但内容
// 未超出，真正可滚的是祖先 `.modal-content`（内容超出 422px）——wb-ui 此前按
// 「最近的 overflow 非 visible 祖先」返回，滚轮事件送达 settings-content 却
// 滚不动（实测 7 个 ev:scroll、modal-content.scrollTop 恒为 0）。
func TestScrollTargetSkipsNonScrollableAncestor(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="outer" style="width:200px;height:100px;overflow-y:auto">
  <div style="height:400px">
    <div id="inner" style="height:40px;overflow-y:auto"></div>
  </div>
</div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 为空")
	}
	// 点在 #inner 上（内容 40px < 自身高 40px → 不可滚）。
	box := rv.HitTestScrollContainer(20, 20)
	if box == nil {
		t.Fatal("未解析到滚动目标：应继续向上找到 #outer")
	}
	el, ok := box.Node().(*dom.Element)
	if !ok {
		t.Fatalf("滚动目标不是元素盒：%T", box.Node())
	}
	if got := el.GetAttribute("id"); got != "outer" {
		t.Fatalf("滚动目标未上溯到可滚祖先：got id=%q, want \"outer\"（#inner 声明 auto 但内容未超出，应跳过）", got)
	}
}
