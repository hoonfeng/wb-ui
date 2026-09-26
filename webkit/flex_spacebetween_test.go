package webkit

import "testing"

// TestFlexSpaceBetweenRightEdge 钉死：flex + justify-content:space-between 时
// 最后一个 item 必须贴容器右内边缘（padding 内）。
//
// 这是 gou-ide 底部状态栏右段（"已连接 · 本地 / UTF-8 / 上下文 0 / Midnight"）
// 在 wb-ui 里被推到视口外的最小复现：
//   浏览器 1280 视口 → .status-right x=1033（=1280-8-239 右对齐）
//   wb-ui  1600 视口 → .status-right x=2219（视口外，整段不可见）
func TestFlexSpaceBetweenRightEdge(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1600, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="bar" style="display:flex;justify-content:space-between;width:1600px;height:28px;padding:0 8px;box-sizing:border-box;background:#eeeeee">
<div id="left" style="width:92px;height:15px;background:#cccccc"></div>
<div id="mid" style="display:flex;height:auto;margin:0 auto"></div>
<div id="right" style="width:239px;height:15px;background:#999999"></div>
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
	got := dynEval(t, wv, `(function(){var l=document.getElementById("left").getBoundingClientRect();var m=document.getElementById("mid").getBoundingClientRect();var r=document.getElementById("right").getBoundingClientRect();return "L="+Math.round(l.x)+",M="+Math.round(m.x)+"x"+Math.round(m.width)+",R="+Math.round(r.x)+"x"+Math.round(r.width);})()`)
	// 关键断言（与浏览器对照）：left 贴左内边缘 8；right 贴右内边缘 1592-239=1353。
	// 中间项为 0 宽 + margin:0 auto：auto margin 先吸收剩余空间（左右各半），
	// 因此它应落在容器内容区中部（≠ 容器右内边缘 1592）。
	// 曾出错：wb-ui 把 mid 放到 1592（=右内边缘），space-between 再叠加间距 → R=2219（视口外）。
	want := "L=8,M=727x0,R=1353x239"
	if got != want {
		t.Fatalf("space-between + auto margin 布局错误：\n  got  %s\n  want %s（浏览器：mid 居中≈726，right 贴右内边缘 1353）", got, want)
	}
	// 额外守卫：右项必须落在容器内（防止"溢出视口"回归）。
	outOfBounds := dynEval(t, wv, `(function(){var r=document.getElementById("right").getBoundingClientRect();return (r.x+r.width<=1592.5 && r.x>=0)?"ok":"OUT("+Math.round(r.x)+")";})()`)
	if outOfBounds != "ok" {
		t.Fatalf("右项溢出容器右内边缘：%s", outOfBounds)
	}
}
