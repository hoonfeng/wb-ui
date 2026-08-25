package webkit

// 探针：复刻 configwin「新建配置」弹窗实机场景（树 + 画布 wbox + 弹窗
// mask/panel 覆盖）——真实点击弹窗「确定」按钮：期望执行 newPresetApply
//（window.__npApply 置位）；遮罩返回 true 前不得穿透。用户实测「点确定
// 不出现配置-5」——弹窗关闭但 newPresetApply 未执行 = 命中未达按钮。

import (
	"strings"
	"testing"

	"wb-ui/rendering"
)

func npDialogHTML() string {
	return `<html><head><style>
body{margin:0;font-family:sans-serif;font-size:14px}
.col-left{position:absolute;left:0;top:0;width:260px;height:800px;background:#161b29}
.stage{position:absolute;left:260px;top:0;width:800px;height:600px;background:#0d1117;padding:12px}
.wbox{position:absolute;left:100px;top:80px;width:160px;height:60px;background:rgba(59,111,212,.12);border:1px solid #4a80e8}
.tree-add{position:absolute;left:8px;top:742px;width:244px;height:34px;background:#1e2740;border:1px solid #273349;border-radius:8px}
.cp-mask{position:absolute;left:0;top:0;right:0;bottom:0;background:rgba(0,0,0,.45);z-index:999}
.cp-panel{position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);background:#1a2030;border:1px solid #35456e;border-radius:8px;padding:14px;width:280px;z-index:1000}
.cp-actions{display:flex;gap:6px;margin-top:8px}
.cp-actions button{flex:1;padding:5px 0;font-size:14px;border-radius:4px}
.np-input{width:100%;height:30px;background:#131722;border:1px solid #35456e;box-sizing:border-box}
</style></head><body>
<div class="col-left" id="treeCol"></div>
<div class="stage" id="stage"></div>
<div class="wbox" id="wx" onclick="window.__wbox=1" style="position:absolute"></div>
<div class="tree-add" id="add" onclick="openDialog()"></div>
<div class="cp-mask" id="m" onclick="closeNewPreset()" style="display:none"></div>
<div class="cp-panel" id="p" style="display:none">
  <div class="cp-title">新建配置</div>
  <input class="np-input" id="npInput" value="配置-5">
  <div class="cp-actions">
    <button class="cp-ok" id="okbtn" onclick="newPresetApply()">确定</button>
    <button class="cp-cancel" id="ccbtn" onclick="closeNewPreset()">取消</button>
  </div>
</div>
<script>
var __maskOpen = false;
function openDialog(){ window.__opened=(window.__opened||0)+1; document.getElementById('m').style.display='block'; document.getElementById('p').style.display='block'; }
function closeNewPreset(){ __maskOpen = false; var m=document.getElementById('m'); if(m) m.style.display='none'; var pp=document.getElementById('p'); if(pp) pp.style.display='none'; }
function newPresetApply(){ window.__npApply = (window.__npApply||0)+1; window.__npName = document.getElementById('npInput').value; closeNewPreset(); }
</script>
</body></html>`
}

