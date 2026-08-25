package webkit

// Interaction 引擎级交互管线测试：select 弹层/click/onclick 属性/dblclick/
// hover/checkbox（无窗口，直接操作 WebView + Interaction）。

import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/rendering"
)

func interactTestWebView(t *testing.T, src string) *WebView {
	t.Helper()
	wv := NewWebView()
	wv.Resize(400, 300)
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if wv.Interaction() == nil {
		t.Fatal("Interaction() = nil")
	}
	// 初始布局
	wv.EnsureHitTestReady()
	return wv
}

// TestInteractionSelectPopup 点击 select → 弹层打开；点 option → value 变化
// + change 事件；点外关闭。
func TestInteractionSelectPopup(t *testing.T) {
	wv := interactTestWebView(t, `<select id="s">
<option value="">自动（第一个设备）</option>
<option value="HD Webcam">HD Webcam</option>
<option value="视频连麦">视频连麦</option>
</select>
<script>
window.__chg = '';
document.getElementById('s').addEventListener('change', function(e){
  window.__chg = document.getElementById('s').value;
});
</script>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	if sel == nil {
		t.Fatal("select #s not found")
	}
	// 打开弹层：点击 select 中心
	box := findBox(wv, sel)
	if box == nil {
		t.Fatal("select render box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0) // press（命中 select → 弹层打开）
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	popEls := collectByClass(doc, "select-popup")
	if len(popEls) == 0 {
		t.Fatal("select-popup 未创建（点击 select 无弹层）")
	}
	// 弹层选项数 = 3
	opts := collectByClass(doc, "select-popup-option")
	if len(opts) != 3 {
		t.Fatalf("弹层选项数=%d 期望 3", len(opts))
	}
	if opts[0].GetAttribute("data-value") != "" {
		t.Fatalf("首项 data-value=%q 期望空串（自动）", opts[0].GetAttribute("data-value"))
	}
	// 点击第二项（HD Webcam）→ value 更新 + change
	obox := findBox(wv, opts[1])
	if obox == nil {
		t.Fatal("option render box not found")
	}
	ox, oy := obox.Center()
	wv.HandleMouseButton(ox, oy, 0, 0) // press：popup 内 option → 选择+关闭
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render2: %v", err)
	}
	if v, err := wv.EvalJS(`document.getElementById('s').value`); err == nil {
		if v.ToString() != "HD Webcam" {
			t.Fatalf("select.value=%q 期望 HD Webcam", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS value: %v", err)
	}
	if v, err := wv.EvalJS(`window.__chg`); err == nil {
		if v.ToString() != "HD Webcam" {
			t.Fatalf("change 事件未同步: %q", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS chg: %v", err)
	}
	if len(collectByClass(doc, "select-popup")) != 0 {
		t.Fatal("选择后弹层未关闭")
	}
	// 重新打开 → 点外部关闭
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.RebuildRenderTree()
	if len(collectByClass(doc, "select-popup")) == 0 {
		t.Fatal("第二次点击未重新打开弹层")
	}
	wv.HandleMouseButton(5, 5, 0, 0)
	wv.RebuildRenderTree()
	if len(collectByClass(doc, "select-popup")) != 0 {
		t.Fatal("点击外部后弹层未关闭")
	}
}

// TestInteractionClickOnclick 点击带 onclick 属性的元素 → 内联脚本执行。
func TestInteractionClickOnclick(t *testing.T) {
	wv := interactTestWebView(t, `<button id="b" onclick="window.__clicks=(window.__clicks||0)+1">btn</button>`)
	doc := wv.Document()
	btn := doc.GetElementById("b")
	if btn == nil {
		t.Fatal("button #b not found")
	}
	box := findBox(wv, btn)
	if box == nil {
		t.Fatal("button box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0) // press
	wv.HandleMouseButton(cx, cy, 0, 1) // release → click
	if v, err := wv.EvalJS(`window.__clicks||0`); err == nil {
		if v.ToString() != "1" {
			t.Fatalf("onclick 执行次数=%s 期望 1", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS: %v", err)
	}
}

// TestInteractionDblclick 两次连续点击（400ms 内）→ onclick 执行两次
// （浏览器标准：每次 click 都执行）+ 派发 dblclick。
func TestInteractionDblclick(t *testing.T) {
	wv := interactTestWebView(t, `<div id="d" style="width:150px;height:50px" onclick="window.__dc=(window.__dc||0)+1"></div>
<script>
window.__db = 0;
var d = document.getElementById('d');
d.addEventListener('dblclick', function(){ window.__db = (window.__db||0)+1; });
</script>`)
	doc := wv.Document()
	d := doc.GetElementById("d")
	if d == nil {
		t.Fatal("#d not found")
	}
	box := findBox(wv, d)
	if box == nil {
		t.Fatal("#d box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
	if v, err := wv.EvalJS(`window.__dc||0`); err == nil {
		if v.ToString() != "2" {
			t.Fatalf("双击 onclick 执行次数=%s 期望 2", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS dc: %v", err)
	}
	if v, err := wv.EvalJS(`window.__db||0`); err == nil {
		if v.ToString() != "1" {
			t.Fatalf("dblclick 派发次数=%s 期望 1", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS db: %v", err)
	}
}

// TestInteractionHover 鼠标移动 → 命中元素 SetHovered。
func TestInteractionHover(t *testing.T) {
	wv := interactTestWebView(t, `<div id="h" style="width:100px;height:50px;background:#888"></div>`)
	doc := wv.Document()
	h := doc.GetElementById("h")
	if h == nil {
		t.Fatal("#h not found")
	}
	box := findBox(wv, h)
	if box == nil {
		t.Fatal("#h box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseMove(cx, cy)
	if !h.IsHovered() {
		t.Fatal("hover 后元素未 SetHovered(true)")
	}
	wv.HandleMouseMove(5, 5)
	if h.IsHovered() {
		t.Fatal("移出后元素仍 Hovered")
	}
}

// TestInteractionCheckbox click 复选框 → checked 切换 + change。
func TestInteractionCheckbox(t *testing.T) {
	wv := interactTestWebView(t, `<input type="checkbox" id="c">
<script>
window.__chg = 0;
document.getElementById('c').addEventListener('change', function(){ window.__chg++; });
</script>`)
	doc := wv.Document()
	c := doc.GetElementById("c")
	if c == nil {
		t.Fatal("#c not found")
	}
	box := findBox(wv, c)
	if box == nil {
		t.Fatal("#c box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0) // press → toggle + change
	v, err := wv.EvalJS(`document.getElementById('c').checked`)
	if err != nil {
		t.Fatalf("EvalJS checked: %v", err)
	}
	if v.ToString() != "true" {
		t.Fatalf("checked=%s 期望 true", v.ToString())
	}
	if cv, _ := wv.EvalJS(`window.__chg`); cv.ToString() != "1" {
		t.Fatalf("change 次数=%s 期望 1", cv.ToString())
	}
}

// TestInteractionWheelScroll 滚轮滚动容器（引擎偏移断言）。
func TestInteractionWheelScroll(t *testing.T) {
	wv := interactTestWebView(t, `<div id="s" style="width:200px;height:100px;overflow-y:auto">
<div style="height:300px;background:#888"></div>
<div style="height:100px;background:#999"></div></div>`)
	doc := wv.Document()
	s := doc.GetElementById("s")
	if s == nil {
		t.Fatal("#s not found")
	}
	box := findBox(wv, s)
	if box == nil {
		t.Fatal("#s box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseMove(cx, cy)
	wv.HandleWheel(-120) // 向下滚
	wv.RebuildRenderTree()
	// 引擎偏移断言：滚动后 scrollTop 应 > 0
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView nil")
	}
	b := rv.FindRenderBoxForNode(s)
	if b == nil {
		t.Fatal("box nil after rebuild")
	}
	_, sy := rv.BoxScrollOffset(b)
	if sy <= 0 {
		t.Fatalf("滚轮后 scrollTop=%.0f 期望 >0", sy)
	}
}

// TestInteractionMovePressDrag mousedown 后 mousemove（拖拽）持续派发，
// release 在另元素上不触发 click。
func TestInteractionMovePressDrag(t *testing.T) {
	wv := interactTestWebView(t, `<div id="a" style="width:100px;height:50px" onclick="window.__hit=1"></div>
<div id="b" style="width:100px;height:50px;position:absolute;left:150px;top:0" onclick="window.__hit=2"></div>
<script>
window.__mv = 0;
document.addEventListener('mousemove', function(){ window.__mv++; });
</script>`)
	doc := wv.Document()
	a := doc.GetElementById("a")
	box := findBox(wv, a)
	if box == nil {
		t.Fatal("#a box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0) // press on a
	wv.HandleMouseMove(cx+10, cy)      // 拖拽中 mousemove
	wv.HandleMouseMove(cx+60, cy)      // 移到 b 上
	if v, _ := wv.EvalJS(`window.__mv`); v.ToString() == "0" {
		t.Fatal("拖拽中未派发 mousemove")
	}
	wv.HandleMouseButton(cx+60, cy, 0, 1) // release on b → 不同元素 → 无 click
	if v, _ := wv.EvalJS(`window.__hit||0`); v.ToString() != "0" {
		t.Fatalf("跨元素释放触发了 click：%s", v.ToString())
	}
}

// ├─ 辅助 ──────────────────────────────────────────────

type boxInfo struct{ X, Y, W, H float64 }

func (b boxInfo) Center() (float64, float64) { return b.X + b.W/2, b.Y + b.H/2 }

func findBox(wv *WebView, el *dom.Element) *boxInfo {
	rv := wv.RenderView()
	if rv == nil || el == nil {
		return nil
	}
	b := rv.FindRenderBoxForNode(el)
	if b == nil {
		return nil
	}
	return &boxInfo{X: b.AbsoluteX(), Y: b.AbsoluteY(), W: b.Width(), H: b.Height()}
}

func collectByClass(doc *dom.Document, cls string) []*dom.Element {
	var out []*dom.Element
	for _, e := range doc.GetElementsByTagName("div") {
		if strings.Contains(e.GetAttribute("class"), cls) {
			out = append(out, e)
		}
	}
	return out
}

var _ = rendering.HitTest

// TestFormSubmitBlurEnter 表单提交语义（下沉）：input 值变化后 blur（点击
// 他处）→ onchange 属性执行（this=元素）+ change 事件；不变化不触发；
// input 上的 Enter 同样提交。
func TestFormSubmitBlurEnter(t *testing.T) {
	wv := interactTestWebView(t, `<input id="x" value="100" onchange="window.__log=this.value">
<input id="y" value="200"><div id="other">o</div>`)
	ff := wv.FormFocus()
	doc := wv.Document()
	x := doc.GetElementById("x")
	box := findBox(wv, x)
	if box == nil {
		t.Fatal("input #x box not found")
	}
	cx, cy := box.Center()
	// 点击聚焦（MouseButton 内建 FocusFromHit）
	wv.HandleMouseButton(cx, cy, 0, 0)
	if ff.Focused() == nil || ff.Focused().LocalName() != "input" {
		t.Fatal("点击 input 未聚焦")
	}
	// End 光标到末尾后输入 '5'（→ 1005）
	ff.KeyInput("End")
	if !ff.CharInput('5') {
		t.Fatal("CharInput 应消费")
	}
	// 值变但未提交：onchange 未执行
	if v, _ := wv.EvalJS("window.__log||''"); v.ToString() != "" {
		t.Fatalf("未 blur 不应触发 onchange: %q", v.ToString())
	}
	// blur：点击其他区域（#other）
	o := doc.GetElementById("other")
	ob := findBox(wv, o)
	if ob == nil {
		t.Fatal("other box not found")
	}
	oy, oy2 := ob.Center()
	_ = oy2
	wv.HandleMouseButton(oy, oy2, 0, 0)
	if v, _ := wv.EvalJS("window.__log||''"); v.ToString() != "1005" {
		t.Fatalf("blur 提交 onchange 失败: %q（期望 1005，this.value）", v.ToString())
	}
	if ff.Focused() != nil {
		t.Fatal("点击非控件应失焦")
	}
	// ── Enter 提交路径 ──
	wv.HandleMouseButton(cx, cy, 0, 0) // 重新聚焦 x
	ff.KeyInput("End")
	if !ff.CharInput('7') {
		t.Fatal("CharInput 应消费")
	}
	wv.EvalJS("window.__log=''")
	if ff.KeyInput("Enter") {
		t.Fatal("KeyInput Enter 应返回 false（input 提交语义）")
	}
	if v, _ := wv.EvalJS("window.__log||''"); v.ToString() != "10057" {
		t.Fatalf("Enter 提交 onchange 失败: %q（期望 10057）", v.ToString())
	}
	// ── 无变化 blur 不触发 ──
	wv.EvalJS("window.__log=''")
	wv.HandleMouseButton(oy, oy2, 0, 0) // 再点其他（x 无变化）
	wv.HandleMouseButton(cx, cy, 0, 0) // 再聚焦
	if v, _ := wv.EvalJS("window.__log||''"); v.ToString() != "" {
		t.Fatalf("值未变化不应触发 onchange: %q", v.ToString())
	}
}

// TestFormSubmitSelectOnChange select 选值触发 onchange 属性（this.value）。
func TestFormSubmitSelectOnChange(t *testing.T) {
	wv := interactTestWebView(t, `<select id="s" onchange="window.__sel=this.value">
<option value="">自动</option>
<option value="HD Webcam">HD Webcam</option>
</select>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	box := findBox(wv, sel)
	if box == nil {
		t.Fatal("select box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0) // 打开弹层
	if _, err := wv.Render(); err != nil {
		t.Fatal(err)
	}
	popEls := collectByClass(doc, "select-popup-option")
	if len(popEls) < 2 {
		t.Fatalf("弹层选项 %d", len(popEls))
	}
	pb := findBox(wv, popEls[1])
	if pb == nil {
		t.Fatal("option box not found")
	}
	px, py := pb.Center()
	wv.HandleMouseButton(px, py, 0, 0) // 选值
	if v, _ := wv.EvalJS("window.__sel||''"); v.ToString() != "HD Webcam" {
		t.Fatalf("select onchange 未执行: %q", v.ToString())
	}
	if selEl, ok := html5.ToSelectElement(sel); !ok || selEl.Value() != "HD Webcam" {
		t.Fatalf("select value 未更新: %v", selEl.Value())
	}
}
