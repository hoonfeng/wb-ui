package rendering

// z-index 层叠命中测试探针：paint 层按 layer 树排序（z 大者绘制在上，
// TestZIndexHigherPaintsOnTop 已覆盖），但命中测试 Pass2（hitTestWalk）
// 只按树序+最小面积——absolute 定位 + z-index 的浮层（mask 999 / panel
// 1000）命中顺序与视觉不符：点击浮层按钮应命中按钮（最上层），实测
// 会命中底层遮罩（mask 的 onclick=close 先被选中）——「弹窗不在最前/
// 点击确定无效」根因。

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

func dialogHTML() string {
	return `<html><head><style>
body { margin: 0; }
.cp-mask{position:absolute;left:0;top:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:999}
.cp-panel{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);background:#1a2030;border:1px solid #35456e;border-radius:8px;padding:14px;width:280px;z-index:1000}
.cp-actions{display:flex;gap:6px;margin-top:8px}
.cp-actions button{flex:1;padding:5px 0;font-size:14px;border-radius:4px}
</style></head><body>
<div class="cp-mask" id="m" onclick="window.__hit='mask'"></div>
<div class="cp-panel" id="p">
  <div class="cp-title">新建配置</div>
  <input class="np-input" id="npInput" value="配置-5">
  <div class="cp-actions">
    <button class="cp-ok" id="okbtn" onclick="window.__hit='ok'">确定</button>
    <button class="cp-cancel" id="ccbtn" onclick="window.__hit='cc'">取消</button>
  </div>
</div>
</body></html>`
}

func buildDialogRV(t *testing.T) (*RenderView, *dom.Document) {
	t.Helper()
	doc, err := html.ParseDocument(dialogHTML())
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(dialogHTML()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)
	return rv, doc
}

func btnC(t *testing.T, rv *RenderView, el *dom.Element) (float64, float64) {
	t.Helper()
	b := rv.FindRenderBoxForNode(el)
	if b == nil {
		t.Fatalf("no render box for %s", el.GetAttribute("id"))
	}
	return b.AbsoluteX() + b.Width()/2, b.AbsoluteY() + b.Height()/2
}

// TestZIndexHitTestButton 弹窗 z-index:1000 的确定按钮应命中（而非
// z-index:999 的全屏遮罩）——浏览器标准：点击最上层元素。
// ★ 面板带 transform:translate(-50%,-50%)：点击位置用「视觉坐标」
//（布局中心 -140,-61），模拟真实用户点击（命中测试经 transform 逆变换）。
func TestZIndexHitTestButton(t *testing.T) {
	rv, doc := buildDialogRV(t)
	btn := doc.GetElementById("okbtn")
	// 布局中心
	bx, by := btnC(t, rv, btn)
	// 视觉中心 = 布局中心 + translate(-50%,-50%) 偏移（panel 280x122）
	vx, vy := bx-140, by-61
	t.Logf("okbtn 布局中心(%.0f,%.0f) 视觉中心(%.0f,%.0f)", bx, by, vx, vy)
	got := HitTest(rv, vx, vy, "onclick")
	if got == nil {
		t.Fatalf("HitTest nil at (%.0f,%.0f)", vx, vy)
	}
	t.Logf("HitTest(onclick) @视觉中心 = id=%s onclick=%q", got.GetAttribute("id"), got.GetAttribute("onclick"))
	if got.GetAttribute("id") != "okbtn" {
		t.Fatalf("应命中确定按钮（视觉位置，z-index 1000 最上层），实得 id=%s onclick=%q", got.GetAttribute("id"), got.GetAttribute("onclick"))
	}
	// 弹窗外（遮罩空白）应命中 mask
	got2 := HitTest(rv, 20, 400, "onclick")
	if got2 == nil || got2.GetAttribute("id") != "m" {
		t.Fatalf("弹窗外应命中 mask，实得 %v", got2)
	}
	// 取消按钮同样应命中（视觉中心）
	cc := doc.GetElementById("ccbtn")
	ccx, ccy := btnC(t, rv, cc)
	got3 := HitTest(rv, ccx-140, ccy-61, "onclick")
	t.Logf("HitTest(@cancel 视觉) = id=%s onclick=%q", got3.GetAttribute("id"), got3.GetAttribute("onclick"))
	if got3 == nil || got3.GetAttribute("id") != "ccbtn" {
		t.Fatalf("应命中取消按钮，实得 %v", got3)
	}
	// 按钮中心深度命中（attrName="" 时返回最深元素，也应是 btn 内元素）
	d := HitTest(rv, vx, vy, "")
	if d == nil || d.GetAttribute("id") != "okbtn" {
		t.Fatalf("深度命中应返回 okbtn，实得 %v", d)
	}
}

