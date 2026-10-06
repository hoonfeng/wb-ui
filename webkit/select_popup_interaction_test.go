package webkit

// select 弹层交互闭环回归（2026-09）：
//   「点击触发条 → 弹层展开 → 点到选项 → select.value 更新 + change 事件 → 弹层关闭」
// 这条闭环此前只在浏览器侧验证过（引擎侧属观测盲区，见「桌面端模型/工具集下拉」反馈）。
// 本测试把它钉在自动化里，与 select_popup_style_test.go（样式来源：主题令牌 / class 驱动 /
// 兜底样式插 head 首位）互补：一个管「长得一样」，一个管「点得动、值变得对」。
//
// 实测基准（2026-09-27 headless）：
//   press1 → popup=true；opts=2；opt#2 box=(9,52)90x24 且命中 data-value="HD Webcam"；
//   点击后 → popup=false；value="HD Webcam"；change 事件 window.__chg="HD Webcam"。

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/rendering"
)

// popupContainerCount 数「精确类名」为 select-popup 的容器
// （collectByClass 是子串匹配，select-popup-option 也会被它命中）。
func popupContainerCount(doc *dom.Document) int {
	n := 0
	for _, e := range collectByClass(doc, "select-popup") {
		if e.ClassName() == "select-popup" {
			n++
		}
	}
	return n
}

func TestSelectPopupClickSelectsOption(t *testing.T) {
	wv := interactTestWebView(t, `<select id="s"><option value="">自动</option><option value="HD Webcam">HD Webcam</option></select>
<script>window.__chg='';document.getElementById('s').addEventListener('change',function(){window.__chg=document.getElementById('s').value});</script>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	if sel == nil {
		t.Fatal("select #s not found")
	}

	// ① 点击触发条 → 弹层展开
	rv0 := wv.RenderView()
	b := rv0.FindRenderBoxForNode(sel)
	if b == nil {
		t.Fatal("select 没有 render box")
	}
	cx, cy := b.AbsoluteX()+b.Width()/2, b.AbsoluteY()+b.Height()/2
	wv.HandleMouseButton(cx, cy, 0, 0)
	if n := popupContainerCount(doc); n != 1 {
		t.Fatalf("点击触发条后 .select-popup 容器数=%d，期望 1（弹层未展开）", n)
	}

	// ② 选项是真实布局（有盒子、能命中、带对的数据）
	opts := collectByClass(doc, "select-popup-option")
	if len(opts) != 2 {
		t.Fatalf("弹层选项数=%d，期望 2", len(opts))
	}
	rv := wv.RenderView()
	ob := rv.FindRenderBoxForNode(opts[1])
	if ob == nil {
		t.Fatal("选项 #2 没有 render box")
	}
	ox, oy := ob.AbsoluteX()+ob.Width()/2, ob.AbsoluteY()+ob.Height()/2
	hit := rendering.HitTest(rv, ox, oy, "")
	if hit == nil {
		t.Fatalf("命中测试 (%.0f,%.0f) 未命中任何元素", ox, oy)
	}
	if dv := hit.GetAttribute("data-value"); dv != "HD Webcam" {
		t.Errorf("命中元素 data-value=%q，期望 \"HD Webcam\"", dv)
	}
	if hit.GetAttribute("data-select-popup") == "" {
		t.Error("命中元素缺 data-select-popup 标记（点击管线无法识别为弹层项）")
	}

	// ③ 点击选项 → 值更新 + change 事件 + 弹层关闭
	wv.HandleMouseButton(ox, oy, 0, 0)
	if v, err := wv.EvalJS(`document.getElementById('s').value`); err != nil {
		t.Fatalf("EvalJS value: %v", err)
	} else if v.ToString() != "HD Webcam" {
		t.Errorf("点击选项后 select.value=%q，期望 \"HD Webcam\"", v.ToString())
	}
	if c, err := wv.EvalJS(`window.__chg`); err != nil {
		t.Fatalf("EvalJS __chg: %v", err)
	} else if c.ToString() != "HD Webcam" {
		t.Errorf("change 事件未携带新值：window.__chg=%q，期望 \"HD Webcam\"", c.ToString())
	}
	if n := popupContainerCount(doc); n != 0 {
		t.Errorf("点击选项后 .select-popup 容器数=%d，期望 0（弹层未关闭）", n)
	}
}
