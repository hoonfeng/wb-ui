package webkit

// FormFocus 剪贴板 / 编辑快捷键 / 右键默认菜单引擎测试（无窗口）：
//   - Ctrl+C/V/X/A 走真实 CtrlShortcut 路径（host 注入 mockClip）；
//   - 粘贴选中替换（浏览器语义）+ insertFromPaste input 事件；
//   - 剪切 deleteByCut + change 事件；
//   - 右键 contextmenu 派发 + preventDefault 时引擎不弹默认菜单；
//   - 默认菜单命令执行（menuFn fake → Cut/Copy/Paste/SelectAll）。

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/rendering"
)

type ffMockClip struct{ s string }

func (m *ffMockClip) GetText() string    { return m.s }
func (m *ffMockClip) SetText(s string)   { m.s = s }

func newClipForm(t *testing.T, src string) (*WebView, *FormFocus, *ffMockClip, *dom.Element) {
	t.Helper()
	wv, f := formTestWebView(t, src)
	clip := &ffMockClip{}
	f.SetClipboard(clip)
	return wv, f, clip, wv.Document().GetElementById("i")
}

// TestFormFocusCtrlSelectAll 全选登记 Selection（供 copy/cut 使用）。
func TestFormFocusCtrlSelectAll(t *testing.T) {
	_, f, _, el := newClipForm(t, `<input id="i" value="hello world">`)
	f.Focus(el)
	if !f.CtrlShortcut("selectall") {
		t.Fatal("selectall 未消费")
	}
	sel := rendering.FocusedFormControlSel
	if sel == nil || sel.Start != 0 || sel.End != 11 {
		t.Fatalf("全选 = %+v, want {0,11}", sel)
	}
}

// TestFormFocusCtrlCopy 复制：替换/光标场景 + 剪贴板内容。
func TestFormFocusCtrlCopy(t *testing.T) {
	_, f, clip, el := newClipForm(t, `<input id="i" value="hello world">`)
	f.Focus(el)
	// 光标定位后拖动选择 hello（模拟）
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	if !f.CtrlShortcut("copy") {
		t.Fatal("copy 未消费")
	}
	if clip.s != "hello" {
		t.Fatalf("剪贴板 = %q, want hello", clip.s)
	}
	// 无选区：不改变剪贴板
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 6, End: 6}
	clip.s = "KEEP"
	f.CtrlShortcut("copy")
	if clip.s != "KEEP" {
		t.Fatalf("无选区复制应保持剪贴板, got %q", clip.s)
	}
}

// TestFormFocusCtrlPaste 粘贴：光标插入/选区替换/事件。
func TestFormFocusCtrlPaste(t *testing.T) {
	wv, f, clip, el := newClipForm(t, `<input id="i" value="hello world">`)
	f.Focus(el)
	if _, err := wv.JSInterpreter().RunJS(`window.__in=[];document.getElementById('i').addEventListener('input',function(e){window.__in.push(e.inputType+':'+e.data)});`); err != nil {
		t.Fatal(err)
	}
	clip.s = "PASTE"
	// 光标位置 5（"hello" 后）
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 5, End: 5}
	if !f.CtrlShortcut("paste") {
		t.Fatal("paste 未消费")
	}
	if got := f.Value(); got != "helloPASTE world" {
		t.Fatalf("光标插入粘贴 = %q", got)
	}
	// 全选替换
	f.CtrlShortcut("selectall")
	f.CtrlShortcut("paste")
	if got := f.Value(); got != "PASTE" {
		t.Fatalf("选区替换粘贴 = %q, want PASTE", got)
	}
	v, _ := wv.JSInterpreter().RunJS(`JSON.stringify(window.__in)`)
	if !strings.Contains(v.ToString(), "insertFromPaste:PASTE") {
		t.Fatalf("缺少 insertFromPaste input, got %s", v.ToString())
	}
}

// TestFormFocusCtrlCut 剪切：选区删除 + 剪贴板 + deleteByCut + change。
func TestFormFocusCtrlCut(t *testing.T) {
	wv, f, clip, el := newClipForm(t, `<input id="i" value="hello world">`)
	f.Focus(el)
	if _, err := wv.JSInterpreter().RunJS(`window.__in=[];window.__chg=0;document.getElementById('i').addEventListener('input',function(e){window.__in.push(e.inputType)});document.getElementById('i').addEventListener('change',function(){window.__chg++});`); err != nil {
		t.Fatal(err)
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	if !f.CtrlShortcut("cut") {
		t.Fatal("cut 未消费")
	}
	if clip.s != "hello" {
		t.Fatalf("剪贴板 = %q, want hello", clip.s)
	}
	if got := f.Value(); got != " world" {
		t.Fatalf("剪切后 = %q, want ' world'", got)
	}
	v, _ := wv.JSInterpreter().RunJS(`JSON.stringify({in:window.__in,chg:window.__chg})`)
	s := v.ToString()
	if !strings.Contains(s, "deleteByCut") {
		t.Fatalf("缺少 deleteByCut input, got %s", s)
	}
	if strings.Contains(s, `"chg":1`) {
		t.Fatalf("剪切未 blur 不应派发 change（浏览器语义：change 在 blur 提交时）, got %s", s)
	}
	// blur（焦点清除）→ 值已变 → Submit 派发 change
	f.Clear()
	v2, _ := wv.JSInterpreter().RunJS(`JSON.stringify(window.__chg)`)
	if v2.ToString() != "1" {
		t.Fatalf("blur 后缺少 change 事件, got %s", v2.ToString())
	}
}

// TestFormFocusCtrlNoClip 无剪贴板后端：消费按键但安全（不崩）。
func TestFormFocusCtrlNoClip(t *testing.T) {
	_, f, _, el := newClipForm(t, `<input id="i" value="hi">`)
	f.Focus(el)
	f.clip = nil
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 1}
	if !f.CtrlShortcut("copy") || !f.CtrlShortcut("cut") || !f.CtrlShortcut("paste") {
		t.Fatal("无剪贴板后端时快捷键仍应消费（安全）")
	}
	if f.Value() != "hi" {
		t.Fatalf("cut 不应生效（无后端）, got %q", f.Value())
	}
}

