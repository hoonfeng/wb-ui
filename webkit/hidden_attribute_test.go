package webkit

import "testing"

// TestHiddenAttributeExitsLayout 钉死 HTML §4.12.2 的 `hidden` 属性语义：
// 带 hidden 的元素必须 display:none（UA 规则 `[hidden]{display:none}`），
// 在 flex 容器中**完全不占位**（既不参与主轴尺寸、也不产生 gap 间隔）。
//
// 根因（监督者定位、已修）：`engine/style/resolver.go` 的 hidden 分支此前只写
// `cs.Display = DisplayNone`，**未写 `cs.DisplaySet = true`**；而
// rendering（rendertreebuilder.go）与 layout（box.go）两处都有
// `if !cs.DisplaySet { cs.Display = defaultDisplayForTag(el.LocalName()) }`，
// `<span>` 恰好走 applyDefaultDisplay 的 default 分支（不设 DisplaySet）
// → DisplayNone 被覆盖回 inline → 元素照常参与 flex 布局、照常占 gap。
// 真实表现：gou-ide autopilot 该元素块 `某组件` 内被 `el.hidden = true` 隐藏的
// `被隐藏元素`/`另一被隐藏元素` 各占一个 5px gap → 该元素块宽 83（浏览器 73），
// 右段容器被撑到 259（浏览器 248）。
func TestHiddenAttributeExitsLayout(t *testing.T) {
	t.Run("static_hidden_span", func(t *testing.T) {
		wv := NewWebView()
		defer wv.Destroy()
		wv.Resize(400, 200)
		html := `<!DOCTYPE html><html><body style="margin:0">
<div id="row" style="display:flex;gap:10px;width:200px">
  <span id="a" style="width:20px;height:10px;background:#111111"></span>
  <span id="b" hidden style="width:5px;height:10px;background:#222222"></span>
  <span id="d" style="width:30px;height:10px;background:#333333"></span>
</div></body></html>`
		if err := wv.LoadHTML(html); err != nil {
			t.Fatalf("LoadHTML: %v", err)
		}
		for i := 0; i < 8; i++ {
			wv.EnsureLayout()
			if _, err := wv.Render(); err != nil {
				t.Fatalf("Render: %v", err)
			}
		}
		got := dynEval(t, wv, `(function(){
  var a=document.getElementById("a"), b=document.getElementById("b"), d=document.getElementById("d");
  var ar=a.getBoundingClientRect(), br=b.getBoundingClientRect(), dr=d.getBoundingClientRect();
  return "a=" + Math.round(ar.x) + "," + Math.round(ar.width) +
         " b=" + Math.round(br.x) + "," + Math.round(br.width) +
         " d=" + Math.round(dr.x) + "," + Math.round(dr.width);
})()`)
		// 浏览器语义：a(0,20) → b 退出布局(0,0) → d 紧跟 a+gap = 30
		want := "a=0,20 b=0,0 d=30,30"
		if got != want {
			t.Fatalf("静态 hidden 未退出 flex 布局：\n  got  %s\n  want %s", got, want)
		}
	})

	t.Run("dynamic_hidden_then_relayout", func(t *testing.T) {
		wv := NewWebView()
		defer wv.Destroy()
		wv.Resize(400, 200)
		html := `<!DOCTYPE html><html><body style="margin:0">
<div id="row" style="display:flex;gap:10px;width:200px">
  <span id="a" style="width:20px;height:10px;background:#111111"></span>
  <span id="b" style="width:5px;height:10px;background:#222222"></span>
  <span id="d" style="width:30px;height:10px;background:#333333"></span>
</div></body></html>`
		if err := wv.LoadHTML(html); err != nil {
			t.Fatalf("LoadHTML: %v", err)
		}
		for i := 0; i < 8; i++ {
			wv.EnsureLayout()
			if _, err := wv.Render(); err != nil {
				t.Fatalf("Render: %v", err)
			}
		}
		before := dynEval(t, wv, `(function(){var d=document.getElementById("d").getBoundingClientRect();return Math.round(d.x);})()`)
		// 前置：a(20) + gap(10) + b(5) + gap(10) = 45（两个 gap 都要算）
		if before != "45" {
			t.Fatalf("前置状态异常：d.x=%s（期望 45）", before)
		}
		// 动态隐藏 + 重新布局
		if _, err := wv.EvalJS(`document.getElementById("b").hidden = true;`); err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		for i := 0; i < 8; i++ {
			wv.EnsureLayout()
			if _, err := wv.Render(); err != nil {
				t.Fatalf("Render: %v", err)
			}
		}
		got := dynEval(t, wv, `(function(){
  var b=document.getElementById("b"), d=document.getElementById("d");
  var br=b.getBoundingClientRect(), dr=d.getBoundingClientRect();
  return "b=" + Math.round(br.width) + " d=" + Math.round(dr.x) + " attr=" + b.hasAttribute("hidden");
})()`)
		// b 退出布局(宽 0) → d 前移到 a+gap = 30
		want := "b=0 d=30 attr=true"
		if got != want {
			t.Fatalf("动态 el.hidden=true 后未退出布局：\n  got  %s\n  want %s", got, want)
		}
	})
}