// TestNPDialogApply 弹窗打开时点击「确定」→ newPresetApply 执行。
func TestNPDialogApply(t *testing.T) {
	wv := interactTestWebView(t, npDialogHTML())
	doc := wv.Document()
	wv.Resize(1280, 800)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	rv := wv.RenderView()
	// 打开弹窗：真实点击 tree-add（动态路径——与实机一致：JS 设置 display）
	add := doc.GetElementById("add")
	ab := rv.FindRenderBoxForNode(add)
	if ab == nil {
		t.Fatal("add box nil")
	}
	acx, acy := ab.AbsoluteX()+ab.Width()/2, ab.AbsoluteY()+ab.Height()/2
	t.Logf("add center (%.0f,%.0f)", acx, acy)
	// 侦探：点击前 HitTest 返回什么
	ph := rendering.HitTest(rv, acx, acy, "onclick")
	if ph != nil {
		t.Logf("HitTest(onclick) @add = id=%s onclick=%q", ph.GetAttribute("id"), ph.GetAttribute("onclick"))
	}
	ph2 := rendering.HitTest(rv, acx, acy, "")
	if ph2 != nil {
		t.Logf("HitTest('') @add = id=%s", ph2.GetAttribute("id"))
	}
	wv.HandleMouseButton(acx, acy, 0, 0)
	wv.HandleMouseButton(acx, acy, 0, 1)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render2: %v", err)
	}
	vopen, _ := wv.EvalJS(`window.__opened||0`)
	t.Logf("openDialog 执行次数 = %s", vopen.ToString())
	if vopen.ToString() != "1" {
		t.Fatalf("点击 tree-add 未打开弹窗：%s", vopen.ToString())
	}
	rv = wv.RenderView()
	btn := doc.GetElementById("okbtn")
	b := rv.FindRenderBoxForNode(btn)
	if b == nil {
		t.Fatal("okbtn render box nil")
	}
	// 视觉坐标：panel 带 translate(-50%,-50%)——按钮视觉中心 = 布局中心 - w/2, -h/2
	pb := rv.FindRenderBoxForNode(doc.GetElementById("p"))
	if pb == nil {
		t.Fatal("panel box nil")
	}
	cx, cy := b.AbsoluteX()+b.Width()/2-pb.Width()/2, b.AbsoluteY()+b.Height()/2-pb.Height()/2
	t.Logf("okbtn 视觉中心 (%.0f,%.0f) panel %.0fx%.0f", cx, cy, pb.Width(), pb.Height())
	// 命中预览
	hit := rendering.HitTest(rv, cx, cy, "onclick")
	if hit == nil {
		t.Fatal("HitTest nil")
	}
	t.Logf("HitTest(onclick) @btn center = id=%s onclick=%q", hit.GetAttribute("id"), hit.GetAttribute("onclick"))
	if hit.GetAttribute("id") != "okbtn" {
		t.Fatalf("点击确定按钮应命中 okbtn，实得 %s", hit.GetAttribute("id"))
	}
	// 真实点击
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
	v, _ := wv.EvalJS(`window.__npApply||0`)
	t.Logf("newPresetApply 执行次数 = %s", v.ToString())
	if v.ToString() == "0" || v.ToString() == "" {
		t.Fatal("点击确定未执行 newPresetApply（弹窗关闭但未保存——实机 bug 复现）")
	}
	v2, _ := wv.EvalJS(`window.__npName||''`)
	t.Logf("保存名字 = %s", v2.ToString())
	if !strings.Contains(v2.ToString(), "配置-") {
		t.Fatalf("保存名字异常：%s", v2.ToString())
	}
}

// TestNPDialogMaskNoPassThrough 弹窗打开时点击遮罩空白（wbox 区域上方）：
// 命中 mask（关闭弹窗），**不得穿透**选中下层 wbox。
func TestNPDialogMaskNoPassThrough(t *testing.T) {
	wv := interactTestWebView(t, npDialogHTML())
	doc := wv.Document()
	wv.Resize(1280, 800)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := wv.EvalJS(`document.getElementById('m').style.display='block';
		document.getElementById('p').style.display='block'; 'ok'`); err != nil {
		t.Fatalf("open dialog: %v", err)
	}
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render2: %v", err)
	}
	rv := wv.RenderView()
	wx := doc.GetElementById("wx")
	b := rv.FindRenderBoxForNode(wx)
	cx, cy := b.AbsoluteX()+b.Width()/2, b.AbsoluteY()+b.Height()/2
	hit := rendering.HitTest(rv, cx, cy, "onclick")
	if hit == nil {
		t.Fatal("HitTest nil")
	}
	t.Logf("HitTest(@wbox 上方) = id=%s onclick=%q", hit.GetAttribute("id"), hit.GetAttribute("onclick"))
	if hit.GetAttribute("id") != "m" {
		t.Fatalf("遮罩下 wbox 被穿透命中：应命中 mask，实得 %s", hit.GetAttribute("id"))
	}
	// 实际点击 wbox 区域（遮罩上）：弹窗关闭，wbox onclick 不得触发
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render3: %v", err)
	}
	vw, _ := wv.EvalJS(`window.__wbox||0`)
	t.Logf("wbox 触发 = %s", vw.ToString())
	if vw.ToString() != "0" {
		t.Fatalf("遮罩点击穿透到 wbox（__wbox=%s）", vw.ToString())
	}
}

var _ = rendering.HitTest
