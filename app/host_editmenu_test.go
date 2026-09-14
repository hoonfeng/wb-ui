package app

// 默认右键编辑菜单集成测试：contextmenu 未被 JS preventDefault 且右击
// 文本编辑控件 → 引擎弹编辑菜单（menuFn 注入 fake，返回命令执行）。
// 回归点：
//   - 无默认菜单：编辑框右键无法剪切/复制/粘贴（用户「编辑框不支持
//     复制粘贴」的另一半）；
//   - JS preventDefault（前端自建菜单）时引擎不得再弹默认菜单。
import (
	"testing"

	"wb-ui/engine/platform/window"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

func newEditMenuFixture(t *testing.T) (*webkit.WebView, *Host, *editMenuRecorder) {
	t.Helper()
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	if err := wv.LoadHTML(`<html><body style="font-family:'Microsoft YaHei',sans-serif;font-size:14px">
		<input id="e1" value="hello world" style="width:300px">
		<div id="d1" style="width:200px;height:60px;background:#eee">div</div>
	</body></html>`); err != nil {
		t.Fatal(err)
	}
	h := NewHostForTest(wv, 800, 600)
	h.cls = &mockClip{}
	cursorTestEnv(t, wv)
	rec := &editMenuRecorder{}
	h.menuFn = rec.record
	return wv, h, rec
}

// editMenuRecorder 注入菜单后端：记录调用并返回预设命令。
type editMenuRecorder struct {
	called   bool
	x, y     int
	canCut   bool
	canCopy  bool
	canPaste bool
	canAll   bool
	reply    int
}

func (r *editMenuRecorder) record(x, y int, canCut, canCopy, canPaste, canSelectAll bool) int {
	r.called = true
	r.x, r.y = x, y
	r.canCut, r.canCopy, r.canPaste, r.canAll = canCut, canCopy, canPaste, canSelectAll
	return r.reply
}

// TestEditContextMenu_InputShowsMenu 右键 input（未阻止）→ 弹编辑菜单。
func TestEditContextMenu_InputShowsMenu(t *testing.T) {
	wv, h, rec := newEditMenuFixture(t)
	in := wv.MainFrame().Document().GetElementById("e1")
	h.MockFocus(in)

	hit := h.MockContextMenu(wv, 50, 10)
	if hit == nil {
		t.Fatal("hit nil")
	}
	if !rec.called {
		t.Fatal("input 上右键应弹默认编辑菜单")
	}
	if !rec.canPaste {
		t.Errorf("粘贴项应使能")
	}
	if rec.x < 0 || rec.y < 0 {
		t.Errorf("菜单坐标异常 (%d,%d)", rec.x, rec.y)
	}
}

// TestEditContextMenu_NoMenuWhenPrevented JS preventDefault → 不弹默认菜单。
func TestEditContextMenu_NoMenuWhenPrevented(t *testing.T) {
	wv, h, rec := newEditMenuFixture(t)
	in := wv.MainFrame().Document().GetElementById("e1")
	h.MockFocus(in)
	if _, err := wv.JSInterpreter().RunJS(`
		document.getElementById('e1').addEventListener('contextmenu', function(e){ e.preventDefault(); });
	`); err != nil {
		t.Fatal(err)
	}
	h.MockContextMenu(wv, 50, 10)
	if rec.called {
		t.Error("JS preventDefault 后引擎不应弹默认编辑菜单")
	}
}

// TestEditContextMenu_NoMenuOnDiv 非编辑控件右键不弹编辑菜单。
func TestEditContextMenu_NoMenuOnDiv(t *testing.T) {
	wv, h, rec := newEditMenuFixture(t)
	h.MockContextMenu(wv, 100, 120)
	if rec.called {
		t.Error("非编辑控件不应弹默认编辑菜单")
	}
}

// TestEditContextMenu_CutCommand 菜单剪切：选区删除 + 剪贴板 = 选区文本。
func TestEditContextMenu_CutCommand(t *testing.T) {
	wv, h, rec := newEditMenuFixture(t)
	in := wv.MainFrame().Document().GetElementById("e1")
	h.MockFocus(in)
	// 选中 hello（模拟选择状态）
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 5}
	rec.reply = window.EditMenuCut
	h.MockContextMenu(wv, 50, 10)
	if !rec.called {
		t.Fatal("菜单未调用")
	}
	if got := h.cls.GetClipboardString(); got != "hello" {
		t.Fatalf("剪切后剪贴板 = %q, want hello", got)
	}
	if got := elementValueFromJS(wv, "e1"); got != " world" {
		t.Fatalf("剪切后 value = %q, want %q", got, " world")
	}
	if !rec.canCut || !rec.canCopy {
		t.Errorf("有选区时剪切/复制应使能, got cut=%v copy=%v", rec.canCut, rec.canCopy)
	}
}

// TestEditContextMenu_PasteCommand 菜单粘贴：剪贴板文本插入。
func TestEditContextMenu_PasteCommand(t *testing.T) {
	wv, h, rec := newEditMenuFixture(t)
	in := wv.MainFrame().Document().GetElementById("e1")
	h.MockFocus(in)
	h.cls.SetClipboardString("MENU_PASTE")
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: 0, End: 0}
	rec.reply = window.EditMenuPaste
	h.MockContextMenu(wv, 50, 10)
	if !rec.called {
		t.Fatal("菜单未调用")
	}
	if got := elementValueFromJS(wv, "e1"); got != "MENU_PASTEhello world" {
		t.Fatalf("菜单粘贴后 value = %q", got)
	}
}
