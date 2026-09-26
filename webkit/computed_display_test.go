package webkit

import "testing"

// TestComputedStyleDisplayAlwaysResolved 钉死 `getComputedStyle(el).display` 必须
// **恒有定义**（读值 API 缺口）：级联未声明 display 的元素（span 等）此前返回
// undefined，污染一切依赖 display 的 JS 逻辑。
// 语义：带 hidden → "none"；块级 → "block"；其它 → "inline"。
func TestComputedStyleDisplayAlwaysResolved(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="row" style="display:flex;gap:10px;width:200px">
  <span id="a" style="width:20px;height:10px"></span>
  <span id="b" hidden style="width:5px;height:10px"></span>
  <div id="d" style="width:30px;height:10px"></div>
</div>
<span id="plain" style="width:8px"></span>
<input id="ctrl" type="text">
</body></html>`
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
  function d(id){ var e=document.getElementById(id); var v=getComputedStyle(e).display; return id+":"+v+"("+typeof v+")"; }
  return d("a") + " | " + d("b") + " | " + d("d") + " | " + d("plain") + " | " + d("ctrl");
})()`)
	t.Logf("display 实测 = %s", got)
	if contains(got, "undefined") {
		t.Fatalf("getComputedStyle().display 仍存在 undefined：%s", got)
	}
	if !contains(got, "b:none(string)") {
		t.Fatalf("hidden 元素的 computed display 应为 \"none\"：%s", got)
	}
	if !contains(got, "d:block(string)") {
		t.Fatalf("块级元素的 computed display 应为 \"block\"：%s", got)
	}
	// ★ 动态 hidden：对 flex 子项 #a 用 `el.hidden = true`（走 accessor → 标脏
	//   渲染树，而非 HTML 属性）后重排，computed display 必须同样为 "none"。
	//   （验收标准 2 明文要求「动态/静态 hidden 元素上 display === 'none'」；
	//     静态分支由上方 #b（HTML `hidden` 属性）覆盖，此处钉死动态分支。）
	if got2 := dynEval(t, wv, `(function(){
  document.getElementById("a").hidden = true;
  return "set";
})()`); got2 != "set" {
		t.Fatalf("设置动态 hidden 失败：%q", got2)
	}
	for i := 0; i < 8; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	dynGot := dynEval(t, wv, `(function(){
  var a = document.getElementById("a");
  var v = getComputedStyle(a).display;
  return "a.hidden=" + a.hidden + " a.display=" + v + "(" + typeof v + ")";
})()`)
	t.Logf("动态 hidden 实测 = %s", dynGot)
	if !contains(dynGot, "a.hidden=true") {
		t.Fatalf("动态 hidden 未生效（a.hidden != true）：%s", dynGot)
	}
	if !contains(dynGot, "a.display=none(string)") {
		t.Fatalf("动态 hidden（el.hidden=true 后 EnsureLayout+Render）的 computed display 应为 \"none\"：%s", dynGot)
	}
}
