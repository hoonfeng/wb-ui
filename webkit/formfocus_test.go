package webkit

// FormFocus 引擎层测试：聚焦/编辑/按键/闪烁语义（无窗口，直接操作
// WebView + FormFocus）。

import (
	"strings"
	"testing"
	"time"

	"wb-ui/engine/html5"
	"wb-ui/engine/rendering"
)

// TestFormFocusCharInputFlipsValidityInFocusSession 覆盖引擎真实键入路径
// （FormFocus.CharInput → applyValue）上的 user validity：聚焦时无效的
// required 控件，用户键入使其有效 → 立即获得 user validity（:user-valid
// 立即可匹配，不必等失焦）；聚焦本身不置位，失焦结束焦点会话。
func TestFormFocusCharInputFlipsValidityInFocusSession(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" required>`)
	el := f.WebView().Document().GetElementById("i")
	if el == nil {
		t.Fatal("input #i not found")
	}
	in, ok := html5.ToInputElement(el)
	if !ok {
		t.Fatal("ToInputElement failed")
	}
	if in.Validity().Valid() {
		t.Fatal("空 required 应为无效")
	}
	f.Focus(el)
	if valid, known := el.FocusValidity(); !known || valid {
		t.Fatalf("聚焦瞬间的焦点会话记忆 = (%v, %v), want (false, true)", valid, known)
	}
	if el.UserInteracted() {
		t.Fatal("聚焦本身不应置 user validity")
	}
	if !f.CharInput('x') {
		t.Fatal("CharInput('x') not consumed")
	}
	if !in.Validity().Valid() {
		t.Fatal("键入后应为有效")
	}
	if !el.UserInteracted() {
		t.Error("键入使无效变有效后应置 user validity（:user-valid 立即生效）")
	}
	// 失焦：焦点会话结束，记忆被清除。
	f.Clear()
	if _, known := el.FocusValidity(); known {
		t.Error("失焦后应清除焦点会话记忆")
	}
}

func formTestWebView(t *testing.T, src string) (*WebView, *FormFocus) {
	t.Helper()
	wv := NewWebView()
	wv.Resize(400, 300)
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	return wv, NewFormFocus(wv)
}

