package webkit

import "testing"

// TestChildRectInsideTransformedAncestor 钉死两个 CSSOM-View / Flexbox 语义缺陷：
//
//	① 子元素 getBoundingClientRect 必须包含**祖先** transform 的位移
//	  （CSSOM-View §4.2：返回元素在视口中的视觉矩形）。gou-ide 顶栏 .tb-nav
//	  用 left:50% + translate(-50%) 绝对居中，它的子胶囊 .tb-nav-pill 在
//	  wb-ui 里 rect.x 仍是布局位置（1280 视口实测 640），浏览器为 534
//	  （=640-212/2）——凡位于 transform 祖先内的元素（居中组、弹窗内容、
//	  拖拽层），定位/命中/测量全部错位。
//	② flex 容器内 position:absolute 且无 inset 的元素，静态位置须按容器的
//	  align-items 对齐（CSS Flexbox §4.1 "static position of an abspos child"）
//	  → 顶栏 .tb-nav 应垂直居中 y=8，wb-ui 给 y=0（贴容器内容顶）。
func TestChildRectInsideTransformedAncestor(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1600, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="bar" style="display:flex;align-items:center;height:40px;position:relative;background:#222222">
<nav id="nav" style="position:absolute;left:50%;transform:translate(-50%);display:flex;gap:4px;background:#333333">
<button id="p1" style="width:42px;height:24px">A</button>
<button id="p2" style="width:53px;height:24px">B</button>
</nav></div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got := dynEval(t, wv, `(function(){var f=function(id){var b=document.getElementById(id).getBoundingClientRect();return id+"="+(Math.round(b.x*10)/10)+","+(Math.round(b.y*10)/10)+" "+Math.round(b.width)+"x"+Math.round(b.height);};return f("nav")+" | "+f("p1")+" | "+f("p2");})()`)
	// 浏览器基准：nav 宽 = 42+4+53 = 99 → left:50% = 800，translate(-50%) = -49.5
	// → nav.x = 750.5；nav.y = (40-24)/2 = 8（align-items:center）。
	// p1 贴 nav 左内边缘 → 视觉 x 必须等于 nav.x（缺陷值 800 = 布局位置，未含祖先变换）。
	// p2 = nav.x + 42 + 4 = 796.5；两者 y 均 = 8。
	want := "nav=750.5,8 99x24 | p1=750.5,8 42x24 | p2=796.5,8 53x24"
	if got != want {
		t.Fatalf("子元素几何未含祖先 transform / abs 静态位置未按 flex 居中：\n  got  %s\n  want %s", got, want)
	}
}
