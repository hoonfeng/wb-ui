package app

// 真实点击链路（挂起事件 → Release 分支）的表单状态样式失效回归（2026-09）：
//
//	host.handleClick → handleLabelToggle → webkit.ToggleLabeledControl
//	                        → ToggleCheckboxRadio（切换 + change 派发）
//
// 判据：① checkbox 状态翻转；② change 抵达 JS（Vue v-model 靠它同步）；
// ③ 相邻 `#c:checked + #track` 的像素重绘——状态失效必须走到画面，而不只是
// JS 侧读数。
//
// 与 webkit/formstate_invalidation_test.go 的分工：那边验证引擎内部
// SetChecked / IDL setter / setAttribute 三条失效链路；这里验证 **app 宿主
// 的真实点击链路**（含 label 转发与 win=nil 的测试宿主路径）。
//
// 实测基准（2026-09-28 headless）：
//
//	点击 #track → checked=true、change=1、像素 rgb(7,7,7) → rgb(5,5,5)
//	再点一次   → checked=false、change=2、像素 rgb(5,5,5) → rgb(7,7,7)

import (
	"testing"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/engine/platform/window"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// labelToggleFixture：**真实 IDE 开关组件**（plugins-src/ui-app/src/components/
// SettingsModal.vue 的 .pp-switch）的同构简化版——flex 行的 `.field` 内放
// `label.pp-switch`，label 内是绝对定位隐藏的 input + 兄弟 track，
// `input:checked + .pp-switch-track` 决定滑块底色。它同时提供两个判据：
// ① 点击 label 内 track 的转发语义；② 状态属性变更是否触发**相邻**元素
// （`+` 兄弟组合器）重绘。
//
// 注意外层 flex 行不可省：CSS 会把 flex 容器的子项 blockify（inline-flex
// label → flex、inline-block span → block），真实 IDE 的开关正是画在 flex
// 行里的；若把 label 直接挂在 body 下，引擎的 inline 布局不会给它生成
// box（引擎只对 block 级与 replaced 元素建 box），点击只能命中 body。
const labelToggleFixture = `<!DOCTYPE html><html><head><style>
body{margin:0}
.field{display:flex;align-items:center}
.pp-switch{position:relative;display:inline-flex;align-items:center;cursor:pointer}
.pp-switch input{position:absolute;opacity:0;width:0;height:0}
.pp-switch-track{display:inline-block;width:60px;height:30px;background:rgb(7,7,7)}
.pp-switch input:checked + .pp-switch-track{background:rgb(5,5,5)}
</style></head><body><div class="field"><label class="pp-switch" id="lb"><input type="checkbox" id="c"><span class="pp-switch-track" id="track"></span></label></div></body></html>`

// hostFormPixels 连续渲染几帧（首帧可能仍是上一次的缓存）后返回 RGBA8888 像素。
func hostFormPixels(t *testing.T, wv *webkit.WebView) []byte {
	t.Helper()
	var pix []byte
	for i := 0; i < 3; i++ {
		p, err := wv.Render()
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		pix = p
		time.Sleep(15 * time.Millisecond)
	}
	return pix
}

// hostFormPixel 读 Render() 输出（RGBA8888，1 像素 4 字节）中 (x,y) 的 RGB。
func hostFormPixel(t *testing.T, wv *webkit.WebView, x, y int) (r, g, b uint8) {
	t.Helper()
	pix := hostFormPixels(t, wv)
	i := (y*wv.Width() + x) * 4
	if i < 0 || i+2 >= len(pix) {
		t.Fatalf("像素 (%d,%d) 越界（w=%d len=%d）", x, y, wv.Width(), len(pix))
	}
	return pix[i], pix[i+1], pix[i+2]
}

// hostFormClick 走真实 Release 链路点击视口 CSS 坐标 (x,y)。
func hostFormClick(h *Host, rv *rendering.RenderView, x, y float64) {
	h.handleClick(rv, window.Event{
		Type:   window.EventMouseButton,
		X:      x,
		Y:      y,
		Button: int(glfw.MouseButton1),
		Action: int(glfw.Release),
	})
}

// TestHostClickLabelTogglesCheckboxAndRepaints：点击 label 内的开关滑块，
// checkbox 必须切换、派发 change，并让相邻 `:checked` 规则重绘到画面。
func TestHostClickLabelTogglesCheckboxAndRepaints(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(400, 200)
	if err := wv.LoadHTML(labelToggleFixture); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	rv := cursorTestEnv(t, wv)
	if rv == nil {
		t.Fatal("RenderView() = nil")
	}
	h := NewHostForTest(wv, 400, 200)

	// change 监听（Vue v-model 等靠 change 同步 checked 状态）
	if _, err := wv.JSInterpreter().RunJS(`
		window.__chg = 0;
		document.getElementById('c').addEventListener('change', function(){ window.__chg++; });
	`); err != nil {
		t.Fatalf("RunJS 注册 change: %v", err)
	}

	track := wv.Document().GetElementById("track")
	if track == nil {
		t.Fatal("#track 缺失")
	}
	box := rv.FindRenderBoxForNode(track)
	if box == nil {
		t.Fatal("#track 无 render box")
	}
	px := int(box.AbsoluteX() + box.Width()/2)
	py := int(box.AbsoluteY() + box.Height()/2)

	// 命中判据：点击点必须落在 label 子树内（label 转发的前提）——
	// 命中 label 本身、隐藏 input 或 track 都能完成切换，命中 label 之外的
	// 元素（如 body）则点击到不了开关。
	hit := rendering.HitTest(rv, float64(px), float64(py), "")
	if hit == nil {
		t.Fatalf("HitTest(%d,%d) = nil", px, py)
	}
	inside := false
	for e := hit; e != nil; e = e.ParentElement() {
		if e.GetAttribute("id") == "lb" {
			inside = true
			break
		}
	}
	if !inside {
		t.Fatalf("HitTest(%d,%d) 命中 %s#%s（class=%q），不在 label 子树内——点击到不了开关",
			px, py, hit.LocalName(), hit.GetAttribute("id"), hit.ClassName())
	}

	if r, g, b := hostFormPixel(t, wv, px, py); r != 7 || g != 7 || b != 7 {
		t.Fatalf("初始像素=(%d,%d,%d)，want (7,7,7)（夹具没生效，后续断言无意义）", r, g, b)
	}

	// ① 点击 → 选中
	hostFormClick(h, rv, float64(px), float64(py))
	if v, err := wv.JSInterpreter().RunJS(`String(document.getElementById('c').checked)`); err != nil || v.ToString() != "true" {
		t.Errorf("点击后 checked=%v (err=%v)，want true（label 转发未生效）", v, err)
	}
	if v, err := wv.JSInterpreter().RunJS(`String(window.__chg)`); err != nil || v.ToString() != "1" {
		t.Errorf("点击后 change 次数=%v (err=%v)，want 1（change 未派发，v-model 不会同步）", v, err)
	}
	if r, g, b := hostFormPixel(t, wv, px, py); r != 5 || g != 5 || b != 5 {
		t.Errorf("点击后像素=(%d,%d,%d)，want (5,5,5)——相邻 :checked 规则未重绘", r, g, b)
	}

	// ② 再点一次 → 取消选中（同样要重绘）
	hostFormClick(h, rv, float64(px), float64(py))
	if v, err := wv.JSInterpreter().RunJS(`String(document.getElementById('c').checked)`); err != nil || v.ToString() != "false" {
		t.Errorf("二次点击后 checked=%v (err=%v)，want false", v, err)
	}
	if v, err := wv.JSInterpreter().RunJS(`String(window.__chg)`); err != nil || v.ToString() != "2" {
		t.Errorf("二次点击后 change 次数=%v (err=%v)，want 2", v, err)
	}
	if r, g, b := hostFormPixel(t, wv, px, py); r != 7 || g != 7 || b != 7 {
		t.Errorf("二次点击后像素=(%d,%d,%d)，want (7,7,7)——取消选中未重绘", r, g, b)
	}
}