// TestZIndexHitTestThroughMask 弹窗遮罩下层存在小面积元素（画布 wbox 等）
// 时点击遮罩空白区：应命中最上层的 mask（z-index 999），**不得穿透**到
// 下层带 onclick 的小元素（用户实机反馈「事件穿透到下层」——点弹窗
// 空白处选中了画布挂件）。
func TestZIndexHitTestThroughMask(t *testing.T) {
	src := `<html><head><style>
body { margin: 0; }
/* 下层画布小元素（模拟 wbox：absolute + z auto，带 onclick） */
.wbox{position:absolute;left:200px;top:200px;width:160px;height:60px;background:rgba(59,111,212,.12)}
/* 弹窗：全屏遮罩 z999 + 面板 z1000 */
.cp-mask{position:absolute;left:0;top:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:999}
.cp-panel{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);background:#1a2030;border:1px solid #35456e;border-radius:8px;padding:14px;width:280px;z-index:1000}
.cp-actions{display:flex;gap:6px;margin-top:8px}
.cp-actions button{flex:1;padding:5px 0;font-size:14px;border-radius:4px}
</style></head><body>
<div class="wbox" id="wx" onclick="window.__wbox=1"></div>
<div id="treebtn" onclick="window.__tree=1" style="position:absolute;left:20px;top:700px;width:100px;height:30px"></div>
<div class="cp-mask" id="m" onclick="window.__hit='mask'"></div>
<div class="cp-panel" id="p">
  <div class="cp-title">新建配置</div>
  <div class="cp-actions">
    <button class="cp-ok" id="okbtn" onclick="window.__hit='ok'">确定</button>
    <button class="cp-cancel" id="ccbtn" onclick="window.__hit='cc'">取消</button>
  </div>
</div>
</body></html>`
	doc, err := html.ParseDocument(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(src).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	// 1) 点击 wbox 区域（被遮罩覆盖）：最上层是 mask → 应命中 mask，
	//    不得穿透到 wbox。
	wx := doc.GetElementById("wx")
	cx, cy := btnC(t, rv, wx)
	got := HitTest(rv, cx, cy, "onclick")
	t.Logf("点击 wbox 中心(%.0f,%.0f) → 命中 id=%s onclick=%q", cx, cy, got.GetAttribute("id"), got.GetAttribute("onclick"))
	if got.GetAttribute("id") != "m" {
		t.Fatalf("遮罩下的小元素被穿透命中（应命中 mask z999，实得 %s）", got.GetAttribute("id"))
	}
	// 2) 点击树下按钮区域（同样被遮罩覆盖）
	tb := doc.GetElementById("treebtn")
	cx2, cy2 := btnC(t, rv, tb)
	got2 := HitTest(rv, cx2, cy2, "onclick")
	t.Logf("点击树按钮(%.0f,%.0f) → 命中 id=%s", cx2, cy2, got2.GetAttribute("id"))
	if got2.GetAttribute("id") != "m" {
		t.Fatalf("遮罩下的树按钮被穿透命中（应命中 mask，实得 %s）", got2.GetAttribute("id"))
	}
}

// TestZIndexHitTestSpanNested 无浮层时深层子元素（后序 DOM）正常命中。
func TestZIndexHitTestSpanNested(t *testing.T) {
	src := `<html><head><style>body{margin:0}
div{position:absolute;left:0;top:0;width:200px;height:200px;z-index:1}
span{display:block;width:50px;height:50px}</style></head><body>
<div onclick="window.__hit='a'"><span id="s" onclick="window.__hit='b'">b</span></div>
</body></html>`
	doc, err := html.ParseDocument(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(src).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(800, 600)
	rv.Layout(nil)
	s := doc.GetElementById("s")
	cx, cy := btnC(t, rv, s)
	hit := HitTest(rv, cx, cy, "onclick")
	if hit == nil || hit.GetAttribute("id") != "s" {
		t.Fatalf("应命中 span#s，实得 %v", hit)
	}
}
