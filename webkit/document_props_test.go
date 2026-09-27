package webkit

import "testing"

// TestDocumentReadyStateAndCollections 钉死 document.readyState / scripts /
// styleSheets 三项 P0-2 覆盖缺口
// （见 关键点/Web平台API覆盖矩阵与剩余工作 §五「P0 —— 解锁类」第 2 条）。
//
// 语义（HTML §3.1.4 / CSSOM §document.styleSheets）：
//   - readyState 恒为字符串，且**随加载阶段推进**：页面脚本执行时为
//     "interactive"，装载收尾后为 "complete"。读到 undefined 会让库的
//     `readyState !== 'loading'` 门禁恒假 —— jQuery 的 ready()、Vue 的
//     mount 时机探测都据此判断「何时可安全操作 DOM」。
//   - scripts 是文档内全部 <script> 的集合。
//   - styleSheets 列出已装配样式表（<style> 提取的 + 运行时注入的 <link>），
//     且 list[0] === list.item(0)（reflector 身份语义），sheet.cssRules.length 可读。
func TestDocumentReadyStateAndCollections(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 200)
	html := `<!DOCTYPE html><html><head>
<style id="s1">.a { color: rgb(1, 2, 3); }</style>
<style id="s2">.b { color: rgb(4, 5, 6); } .c { color: rgb(7, 8, 9); }</style>
<script>window.__rs1 = document.readyState;</script>
</head><body style="margin:0">
<div class="a" style="width:20px;height:10px">A</div>
<script>window.__rs2 = document.readyState;</script>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 4; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got := dynEval(t, wv, `(function(){
  var rs = document.readyState, ss = document.styleSheets;
  var s0 = ss.item(0);
  return "rs=" + rs + "(" + typeof rs + ")"
    + " duringScript1=" + window.__rs1
    + " duringScript2=" + window.__rs2
    + " scripts=" + document.scripts.length
    + " sheets=" + ss.length
    + " item0Type=" + (s0 ? s0.type : "null")
    + " item0Rules=" + (s0 ? s0.cssRules.length : -1)
    + " idxSameItem=" + (ss[0] === s0)
    + " outOfRange=" + (ss.item(99) === null);
})()`)
	t.Logf("document 属性实测 = %s", got)

	if contains(got, "rs=undefined") {
		t.Fatalf("document.readyState 仍为 undefined：%s", got)
	}
	if !contains(got, "rs=complete(string)") {
		t.Fatalf("装载收尾后 readyState 应为 \"complete\"：%s", got)
	}
	// ★ 阶段推进验证：页面脚本执行时（DOMContentLoaded 之前）必须已是
	//   "interactive" —— 证明它是一个真状态机值，而不是加载完统一给的死值。
	if !contains(got, "duringScript1=interactive") || !contains(got, "duringScript2=interactive") {
		t.Fatalf("页面脚本执行时 readyState 应为 \"interactive\"：%s", got)
	}
	if !contains(got, "scripts=2") {
		t.Fatalf("document.scripts.length 应为 2：%s", got)
	}
	if !contains(got, "sheets=2") {
		t.Fatalf("document.styleSheets.length 应为 2（两个 <style>）：%s", got)
	}
	if !contains(got, "item0Type=text/css") {
		t.Fatalf("styleSheets[0].type 应为 \"text/css\"：%s", got)
	}
	if !contains(got, "item0Rules=1") {
		t.Fatalf("styleSheets[0].cssRules.length 应为 1：%s", got)
	}
	if !contains(got, "idxSameItem=true") {
		t.Fatalf("styleSheets[0] 与 styleSheets.item(0) 应为同一对象：%s", got)
	}
	if !contains(got, "outOfRange=true") {
		t.Fatalf("越界 item(99) 应返回 null：%s", got)
	}

	// ★ 动态注入 <style>（Vue scoped CSS 的通道）：下一帧重建后必须出现在
	//   document.styleSheets 里 —— 只报静态表会让库误判「样式尚未就绪」。
	if got2 := dynEval(t, wv, `(function(){
  var s = document.createElement("style");
  s.textContent = ".d { color: rgb(10, 11, 12); }";
  document.head.appendChild(s);
  return "appended";
})()`); got2 != "appended" {
		t.Fatalf("动态插入 <style> 失败：%q", got2)
	}
	for i := 0; i < 4; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got3 := dynEval(t, wv, `document.styleSheets.length + ":" + (document.styleSheets.item(2) ? document.styleSheets.item(2).cssRules.length : -1)`)
	t.Logf("动态 <style> 后 styleSheets = %s", got3)
	if got3 != "3:1" {
		t.Fatalf("动态注入 <style> 后应为 3 张表、末张 1 条规则，实际 %q", got3)
	}
}
