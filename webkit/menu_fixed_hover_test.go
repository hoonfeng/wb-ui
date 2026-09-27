package webkit

import (
	"strconv"
	"strings"
	"testing"
)

// TestFixedMenuHoverKeepsMenuOpen 钉死 PairCode 顶栏「帮助」菜单的两个回归：
//
//	① 命中测试与绘制层叠一致：position:fixed + z-index:9999 的下拉面板（DOM
//	   上位于 .titlebar{z-index:100} 之内，绘制上被提升到根层叠上下文）在其
//	   区域内必须优先命中面板自身与菜单项，不得穿透到下层内容；真实点击菜单项
//	   必须真正派发到菜单项（click 计数递增）。
//	② hover 语义完整：指针从面板 padding（命中面板自身）移入菜单项时，面板收到
//	   mouseenter 且**不得**收到 mouseleave；只有指针真正移出菜单才派 mouseleave。
//	   Windows 宿主把 @mouseleave 绑在面板上（hover-to-close，200ms 关闭定时器），
//	   修复前这一步误派 mouseleave → 菜单在鼠标移到菜单项上约 200ms 后自消失。
//
// 复刻真实结构：面板带 4px padding（鼠标从面板边缘进入时先命中面板自身），
// DOM 中另有 fixed 定位的 modals-host(z-index:200)/escape-btn(z-index:300)
// 位于菜单之后，确保「fixed 层按树序而非 z-index 编号」的路径被覆盖。
func TestFixedMenuHoverKeepsMenuOpen(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(1400, 900)
	html := `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0}
#titlebar{position:relative;z-index:100;height:40px;background:#111;color:#ddd}
#menubar{display:flex;height:40px;align-items:center}
#help-btn{height:24px;padding:0 8px}
#menu{position:fixed;left:8px;top:28px;width:200px;background:#fff;color:#000;padding:4px;z-index:9999}
.menu-item{height:32px;line-height:32px;padding-left:8px}
#content{height:600px;background:#eee}
#under{height:600px;background:#ddd}
#modals-host{position:fixed;left:0;top:0;right:0;bottom:0;z-index:200;pointer-events:none}
#escape-btn{position:fixed;left:1300px;top:0;width:80px;height:30px;z-index:300}
</style></head><body>
<div id="titlebar"><div id="menubar"><button id="help-btn">帮助</button></div></div>
<div id="content"><div id="under">under</div></div>
<div id="modals-host"></div>
<div id="escape-btn">esc</div>
<script>
window.__log = [];
window.__menu = null;
document.getElementById('help-btn').addEventListener('click', function(){
  if (window.__menu) return;
  var m = document.createElement('div');
  m.id = 'menu';
  m.innerHTML = '<div class="menu-item" id="mi1">常见问题</div><div class="menu-item" id="mi2">快速开始</div>';
  document.getElementById('menubar').appendChild(m);
  window.__menu = m;
  m.addEventListener('mouseleave', function(){ window.__log.push('menu-mouseleave'); });
  m.addEventListener('mouseenter', function(){ window.__log.push('menu-mouseenter'); });
  document.getElementById('mi1').addEventListener('click', function(){ window.__log.push('click-mi1'); });
  document.getElementById('under').addEventListener('click', function(){ window.__log.push('click-under'); });
});
</script>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	settle := func() {
		for i := 0; i < 6; i++ {
			wv.EnsureLayout()
			if _, err := wv.Render(); err != nil {
				t.Fatalf("Render: %v", err)
			}
		}
	}
	settle()
	eval := func(js string) string {
		v, err := wv.EvalJS(js)
		if err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		return v.ToString()
	}
	parseXY := func(s string) (float64, float64) {
		parts := strings.Split(s, ",")
		if len(parts) != 2 {
			t.Fatalf("bad coords %q", s)
		}
		x, _ := strconv.ParseFloat(parts[0], 64)
		y, _ := strconv.ParseFloat(parts[1], 64)
		return x, y
	}
	logStr := func() string { return eval(`window.__log.join('|')`) }

	// 真实点击「帮助」按钮打开菜单（走 HitTest + 真实事件派发）。
	btnXY := eval(`(function(){var r=document.getElementById('help-btn').getBoundingClientRect();return (r.left+r.width/2)+','+(r.top+r.height/2);})()`)
	bx, by := parseXY(btnXY)
	wv.HandleMouseButton(bx, by, 0, 0)
	wv.HandleMouseButton(bx, by, 0, 1)
	settle()
	if eval(`window.__menu ? 'yes' : 'no'`) != "yes" {
		t.Fatalf("点击「帮助」未创建菜单（点击未达按钮）")
	}

	// ① 命中测试与绘制层叠一致。
	hit := eval(`(function(){
  function at(x,y){ var e=document.elementFromPoint(x,y); return e ? (e.id || e.tagName) : 'null'; }
  var m=document.getElementById('menu').getBoundingClientRect();
  var i1=document.getElementById('mi1').getBoundingClientRect();
  var i2=document.getElementById('mi2').getBoundingClientRect();
  return 'panelPadding=' + at(m.left+2, m.top+2)
       + ' mi1Area=' + at(i1.left+i1.width/2, i1.top+i1.height/2)
       + ' mi2Area=' + at(i2.left+i2.width/2, i2.top+i2.height/2)
       + ' contentArea=' + at(700, 400);
})()`)
	t.Logf("命中测试 = %s", hit)
	for _, want := range []string{"panelPadding=menu", "mi1Area=mi1", "mi2Area=mi2", "contentArea=under"} {
		if !strings.Contains(hit, want) {
			t.Errorf("命中测试缺失 %q（fixed+z-index 提升层未优先命中）：%s", want, hit)
		}
	}

	// ② 鼠标移入菜单项：菜单必须保持打开（无 mouseleave，且面板收到 enter）。
	mi1XY := eval(`(function(){var r=document.getElementById('mi1').getBoundingClientRect();return (r.left+r.width/2)+','+(r.top+r.height/2);})()`)
	mx, my := parseXY(mi1XY)
	// 先进入面板 padding（命中面板自身），再移入菜单项 —— 用户实测路径。
	mPos := eval(`(function(){var r=document.getElementById('menu').getBoundingClientRect();return (r.left+2)+','+(r.top+2);})()`)
	px, py := parseXY(mPos)
	wv.HandleMouseMove(px, py)
	settle()
	wv.HandleMouseMove(mx, my)
	settle()
	afterHover := logStr()
	t.Logf("移入菜单项后 log = %q", afterHover)
	if !strings.Contains(afterHover, "menu-mouseenter") {
		t.Errorf("面板未收到 mouseenter（进入链未沿祖先派发）：log=%q", afterHover)
	}
	if strings.Contains(afterHover, "menu-mouseleave") {
		t.Errorf("鼠标仍在菜单内却派发 mouseleave（菜单会 200ms 后自消失）：log=%q", afterHover)
	}

	// ③ 真实点击菜单项：事件必须落到菜单项，不得穿透到下层内容。
	wv.HandleMouseButton(mx, my, 0, 0)
	wv.HandleMouseButton(mx, my, 0, 1)
	settle()
	afterClick := logStr()
	t.Logf("点击菜单项后 log = %q", afterClick)
	if !strings.Contains(afterClick, "click-mi1") {
		t.Errorf("点击未落到菜单项（穿透到下层）：log=%q", afterClick)
	}
	if strings.Contains(afterClick, "click-under") {
		t.Errorf("点击穿透到下层内容：log=%q", afterClick)
	}

	// ④ 指针真正移出菜单（到内容区）时才必须派 mouseleave。
	wv.HandleMouseMove(700, 400)
	settle()
	afterLeave := logStr()
	t.Logf("移出菜单后 log = %q", afterLeave)
	if !strings.Contains(afterLeave, "menu-mouseleave") {
		t.Errorf("指针离开菜单未派 mouseleave：log=%q", afterLeave)
	}
}
