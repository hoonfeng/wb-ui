package webkit

// 回归测试（来源：gou-ide 桌面端用户反馈）
//   「监督者展开后那些项左边都没有展开下三角，都不能展开；浏览器正常」——即
//   <details>/<summary> 的点击激活行为 + ::before 伪元素三角必须与浏览器一致。
//
// 页面结构（AutopilotPanel.vue 的真实写法）：<details class="ap-round"> +
// <summary class="ap-sum">，左侧三角由 .ap-sum::before{content:'▸'}（[open] 时
// 换成 '▾'）提供。
//
// 本测试回答两件事：
//  ① 点击 <summary> 是否切换 <details> 的 open（浏览器的 click activation behavior）；
//  ② ::before 的三角是否产生渲染对象 / 文本段（是否「看不见」）。

import (
	"testing"

	"wb-ui/engine/rendering"
)

const summaryReproHTML = `<html><head><style>
	body { margin: 0; font-family: sans-serif; }
	.ap-sum { display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
	          min-height: 28px; padding: 4px 10px; box-sizing: border-box;
	          cursor: pointer; user-select: none; list-style: none;
	          font-size: 11px; color: #888888; }
	.ap-sum::-webkit-details-marker { display: none; }
	.ap-sum::before { content: '▸'; flex-shrink: 0; font-size: 10px; color: #888888; }
	.ap-round[open] > .ap-sum::before { content: '▾'; }
	.ap-round-no { flex-shrink: 0; }
	.ap-round-body { padding: 0 10px 8px; font-size: 12px; }
</style></head><body>
<details class="ap-round" id="d1"><summary class="ap-sum" id="s1"><span class="ap-round-no">#1</span><span>收尾</span></summary><div class="ap-round-body" id="b1">正文一</div></details>
<details class="ap-round" id="d2" open><summary class="ap-sum" id="s2"><span class="ap-round-no">#2</span><span>续跑</span></summary><div class="ap-round-body" id="b2">正文二</div></details>
</body></html>`

// dumpTextObjects 打印渲染树中全部文本对象及其行盒几何。
func dumpTextObjects(t *testing.T, wv *WebView, label string) {
	t.Helper()
	rv := wv.RenderView()
	if rv == nil {
		t.Fatalf("%s: RenderView nil", label)
	}
	var walk func(ro rendering.RenderObject)
	walk = func(ro rendering.RenderObject) {
		if ro == nil {
			return
		}
		if rt, ok := ro.(*rendering.RenderText); ok {
			segs := rt.Segments()
			if len(segs) == 0 {
				t.Logf("[%s] TEXT %q (no line segment)", label, rt.Text())
			}
			for _, s := range segs {
				t.Logf("[%s] TEXT %q seg=(x=%.1f y=%.1f w=%.1f h=%.1f)", label, rt.Text(), s.X, s.Y, s.Width, s.Height)
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
}

// TestSummaryDetailsActivation 复现①：点击 summary 是否切换 details 的 open。
func TestSummaryDetailsActivation(t *testing.T) {
	wv := interactTestWebView(t, summaryReproHTML)
	doc := wv.Document()
	d1 := doc.GetElementById("d1")
	d2 := doc.GetElementById("d2")
	s1 := doc.GetElementById("s1")
	s2 := doc.GetElementById("s2")
	b1 := doc.GetElementById("b1")
	b2 := doc.GetElementById("b2")
	if d1 == nil || d2 == nil || s1 == nil || s2 == nil || b1 == nil || b2 == nil {
		t.Fatal("复现页面元素缺失")
	}
	t.Logf("初始：d1.open=%v d2.open=%v | b1 盒=%v b2 盒=%v",
		d1.HasAttribute("open"), d2.HasAttribute("open"), findBox(wv, b1) != nil, findBox(wv, b2) != nil)

	// 收起态内容体应不可见（UA 规则 details:not([open]) > :not(summary){display:none}）
	if findBox(wv, b1) != nil {
		t.Errorf("收起轮 #1 的正文体仍有渲染盒（应 display:none）")
	}
	if findBox(wv, b2) == nil {
		t.Errorf("展开轮 #2 的正文体没有渲染盒（应可见）")
	}

	// 点击收起轮的 summary → 期望 open 变 true（浏览器行为）
	box := findBox(wv, s1)
	if box == nil {
		t.Fatal("summary #1 没有渲染盒")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	t.Logf("点击 summary#1 后：d1.open=%v | b1 盒=%v", d1.HasAttribute("open"), findBox(wv, b1) != nil)
	if !d1.HasAttribute("open") {
		t.Errorf("点击 <summary> 未切换 <details> 的 open —— 引擎缺 summary 激活行为")
	}

	// 点击已展开轮的 summary → 期望 open 变 false
	box2 := findBox(wv, s2)
	if box2 == nil {
		t.Fatal("summary #2 没有渲染盒")
	}
	cx2, cy2 := box2.Center()
	wv.HandleMouseButton(cx2, cy2, 0, 0)
	wv.HandleMouseButton(cx2, cy2, 0, 1)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
	t.Logf("点击 summary#2 后：d2.open=%v | b2 盒=%v", d2.HasAttribute("open"), findBox(wv, b2) != nil)
	if d2.HasAttribute("open") {
		t.Errorf("点击已展开的 <summary> 未收起 <details>")
	}
}

// TestSummaryBeforeTriangle 复现②：summary::before 的三角是否被渲染。
func TestSummaryBeforeTriangle(t *testing.T) {
	wv := interactTestWebView(t, summaryReproHTML)
	dumpTextObjects(t, wv, "初始")
	d1 := wv.Document().GetElementById("d1")
	if d1 == nil {
		t.Fatal("d1 缺失")
	}
	if !d1.HasAttribute("open") {
		t.Log("（收起态，三角应为 ▸）")
	}
}
