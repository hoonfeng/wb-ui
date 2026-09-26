package webkit

import "testing"

// TestMenuDropdownClickHitsTopmostItem 钉死 (c) 菜单展开后的点击穿透问题：
// 展开的下拉菜单必须浮在下层内容之上 —— 菜单项区域的命中测试必须返回菜单项
// 自身（浏览器 z 序语义），且点击真正派发到菜单项（事件计数递增）。
// 断言可量化：elementFromPoint 返回的 id、菜单项 click 计数、菜单容器外坐标的命中。
func TestMenuDropdownClickHitsTopmostItem(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="host" style="position:relative;width:300px;height:200px">
  <div id="under" style="position:absolute;left:0;top:0;width:300px;height:200px">under</div>
  <div id="menu" style="position:absolute;left:20px;top:20px;width:120px;height:80px;
       z-index:10;background:#fff">
    <div class="menu-item" id="i1" style="width:120px;height:30px">item1</div>
    <div class="menu-item" id="i2" style="width:120px;height:30px">item2</div>
  </div>
</div>
<script>
window.__hits = [];
document.getElementById('i1').addEventListener('click', function(){ window.__hits.push('i1'); });
document.getElementById('i2').addEventListener('click', function(){ window.__hits.push('i2'); });
document.getElementById('under').addEventListener('click', function(){ window.__hits.push('under'); });
</script>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	eval := func(js string) string {
		v, err := wv.EvalJS(js)
		if err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		return v.ToString()
	}
	hit := eval(`(function(){
  function at(x,y){ var e=document.elementFromPoint(x,y); return e ? (e.id || e.tagName) : 'null'; }
  return 'item1Area=' + at(80, 35) + ' item2Area=' + at(80, 65) + ' outsideMenu=' + at(260, 180);
})()`)
	t.Logf("命中测试 = %s", hit)
	if !contains(hit, "item1Area=i1") {
		t.Fatalf("菜单项区域命中错误（被下层截获）：%s", hit)
	}
	if !contains(hit, "item2Area=i2") {
		t.Fatalf("第二菜单项区域命中错误：%s", hit)
	}
	if !contains(hit, "outsideMenu=under") {
		t.Fatalf("菜单外区域应命中下层 #under：%s", hit)
	}
	// 真实点击 i1 中心（20+60=80, 20+15=35），事件必须落到 i1 而非 under
	wv.HandleMouseButton(80, 35, 0, 0)
	wv.HandleMouseButton(80, 35, 0, 1)
	for i := 0; i < 4; i++ {
		wv.EnsureLayout()
		wv.Render()
	}
	hits := eval(`(function(){ return window.__hits.join(','); })()`)
	t.Logf("点击后事件序列 = %q", hits)
	if hits != "i1" {
		t.Fatalf("点击落点错误：got %q, want \"i1\"（菜单展开时不应有事件穿透到下层）", hits)
	}
}
