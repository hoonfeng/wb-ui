package app

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/platform/window"
	"wb-ui/rendering"
	"wb-ui/webkit"
)

// 测试公共准备：字体 + 布局测量。
func cursorTestEnv(t *testing.T, wv *webkit.WebView) *rendering.RenderView {
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.MarkRenderTreeDirty()
		}
	}
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	return wv.RenderView()
}

// TestCursorShapeForElement_WebKitSemantics 验证光标形状判定对齐 WebKit
// EventHandler::selectCursor + CursorWin 语义：
//
//	① cursor:auto（默认）下只有链接(a[href])→手型、textarea 手柄→nwse、
//	   文本输入→IBeam，其余（button/select/checkbox/radio/range/label）
//	   一律箭头——Chromium/Edge 在 Windows 上原生控件 hover 都是箭头。
//	② CSS cursor 显式值（pointer/text/nwse-resize/…）优先于元素类型。
func TestCursorShapeForElement_WebKitSemantics(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	wv.LoadHTML(`<html><body style="font-family:sans-serif; font-size:14px">
		<a id="link" href="https://x">link</a>
		<a id="nolink">nolink</a>
		<button id="btn">btn</button>
		<select id="sel"><option>a</option></select>
		<label id="lbl">lbl</label>
		<input id="range" type="range">
		<input id="chk" type="checkbox">
		<input id="radio" type="radio">
		<input id="text" type="text">
		<input id="pw" type="password">
		<div id="poi" style="cursor:pointer">poi</div>
		<div id="txt" style="cursor:text">txt</div>
		<div id="nwse" style="cursor:nwse-resize">nwse</div>
		<div id="def" style="cursor:default">def</div>
	</body></html>`)
	rv := cursorTestEnv(t, wv)
	doc := wv.Document()

	shape := func(id string) window.CursorShape {
		e := doc.GetElementById(id)
		if e == nil {
			t.Fatalf("element #%s not found", id)
		}
		return cursorShapeForElement(rv, e, 400, 300)
	}
	got := func(id string) string {
		names := map[window.CursorShape]string{
			window.CursorArrow: "Arrow", window.CursorIBeam: "IBeam",
			window.CursorHand: "Hand", window.CursorNWSE: "NWSE",
			window.CursorNESW: "NESW", window.CursorNS: "NS", window.CursorEW: "EW",
		}
		return names[shape(id)]
	}

	cases := []struct {
		id   string
		want string
	}{
		// ① auto 语义：原生控件一律箭头（WebKit useHandCursor 只认链接+可编辑）
		{"btn", "Arrow"},
		{"sel", "Arrow"},
		{"lbl", "Arrow"},
		{"range", "Arrow"},
		{"chk", "Arrow"},
		{"radio", "Arrow"},
		// 文本输入 → IBeam
		{"text", "IBeam"},
		{"pw", "IBeam"},
		// 链接：有 href → Hand，无 href → Arrow
		{"link", "Hand"},
		{"nolink", "Arrow"},
		// ② CSS cursor 显式值优先
		{"poi", "Hand"},
		{"txt", "IBeam"},
		{"nwse", "NWSE"},
		{"def", "Arrow"},
	}
	for _, c := range cases {
		if g := got(c.id); g != c.want {
			t.Errorf("#%s: shape=%s want %s", c.id, g, c.want)
		}
	}
}

// TestTextareaHandleHit 验证 textarea 本体 → IBeam、右下角 15px 手柄 → NWSE
// （手柄命中走真实 rendering.BoxViewportRect 视口坐标，与 Press 一致）。
func TestTextareaHandleHit(t *testing.T) {
	wv := webkit.NewWebView()
	wv.Resize(800, 600)
	wv.LoadHTML(`<html><body style="font-family:sans-serif; font-size:14px">
		<textarea id="ta" style="resize:both; width:200px; height:80px;"></textarea>
	</body></html>`)
	rv := cursorTestEnv(t, wv)
	ta := wv.Document().GetElementById("ta")
	if ta == nil {
		t.Fatal("no #ta")
	}
	rb := rv.FindRenderBoxForNode(ta)
	if rb == nil {
		t.Fatal("textarea no render box")
	}
	bx, by, bw, bh := rendering.BoxViewportRect(rv, rb)
	// 手柄区域：右下角 15px（与 Press/updateCursor 一致）
	if got := cursorShapeForElement(rv, ta, bx+bw-5, by+bh-5); got != window.CursorNWSE {
		t.Errorf("handle: got %v want NWSE (box=(%.0f,%.0f) %.0fx%.0f)", got, bx, by, bw, bh)
	}
	// 左上角（非手柄）→ IBeam
	if got := cursorShapeForElement(rv, ta, bx+10, by+10); got != window.CursorIBeam {
		t.Errorf("body: got %v want IBeam", got)
	}
}

// TestCursorShapeNilGuard 空元素/空 RenderView 兜底返回箭头。
func TestCursorShapeNilGuard(t *testing.T) {
	if got := cursorShapeForElement(nil, nil, 0, 0); got != window.CursorArrow {
		t.Errorf("nil/nil: got %v want Arrow", got)
	}
	var e *dom.Element
	if got := cursorShapeForElement(nil, e, 0, 0); got != window.CursorArrow {
		t.Errorf("nil el: got %v want Arrow", got)
	}
}
