// TestHitTestTransformPanelButton: transform 居中弹窗（configwin 通用设置
// 同构：absolute + translate(-50%,-50%) + z-index 层）内层化 button 的命中。
// 回归：完成按钮层（UA overflow:hidden → 独立子层，自身无 transform）布局
// 坐标在弹窗空间内（y>视口高），旧实现子层 bucket 递归用未逆变换的视口
// 坐标 → 全 miss → 「点完成按钮无反应」；修复后子层沿用父层逆变换坐标。
package rendering_test

import (
	"strings"
	"testing"

	"wb-ui/rendering"
)

func TestHitTestTransformPanelButton(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<div class="mask" style="position:absolute;left:0;top:0;width:800px;height:600px;z-index:999;background:rgba(0,0,0,.45)"></div>
<div class="panel" style="position:absolute;left:50%;top:50%;transform:translate(-50%,-50%);background:#1a2030;z-index:1000;padding:14px;width:280px">
  <div class="body" style="height:400px"></div>
  <button class="done" style="width:280px;height:28px">完成</button>
</div>`)
	doneBtn := findEl(t, doc, "done")

	// 面板底部按钮视觉中心（布局 box 在 (400,300)，transform 平移后绘制
	// top≈72，按钮位于面板底部 → 视觉中心约 (400, 500)）。
	btn := rendering.HitTest(rv, 400, 500, "")
	if btn == nil {
		t.Fatal("HitTest on transform-panel button = nil")
	}
	if !strings.Contains(btn.GetAttribute("class"), "done") {
		t.Fatalf("HitTest on transform-panel button = %v (class=%q), want done", btn, btn.GetAttribute("class"))
	}
	_ = doneBtn

	// 面板空白区（按钮上方）命中 panel 内部元素（.body 最小面积胜出），
	// 不穿透到 mask/下层（层叠顺序：panel z=1000 > mask z=999）。
	p := rendering.HitTest(rv, 400, 200, "")
	if p == nil {
		t.Fatal("HitTest on panel area = nil")
	}
	cls := p.GetAttribute("class")
	if !strings.Contains(cls, "panel") && !strings.Contains(cls, "body") {
		t.Fatalf("HitTest on panel area = %v (class=%q), want panel/body", p, cls)
	}
}
