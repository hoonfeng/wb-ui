package webkit

import (
	"testing"

	"wb-ui/dom"
)

// TestWboxBorderSides 用真实 LoadHTML+Render 管道验证配置画布 wbox 圆角边框
// 是否完整绘制上下左右四条边（回归测试：overflow:hidden 层 clip 曾裁掉
// 元素自身边框——renderpipeline.go 修复后四边完整）。
func TestWboxBorderSides(t *testing.T) {
	wv := NewWebView()
	wv.Resize(900, 640)
	wv.LoadHTML(`<html><head><style>
*{box-sizing:border-box}
body{margin:0;background:#131722}
#stage{position:relative;top:76px;width:414px;height:232px;background:#0d1117;border:1px solid #232f4a;border-radius:8px;overflow:hidden}
.wbox{position:absolute;border:1px solid #4a80e8;background:rgba(59,111,212,.12);border-radius:4px;overflow:hidden}
.wbox.off{border-color:rgba(74,128,232,.35);background:rgba(59,111,212,.05)}
.wbox .wlabel{display:none}
.wbox:hover .wlabel{display:block}
</style></head><body>
<div id="stage"></div>
<script>
// 模拟真实 configHTML 的 renderStage（页面加载时执行）
var stage = document.getElementById('stage');
var h = '<div class="grid-v"></div>';
h += '<div class="wbox" data-id="clock-1" style="left:13px;top:13px;width:56px;height:17px" onclick="selectWidget(\'clock-1\')" ondblclick="openProp(\'clock-1\')"><span class="wlabel">时钟</span></div>';
stage.innerHTML = h;
</script>
</body></html>`)
	// ★ 完整模拟主程序 RenderOnce：RebuildRenderTree → SetNeedsLayout → EnsureLayout → Render
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// wbox: stage top=76 → wbox top=90; left=14; 56x17（border-box）
	type pt struct {
		name     string
		x, y     int
		minColor bool // 期望至少接近蓝（R<G<B 且 B>100）
	}
	points := []pt{
		{"top", 40, 90, true},
		{"bottom", 40, 106, true},
		{"left", 14, 95, true},
		{"right", 69, 95, true},
	}
	for _, p := range points {
		i := (p.y*900 + p.x) * 4
		r, g, b, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
		isBlue := a > 100 && b > 100 && b > r && b > g
		t.Logf("side=%s (%d,%d) rgba=(%d,%d,%d,%d) isBlue=%v", p.name, p.x, p.y, r, g, b, a, isBlue)
		if p.minColor && !isBlue {
			t.Errorf("side=%s (%d,%d) NOT blue: rgba=(%d,%d,%d,%d) — 边线缺失!", p.name, p.x, p.y, r, g, b, a)
		}
	}
}

