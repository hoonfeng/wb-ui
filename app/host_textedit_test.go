package app

// 文本编辑快捷键（Ctrl+C/V/X/A）集成测试：走真实处理路径
// （handleTextEditingShortcuts，与 processEvents 的 Ctrl 分支共享同一
// 实现），使用真实系统剪贴板（GLFW Set/GetClipboardString）。
//
// 历史回归点：
//   - Ctrl+C 缺失：复制无快捷键（用户「编辑框复制不可用」）；
//   - Ctrl+X 不派发 input/change 事件：前端 @input/@change 监听不感知剪切；
//   - 粘贴须替换选区（浏览器语义），光标插入须在正确位置。
import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/window"
	"wb-ui/rendering"
	"wb-ui/webkit"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// mockClip 内存剪贴板（测试注入，避免依赖系统剪贴板/真实窗口）。
type mockClip struct{ s string }

func (m *mockClip) GetClipboardString() string { return m.s }
func (m *mockClip) SetClipboardString(s string) { m.s = s }

func ctrlEvent(key glfw.Key) window.Event {
	return window.Event{
		Type:   window.EventKey,
		Key:    int(key),
		Mods:   int(glfw.ModControl),
		Action: int(glfw.Press),
	}
}

func loadEditFixture(t *testing.T) (*webkit.WebView, *Host, *dom.Element, *dom.Element) {
	t.Helper()
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	if err := wv.LoadHTML(`<html><body style="font-family:'Microsoft YaHei',sans-serif;font-size:14px">
		<input id="e1" value="hello world" style="width:300px">
		<textarea id="e2" style="width:300px;height:80px">line1&#10;line2</textarea>
	</body></html>`); err != nil {
		t.Fatal(err)
	}
	h := NewHostForTest(wv, 800, 600)
	h.cls = &mockClip{}
	in := wv.MainFrame().Document().GetElementById("e1")
	ta := wv.MainFrame().Document().GetElementById("e2")
	if in == nil || ta == nil {
		t.Fatal("fixture elements missing")
	}
	return wv, h, in, ta
}

// TestTextEditCtrlC_Copy 复制：选区文本写入系统剪贴板。
func TestTextEditCtrlC_Copy(t *testing.T) {
	_, h, in, _ := loadEditFixture(t)
	h.MockFocus(in)
	// 选中 "hello"（0..5）
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	h.cls.SetClipboardString("__CLR__")
	if !h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyC)) {
		t.Fatal("Ctrl+C 应被处理")
	}
	if got := h.cls.GetClipboardString(); got != "hello" {
		t.Fatalf("剪贴板 = %q, want %q", got, "hello")
	}
	// 无选区：复制不改变剪贴板
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 6, End: 6}
	h.cls.SetClipboardString("KEEP")
	h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyC))
	if got := h.cls.GetClipboardString(); got != "KEEP" {
		t.Fatalf("无选区复制应保持剪贴板, got %q", got)
	}
}

// TestTextEditCtrlV_Paste 粘贴：光标插入 / 选区替换 / 派发 insertFromPaste。
func TestTextEditCtrlV_Paste(t *testing.T) {
	wv, h, in, _ := loadEditFixture(t)
	h.MockFocus(in)
	if _, err := wv.JSInterpreter().RunJS(`window.__in=[];document.getElementById('e1').addEventListener('input',function(e){window.__in.push(e.inputType+':'+e.data)});`); err != nil {
		t.Fatal(err)
	}
	// 光标在 6（"hello " 之后）插入
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 6, End: 6}
	h.cls.SetClipboardString("PASTE01")
	if !h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyV)) {
		t.Fatal("Ctrl+V 应被处理")
	}
	if got, _ := h.FocusedElement().GetAttribute("value"), ""; got == "" {
		// value attribute 不更新（property 语义），DOM property 为准
	}
	if got := elementValueFromJS(wv, "e1"); got != "hello PASTE01world" {
		t.Fatalf("光标插入粘贴 value = %q, want %q", got, "hello PASTE01world")
	}
	// 选区替换：全选后粘贴
	h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyA))
	h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyV))
	if got := elementValueFromJS(wv, "e1"); got != "PASTE01" {
		t.Fatalf("选区替换粘贴 value = %q, want %q", got, "PASTE01")
	}
	// input 事件断言
	v, _ := wv.JSInterpreter().RunJS(`JSON.stringify(window.__in)`)
	if !strings.Contains(v.ToString(), "insertFromPaste:PASTE01") {
		t.Fatalf("缺少 insertFromPaste input 事件, got %s", v.ToString())
	}
}