// TestFormFocusContextMenu_Default 右键编辑框：菜单弹出 + 命令执行。
func TestFormFocusContextMenu_Default(t *testing.T) {
	_, f, clip, el := newClipForm(t, `<input id="i" value="hello world">`)
	f.Focus(el)
	called := false
	var gotCut, gotCopy, gotPaste bool
	f.menuFn = func(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
		called = true
		gotCut, gotCopy, gotPaste = canCut, canCopy, canPaste
		if !canCut || !canCopy || !canPaste {
			t.Errorf("菜单使能错误 cut=%v copy=%v paste=%v", canCut, canCopy, canPaste)
		}
		return EditMenuCmdCut
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	f.ShowDefaultEditMenu(100, 40, el)
	if !called {
		t.Fatal("菜单未弹出")
	}
	if clip.s != "hello" {
		t.Fatalf("剪切命令未执行, 剪贴板=%q", clip.s)
	}
	if gotCut != true || gotCopy != true || gotPaste != true {
		t.Fatalf("使能传递错误")
	}
}

// TestFormFocusContextMenu_NonEdit 右键非编辑控件（div）：不弹菜单。
func TestFormFocusContextMenu_NonEdit(t *testing.T) {
	wv, f, _, el := newClipForm(t, `<input id="i" value="hi"><div id="d" style="width:100px;height:40px">d</div>`)
	f.Focus(el)
	called := false
	f.menuFn = func(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
		called = true
		return 0
	}
	div := wv.Document().GetElementById("d")
	f.ShowDefaultEditMenu(100, 100, div)
	if called {
		t.Fatal("非编辑控件不应弹编辑菜单")
	}
}

// TestFormFocusContextMenu_Prevented 前端 preventDefault：不弹菜单。
// （Interaction.MouseButton 右键路径验证 contextmenu 事件派发与止损。）
func TestFormFocusContextMenu_Prevented(t *testing.T) {
	wv, _, _, el := newClipForm(t, `<input id="i" value="hello" style="position:absolute;left:10px;top:10px;width:200px;height:30px;font-size:16px;font-family:Consolas;">`)
	// ★ Interaction 内部使用 wv.FormFocus()（懒建缓存实例）——必须用同一实例
	f := wv.FormFocus()
	f.Focus(el)
	if _, err := wv.JSInterpreter().RunJS(`
		window.__ctx = {fired:0, prevented:0};
		document.getElementById('i').addEventListener('contextmenu', function(e){
			window.__ctx.fired++; e.preventDefault();
		});
	`); err != nil {
		t.Fatal(err)
	}
	called := false
	f.menuFn = func(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
		called = true
		return 0
	}
	f.SetClipboard(&ffMockClip{})
	// 走 Interaction.MouseButton 右键释放真实路径
	wv.EnsureLayout()
	// 右键按下+释放（Interaction 不依赖窗口）
	wv.HandleMouseButton(50, 10, 2, 0)
	wv.HandleMouseButton(50, 10, 2, 1)
	v, _ := wv.JSInterpreter().RunJS(`JSON.stringify(window.__ctx)`)
	s := v.ToString()
	if !strings.Contains(s, `"fired":1`) {
		t.Errorf("contextmenu 事件未派发, got %s", s)
	}
	if called {
		t.Error("preventDefault 后不应弹默认菜单")
	}
}

// TestFormFocusContextMenu_InteractionDefault 走 Interaction.MouseButton
// 右键释放真实路径：contextmenu 未阻止 → 引擎弹默认编辑菜单并执行命令。
func TestFormFocusContextMenu_InteractionDefault(t *testing.T) {
	wv, _, _, el := newClipForm(t, `<input id="i" value="hello world" style="position:absolute;left:10px;top:10px;width:200px;height:30px;font-size:16px;font-family:Consolas;">`)
	// ★ Interaction 内部使用 wv.FormFocus()——同一实例注入 clip/menuFn
	f := wv.FormFocus()
	clip := &ffMockClip{}
	f.SetClipboard(clip)
	f.Focus(el)
	wv.EnsureLayout()
	called := false
	f.menuFn = func(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
		called = true
		return EditMenuCmdCopy
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	// 右键按下+释放（命中编辑框）
	wv.HandleMouseButton(50, 20, 2, 0)
	wv.HandleMouseButton(50, 20, 2, 1)
	if !called {
		t.Fatal("右键未阻止时应弹默认编辑菜单")
	}
	if clip.s != "hello" {
		t.Fatalf("复制命令未执行, 剪贴板=%q", clip.s)
	}
}
