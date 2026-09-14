package app

import (
	"testing"

	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// TestContextMenuDispatch 验证右键释放派发 contextmenu 事件（浏览器标准）：
// ① 命中元素上的 addEventListener('contextmenu') 监听器被触发
// ② preventDefault() 有效（defaultPrevented 翻转）
// ③ 事件可冒泡到 document（App.vue 的全局 contextmenu 监听依赖冒泡）
func TestContextMenuDispatch(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	wv.LoadHTML(`<html><body style="font-family:sans-serif; font-size:14px">
		<div id="target" style="width:200px;height:80px;background:#eee">target</div>
	</body></html>`)
	cursorTestEnv(t, wv)

	rv := wv.RenderView()
	t.Logf("rv=%v layoutState=%v", rv != nil, rv != nil && rv.LayoutState() != nil)
	if rv != nil && rv.LayoutState() != nil {
		if lb := rv.LayoutBox(); lb != nil {
			g := rv.LayoutState().GeometryForBox(lb)
			t.Logf("layoutRoot=(%.0f,%.0f %.0fx%.0f)", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
		}
	}

	if _, err := wv.JSInterpreter().RunJS(`
		window.__ctx = { fired: false, onTarget: false, bubbled: false, prevented: false, btn: -1, x: -1, y: -1 };
		document.getElementById('target').addEventListener('contextmenu', function(e){
			window.__ctx.fired = true;
			window.__ctx.onTarget = true;
			window.__ctx.btn = e.button;
			window.__ctx.x = e.clientX;
			window.__ctx.y = e.clientY;
			e.preventDefault();
		});
		document.addEventListener('contextmenu', function(e){
			window.__ctx.bubbled = true;
			window.__ctx.prevented = e.defaultPrevented;
		});
	`); err != nil {
		t.Fatalf("RunJS setup: %v", err)
	}

	h := NewHostForTest(wv, 800, 600)
	rv2 := wv.RenderView()
	hitDirect := rendering.HitTest(rv2, 60, 40, "")
	if hitDirect == nil {
		t.Logf("direct HitTest(60,40) = nil (渲染树可能未布局)")
	} else {
		t.Logf("direct HitTest(60,40) = %s.%s", hitDirect.LocalName(), hitDirect.GetAttribute("class"))
	}
	hit := h.MockContextMenu(wv, 60, 40)
	if hit == nil {
		t.Fatal("MockContextMenu hit nil")
	}
	if hit.LocalName() != "div" {
		t.Fatalf("hit=%s want div", hit.LocalName())
	}

	v, err := wv.JSInterpreter().RunJS(`JSON.stringify(window.__ctx)`)
	if err != nil {
		t.Fatalf("RunJS read: %v", err)
	}
	got := v.ToString()
	if !containsStr(got, `"fired":true`) {
		t.Errorf("contextmenu listener NOT fired, state=%s", got)
	}
	if !containsStr(got, `"bubbled":true`) {
		t.Errorf("contextmenu NOT bubbled to document, state=%s", got)
	}
	if !containsStr(got, `"prevented":true`) {
		t.Errorf("defaultPrevented not visible after preventDefault(), state=%s", got)
	}
	if !containsStr(got, `"btn":2`) {
		t.Errorf("event.button != 2 (right), state=%s", got)
	}
	if !containsStr(got, `"x":60`) || !containsStr(got, `"y":40`) {
		t.Errorf("clientX/clientY wrong, state=%s", got)
	}
}

// TestContextMenuNotClick 验证右键不派发 click（浏览器标准：右键只触发
// contextmenu，不触发 click）。
func TestContextMenuNotClick(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	wv.LoadHTML(`<html><body style="font-family:sans-serif; font-size:14px">
		<div id="target" style="width:200px;height:80px;background:#eee">target</div>
	</body></html>`)
	cursorTestEnv(t, wv)

	if _, err := wv.JSInterpreter().RunJS(`
		window.__clicks = 0;
		document.getElementById('target').addEventListener('click', function(){ window.__clicks++; });
	`); err != nil {
		t.Fatalf("RunJS setup: %v", err)
	}

	h := NewHostForTest(wv, 800, 600)
	h.MockContextMenu(wv, 60, 40)

	v, err := wv.JSInterpreter().RunJS(`window.__clicks`)
	if err != nil {
		t.Fatalf("RunJS read: %v", err)
	}
	if n := v.ToNumber(); n != 0 {
		t.Errorf("right-click fired %d click events, want 0", int(n))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