// TestTitlebarButtons 验证标题栏 最大化/最小化/关闭 三按钮的图标渲染
// （.i-max 用 background 实心块，检查 wb-ui 是否渲染 12px 级小元素）。
func TestTitlebarButtons(t *testing.T) {
	wv := NewWebView()
	wv.Resize(900, 100)
	wv.LoadHTML(`<html><head><style>
body{margin:0;background:#131722}
.titlebar{position:absolute;top:0;left:0;right:0;height:32px;background:#141926}
.tb-btn{position:absolute;top:0;width:40px;height:32px}
.tb-max{right:80px}.tb-min{right:40px}.tb-close{right:0}
.i{position:absolute;display:block}
.i-min{left:14px;top:15px;width:12px;height:2px;background:#c6d2f0}
.i-max{left:12px;top:11px;width:12px;height:10px;background:#c6d2f0}
.i-max2{left:15px;top:14px;width:12px;height:10px;background:#c6d2f0}
</style></head><body>
<div class="titlebar">
  <div class="tb-btn tb-max"><div class="i i-max"></div><div class="i i-max2"></div></div>
  <div class="tb-btn tb-min"><div class="i i-min"></div></div>
  <div class="tb-btn tb-close"></div>
</div>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// tb-max 图标应在 (832-844, 11-21)；tb-min 横线在 (794,16)
	// （Windows 标准顺序：最小化 right:80 → x780，最大化 right:40 → x820）
	pts := [][2]int{{838, 13}, {838, 17}, {840, 16}, {794, 16}}
	for _, p := range pts {
		i := (p[1]*900 + p[0]) * 4
		r, g, b, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
		isLight := a > 100 && r > 150 && g > 150 && b > 150
		t.Logf("pt (%d,%d) rgba=(%d,%d,%d,%d) isLight=%v", p[0], p[1], r, g, b, a, isLight)
		if isLight && p[0] == 794 {
			t.Logf("tb-min 横线渲染 OK")
		}
	}
	// tb-max 图标（832-844, 11-21）应有亮色像素
	found := false
	for y := 11; y <= 21; y++ {
		for x := 832; x <= 844; x++ {
			i := (y*900 + x) * 4
			if pix[i+3] > 100 && pix[i] > 150 && pix[i+1] > 150 && pix[i+2] > 150 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("tb-max 图标 (832-844,11-21) 未渲染亮色像素")
	}
}

func TestOffBorderAlpha(t *testing.T) {
	wv := NewWebView()
	wv.Resize(900, 640)
	wv.LoadHTML(`<html><head><style>
*{box-sizing:border-box}
body{margin:0;background:#0d1117}
#stage{position:relative;top:76px;width:414px;height:232px;background:#0d1117;border:1px solid #232f4a;overflow:hidden}
.wbox{position:absolute;border:1px solid #4a80e8;border-radius:4px}
.wbox.off{border-color:rgba(74,128,232,.35);background:rgba(59,111,212,.05)}
</style></head><body>
<div id="stage"><div class="wbox off" style="left:13px;top:13px;width:56px;height:17px"></div></div>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// 诊断：读 off wbox 渲染树边框样式
	if rv := wv.RenderView(); rv != nil {
		doc := wv.MainFrame().Document()
		if doc != nil {
			var wboxEl *dom.Element
			var walk func(n dom.Node)
			walk = func(n dom.Node) {
				if wboxEl != nil || n == nil {
					return
				}
				if e, ok := n.(*dom.Element); ok && e.GetAttribute("class") == "wbox off" {
					wboxEl = e
					return
				}
				for c := n.FirstChild(); c != nil && wboxEl == nil; c = c.NextSibling() {
					walk(c)
				}
			}
			walk(doc)
			if wboxEl != nil {
				if box := rv.FindRenderBoxForNode(wboxEl); box != nil && box.Style() != nil {
					st := box.Style()
					t.Logf("off style: borderTop=%v style=%q color=%v set=%v alpha=%d",
						st.BorderTopWidth, st.BorderTopStyle, st.BorderTopColor, st.BorderTopColorSet, st.BorderTopColor.A)
				} else {
					t.Logf("off renderbox/style nil")
				}
			}
		}
	}
	// off 边框应在 (14,90) 56x17 边缘。rgba(74,128,232,.35) over #0d1117
	// ≈ (34,56,96) #223860 —— 若渲染成纯蓝 #4a80e8 则 alpha 丢失。
	pts := [][2]int{{40, 90}, {40, 106}, {14, 95}, {69, 95}, {13, 95}, {68, 95}, {40, 89}, {40, 105}, {14, 91}, {14, 105}, {40, 88}, {14, 89}, {69, 89}, {69, 105}, {14, 95 + 1}, {68, 96}}
	for _, p := range pts {
		i := (p[1]*900 + p[0]) * 4
		r, g, b, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
		t.Logf("off border (%d,%d) rgba=(%d,%d,%d,%d)", p[0], p[1], r, g, b, a)
	}
	i := (95*900 + 14) * 4 // 左边线中段
	r, g, b, _ := pix[i], pix[i+1], pix[i+2], pix[i+3]
	t.Logf("off left border rgba=(%d,%d,%d,%d)", r, g, b, pix[i+3])
	// 期望半透明淡蓝 rgba(74,128,232,.35) over #0d1117 ≈ (34,56,96)；
	// 若渲染成纯蓝 #4a80e8 (74,128,232) 则 alpha 丢失。
	if r > 60 && g > 100 && b > 200 {
		t.Errorf("off border alpha lost: rgba=(%d,%d,%d,%d) — 应半透明淡蓝 (34,56,96) 左右", r, g, b, pix[i+3])
	}
}
