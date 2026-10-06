package webkit

// select 弹层样式来源回归（2026-09 修复）：桌面端下拉浮层与浏览器/主题观感
// 不一致的三个来源，逐条钉死。
//
//  1. 弹层容器内联写死 #1c2333/#3a4a75（脱主题的深蓝黑）→ 改为主题令牌
//     var(--bg-secondary)/var(--border-color)，与 app.Host 的弹层同源。
//  2. 每个选项打内联 padding/font-size/line-height/color（内联优先级最高）
//     → 改纯 class 驱动，页面 .select-popup-option 规则族（含 :hover/:selected
//     伪类）才能生效。
//  3. 兜底样式 append 到 <head> 末尾，同特异性下反而**压过**页面规则（旧注释
//     声称的相反），把选中态压成亮蓝 #3b6fd4 + 白字 → 改为插入 head 首位，
//     且值改用主题令牌。

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
)

// TestSelectPopupStyleSourceUnified：弹层 DOM 必须走「主题令牌 + class 驱动」。
func TestSelectPopupStyleSourceUnified(t *testing.T) {
	wv := interactTestWebView(t, `<select id="s">
<option value="a" selected>A</option>
<option value="b">B</option>
</select>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	if sel == nil {
		t.Fatal("select #s not found")
	}
	box := findBox(wv, sel)
	if box == nil {
		t.Fatal("select render box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0)

	// collectByClass 是子串匹配（"select-popup-option" 也含 "select-popup"），
	// 这里要的是容器本身 → 按 class 精确匹配。
	var overlay *dom.Element
	for _, e := range collectByClass(doc, "select-popup") {
		if e.ClassName() == "select-popup" {
			overlay = e
			break
		}
	}
	if overlay == nil {
		t.Fatal("未找到 .select-popup 弹层容器")
	}
	style := overlay.GetAttribute("style")
	for _, want := range []string{"var(--bg-secondary", "var(--border-color"} {
		if !strings.Contains(style, want) {
			t.Errorf("弹层容器样式缺少主题令牌 %s\n  实际: %s", want, style)
		}
	}
	for _, bad := range []string{"#1c2333", "#3a4a75", "select-popup-bg", "select-popup-border"} {
		if strings.Contains(style, bad) {
			t.Errorf("弹层容器仍写死脱主题值 %s\n  实际: %s", bad, style)
		}
	}

	opts := collectByClass(doc, "select-popup-option")
	if len(opts) != 2 {
		t.Fatalf("弹层选项数=%d 期望 2", len(opts))
	}
	for i, o := range opts {
		if v := o.GetAttribute("style"); v != "" {
			t.Errorf("选项 %d 仍带内联 style（会压掉 :hover/:selected 规则）: %q", i, v)
		}
	}

	// 兜底样式：插在 head 首位（页面规则才能覆盖），且不含脱主题写死色。
	sty := doc.GetElementById("wb-ui-select-popup-style")
	if sty == nil {
		t.Fatal("兜底样式 #wb-ui-select-popup-style 未注入")
	}
	head := doc.Head()
	if head == nil {
		t.Fatal("document.head 不存在")
	}
	if head.FirstChild() != sty {
		t.Error("兜底样式不在 head 首位 —— 同特异性下会抢在页面规则之后生效（覆盖页面样式）")
	}
	css := sty.TextContent()
	for _, bad := range []string{"#3b6fd4", "#2a3a5f"} {
		if strings.Contains(css, bad) {
			t.Errorf("兜底样式仍含脱主题写死色 %s: %s", bad, css)
		}
	}
	for _, want := range []string{"var(--bg-hover", "var(--accent-bg", "height:24px", "line-height:24px", "font-size:14px"} {
		if !strings.Contains(css, want) {
			t.Errorf("兜底样式缺少 %s（与页面规则族不一致）: %s", want, css)
		}
	}
}

// TestSelectPopupStylePageWins：页面自带 .select-popup-option 规则时以页面为准，
// 引擎兜底不得抢优先（修复前 append 到 head 末尾 → 页面规则被压掉）。
func TestSelectPopupStylePageWins(t *testing.T) {
	wv := interactTestWebView(t, `<style>
.select-popup-option-selected { background: rgb(18, 52, 86); }
.select-popup-option { height: 40px; }
</style>
<select id="s">
<option value="a" selected>A</option>
<option value="b">B</option>
</select>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	if sel == nil {
		t.Fatal("select #s not found")
	}
	box := findBox(wv, sel)
	if box == nil {
		t.Fatal("select render box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0)

	if len(collectByClass(doc, "select-popup-option-selected")) == 0 {
		t.Fatal("没有选中态选项（-selected 类未加）")
	}
	// 页面规则 background: rgb(18,52,86) → #123456；若引擎兜底抢优先会得到
	// --accent-bg 兜底值 rgba(111,168,255,0.10)。
	if v, err := wv.EvalJS(`getComputedStyle(document.querySelector('.select-popup-option-selected')).backgroundColor`); err == nil {
		got := strings.ToLower(v.ToString())
		if !strings.Contains(got, "123456") && !strings.Contains(got, "18, 52, 86") {
			t.Errorf("选中项背景=%q，页面规则未生效（引擎兜底抢了优先？期望 #123456）", got)
		}
	} else {
		t.Fatalf("EvalJS backgroundColor: %v", err)
	}
	// 页面规则 height: 40px 必须胜出（引擎兜底是 24px）。
	if v, err := wv.EvalJS(`getComputedStyle(document.querySelector('.select-popup-option')).height`); err == nil {
		got := strings.TrimSpace(v.ToString())
		if got != "" && got != "40px" {
			t.Errorf("选项高度=%q，期望页面规则的 40px（引擎兜底 24px 抢先？）", got)
		}
	}
}

func pxAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func pxNear(r, g, b uint8, tr, tg, tb int) bool {
	return pxAbs(int(r)-tr) <= 6 && pxAbs(int(g)-tg) <= 6 && pxAbs(int(b)-tb) <= 6
}

// TestSelectPopupPaintUsesTheme：像素级验证 —— 弹层真正画出来的颜色必须来自
// 主题令牌（容器 --bg-secondary、选中项 --accent-bg），且弹层区域内**不得**再出现
// 旧的硬编码值 #1c2333(28,35,51) 与亮蓝 #3b6fd4(59,111,212)。这是「桌面端下拉与
// 浏览器/主题不一致」的视觉判据：改样式来源后，之前那套脱主题配色必须彻底消失。
func TestSelectPopupPaintUsesTheme(t *testing.T) {
	wv := interactTestWebView(t, `<style>
html,body{margin:0;padding:0;background:rgb(0,0,0)}
:root{--bg-secondary:#151B29;--border-color:#2A3550;--accent-bg:rgb(30,40,62);--text-primary:#E7ECF5;--bg-hover:rgb(35,43,60)}
.select-popup-option{display:block;height:24px;line-height:24px;padding:0 8px;font-size:14px;color:var(--text-primary);overflow:hidden;white-space:nowrap;text-overflow:ellipsis}
.select-popup-option-selected{background:var(--accent-bg)}
</style>
<select id="s" style="margin:20px">
<option value="a" selected>A</option>
<option value="b">B</option>
</select>`)
	doc := wv.Document()
	sel := doc.GetElementById("s")
	if sel == nil {
		t.Fatal("select #s not found")
	}
	box := findBox(wv, sel)
	if box == nil {
		t.Fatal("select render box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseButton(cx, cy, 0, 0)
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}

	var overlay *dom.Element
	for _, e := range collectByClass(doc, "select-popup") {
		if e.ClassName() == "select-popup" {
			overlay = e
			break
		}
	}
	if overlay == nil {
		t.Fatal("未找到 .select-popup 弹层容器")
	}
	ob := findBox(wv, overlay)
	if ob == nil {
		t.Fatal("弹层容器没有 render box")
	}
	pix := fragmentPixels(t, wv)
	w := wv.Width()

	// ① 选中项底色：取右侧空白处（避开文字像素）→ 必须是主题 --accent-bg
	selOpts := collectByClass(doc, "select-popup-option-selected")
	if len(selOpts) == 0 {
		t.Fatal("弹层里没有选中项（-selected 类未加）")
	}
	sb := findBox(wv, selOpts[0])
	if sb == nil {
		t.Fatal("选中项没有 render box")
	}
	sx, sy := int(sb.X)+int(sb.W)-3, int(sb.Y)+int(sb.H)/2
	r2, g2, b2, a2 := pixelAt(pix, w, sx, sy)
	if a2 < 200 {
		t.Fatalf("选中项像素未绘制（a=%d @ %d,%d）", a2, sx, sy)
	}
	if !pxNear(r2, g2, b2, 30, 40, 62) {
		t.Errorf("选中项底色 = (%d,%d,%d)，期望主题 --accent-bg (30,40,62)", r2, g2, b2)
	}

	// ② 全画面扫描（不依赖几何计算，避免阴影/边框混合像素误报）：
	//    不得出现旧硬编码的选中亮蓝 #3b6fd4 —— 它只在修复前那套样式里出现；
	//    且必须存在主题选中底色像素，证明选中态确实按主题绘制。
	foundAccent := false
	h := wv.Height()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			pr, pg, pb, pa := pixelAt(pix, w, x, y)
			if pa < 200 {
				continue
			}
			if pxNear(pr, pg, pb, 0x3B, 0x6F, 0xD4) {
				t.Fatalf("画面出现旧硬编码选中亮蓝 #3b6fd4 @ (%d,%d) = (%d,%d,%d)", x, y, pr, pg, pb)
			}
			if pxNear(pr, pg, pb, 30, 40, 62) {
				foundAccent = true
			}
		}
	}
	if !foundAccent {
		t.Error("画面里没有主题选中底色（--accent-bg 30,40,62）像素 —— 选中态未按主题绘制")
	}
	_ = ob
}