// TestFormFocusFocusValueChanged 聚焦/值快照/Changed 语义。
func TestFormFocusFocusValueChanged(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="hi">`)
	el := f.WebView().Document().GetElementById("i")
	if el == nil {
		t.Fatal("input #i not found")
	}
	f.Focus(el)
	if f.Focused() != el {
		t.Fatal("Focused() != el")
	}
	if f.Value() != "hi" {
		t.Fatalf("Value() = %q, want hi", f.Value())
	}
	if f.Changed() {
		t.Fatal("Changed() = true before edit")
	}
	if !el.IsFocused() {
		t.Fatal("el.IsFocused() = false")
	}
	// 聚焦登记（渲染层光标绘制条件）
	if rendering.FocusedFormControl != el {
		t.Fatal("FocusedFormControl not registered")
	}
	// 非表单控件聚焦应被拒绝
	div := f.WebView().Document().GetElementById("d")
	if div != nil {
		f.Focus(div)
		if f.Focused() != el {
			t.Fatal("Focus(non-control) should not change focus")
		}
	}
}

// TestFormFocusCharInput 光标处插入 + 选区替换 + input/change 事件。
func TestFormFocusCharInput(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="hello">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el) // 光标默认末尾（len=5）
	// 末尾追加
	if !f.CharInput('!') {
		t.Fatal("CharInput(!) not consumed")
	}
	if f.Value() != "hello!" {
		t.Fatalf("Value() = %q, want hello!", f.Value())
	}
	if !f.Changed() {
		t.Fatal("Changed() = false after edit")
	}
	// 方向键移动光标后插入
	f.KeyInput("Home")
	f.KeyInput("ArrowRight")
	f.CharInput('X')
	if f.Value() != "hXello!" {
		t.Fatalf("mid insert = %q, want hXello!", f.Value())
	}
	// \r 在 input：未消费（宿主提交）
	if f.CharInput('\r') {
		t.Fatal("CharInput(\\r) on input should not be consumed")
	}
	// 光标位置正确（FocusedFormControlSel 同步）
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != 2 {
		t.Fatalf("caret = %+v, want Start=2", s)
	}
}

// TestFormFocusSelectionReplace 选区替换（浏览器语义）。
func TestFormFocusSelectionReplace(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="abcdef">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el)
	// 手动设置选区 [2,4)（cd）→ 插入 X → abXef
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 2, End: 4}
	f.CharInput('X')
	if f.Value() != "abXef" {
		t.Fatalf("replacement = %q, want abXef", f.Value())
	}
}

// TestFormFocusReadonlyMaxlength 只读/超长限制。
func TestFormFocusReadonlyMaxlength(t *testing.T) {
	_, f := formTestWebView(t, `<input id="ro" value="abc" readonly><input id="mx" value="123" maxlength="3">`)
	ro := f.WebView().Document().GetElementById("ro")
	mx := f.WebView().Document().GetElementById("mx")
	// readonly 可聚焦但不可编辑
	f.Focus(ro)
	if f.Focused() != ro {
		t.Fatal("readonly input should be focusable")
	}
	f.CharInput('x')
	if f.Value() != "abc" {
		t.Fatalf("readonly input edited: %q", f.Value())
	}
	// maxlength=3 拒插
	f.Focus(mx)
	f.KeyInput("End")
	f.CharInput('4')
	if f.Value() != "123" {
		t.Fatalf("maxlength exceeded: %q", f.Value())
	}
}

// TestFormFocusKeyInput 方向/Home/End/Backspace/Delete。
func TestFormFocusKeyInput(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="abcd">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el)
	f.KeyInput("Home")
	f.KeyInput("ArrowRight") // caret=1
	f.KeyInput("Backspace")  // 删 a → bcd, caret=0
	if f.Value() != "bcd" {
		t.Fatalf("backspace = %q, want bcd", f.Value())
	}
	f.KeyInput("ArrowRight") // caret=1
	f.KeyInput("Delete")     // 删 c → bd
	if f.Value() != "bd" {
		t.Fatalf("delete = %q, want bd", f.Value())
	}
	f.KeyInput("Home")
	f.KeyInput("ArrowRight")
	f.KeyInput("ArrowRight") // caret=2
	f.KeyInput("ArrowLeft")  // caret=1
	if s := rendering.FocusedFormControlSel; s == nil || s.Start != 1 {
		t.Fatalf("caret after arrows = %+v, want 1", s)
	}
}

// TestFormFocusTick 500ms 闪烁节奏（挂钟驱动，idle 也翻转）。
func TestFormFocusTick(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="x">`)
	el := f.WebView().Document().GetElementById("i")
	now := time.Now()
	f.Focus(el)
	f.Tick(now) // 刚聚焦：不翻转
	renderBlink := func() bool { return rendering.CaretVisibleControl }
	before := renderBlink()
	t.Logf("pre: blinkOn=%v CVC=%v blinkDiff=%v", f.blinkOn, before, now.Sub(f.blinkTime))
	if f.Tick(now.Add(300 * time.Millisecond)) {
		t.Fatal("blink at 300ms (too early)")
	}
	if !f.Tick(now.Add(600 * time.Millisecond)) {
		t.Fatal("no blink at 600ms")
	}
	if renderBlink() == before {
		t.Fatal("CaretVisibleControl not toggled")
	}
	// 失焦后不再闪烁
	f.Clear()
	if f.Tick(now.Add(1200 * time.Millisecond)) {
		t.Fatal("blink after Clear")
	}
}

// TestFormFocusClear 失焦清理登记 + blur。
func TestFormFocusClear(t *testing.T) {
	_, f := formTestWebView(t, `<input id="i" value="x">`)
	el := f.WebView().Document().GetElementById("i")
	f.Focus(el)
	f.CharInput('y')
	f.Clear()
	if f.Focused() != nil {
		t.Fatal("Focused() != nil after Clear")
	}
	if rendering.FocusedFormControl != nil {
		t.Fatal("FocusedFormControl not cleared")
	}
	if rendering.CaretVisibleControl {
		t.Fatal("CaretVisibleControl not reset")
	}
}

// TestFormFocusFocusFromHit 命中聚焦 + 点击定位光标。
func TestFormFocusFocusFromHit(t *testing.T) {
	wv, f := formTestWebView(t, `<input id="i" value="hello" style="position:absolute;left:10px;top:10px;width:200px;height:30px;font-size:16px;font-family:Consolas;"><div id="d" style="position:absolute;left:10px;top:100px;width:100px;height:40px;"></div>`)
	wv.EnsureLayout()
	// 点输入框 → 聚焦；点击位置靠近文本开头（x=12）→ 光标靠前
	prev, hit := f.FocusFromHit(12, 20)
	if prev != nil {
		t.Fatal("prev should be nil")
	}
	if hit == nil || hit.GetAttribute("id") != "i" {
		t.Fatalf("hit = %v, want #i", hit)
	}
	pos := 0
	if s := rendering.FocusedFormControlSel; s != nil {
		pos = s.Start
	}
	if pos < 0 || pos > 5 {
		t.Fatalf("click caret = %d (out of range)", pos)
	}
	t.Logf("click caret at x=12 → %d", pos)
	// 点空 div → 失焦
	prev, hit = f.FocusFromHit(50, 110)
	if hit != nil {
		t.Fatal("hit on div should not focus")
	}
	if f.Focused() != nil {
		t.Fatal("focus not cleared")
	}
	_ = strings.TrimSpace
}