// TestTextEditCtrlX_Cut 剪切：选区入剪贴板 + 删除 + 派发 deleteByCut。
func TestTextEditCtrlX_Cut(t *testing.T) {
	wv, h, in, _ := loadEditFixture(t)
	h.MockFocus(in)
	if _, err := wv.JSInterpreter().RunJS(`window.__in=[];window.__chg=[];document.getElementById('e1').addEventListener('input',function(e){window.__in.push(e.inputType)});document.getElementById('e1').addEventListener('change',function(){window.__chg.push(1)});`); err != nil {
		t.Fatal(err)
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	h.cls.SetClipboardString("__CLR__")
	if !h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyX)) {
		t.Fatal("Ctrl+X 应被处理")
	}
	if got := h.cls.GetClipboardString(); got != "hello" {
		t.Fatalf("剪切后剪贴板 = %q, want %q", got, "hello")
	}
	if got := elementValueFromJS(wv, "e1"); got != " world" {
		t.Fatalf("剪切后 value = %q, want %q", got, " world")
	}
	v, _ := wv.JSInterpreter().RunJS(`JSON.stringify({in:window.__in,chg:window.__chg})`)
	s := v.ToString()
	if !strings.Contains(s, "deleteByCut") {
		t.Fatalf("缺少 deleteByCut input 事件, got %s", s)
	}
	if !strings.Contains(s, "\"chg\":[1]") {
		t.Fatalf("缺少 change 事件, got %s", s)
	}
}

// TestTextEditCtrlA_SelectAll 全选：选中整个值。
func TestTextEditCtrlA_SelectAll(t *testing.T) {
	_, h, in, _ := loadEditFixture(t)
	h.MockFocus(in)
	if !h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyA)) {
		t.Fatal("Ctrl+A 应被处理")
	}
	sel := rendering.FocusedFormControlSel
	if sel == nil || sel.Start != 0 || sel.End != 11 {
		t.Fatalf("全选选区 = %+v, want {0,11}", sel)
	}
}

// TestTextEditCtrl_Textarea 多行控件：粘贴/剪切对 textarea 生效。
func TestTextEditCtrl_Textarea(t *testing.T) {
	wv, h, _, ta := loadEditFixture(t)
	h.MockFocus(ta)
	// textarea 光标 0 处粘贴
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 0}
	h.cls.SetClipboardString("P1\nP2")
	h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyV))
	if got := elementValueFromJS(wv, "e2"); got != "P1\nP2line1\nline2" {
		t.Fatalf("textarea 粘贴 = %q", got)
	}
}

// TestTextEditCtrl_NoFocus 无聚焦：快捷键不处理（不 panic）。
func TestTextEditCtrl_NoFocus(t *testing.T) {
	_, h, _, _ := loadEditFixture(t)
	// 未 MockFocus：imeFocusedEl 为 nil
	if h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyC)) {
		t.Fatal("无聚焦不应处理 Ctrl+C")
	}
	if h.handleTextEditingShortcuts(ctrlEvent(glfw.KeyV)) {
		t.Fatal("无聚焦不应处理 Ctrl+V")
	}
}

// elementValueFromJS 通过 JS 读取 DOM property value（引擎内部
// setFocusedElementValue 更新的是 property，getAttribute 读到的是
// 初始 attribute——与浏览器一致，测试必须用 property 语义读值）。
func elementValueFromJS(wv *webkit.WebView, id string) string {
	v, err := wv.JSInterpreter().RunJS(`document.getElementById('` + id + `').value`)
	if err != nil {
		return "JS_ERR:" + err.Error()
	}
	return v.ToString()
}
