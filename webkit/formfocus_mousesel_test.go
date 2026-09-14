package webkit

// 鼠标选择（拖选/双击词选）引擎层测试：Begin/Extend/EndMouseSelect
// 状态机与 calcClickCaret 等价性 + wordRangeAt 边界 + 选区替换联动。
// 坐标→字符位置不假设像素值：期望值均来自 calcClickCaret 自身。

import (
	"testing"
	"time"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
)

func mouseSelTestWebView(t *testing.T, src string) (*WebView, *FormFocus) {
	t.Helper()
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		graphics.GetFontManager().LoadSystemFonts()
	}
	wv := NewWebView()
	wv.Resize(400, 300)
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	return wv, NewFormFocus(wv)
}

func TestWordRangeAt(t *testing.T) {
	cases := []struct {
		text       string
		pos        int
		wantS, wantE int
	}{
		{"hello world", 1, 0, 5},     // 词中
		{"hello world", 0, 0, 5},     // 词首
		{"hello world", 6, 6, 11},    // 第二个词
		{"hello world", 5, 5, 6},     // 空格：单字符区间
		{"hello world", 10, 6, 11},   // 词尾
		{"_foo_bar", 3, 0, 8},        // 下划线连续段
		{"12345", 2, 0, 5},           // 纯数字
		{"加班挑战+5分钟", 0, 0, 4},   // 连续汉字一段
		{"加班挑战+5分钟", 6, 5, 8},   // "5分钟"（数字+CJK 连续段）
		{"abc", -1, 0, 3},            // 负 pos 收敛
		{"abc", 99, 3, 3},            // 越界收敛
		{"", 0, 0, 0},                // 空
		{"a_b c", 1, 0, 3},           // 词边界停空格
		{"A1", 1, 0, 2},              // 字母+数字连续
	}
	for i, c := range cases {
		s, e := wordRangeAt(c.text, c.pos)
		if s != c.wantS || e != c.wantE {
			t.Errorf("case %d wordRangeAt(%q,%d) = (%d,%d), want (%d,%d)",
				i, c.text, c.pos, s, e, c.wantS, c.wantE)
		}
	}
}

func TestMouseSelectDrag(t *testing.T) {
	_, f := mouseSelTestWebView(t, `<input id="i" value="hello world" style="font-family:'Microsoft YaHei',sans-serif;font-size:16px;width:390px;height:34px">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el)
	// 单击定位（FocusFromHit 内部计算 pos）
	prev, hit := f.FocusFromHit(230, 15)
	if hit != el {
		t.Fatalf("hit = %v, want input", hit)
	}
	if prev != el {
		t.Fatalf("prev = %v, want already-focused el", prev)
	}
	p1 := f.calcClickCaret(el, 230, 15)
	// 第一次按下（单击）：anchor=p1，选区坍缩
	f.BeginMouseSelect(230, 15)
	if !f.mouseSel {
		t.Fatal("mouseSel = false after begin")
	}
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != p1 || s.End != p1 {
		t.Fatalf("after click sel = %+v, want collapsed at %d", s, p1)
	}
	// 向右拖动：选区 = [min(p1,p2), max(p1,p2)]
	f.ExtendMouseSelect(300, 15)
	p2 := f.calcClickCaret(el, 300, 15)
	lo, hi := p1, p2
	if lo > hi {
		lo, hi = hi, lo
	}
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != lo || s.End != hi {
		t.Fatalf("after drag-right sel = %+v, want [%d,%d]", s, lo, hi)
	}
	// 向左回拖越过锚点：方向反转（anchor 固定）
	f.ExtendMouseSelect(60, 15)
	p3 := f.calcClickCaret(el, 60, 15)
	lo, hi = p1, p3
	if lo > hi {
		lo, hi = hi, lo
	}
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != lo || s.End != hi {
		t.Fatalf("after drag-back sel = %+v, want [%d,%d]", s, lo, hi)
	}
	// 释放：结束拖动、选区保留
	f.EndMouseSelect()
	if f.mouseSel {
		t.Fatal("mouseSel = true after end")
	}
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != lo || s.End != hi {
		t.Fatalf("sel lost after release: %+v", s)
	}
	// 选区替换联动：输入字符应替换整个区间
	f.CharInput('X')
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != s.End {
		t.Fatalf("caret after replacement = %+v, want collapsed", s)
	}
	// 释放后第一次单击：清除旧选区（FocusFromHit setCaret）
	f.BeginMouseSelect(230, 15)
	f.EndMouseSelect()
}

func TestMouseSelectDoubleClickWord(t *testing.T) {
	_, f := mouseSelTestWebView(t, `<input id="i" value="hello world foo" style="font-family:'Microsoft YaHei',sans-serif;font-size:16px;width:390px;height:34px">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el)
	p := f.calcClickCaret(el, 160, 15) // 预计落在 "hello" 区域
	// 第一次按下（单击）→ 释放
	f.BeginMouseSelect(160, 15)
	f.EndMouseSelect()
	// 400ms 内同控件同位置再次按下 → 双击词选
	f.BeginMouseSelect(160, 15)
	ws, we := wordRangeAt(f.Value(), p)
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != ws || s.End != we {
		t.Fatalf("double-click sel = %+v, want word [%d,%d) at %d", s, ws, we, p)
	}
	if !f.wordSel {
		t.Fatal("wordSel = false after double-click")
	}
	// 词模式拖动到远处：两端都取词边界
	f.ExtendMouseSelect(330, 15)
	q := f.calcClickCaret(el, 330, 15)
	lo, hi := p, q
	if lo > hi {
		lo, hi = hi, lo
	}
	ws2, _ := wordRangeAt(f.Value(), lo)
	_, we2 := wordRangeAt(f.Value(), hi)
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != ws2 || s.End != we2 {
		t.Fatalf("word-drag sel = %+v, want [%d,%d]", s, ws2, we2)
	}
	// 释放后 500ms 再点：不算双击（间隔超时）
	f.EndMouseSelect()
	f.dblAt = time.Now().Add(-time.Second) // 模拟时间流逝
	f.BeginMouseSelect(160, 15)
	if f.wordSel {
		t.Fatal("wordSel = true after slow click")
	}
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != s.End {
		t.Fatalf("slow click should collapse: %+v", s)
	}
}
