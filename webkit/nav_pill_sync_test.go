package webkit

import "testing"

// TestTopNavPillSwitchSync 钉死 (b) 顶栏导航胶囊切换：点击胶囊后
// `.active` 高亮与视图 display 必须**同步**更新（浏览器行为）。
// 断言可量化：`.tb-nav-pill.active` 的 id、两个视图的 computed display、
// document.activeElement。
func TestTopNavPillSwitchSync(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="nav">
  <button class="tb-nav-pill active" id="p0" style="width:60px;height:30px">A</button>
  <button class="tb-nav-pill" id="p1" style="width:60px;height:30px">B</button>
</div>
<div id="view0" style="height:50px">viewA</div>
<div id="view1" style="height:50px;display:none">viewB</div>
<script>
var pills = document.querySelectorAll('.tb-nav-pill');
for (var i = 0; i < pills.length; i++) {
  pills[i].addEventListener('click', function() {
    for (var j = 0; j < pills.length; j++) { pills[j].classList.remove('active'); }
    this.classList.add('active');
    var idx = (this.id === 'p0') ? 0 : 1;
    document.getElementById('view0').style.display = (idx === 0) ? 'block' : 'none';
    document.getElementById('view1').style.display = (idx === 1) ? 'block' : 'none';
  });
}
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
	snapshot := func() string {
		got, err := wv.EvalJS(`(function(){
  var act = document.querySelectorAll('.tb-nav-pill.active');
  var a = (act.length === 1) ? act[0].id : ('count=' + act.length);
  var v0 = getComputedStyle(document.getElementById('view0')).display;
  var v1 = getComputedStyle(document.getElementById('view1')).display;
  var ae = document.activeElement;
  return 'active=' + a + ' | view0.display=' + v0 + ' | view1.display=' + v1 +
         ' | activeElement=' + (ae ? (ae.id || ae.tagName) : 'null');
})()`)
		if err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		return got.ToString()
	}
	before := snapshot()
	t.Logf("点击前 = %s", before)
	if !contains(before, "active=p0") || !contains(before, "view0.display=block") {
		t.Fatalf("初始状态错误：%s", before)
	}
	// 点击第二个胶囊（p1 中心 x=60+30=90, y=15）
	wv.HandleMouseButton(90, 15, 0, 0)
	wv.HandleMouseButton(90, 15, 0, 1)
	for i := 0; i < 6; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	after := snapshot()
	t.Logf("点击 p1 后 = %s", after)
	if !contains(after, "active=p1") {
		t.Fatalf("点击后 .active 未同步到 p1：%s", after)
	}
	if !contains(after, "view0.display=none") || !contains(after, "view1.display=block") {
		t.Fatalf("点击后视图 display 未同步切换：%s", after)
	}
}
