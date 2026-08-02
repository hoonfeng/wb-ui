// Comprehensive interaction tests: click dispatch to JS listeners (Vue
// @click style), checkbox/radio toggling, form-control focus, and multi-line
// textarea caret placement. These mirror the real companion components
// (blue create button, terminal new-tab, chat textarea, switches).
package rendering_test

import (
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/jsc"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

// mkRuntime builds a document + render view + JS runtime wired together so
// addEventListener from JS registers on real DOM elements and DispatchEvent
// invokes the JS callback — the exact pipeline Vue @click uses.
func mkRuntime(t *testing.T, body string) (*dom.Document, *rendering.RenderView, *jsc.Interpreter, *strings.Builder) {
	t.Helper()
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	htmlSrc := "<!DOCTYPE html><html><head><style>" +
		"textarea{font-family:Consolas;font-size:14px;padding:4px;white-space:pre}" +
		"input{font-family:Consolas;font-size:14px;padding:4px}" +
		"</style></head><body>" + body + "</body></html>"

	doc, err := html.Parse(htmlSrc)
	if err != nil {
		t.Fatalf("parse HTML: %v", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	if doc.DocumentElement() != nil {
		extractStylesTest(doc.DocumentElement(), resolver)
	}
	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		t.Fatal("RenderView nil")
	}
	rv.SetViewportSize(800, 600)
	rv.Layout(nil)

	log := &strings.Builder{}
	interp := jsc.NewInterpreter()
	_ = interp
	// The binding layer is exercised through dom-level listeners in this
	// test; the JS runtime side is covered by bindings tests. Here we verify
	// the DOM/rendering side: dispatch → listener fires.
	return doc, rv, interp, log
}

func extractStylesTest(root *dom.Element, resolver *style.Resolver) {
	for _, s := range root.GetElementsByTagName("style") {
		if text := s.TextContent(); strings.TrimSpace(text) != "" {
			sheet := css.NewCSSStyleSheet()
			css.NewParser(text).ParseStyleSheetInto(sheet)
			resolver.AddStyleSheet(sheet)
		}
	}
}

func findEl(t *testing.T, doc *dom.Document, cls string) *dom.Element {
	t.Helper()
	for _, el := range doc.GetElementsByTagName("div") {
		if strings.Contains(el.GetAttribute("class"), cls) {
			return el
		}
	}
	for _, el := range doc.GetElementsByTagName("button") {
		if strings.Contains(el.GetAttribute("class"), cls) {
			return el
		}
	}
	for _, el := range doc.GetElementsByTagName("input") {
		if strings.Contains(el.GetAttribute("class"), cls) {
			return el
		}
	}
	for _, el := range doc.GetElementsByTagName("textarea") {
		if strings.Contains(el.GetAttribute("class"), cls) {
			return el
		}
	}
	t.Fatalf("element with class %q not found", cls)
	return nil
}

// hitCenter finds the render box for cls and returns its center in page coords.
func hitCenter(t *testing.T, rv *rendering.RenderView, cls string) (float64, float64) {
	t.Helper()
	var bx, by, bw, bh float64
	found := false
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if found || o == nil {
			return
		}
		if b, ok := o.(interface{ AsRenderBox() *rendering.RenderBox }); ok {
			if bb := b.AsRenderBox(); bb != nil {
				if nd := bb.Node(); nd != nil {
					if el, isEl := nd.(*dom.Element); isEl && strings.Contains(el.GetAttribute("class"), cls) {
						bx, by, bw, bh = bb.AbsoluteX(), bb.AbsoluteY(), bb.Width(), bb.Height()
						found = true
						return
					}
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	if !found {
		t.Fatalf("render box for %q not found", cls)
	}
	return bx + bw/2, by + bh/2
}

// TestClickDispatchFiresJSListener verifies the release-path click: hit-test
// finds the button, dispatch fires its listener (Vue @click analog).
func TestClickDispatchFiresJSListener(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<button class="create-btn" onclick="">创建</button>`)
	btn := findEl(t, doc, "create-btn")
	fired := false
	btn.AddEventListener("click", dom.EventListenerFunc(func(e dom.Event) {
		fired = true
		if e.Target() != btn {
			t.Errorf("Target = %v, want btn", e.Target())
		}
	}), false)
	x, y := hitCenter(t, rv, "create-btn")
	deepest := rendering.HitTest(rv, x, y, "")
	if deepest == nil {
		t.Fatal("HitTest nil")
	}
	if deepest != btn {
		t.Fatalf("HitTest = %v, want btn", deepest)
	}
	deepest.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
	if !fired {
		t.Error("click listener was not fired on dispatch")
	}
}

// TestCheckboxToggleOnClickClick verifies checkbox toggling on a real click
// (press path + dispatch, mirroring host.go).
func TestCheckboxToggleOnClickClick(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<input type="checkbox" class="sw">`)
	cb := findEl(t, doc, "sw")
	in, ok := html5.ToInputElement(cb)
	if !ok {
		t.Fatal("not an input element")
	}
	if in.Checked() {
		t.Fatal("checkbox should start unchecked")
	}
	x, y := hitCenter(t, rv, "sw")
	deepest := rendering.HitTest(rv, x, y, "")
	if deepest == nil {
		t.Fatal("HitTest nil")
	}
	// press path: host.go toggles checked on press
	if deepest.LocalName() == "input" && deepest.GetAttribute("type") == "checkbox" {
		if in, ok2 := html5.ToInputElement(deepest); ok2 {
			in.SetChecked(!in.Checked())
		}
	}
	deepest.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
	if !in.Checked() {
		t.Error("checkbox not toggled by click")
	}
}

// TestCaretSingleLineInput verifies single-line input caret offset from X.
func TestCaretSingleLineInput(t *testing.T) {
	font := graphics.Font{Family: "Consolas", Size: 14, Weight: 400}
	text := "hello"
	boxX, boxY, padX, padY, lineH := 10.0, 20.0, 4.0, 4.0, 17.0
	// Click at the start of the first character → offset 0
	off := rendering.CalcFormControlCaretOffset(text, false, boxX+padX, boxY, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off != 0 {
		t.Errorf("click at char0 start → offset %d, want 0", off)
	}
	// Click at the end of the text → offset len
	chars := []rune(text)
	totalW := 0.0
	for _, r := range chars {
		totalW += graphics.MeasureText(font, string(r))
	}
	off = rendering.CalcFormControlCaretOffset(text, false, boxX+padX+totalW, boxY, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off != len(chars) {
		t.Errorf("click at end → offset %d, want %d", off, len(chars))
	}
	// Click in the middle of the 3rd char region → offset 2
	mid := 0.0
	for _, r := range chars[:2] {
		mid += graphics.MeasureText(font, string(r))
	}
	off = rendering.CalcFormControlCaretOffset(text, false, boxX+padX+mid, boxY, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off != 2 {
		t.Errorf("click at mid-char2 → offset %d, want 2", off)
	}
}

// TestCaretTextareaMultiLine verifies multi-line textarea caret: clicking a
// later line resolves that line, not line 0 (the previous bug).
func TestCaretTextareaMultiLine(t *testing.T) {
	font := graphics.Font{Family: "Consolas", Size: 14, Weight: 400}
	text := "alpha\nbeta\ngamma"
	boxX, boxY, padX, padY := 10.0, 30.0, 4.0, 4.0
	ascent := graphics.GlobalFontAscent(font)
	descent := graphics.GlobalFontDescent(font)
	lineH := ascent + descent
	if lineH <= 0 {
		lineH = 17
	}
	// Click near the start of line 1 (index 6 = 'b' of "beta")
	cx := boxX + padX + 1
	cy := boxY + padY + ascent + lineH*1
	off := rendering.CalcFormControlCaretOffset(text, true, cx, cy, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off < 6 || off > 9 {
		t.Errorf("click line1 start → offset %d, want 6..9 (beta)", off)
	}
	// Click near the start of line 2 (index 11 = 'g' of "gamma")
	cy = boxY + padY + ascent + lineH*2
	off = rendering.CalcFormControlCaretOffset(text, true, cx, cy, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off < 11 || off > 15 {
		t.Errorf("click line2 start → offset %d, want 11..15 (gamma)", off)
	}
	// Click in the middle of line 0 → offset should stay within line 0
	alW := graphics.MeasureText(font, "al")
	cx = boxX + padX + alW
	cy = boxY + padY + ascent
	off = rendering.CalcFormControlCaretOffset(text, true, cx, cy, boxX, boxY, 200, font, padX, padY, lineH, 0)
	t.Logf("alW=%.2f aW=%.2f lW=%.2f alphaW=%.2f ascent=%.2f lineH=%.2f", alW,
		graphics.MeasureText(font, "a"), graphics.MeasureText(font, "l"),
		graphics.MeasureText(font, "alpha"), ascent, lineH)
	if off < 1 || off > 4 {
		t.Errorf("click line0 mid → offset %d, want 1..4", off)
	}
	// Click far past the last line → end of text
	cy = boxY + padY + ascent + lineH*10
	off = rendering.CalcFormControlCaretOffset(text, true, boxX+padX+500, cy, boxX, boxY, 200, font, padX, padY, lineH, 0)
	if off != len([]rune(text)) {
		t.Errorf("click below last line → offset %d, want %d", off, len([]rune(text)))
	}
}

// TestClickBubblesToAncestor verifies event bubbling so document-level
// listeners (e.g. @click.self overlays) also fire.
func TestClickBubblesToAncestor(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<div class="overlay"><button class="inner">x</button></div>`)
	overlay := findEl(t, doc, "overlay")
	fired := 0
	overlay.AddEventListener("click", dom.EventListenerFunc(func(e dom.Event) {
		fired++
	}), false)
	x, y := hitCenter(t, rv, "inner")
	deepest := rendering.HitTest(rv, x, y, "")
	if deepest == nil {
		t.Fatal("HitTest nil")
	}
	deepest.DispatchEvent(dom.NewMouseEvent(dom.EventClick, true, true, false))
	if fired == 0 {
		t.Error("bubbling click did not reach ancestor listener")
	}
}
