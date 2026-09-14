// Translation of: Source/WebCore/rendering/RenderTextControl.{h,cpp}
//                  Source/WebCore/rendering/RenderButton.{h,cpp}
//                  Source/WebCore/rendering/RenderListBox.{h,cpp}
//                  Source/WebCore/rendering/RenderMenuList.{h,cpp}
//                  Source/WebCore/rendering/RenderTheme.{h,cpp} (paintCheckbox/paintRadio/paintSlider/paintProgressBar/paintMeter)
// Completeness: 35%
// Simplifications:
//   - WebKit delegates native control painting to a per-platform RenderTheme (Windows/Mac/iOS),
//     which in turn calls into the platform UI toolkit (e.g. uxtheme on Windows). This port
//     rasterizes the controls directly via the Skia canvas primitives instead — there is no
//     theme abstraction layer, and the drawn shapes are a simplified cross-platform look
//     rather than pixel-accurate native widgets.
//   - only the "classic" non-themed look is painted (square checkbox, circular radio, plain
//   - the text-input caret is painted separately by the selection/caret subsystem
//     (rendering.PaintCaret) and is not duplicated here.
//
// ⚠️ Global-state note: FocusedFormControl, CaretVisibleControl and
//   FocusedFormControlSel are package-level globals, safe only under the
//   single-WebView model. When multi-WebView support is needed, these must
//   be moved to a per-WebView or per-RenderView context.
//     only the closed-state arrow indicator is painted next to the select box.
//   - the text-input caret is painted separately by the selection/caret subsystem
//     (rendering.PaintCaret) and is not duplicated here.

package rendering

import (
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/debugenv"
	"wb-ui/engine/html5"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// FormControlColors holds the palette used to paint native form controls. The values
// mirror the default colors RenderTheme uses on Windows for the classic look.
var FormControlColors = struct {
	CheckboxBorder    graphics.Color
	CheckboxCheck     graphics.Color
	CheckboxBg        graphics.Color
	CheckboxBgHot     graphics.Color
	RadioBorder       graphics.Color
	RadioDot          graphics.Color
	RadioBg           graphics.Color
	RadioBgHot        graphics.Color
	SliderTrack       graphics.Color
	SliderThumb       graphics.Color
	SliderThumbBorder graphics.Color
	SliderFill        graphics.Color
	ProgressTrack     graphics.Color
	ProgressFill      graphics.Color
	MeterOptimum      graphics.Color
	MeterSuboptimal   graphics.Color
	MeterLow          graphics.Color
	MeterHigh         graphics.Color
	SelectArrow       graphics.Color
	ButtonHighlight   graphics.Color
	ButtonShadow      graphics.Color
}{
	CheckboxBorder:    graphics.Color{R: 120, G: 120, B: 120, A: 255},
	CheckboxCheck:     graphics.Color{R: 255, G: 255, B: 255, A: 255},
	CheckboxBg:        graphics.Color{R: 255, G: 255, B: 255, A: 255},
	// Windows accent blue for the checked state (matches Edge).
	CheckboxBgHot:     graphics.Color{R: 26, G: 115, B: 232, A: 255},
	RadioBorder:       graphics.Color{R: 120, G: 120, B: 120, A: 255},
	RadioDot:          graphics.Color{R: 255, G: 255, B: 255, A: 255},
	RadioBg:           graphics.Color{R: 255, G: 255, B: 255, A: 255},
	RadioBgHot:        graphics.Color{R: 26, G: 115, B: 232, A: 255},
	// Edge (Chromium) 深色主题实测 range 颜色：
	//   track 未填充 #EFEFEF（239,239,239）、已填充段 #0075FF（0,117,255）、
	//   thumb 主体 #0075FF + 右侧白色高光（3D 立体感）。
	SliderTrack:       graphics.Color{R: 239, G: 239, B: 239, A: 255},
	SliderThumb:       graphics.Color{R: 0, G: 117, B: 255, A: 255},
	SliderThumbBorder: graphics.Color{R: 0, G: 117, B: 255, A: 255},
	SliderFill:        graphics.Color{R: 0, G: 117, B: 255, A: 255},
	ProgressTrack:     graphics.Color{R: 220, G: 220, B: 220, A: 255},
	ProgressFill:      graphics.Color{R: 60, G: 130, B: 246, A: 255},
	MeterHigh:         graphics.Color{R: 244, G: 67, B: 54, A: 255},
	SelectArrow:       graphics.Color{R: 80, G: 80, B: 80, A: 255},
	ButtonHighlight:   graphics.Color{R: 255, G: 255, B: 255, A: 255},
	ButtonShadow:      graphics.Color{R: 160, G: 160, B: 160, A: 255},
}

// FocusedFormControl holds the currently focused text-type form control element
// (<input>/<textarea>). When non-nil, a blinking caret is drawn at the end of its
// value text during paintTextInputValue. Set by app.Host.FocusElement/Unfocus.
// This is separate from CaretPos (which targets RenderText segments) because form
// controls are replaced elements with no RenderText children.
//
// ⚠️ Single-WebView global: see package doc.
var FocusedFormControl *dom.Element

// CaretVisibleControl mirrors CaretVisible for the form-control caret blink cycle.
// Toggled by the Host at ~500ms intervals.
//
// ⚠️ Single-WebView global: see package doc.
var CaretVisibleControl = true

// FormControlSelection represents text selection within a form control
// (input/textarea). Start and End are 0-based character offsets into the
// element's value. When Start == End, it indicates a caret position (no
// selection). Reset to nil when focus changes.
type FormControlSelection struct {
	Start  int
	End    int
	Active bool // true while the user is dragging to extend the selection
}

// FocusedFormControlSel holds the current selection within the focused form
// control. It is set by app.Host when the user clicks or drags inside a
// text-type input or textarea. A nil value means no selection/caret position
// is active (the default blinking caret at the end of the value is drawn
// instead).
//
// ⚠️ Single-WebView global: see package doc.
var FocusedFormControlSel *FormControlSelection

// PaintFormControl is the dispatch entry point for native form-control painting
// RenderTheme::paint(). It inspects the element's tag and (for <input>) the type attribute
// and delegates to the corresponding paint function. It is called from the foreground phase
// for box-bearing replaced elements that are form controls. Returns true if the element
// was handled (so the caller can skip the default text-paint path), false otherwise.
//
// debugPaintLog enables verbose paint diagnostics. Set to true to trace form-control paint calls.
var debugPaintLog = false

func PaintFormControl(box *RenderBox, info *PaintInfo) bool {
	if box == nil || info == nil || info.canvas == nil {
		return false
	}
	node := box.Node()
	if node == nil {
		return false
	}
	el, ok := node.(*dom.Element)
	if !ok {
		return false
	}
	localName := el.LocalName()
	// Get the type attribute for input elements
	inputType := ""
	if localName == "input" {
		if in, ok2 := html5.ToInputElement(el); ok2 {
			inputType = string(in.Type())
		}
	}
	if debugPaintLog {
		log.Printf("[dbg/paint] PaintFormControl: <%s> type=%q class=%q xy=(%.0f,%.0f) wh=(%.0f,%.0f)",
			localName, inputType, el.ClassName(), box.X(), box.Y(), box.Width(), box.Height())
	}
	x, y, w, h := box.X(), box.Y(), box.Width(), box.Height()
	op := paintOpacity(box, info)
	st := box.Style()
	switch localName {
	case "input":
		return paintInput(el, info, st, x, y, w, h, op)
	case "button":
		// Button is NOT a replaced element — its children (RenderText)
		// are laid out normally to determine width. The background and
		// border are painted by the normal PhaseBackground path using
		// the resolved CSS style. The LABEL however is drawn here,
		// CENTERED (browsers center button text both horizontally —
		// UA text-align:center — and vertically inside the padding box);
		// the IFC-placed RenderText would otherwise sit top-left of the
		// content box. paintButtonText computes its own centered coords.
		paintButtonText(info, el, st, x, y, w, h, op)
		return true
	case "textarea":
		_, sy := float64(0), float64(0)
		if info.rv != nil {
			_, sy = info.rv.BoxScrollOffset(box)
		}
		paintTextAreaText(info, el, st, x, y, w, h, op, sy)
		return true
	case "progress":
		paintProgressBar(info, st, x, y, w, h, el, op)
		return true
	case "meter":
		paintMeterBar(info, st, x, y, w, h, el, op)
		return true
	case "select":
		paintSelectText(info, el, st, x, y, w, h, op)
		paintSelectArrow(info, st, x, y, w, h, op)
		return true // select is a replaced element — no child text to paint
	}
	return false
}

// paintInput dispatches an <input> element to its type-specific painter.
func paintInput(el *dom.Element, info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, op float64) bool {
	in, ok := html5.ToInputElement(el)
	if !ok {
		return false
	}
	switch in.Type() {
	case html5.InputCheckbox:
		paintCheckbox(info, st, x, y, w, h, in.Checked(), op)
		return true
	case html5.InputRadio:
		paintRadio(info, st, x, y, w, h, in.Checked(), op)
		return true
	case html5.InputRange:
		paintRangeSlider(info, st, x, y, w, h, in, op)
		return true
	case html5.InputColor:
		paintColorSwatch(info, x, y, w, h, in.Value(), op)
		return true
	case html5.InputFile:
		paintFileInput(info, el, st, x, y, w, h, op)
		return true
	case html5.InputHidden:
		return true // nothing to paint
	}
	// Text-like inputs (text/password/search/email/url/tel/number/date/time/...):
	// paint the value text (or placeholder when empty). The caret is painted
	// separately by PaintCaret.
	paintTextInputValue(info, el, st, x, y, w, h, op)
	return true
}

// paintFileInput paints a native file picker look: the chosen file name (or
// placeholder) on the left and a "选择文件" button on the right, mirroring
// RenderFileUploadControl::paint. The button is a small rounded rect with
// centered label text; the filename is truncated to fit.
func paintFileInput(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas
	value := el.GetAttribute("value")
	// Show the base file name when a path is present.
	label := "未选择文件"
	if value != "" {
		label = filepath.Base(value)
	}
	fsV := st.FontSize.Value // px float (em resolved during style cascade)
	if fsV <= 0 {
		fsV = 16
	}
	textCol := toGraphicsColor(st.Color)
	btnW := 96.0
	btnH := h - 4
	if btnH < 20 {
		btnH = 20
	}
	btnX := x + w - btnW - 2
	btnY := y + (h-btnH)/2
	// Button background (light gray, pressed-look border).
	btnCol := graphics.Color{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
	c.FillRoundRect(btnX, btnY, btnW, btnH, 3, btnCol)
	c.StrokeRoundRect(btnX, btnY, btnW, btnH, 3, 1, graphics.Color{R: 0xB0, G: 0xB0, B: 0xB0, A: 0xFF})
	// Button label.
	font := graphics.Font{Family: st.FontFamily, Size: fsV - 2, Weight: 400, Style: st.FontStyle}
	btnLabel := "选择文件"
	lw := graphics.MeasureText(font, btnLabel)
	c.DrawText(btnX+(btnW-lw)/2, btnY+btnH/2+fsV/2-1, btnLabel, font, graphics.Color{R: 0x33, G: 0x33, B: 0x33, A: 0xFF})
	// Filename text (truncated to fit before the button).
	maxW := w - btnW - 12
	if maxW > 0 {
		font2 := graphics.Font{Family: st.FontFamily, Size: fsV - 1, Weight: 400, Style: st.FontStyle}
		if lw2 := graphics.MeasureText(font2, label); lw2 > maxW {
			// Truncate with ellipsis.
			runes := []rune(label)
			for len(runes) > 0 && graphics.MeasureText(font2, string(runes)+"…") > maxW {
				runes = runes[:len(runes)-1]
			}
			label = string(runes) + "…"
		}
		c.DrawText(x+6, y+h/2+fsV/2-1, label, font2, textCol)
	}
}

// paintTextInputValue draws the value text (or placeholder) of a text-type <input>
// element, mirroring RenderTextControl::paintInnerTextField. The text is left-aligned,
// vertically centered within the border box, and clipped to the content area. Password
// inputs render as bullet characters (•) to conceal the value. When this element is
// the focused form control, a blinking caret is drawn at the end of the value text.
func paintTextInputValue(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas
	// Read the value and placeholder from the element.
	value := el.GetAttribute("value")
	in, _ := html5.ToInputElement(el)
	isPassword := in.Type() == html5.InputPassword
	displayText := value
	textColor := applyOpacity(toGraphicsColor(st.Color), op)
	showPlaceholder := false

	// When the value is empty, show the placeholder in a muted color.
	if displayText == "" {
		placeholder := el.GetAttribute("placeholder")
		if placeholder == "" {
			paintFormControlCaret(info, el, st, x, y, w, h, 0, op, 0)
			return
		}
		displayText = placeholder
		textColor = applyOpacity(graphics.Color{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}, op)
		showPlaceholder = true
	}

	// Mask password values with bullet characters.
	if isPassword {
		displayText = strings.Repeat("•", len([]rune(displayText)))
	}

	// Resolve font from the element's computed style.
	font := toGraphicsFont(st)

	// Compute ascent/descent for vertical centering.
	ascent := c.FontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	textHeight := ascent + descent

	// Left padding from resolved style (matching the UA stylesheet's input padding).
	padX := lengthValue(st.PaddingLeft)
	if padX <= 0 {
		padX = 4
	}
	padR := lengthValue(st.PaddingRight)
	if padR <= 0 {
		padR = 4
	}

	// Vertically center the text within the border box:
	// baseline = top + (boxHeight - textHeight)/2 + ascent.
	baselineY := y + (h-textHeight)/2 + ascent
	runes := []rune(displayText)

	// Horizontal scroll: keep the caret inside the visible content area by
	// shifting the text left/right (browsers scroll single-line inputs
	// horizontally instead of ellipsizing overflow). Computed per-frame from
	// the current caret position; non-focused controls just clip. The offset
	// is published globally so caret drawing / hit-testing / IME positioning
	// stay consistent across single-line and pre-mode textarea.
	contentW := w - padX - padR
	// Horizontal scroll: keep the caret inside the visible content area by
	// shifting the text left/right (browsers scroll single-line inputs
	// horizontally instead of ellipsizing overflow). Computed per-frame from
	// the current caret position; non-focused controls just clip. The offset
	// is published globally so caret drawing / hit-testing / IME positioning
	// stay consistent across single-line and pre-mode textarea.
	//
	// The previous frame's offset is preserved (so the scrollbar thumb can
	// drag the text freely); auto-scroll only kicks back in when the caret
	// leaves the visible content area.
	textScrollX := FormControlTextScroll(el)
	if !showPlaceholder && FocusedFormControlSel != nil && el == FocusedFormControl {
		caretPos := FocusedFormControlSel.End
		if FocusedFormControlSel.Start > caretPos {
			caretPos = FocusedFormControlSel.Start
		}
		if caretPos > len(runes) {
			caretPos = len(runes)
		}
		if caretPos < 0 {
			caretPos = 0
		}
		caretPx := graphics.MeasureText(font, string(runes[:caretPos]))
		autoX := computeTextScrollX(caretPx, contentW, textScrollX)
		if caretPx < textScrollX || caretPx > textScrollX+contentW {
			textScrollX = autoX
		}
	}
	SetFormControlTextScroll(el, textScrollX)
	textX := x + padX - textScrollX

	// Clip to the input's content area so long text doesn't overflow.
	// Clip to the input's content area so long text doesn't overflow.
	info.canvas.Save()
	info.canvas.Clip(graphics.Rect{X: x, Y: y, Width: w, Height: h})

	// Draw the display text, splitting into pre/selected/post segments when
	// there is an active selection in this element.
	if FocusedFormControlSel != nil && el == FocusedFormControl && !showPlaceholder {
		sel := FocusedFormControlSel
		selStart := sel.Start
		selEnd := sel.End
		if selStart > len(runes) {
			selStart = len(runes)
		}
		if selEnd > len(runes) {
			selEnd = len(runes)
		}
		if selStart > selEnd {
			selStart, selEnd = selEnd, selStart
		}

		if selStart != selEnd {
			// There is an actual selection (not just a caret).
			preText := string(runes[:selStart])
			selText := string(runes[selStart:selEnd])
			postText := string(runes[selEnd:])

			preW := graphics.MeasureText(font, preText)
			selW := graphics.MeasureText(font, selText)

			// Draw selection highlight rectangle behind the selected text.
			// Browsers highlight the FULL content-area height of a
			// single-line input (vertically centered), not just the glyph
			// height — a glyph-only rect looks like a small uncentered
			// patch inside a tall input.
			selColor := graphics.Color{R: 50, G: 100, B: 200, A: 150}
			padY := lengthValue(st.PaddingTop)
			padB := lengthValue(st.PaddingBottom)
			if padY < 0 {
				padY = 0
			}
			if padB < 0 {
				padB = 0
			}
			selTop := y + padY
			selH := h - padY - padB
			if selH < textHeight {
				selH = textHeight
			}
			c.FillRect(textX+preW, selTop, selW, selH, selColor)

			// Draw pre-selection text in normal color.
			if preText != "" {
				c.DrawText(textX, baselineY, preText, font, textColor)
			}
			// Draw selected text in white (inverted).
			if selText != "" {
				c.DrawText(textX+preW, baselineY, selText, font,
					graphics.Color{R: 255, G: 255, B: 255, A: 255})
			}
			// Draw post-selection text in normal color.
			if postText != "" {
				postW := preW + selW
				c.DrawText(textX+postW, baselineY, postText, font, textColor)
			}

			// Draw the caret at the selection end (right edge of selected text).
			paintFormControlCaret(info, el, st, x, y, w, h, preW+selW-textScrollX, op, 0)
		} else {
			// Caret only (Start == End): draw text normally, caret at offset.
			c.DrawText(textX, baselineY, displayText, font, textColor)
			caretOffset := graphics.MeasureText(font, string(runes[:selStart]))
			paintFormControlCaret(info, el, st, x, y, w, h, caretOffset-textScrollX, op, 0)
		}
	} else {
		// No selection: draw the full text normally.
		c.DrawText(textX, baselineY, displayText, font, textColor)

		// Draw the caret at the end of the value text when there is one, or at
		// the start (textWidth=0) when only the placeholder is shown — matching
		// how browsers place the caret at the beginning of an empty focused input.
		caretW := 0.0
		if !showPlaceholder {
			caretW = graphics.MeasureText(font, displayText)
		}
		paintFormControlCaret(info, el, st, x, y, w, h, caretW-textScrollX, op, 0)
	}
	info.canvas.Restore()
}
// for form controls that have no RenderText (so the regular PaintCaret path
// cannot find them). sy is the textarea's vertical scroll offset (0 for
// single-line inputs) so the caret tracks the scrolled text rows.
func paintFormControlCaret(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h, textWidth, op, sy float64) {
	if el != FocusedFormControl || !CaretVisibleControl {
		return
	}
	// ★ opacity:0 的控件（xterm 的 helper textarea 是 opacity:0 覆盖层）
	// 不画引擎 caret——浏览器里隐藏控件不渲染光标；xterm 自己画
	// .xterm-cursor。此前引擎在 xterm textarea 聚焦时把 caret 画在
	// 终端区域左上角（双光标/干扰）。
	if st.Opacity <= 0 {
		return
	}
	c := info.canvas
	font := toGraphicsFont(st)
	ascent := c.FontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	textHeight := ascent + descent
	padX := lengthValue(st.PaddingLeft)
	if padX <= 0 {
		padX = 4
	}
	padR := lengthValue(st.PaddingRight)
	if padR <= 0 {
		padR = 4
	}
	padY := lengthValue(st.PaddingTop)
	if padY <= 0 {
		padY = 4
	}
	// Line height honoring CSS line-height so multi-line carets land on the
	// painted row (a textarea with line-height:1.5 draws 19.5px rows).
	lineH := cssControlLineHeight(st, font.Size)
	if lineH <= 0 {
		lineH = textHeight
	}
	if lineH <= 0 {
		lineH = font.Size * 1.2
	}

	caretX := x + padX + textWidth
	caretY := y + (h-textHeight)/2

	// Multi-line (textarea): position the caret at the row/column of the
	// current selection Start instead of the box's vertical center. Wrap
	// mode comes from white-space; the X is scrolled back by the published
	// horizontal offset so the caret stays visible in pre/nowrap mode.
	if el.LocalName() == "textarea" {
		value := el.TextContent()
		runes := []rune(value)
		pos := 0
		if FocusedFormControlSel != nil {
			pos = FocusedFormControlSel.Start
			if FocusedFormControlSel.End > pos {
				pos = FocusedFormControlSel.End
			}
		}
		if pos < 0 {
			pos = 0
		}
		if pos > len(runes) {
			pos = len(runes)
		}
		contentW := w - padX - padR
		if contentW < 1 {
			contentW = 1
		}
		wrapped := wrapTextAreaLines(value, font, contentW, textareaWrapMode(st, el))
		row, col, _ := locateWrappedCaret(wrapped, pos)
		lineStart := pos - col
		colW := graphics.MeasureText(font, string(runes[lineStart:pos]))
		caretX = x + padX + colW - FormControlTextScroll(el)
		caretY = y + padY + float64(row)*lineH - sy
	}

	caretCol := applyOpacity(toGraphicsColor(st.Color), op)
	if caretCol.A == 0 {
		caretCol = graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
	}
	c.FillRect(caretX, caretY, 1, lineH, caretCol)
}

// FormControlCaretPosition returns the CSS-pixel position of the text caret
// for the currently focused form control (input/textarea), or false when no
// caret is active. The host calls this each frame to position the IME
// composition/candidate window next to the caret (previously the IME window
// stayed at the top-left corner because the position was never updated).
func FormControlCaretPosition(rv *RenderView) (x, y float64, ok bool) {
	el := FocusedFormControl
	if el == nil || rv == nil {
		return 0, 0, false
	}
	// Find the render box for the focused element.
	var box *RenderBox
	var walk func(o RenderObject) bool
	walk = func(o RenderObject) bool {
		if o == nil {
			return false
		}
		if o.Node() == el {
			if rb := asRenderBox(o); rb != nil {
				box = rb
				return true
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(RenderObject(rv))
	if box == nil {
		return 0, 0, false
	}
	st := box.Style()
	if st == nil {
		return 0, 0, false
	}
	font := toGraphicsFont(st)
	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	textHeight := ascent + descent
	padX := lengthValue(st.PaddingLeft)
	if padX <= 0 {
		padX = 4
	}
	padR := lengthValue(st.PaddingRight)
	if padR <= 0 {
		padR = 4
	}
	padY := lengthValue(st.PaddingTop)
	if padY <= 0 {
		padY = 4
	}
	lineH := cssControlLineHeight(st, font.Size)
	if lineH <= 0 {
		lineH = textHeight
	}
	if lineH <= 0 {
		lineH = font.Size * 1.2
	}
	boxX, boxY := box.X(), box.Y()
	// ★ 祖先滚动补偿：box.X/Y 是布局坐标，IME 候选窗口需跟随「可见」
	// 光标（视口坐标）——控件在滚动容器内时不补偿则候选窗口偏离。
	if sx, sy3 := rv.ScrollStackOffsetFor(box); sx != 0 || sy3 != 0 {
		boxX -= sx
		boxY -= sy3
	}
	boxW := box.Width()
	boxH := box.Height()
	caretX := boxX + padX
	caretY := boxY + (boxH-textHeight)/2
	value := focusedControlText(el)
	runes := []rune(value)
	pos := 0
	if FocusedFormControlSel != nil {
		pos = FocusedFormControlSel.Start
		if FocusedFormControlSel.End > pos {
			pos = FocusedFormControlSel.End
		}
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	if el.LocalName() == "textarea" {
		contentW := boxW - padX - padR
		if contentW < 1 {
			contentW = 1
		}
		wrapped := wrapTextAreaLines(value, font, contentW, textareaWrapMode(st, el))
		row, col, _ := locateWrappedCaret(wrapped, pos)
		lineStart := pos - col
		colW := graphics.MeasureText(font, string(runes[lineStart:pos]))
		// Horizontal scroll compensation: the caret may be scrolled out of
		// view in pre/nowrap mode; the IME anchor follows the VISIBLE caret.
		caretX = boxX + padX + colW - FormControlTextScroll(el)
		// Vertical scroll compensation: the IME anchor follows the VISIBLE
		// caret row (scrolled up by BoxScrollOffset.sy).
		_, sy := rv.BoxScrollOffset(box)
		caretY = boxY + padY + float64(row)*lineH - sy
		// Anchor IME candidate window BELOW the caret line (bottom + gap) so
		// the candidate list renders under the text, not flush with the
		// caret bottom. The returned point is only consumed by the host for
		// IME positioning.
		caretY += lineH + imeCandidateGap(lineH)
	} else {
		caretX = boxX + padX + graphics.MeasureText(font, string(runes[:pos])) - FormControlTextScroll(el)
		// Single-line input: anchor IME candidate window below the text too.
		caretY += lineH + imeCandidateGap(lineH)
	}
	return caretX, caretY, true
}

// imeCandidateGap returns the vertical gap (CSS px) between the caret
// bottom and the IME candidate window anchor. Browsers render candidates a
// few px below the caret; a pure bottom-anchor made the candidate list
// flush with the caret line (reported as "candidate aligned with caret
// bottom instead of below it").
func imeCandidateGap(lineH float64) float64 {
	gap := lineH * 0.3
	if gap < 4 {
		gap = 4
	}
	if gap > 10 {
		gap = 10
	}
	return gap
}

// focusedControlText returns the displayed text of the focused control.
func focusedControlText(el *dom.Element) string {
	if el == nil {
		return ""
	}
	if el.LocalName() == "textarea" {
		return el.TextContent()
	}
	return el.GetAttribute("value")
}

// wrappedLine is one visual line of a wrapped textarea, carrying the rune
// offset range of its text within the original value (selection mapping).
type wrappedLine struct {
	text  string
	start int // rune offset into original text
	end   int // exclusive
}

// Wrap strategies for textarea rows, derived from the CSS white-space
// property (browsers honor it: pre/nowrap → no soft wrap + horizontal
// scroll; pre-wrap → wrap at any character; normal/pre-line → wrap at
// spaces). Textarea UA default is pre-wrap.
const (
	wrapModeNone     = iota // pre / nowrap: hard '\n' breaks only, rows may exceed width
	wrapModeAnywhere         // pre-wrap / break-spaces: wrap at any character
	wrapModeSpaces           // normal / pre-line: wrap at word boundaries
)

// textareaWrapMode derives the wrap strategy from the CSS white-space
// property and the HTML wrap attribute (<textarea wrap="off"> disables soft
// wrapping like white-space: pre). Textarea UA default is pre-wrap.
func textareaWrapMode(st *style.ComputedStyle, el *dom.Element) int {
	if el != nil && el.LocalName() == "textarea" {
		if in, ok := html5.ToTextAreaElement(el); ok && in.Wrap() == "off" {
			return wrapModeNone
		}
	}
	if st == nil {
		return wrapModeAnywhere // UA default: pre-wrap
	}
	switch st.WhiteSpace {
	case style.WhiteSpacePre, style.WhiteSpaceNoWrap:
		return wrapModeNone
	case style.WhiteSpaceNormal, style.WhiteSpacePreLine, style.WhiteSpaceBreakSpaces:
		return wrapModeSpaces
	}
	return wrapModeAnywhere
}

// formControlScroll stores the horizontal text-scroll offset (CSS px) of
// each form control (input / textarea), scoped PER ELEMENT — a pre-mode
// textarea's horizontal scroll must never leak into a sibling single-line
// input. Painted controls recompute their own offset each frame so the
// caret stays visible; scrollbar drag/arrow/track operations write it;
// hit-testing and IME positioning read it back to map coordinates
// correctly (a scrolled caret is drawn at x - scroll; clicks at x map back
// to x + scroll). The map is only touched from the render/event loop
// (single-threaded), mirroring FocusedFormControlSel.
var formControlScroll = make(map[*dom.Element]float64)

// FormControlTextScroll returns the last painted horizontal scroll offset
// of a form control element (0 when it was never painted).
func FormControlTextScroll(el *dom.Element) float64 {
	if el == nil {
		return 0
	}
	return formControlScroll[el]
}

// SetFormControlTextScroll records the horizontal scroll offset of a form
// control element (used by scrollbar drag/arrow/track operations and the
// painter's per-frame recompute).
func SetFormControlTextScroll(el *dom.Element, v float64) {
	if el == nil {
		return
	}
	formControlScroll[el] = v
}

// wrapTextAreaLines breaks text into visual lines honoring hard '\n' breaks
// and soft-wrapping per mode (see wrapMode*). Rows are never ellipsized —
// wrapModeNone rows may exceed the content width and scroll horizontally.
// Returns at least one line so an empty value still maps to row 0.
func wrapTextAreaLines(text string, font graphics.Font, contentW float64, mode int) []wrappedLine {
	var out []wrappedLine
	off := 0 // rune offset into text (includes '\n' chars)
	for _, hard := range strings.Split(text, "\n") {
		runes := []rune(hard)
		if len(runes) == 0 {
			out = append(out, wrappedLine{text: "", start: off, end: off})
			off++ // skip the '\n'
			continue
		}
		rest := runes
		restStart := off
		for len(rest) > 0 {
			if graphics.MeasureText(font, string(rest)) <= contentW || mode == wrapModeNone {
				out = append(out, wrappedLine{text: string(rest), start: restStart, end: restStart + len(rest)})
				rest = nil
				break
			}
			// Longest prefix that fits (binary search).
			lo, hi := 1, len(rest)
			for lo < hi {
				mid := (lo + hi + 1) / 2
				if graphics.MeasureText(font, string(rest[:mid])) <= contentW {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			if mode == wrapModeSpaces {
				// Word-boundary wrap: prefer the last space inside the
				// fitting prefix; the trailing space is dropped (browser
				// collapses it at the break).
				lastSpace := -1
				for i := 0; i < lo; i++ {
					if rest[i] == ' ' || rest[i] == '\t' {
						lastSpace = i
					}
				}
				if lastSpace > 0 {
					out = append(out, wrappedLine{text: string(rest[:lastSpace]), start: restStart, end: restStart + lastSpace})
					restStart += lastSpace + 1 // skip the space
					rest = rest[lastSpace+1:]
					continue
				}
				// No space in the fitting prefix → break the long word.
			}
			out = append(out, wrappedLine{text: string(rest[:lo]), start: restStart, end: restStart + lo})
			restStart += lo
			rest = rest[lo:]
		}
		off += len(runes) + 1 // '\n'
	}
	if len(out) == 0 {
		out = append(out, wrappedLine{})
	}
	return out
}

// computeTextScrollX returns the horizontal scroll offset that keeps the
// caret (at caretPx, measured from the text origin) visible inside a
// content area of contentW CSS px. Stable: the caret only scrolls when it
// leaves [0, contentW]; returning to the left edge resets it.
func computeTextScrollX(caretPx, contentW, curSx float64) float64 {
	if contentW <= 0 {
		return 0
	}
	sx := curSx
	if caretPx-sx < 0 {
		sx = caretPx
	}
	if caretPx-sx > contentW {
		sx = caretPx - contentW
	}
	return sx
}

// locateWrappedCaret finds the visual row/col of a rune offset within
// soft-wrapped textarea lines, plus the wrapped line itself. The caret sits
// at the end of the line whose range contains pos (pos == end is the line's
// trailing edge — never pushed to the next line).
func locateWrappedCaret(lines []wrappedLine, pos int) (row, col int, wl wrappedLine) {
	for i := range lines {
		wl = lines[i]
		if pos >= wl.start && pos <= wl.end {
			return i, pos - wl.start, wl
		}
	}
	if len(lines) > 0 {
		wl = lines[len(lines)-1]
		return len(lines) - 1, len([]rune(wl.text)), wl
	}
	return 0, 0, wrappedLine{}
}

// cssControlLineHeight resolves the CSS line-height (multiplier, px, %) into
// pixels for a given font size, returning 0 when not set.
func cssControlLineHeight(st *style.ComputedStyle, fontSize float64) float64 {
	if st == nil {
		return 0
	}
	lh := st.LineHeight
	switch lh.Unit {
	case "px":
		if lh.Value > 0 {
			return lh.Value
		}
	case "%":
		if lh.Value > 0 {
			return lh.Value / 100 * fontSize
		}
	case "":
		if lh.Value > 0 {
			return lh.Value * fontSize
		}
	}
	return 0
}

// paintCheckbox draws a classic checkbox: a square border with a checkmark when checked.
// Mirrors RenderTheme::paintCheckbox. Uses default classic colors for the widget
// background (light) independent of the element's background-color, matching
// browser behavior where form controls use OS-native widget colors. The
// checkmark color uses the element's text color (accent-color equivalent).
func paintCheckbox(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, checked bool, op float64) {
	c := info.canvas
	size := w
	if h < size {
		size = h
	}
	// Center the square in the box.
	cx := x + (w-size)/2
	cy := y + (h-size)/2
	// Classic widget background: always light (white) by default, like browser's
	// native form control rendering. The element's background-color is NOT used
	// here because form controls are replaced elements whose appearance is
	// independent of the page theme.
	bg := FormControlColors.CheckboxBg
	if checked {
		// Checked state: slightly tinted background for visual feedback.
		bg = FormControlColors.CheckboxBgHot
	}
	border := FormControlColors.CheckboxBorder
	c.FillRect(cx, cy, size, size, applyOpacity(bg, op))
	c.StrokeRect(cx, cy, size, size, 1, applyOpacity(border, op))
	if checked {
		// Use element's text color for checkmark (acts as accent-color),
		// fall back to default dark checkmark.
		check := toGraphicsColor(st.Color)
		if check.A == 0 {
			check = FormControlColors.CheckboxCheck
		}
		check = applyOpacity(check, op)
		// Draw a simple checkmark: two line segments forming an "L" rotated.
		inset := size * 0.22
		p1x := cx + inset
		p1y := cy + size*0.55
		p2x := cx + size*0.42
		p2y := cy + size - inset
		p3x := cx + size - inset
		p3y := cy + inset
		strokeW := size * 0.14
		c.StrokeLine(p1x, p1y, p2x, p2y, strokeW, check)
		c.StrokeLine(p2x, p2y, p3x, p3y, strokeW, check)
	}
}

// paintRadio draws a classic radio button: a circle with a filled dot when checked.
// Mirrors RenderTheme::paintRadio. Uses classic widget colors (light bg) matching
// browser native rendering. The dot color uses the element's text color.
func paintRadio(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, checked bool, op float64) {
	c := info.canvas
	size := w
	if h < size {
		size = h
	}
	cx := x + w/2
	cy := y + h/2
	radius := size / 2
	// Classic widget background: always light, like browser native rendering.
	// Checked state uses the Windows accent blue (matches Edge).
	bg := FormControlColors.RadioBg
	if checked {
		bg = FormControlColors.RadioBgHot
	}
	border := FormControlColors.RadioBorder
	// Outer circle (background + border).
	c.FillCircle(cx, cy, radius, applyOpacity(bg, op))
	c.StrokeCircle(cx, cy, radius, 1, applyOpacity(border, op))
	if checked {
		// Use element's text color for the dot (accent-color equivalent),
		// fall back to default dark.
		dot := toGraphicsColor(st.Color)
		if dot.A == 0 {
			dot = FormControlColors.RadioDot
		}
		// Inner dot: ~45% of the outer radius.
		c.FillCircle(cx, cy, radius*0.45, applyOpacity(dot, op))
	}
}

// paintRangeSlider draws a horizontal slider: a track plus a thumb positioned at the
// paintRangeSlider draws a horizontal slider: a two-segment rounded track plus
// a thumb positioned at the value's fraction along the range. Mirrors
// RenderTheme::paintSlider. Pixel-fitted against Edge (Chromium) native range
// (edge_range.png 实测, 2026-08-08):
//   - track: 固定 6px 高（UA 固定值，与 input box 高度无关），圆角 3px，
//     两段式：已填充段 (0..value) accent blue #0075FF、未填充段灰 #EFEFEF。
//     Edge 像素：grey track 仅 6 行（y=73..78 与 y=105..110）。
//   - thumb: 固定 14px 直径圆（Edge 像素 blue 段仅 14 行 y=69..82），
//     body #0075FF。Chromium 深色主题悬停变色是「分开」的（用户实测）：
//     悬停在圆(thumb)上 → 蓝条 fill + 圆整体变暗；悬停在条(track)上 →
//     只有蓝条 fill 变暗（圆不变、灰 track 不变）。:active 按下时蓝条+
//     圆整体更暗。鼠标在圆内与否用 RenderView.CursorPos（app.Host 每次
//     鼠标移动写入，页面 CSS 坐标）判定——与 :hover/:active 伪类同源。
func paintRangeSlider(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, in html5.HTMLInputElement, op float64) {
	c := info.canvas
	// Track: 固定 6px 圆角条，垂直居中于 input box。浏览器 UA 是固定尺寸
	// （不随 input 高度缩放），此前用 h*0.5 导致 input≈21px 时 track 10px
	// 过粗（浏览器 6px）。
	const trackH = 6.0
	trackY := y + (h-trackH)/2

	min, max := rangeBounds(in)
	val := parseFloatOr(in.Value(), min)
	frac := 0.0
	if max > min {
		frac = (val - min) / (max - min)
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
	}

	// Thumb 几何：固定 14px 直径圆（浏览器 UA 固定值），居中于 track。
	const thumbR = 7.0
	thumbX := x + frac*w
	cy := y + h/2

	// 悬停变色（与浏览器分开，见函数注释）。
	fillCol := FormControlColors.SliderFill
	thumbCol := FormControlColors.SliderThumb
	if in.El != nil {
		onThumb := false
		if info.rv != nil {
			if mx, my := info.rv.CursorPos(); mx > -1e8 {
				// 与 app.setRangeValueFromX 同一套绝对坐标几何：x 是
				// 局部坐标（canvas 已 translate），鼠标位置是页面坐标，
				// 必须用 AbsoluteX/Width 换算 thumb 绝对圆心再判圆内。
				if ab := info.rv.FindRenderBoxForNode(in.El); ab != nil {
					absLeft := ab.AbsoluteX()
					if _, so := info.rv.BoxScrollOffset(ab); so > 0 {
						absLeft -= so
					}
					absThumbX := absLeft + frac*ab.Width()
					absCy := ab.AbsoluteY() + ab.Height()/2
					dx, dy := mx-absThumbX, my-absCy
					onThumb = dx*dx+dy*dy <= thumbR*thumbR
				}
			}
		}
		if in.El.IsActive() {
			// 按下（:active）：蓝条 + 圆整体更暗。
			fillCol = mixWithBlack(fillCol, 0.30)
			thumbCol = mixWithBlack(thumbCol, 0.30)
		} else if in.El.IsHovered() {
			// 悬停条 → 只蓝条变暗；悬停圆 → 圆也变暗（整体）。
			fillCol = mixWithBlack(fillCol, 0.15)
			if onThumb {
				thumbCol = mixWithBlack(thumbCol, 0.15)
			}
		}
	}

	// Unfilled portion first (whole track), then filled portion over it.
	c.FillRoundRect(x, trackY, w, trackH, trackH/2, applyOpacity(FormControlColors.SliderTrack, op))
	if frac > 0 {
		fillW := w * frac
		c.FillRoundRect(x, trackY, fillW, trackH, trackH/2, applyOpacity(fillCol, op))
	}
	c.FillCircle(thumbX, cy, thumbR, applyOpacity(thumbCol, op))
}

// mixWithBlack linearly blends a color toward black by the given factor
// (0..1). Used for slider thumb hover/active darkening — Chromium dark
// theme darkens the thumb while hovered and more while pressed.
func mixWithBlack(c graphics.Color, f float64) graphics.Color {
	if f <= 0 {
		return c
	}
	if f > 1 {
		f = 1
	}
	return graphics.Color{
		R: uint8(float64(c.R) * (1 - f)),
		G: uint8(float64(c.G) * (1 - f)),
		B: uint8(float64(c.B) * (1 - f)),
		A: c.A,
	}
}

// paintProgressBar draws a <progress> element: a rounded track with a filled portion
// proportional to value/max. Indeterminate progress bars (no value attribute) show an
// empty track. Mirrors RenderTheme::paintProgressBar. Default theme colors are used
// for the widget, matching browser native rendering.
func paintProgressBar(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, el *dom.Element, op float64) {
	c := info.canvas
	p, ok := html5.ToProgressElement(el)
	if !ok {
		return
	}
	// Track: use default theme color (browsers paint the track independently of CSS).
	c.FillRoundRect(x, y, w, h, h/2, applyOpacity(FormControlColors.ProgressTrack, op))
	if p.Indeterminate() {
		return
	}
	max := p.Max()
	if max <= 0 {
		max = 1
	}
	frac := p.Value() / max
	if frac <= 0 {
		return
	}
	if frac > 1 {
		frac = 1
	}
	fillW := w * frac
	if fillW < h {
		fillW = h
	}
	// Use element's text color as accent fill, fall back to default blue.
	fillCol := toGraphicsColor(st.Color)
	if fillCol.A == 0 {
		fillCol = FormControlColors.ProgressFill
	}
	c.FillRoundRect(x, y, fillW, h, h/2, applyOpacity(fillCol, op))
}

// paintMeterBar draws a <meter> element: a track with a colored fill whose color depends
// on the value's position relative to low/high/optimum zones. Mirrors
// RenderTheme::paintMeter. Default theme colors for the track.
func paintMeterBar(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, el *dom.Element, op float64) {
	c := info.canvas
	m, ok := html5.ToMeterElement(el)
	if !ok {
		return
	}
	// Track: use default theme color.
	c.FillRoundRect(x, y, w, h, h/2, applyOpacity(FormControlColors.ProgressTrack, op))
	max := m.Max()
	min := m.Min()
	if max <= min {
		return
	}
	val := m.Value()
	if val < min {
		val = min
	} else if val > max {
		val = max
	}
	frac := (val - min) / (max - min)
	if frac <= 0 {
		return
	}
	if frac > 1 {
		frac = 1
	}
	fillW := w * frac
	if fillW < h {
		fillW = h
	}
	// Choose the fill color based on the optimum zone.
	low := m.Low()
	high := m.High()
	optimum := m.Optimum()
	fillColor := FormControlColors.MeterOptimum
	if optimum <= low {
		if val <= low {
			fillColor = FormControlColors.MeterOptimum
		} else if val <= high {
			fillColor = FormControlColors.MeterSuboptimal
		} else {
			fillColor = FormControlColors.MeterHigh
		}
	} else if optimum >= high {
		if val >= high {
			fillColor = FormControlColors.MeterOptimum
		} else if val >= low {
			fillColor = FormControlColors.MeterSuboptimal
		} else {
			fillColor = FormControlColors.MeterHigh
		}
	} else {
		// optimum is in the middle zone.
		if val < low || val > high {
			fillColor = FormControlColors.MeterSuboptimal
		} else {
			fillColor = FormControlColors.MeterOptimum
		}
	}
	c.FillRoundRect(x, y, fillW, h, h/2, applyOpacity(fillColor, op))
}

// paintColorSwatch draws an <input type="color"> as a colored rectangle showing the
// current value. Mirrors RenderTheme::paintColorWell.
func paintColorSwatch(info *PaintInfo, x, y, w, h float64, value string, op float64) {
	c := info.canvas
	col := parseHexColor(value)
	c.FillRect(x, y, w, h, applyOpacity(col, op))
}

// paintSelectArrow draws a downward-pointing triangle on the right side of a <select>
// element, indicating it is a dropdown. The selected option's text is painted by the
// normal text path, so this only adds the arrow indicator. Mirrors
// paintSelectText draws the selected option's text in the <select> box,
// mirroring RenderMenuList::paintMenuListText. The text is left-aligned,
// vertically centered, and clipped to avoid overlapping the arrow area on
// the right. If the text is too long, it is truncated with an ellipsis.
func paintSelectText(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas

	// Read the selected option's TEXT (not its value attribute) — the
	// browser paints option.textContent inside the closed box.
	sel, ok := html5.ToSelectElement(el)
	if !ok {
		return
	}
	selectedText := sel.SelectedText() // e.g. "DeepSeek" not "deepseek"

	if selectedText == "" {
		return
	}

	// Resolve font from computed style.
	font := toGraphicsFont(st)
	ascent := c.FontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	textHeight := ascent + descent

	// Left padding from resolved style.
	padX := lengthValue(st.PaddingLeft)
	if padX <= 0 {
		padX = 4
	}

	// Right margin for the dropdown arrow. The chevron is a fixed 8px wide,
	// centered ~9.5px from the right edge → reserve ~16px so text clips
	// before the arrow (matches browser layout).
	arrowReserve := 16.0

	textColor := applyOpacity(toGraphicsColor(st.Color), op)
	baselineY := y + (h-textHeight)/2 + ascent
	textX := x + padX
	maxTextW := w - padX - arrowReserve

	// Clip to ensure text doesn't overflow into the arrow area.
	info.canvas.Save()
	info.canvas.Clip(graphics.Rect{X: x, Y: y, Width: w - arrowReserve, Height: h})

	// Truncate text with "…" only when it does not fit the box. When it
	// fits (the normal case), draw the full text — never collapse a fitting
	// text into an ellipsis.
	displayText := selectedText
	textW := graphics.MeasureText(font, displayText)
	if textW > maxTextW {
		// 超宽：逐字符截断直到 fits（保留省略号）。与浏览器 text-overflow
		// 一致：截断后的字符 + "…" 必须能放入 maxTextW。
		runes := []rune(displayText)
		for len(runes) > 0 {
			candidate := string(runes) + "…"
			if graphics.MeasureText(font, candidate) <= maxTextW {
				displayText = candidate
				break
			}
			runes = runes[:len(runes)-1]
		}
		if len(runes) == 0 {
			displayText = "…"
		}
	}
	if debugPaintLog {
		log.Printf("[dbg/paint] paintSelectText text=%q at (%.0f,%.0f) font=%s/%v color=(%d,%d,%d) maxW=%.0f",
			displayText, textX, baselineY, font.Family, font.Size, textColor.R, textColor.G, textColor.B, maxTextW)
	}
	c.DrawText(textX, baselineY, displayText, font, textColor)
	info.canvas.Restore()
}

// paintButtonBox draws the background and border of a <button> element.

func paintButtonBox(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	c := info.canvas
	// Use the style's background-color if set, otherwise default to #f0f0f0.
	bg := toGraphicsColor(st.BackgroundColor)
	if bg.A == 0 {
		bg = graphics.Color{R: 240, G: 240, B: 240, A: 255}
	}
	bg = applyOpacity(bg, op)
	radius := 3.0
	c.FillRoundRect(x, y, w, h, radius, bg)
	// Border: use style's border color if available, or a default gray.
	borderCol := toGraphicsColor(st.BorderColor("top"))
	if borderCol.A == 0 {
		borderCol = graphics.Color{R: 204, G: 204, B: 204, A: 255}
	}
	borderCol = applyOpacity(borderCol, op)
	c.StrokeRoundRect(x+0.5, y+0.5, w-1, h-1, radius, 1, borderCol)
}

// paintButtonText draws the text content of a <button> element centered.
func paintButtonText(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas
	text := strings.TrimSpace(el.TextContent())
	if text == "" {
		return
	}
	font := toGraphicsFont(st)
	textColor := applyOpacity(toGraphicsColor(st.Color), op)
	if textColor.A == 0 {
		textColor = graphics.Color{R: 0, G: 0, B: 0, A: 255}
	}
	textW := graphics.MeasureText(font, text)
	ascent := c.FontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	// ★ 垂直居中（浏览器按钮文字 = glyph 中心对齐盒子中心）：baseline 必须
	// 用 glyph 实际度量计算，不能用 usWinAscent/usWinDescent——usWinAscent
	// （如 Segoe UI 12px ≈ 11.2）比字形 bbox ascent（capHeight ≈ 8.6）大，
	// 用它算 textH + baseline 会让文字整体偏下 1-2px（配置器 −/+ 按钮
	// 文字偏下的根因）。与 painter.go 的 flex/行框居中同款：
	//   - CJK 文本用雅黑度量（FontCJKMetrics，glyph 顶 ≈ 0.85em）
	//   - 拉丁/符号用 capHeight + 0.3×capHeight 近似 bbox descent
	var drawAscent, drawDescent float64
	if hasCJKChars([]rune(text)) {
		drawAscent, drawDescent = c.FontCJKMetrics(font)
	} else {
		drawAscent = c.FontCapHeight(font)
		if drawAscent <= 0 {
			drawAscent = ascent
		}
		drawDescent = drawAscent * 0.3
	}
	if drawAscent <= 0 {
		drawAscent = ascent
		drawDescent = 0
	}
	fontH := drawAscent + drawDescent
	tx := x + (w-textW)/2
	ty := y + (h-fontH)/2 + drawAscent
	c.DrawText(tx, ty, text, font, textColor)
}

// paintTextAreaText draws the text content of a <textarea> element.
// The text is left-aligned, top-aligned within the content area, with
// word wrapping at the content width. A blinking caret is drawn at the
// end of the text when this element is focused.
func paintTextAreaText(info *PaintInfo, el *dom.Element, st *style.ComputedStyle, x, y, w, h float64, op float64, sy float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas
	value := el.TextContent()
	displayText := value
	textColor := applyOpacity(toGraphicsColor(st.Color), op)

	// Placeholder support
	if displayText == "" {
		placeholder := el.GetAttribute("placeholder")
		if placeholder == "" {
			paintFormControlCaret(info, el, st, x, y, w, h, 0, op, sy)
			return
		}
		displayText = placeholder
		textColor = applyOpacity(graphics.Color{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}, op)
	}

	font := toGraphicsFont(st)
	ascent := c.FontAscent(font)
	if ascent <= 0 {
		ascent = font.Size * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	// Line height honoring CSS line-height so painted rows and caret rows
	// match (a textarea with line-height:1.5 at 13px draws 19.5px rows).
	lineH := cssControlLineHeight(st, font.Size)
	if lineH <= 0 {
		lineH = ascent + graphics.GlobalFontDescent(font)
	}
	if lineH <= 0 {
		lineH = font.Size * 1.2
	}

	padX := lengthValue(st.PaddingLeft)
	if padX <= 0 {
		padX = 4
	}
	padY := lengthValue(st.PaddingTop)
	if padY <= 0 {
		padY = 4
	}

	textX := x + padX
	// Vertical scroll (BoxScrollOffset.sy, set by scrollbar drag / wheel):
	// the text starts above the box by sy so scrolled-down content becomes
	// visible — the scrollbar thumb follows BoxScrollOffset while the text
	// used to stay put (user: "scrollbar moves but content doesn't").
	textY := y + padY + ascent - sy
	if debugenv.Enabled("WB_TA_DEBUG") {
		log.Printf("[ta] paint sy=%.1f boxY=%.1f textY=%.1f", sy, y, textY)
	}

	// Selection range (in runes) when this textarea is the focused control.
	selStart, selEnd := -1, -1
	hasSel := false
	if FocusedFormControlSel != nil && el == FocusedFormControl {
		s0, s1 := FocusedFormControlSel.Start, FocusedFormControlSel.End
		if s0 > s1 {
			s0, s1 = s1, s0
		}
		if s0 != s1 {
			selStart, selEnd = s0, s1
			hasSel = true
		}
	}
	selColor := graphics.Color{R: 50, G: 100, B: 200, A: 150}

	// Clip to content area
	info.canvas.Save()
	info.canvas.Clip(graphics.Rect{X: x, Y: y, Width: w, Height: h})

	// Draw line by line honoring the white-space wrap mode (pre-wrap wraps,
	// pre/nowrap scrolls horizontally — never ellipsized).
	contentW := w - padX*2
	if contentW < 1 {
		contentW = 1
	}
	mode := textareaWrapMode(st, el)
	wrapped := wrapTextAreaLines(displayText, font, contentW, mode)

	// Horizontal scroll (white-space: pre / nowrap): rows may exceed the
	// content width — keep the caret row's caret visible by shifting all
	// rows, and publish the offset so caret drawing / hit-testing / IME
	// positioning stay consistent.
	//
	// The previous frame's offset is preserved (scrollbar thumb drags the
	// text freely); auto-scroll resumes only when the caret leaves the
	// visible content area.
	textScrollX := FormControlTextScroll(el)
	if mode == wrapModeNone && FocusedFormControlSel != nil && el == FocusedFormControl {
		caretPos := FocusedFormControlSel.End
		if FocusedFormControlSel.Start > caretPos {
			caretPos = FocusedFormControlSel.Start
		}
		_, col, wl := locateWrappedCaret(wrapped, caretPos)
		runes := []rune(displayText)
		caretPx := graphics.MeasureText(font, string(runes[wl.start:wl.start+col]))
		autoX := computeTextScrollX(caretPx, contentW, textScrollX)
		if caretPx < textScrollX || caretPx > textScrollX+contentW {
			textScrollX = autoX
		}
	}
	SetFormControlTextScroll(el, textScrollX)
	textX -= textScrollX

	for i, wl := range wrapped {
		line := wl.text
		lineStart := wl.start
		lineY := textY + float64(i)*lineH
		if lineY > y+h {
			break
		}
		lineRunes := []rune(line)
		lineLen := len(lineRunes)
		// Compute the selection intersection for this line
		// (lineStart..lineStart+lineLen in original text).
		selLineStart := -1
		selLineEnd := -1
		if hasSel {
			ls := selStart - lineStart
			le := selEnd - lineStart
			if ls < 0 {
				ls = 0
			}
			if le > lineLen {
				le = lineLen
			}
			if ls < lineLen && le > 0 && ls < le {
				selLineStart, selLineEnd = ls, le
			}
		}
		if line != "" || selLineStart >= 0 {
			if selLineStart >= 0 && selLineEnd <= lineLen {
				// Split the line into pre/selected/post and draw the
				// selection highlight like the single-line input.
				pre := string(lineRunes[:selLineStart])
				selText := string(lineRunes[selLineStart:selLineEnd])
				post := string(lineRunes[selLineEnd:])
				preW := graphics.MeasureText(font, pre)
				selW := graphics.MeasureText(font, selText)
				// Vertically center the highlight within the LINE BOX, not the
				// glyph box: browsers split the line-height leading evenly
				// above/below the glyphs, so the selection background covers
				// the whole row while the text sits centered in it. Anchoring
				// at (lineY - ascent) put the rect's bottom below the glyphs
				// by the full leading ("text not centered in its highlight").
				leading := lineH - (ascent + descent)
				if leading < 0 {
					leading = 0
				}
				selTop := lineY - ascent - leading/2
				c.FillRect(textX+preW, selTop, selW, lineH, selColor)
				if pre != "" {
					c.DrawText(textX, lineY, pre, font, textColor)
				}
				if selText != "" {
					c.DrawText(textX+preW, lineY, selText, font,
						graphics.Color{R: 255, G: 255, B: 255, A: 255})
				}
				if post != "" {
					c.DrawText(textX+preW+selW, lineY, post, font, textColor)
				}
			} else if line != "" {
				c.DrawText(textX, lineY, line, font, textColor)
			}
		}
	}

	// Draw the caret at the selection/caret position (paintFormControlCaret
	// re-derives the row/col from FocusedFormControlSel with soft-wrapping).
	paintFormControlCaret(info, el, st, x, y, w, h, 0, op, sy)

	// CSS resize handle (textarea resize:vertical/both — Chromium-style
	// diagonal lines in the bottom-right corner).
	paintTextAreaResizeHandle(info, st, x, y, w, h, op)

	info.canvas.Restore()
}

// ResizeModeOf 返回 CSS resize 属性的解析结果：
// 0=none（不可拖拽）、1=horizontal、2=vertical、3=both。
// 值来自 cs.Properties["resize"]（resolver 把所有声明存入 Properties；
// UA 默认 textarea{resize:both}，作者样式如 .inst-textarea{resize:vertical}
// 通过级联覆盖）。
func ResizeModeOf(st *style.ComputedStyle) int {
	if st == nil {
		return 0
	}
	switch st.Properties["resize"] {
	case "horizontal":
		return 1
	case "vertical":
		return 2
	case "both":
		return 3
	}
	return 0
}

// paintTextAreaResizeHandle 在 textarea 右下角绘制浏览器（Chromium）风格的
// resize 手柄。几何与颜色均来自 Edge headless 像素实测（edge_resize_ref.png，
// 深色主题）：右下角 15px 区域内 2 条「/」方向 45° 右对齐斜线——
//   线1: (w-2,h-7)→(w-7,h-1)（长 6px）
//   线2: (w-1,h-3)→(w-3,h-1)（长 3px）
// 每条线 = 1px 暗线 + 右下偏移 1px 亮高光（半透明叠加）：
//   暗 ≈ 背景×0.4（Edge 实测 (5,6,9) vs 背景 13,17,23）
//   亮 ≈ 背景×0.4 + 255×0.6（Edge 实测 158,159,162）
// 半透明叠加保证浅色主题下同样正确（Chromium 的行为）。resize:none 或
// 不可拖拽时不绘制。
func paintTextAreaResizeHandle(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil || ResizeModeOf(st) == 0 {
		return
	}
	c := info.canvas
	const hs = 15.0
	hx := x + w - hs
	hy := y + h - hs
	if hx < x || hy < y {
		return
	}
	bg := st.BackgroundColor
	dark := graphics.Color{
		R: uint8(float64(bg.R) * 0.4), G: uint8(float64(bg.G) * 0.4), B: uint8(float64(bg.B) * 0.4), A: 255,
	}
	light := graphics.Color{
		R: uint8(float64(bg.R)*0.4 + 255*0.6),
		G: uint8(float64(bg.G)*0.4 + 255*0.6),
		B: uint8(float64(bg.B)*0.4 + 255*0.6),
		A: 255,
	}
	dark = applyOpacity(dark, op)
	light = applyOpacity(light, op)
	// 两条斜线（相对手柄左上角 hx,hy；与 Press 命中区域一致）。
	lines := [2][4]float64{
		{13, 8, 8, 14},  // 线1: (w-2,h-7)→(w-7,h-1)
		{14, 12, 12, 14}, // 线2: (w-1,h-3)→(w-3,h-1)
	}
	for _, ln := range lines {
		// 高光（白色半透明，右下偏移 1px）
		c.StrokeLine(hx+ln[0]+1, hy+ln[1]+1, hx+ln[2]+1, hy+ln[3]+1, 1.0, light)
		// 主体（暗线）
		c.StrokeLine(hx+ln[0], hy+ln[1], hx+ln[2], hy+ln[3], 1.0, dark)
	}
}

// paintSelectArrow draws the browser-standard dropdown chevron on the right
// side of a <select> element. It mirrors the native menu-list arrow that
// Chromium/WebKit rasterize from an inline SVG path (kHTMLSelectArrow):
// a downward V-shaped stroke (M 3 4.5 L 6 7.5 L 9 4.5 in a 12x12 viewBox)
// with round caps/joins — NOT a filled triangle. Geometry measured against
// Edge (headless, 20px/32px selects): V 顶宽 ~6px、总高 ~7px（含 stroke），
// 垂直居中，中心距右缘 ~9.5px，stroke ≈2px round cap/join，颜色 = 文字色。
func paintSelectArrow(info *PaintInfo, st *style.ComputedStyle, x, y, w, h float64, op float64) {
	if info == nil || info.canvas == nil {
		return
	}
	c := info.canvas
	// Chromium kHTMLSelectArrow（12x12 viewBox: M3 4.5 L6 7.5 L9 4.5）。Edge 像素级
	// 反推（arrow_scan 扫描验证 score=0）：V 路径臂端 cy-1、尖端 cy+2（高 3px 不对称）、
	// 臂距 6px、stroke 2px round cap/join。白底黑核心分布（相对 cy）：
	//   cy-2: 臂 cap 圆顶 2px×2 ｜ cy-1: 臂 3px×2 ｜ cy+0: 汇合 6px ｜ cy+1: 4px ｜
	//   cy+2: 尖端 2px ｜ cy-3/cy+3: 灰边。颜色 = 文字色，位置距右缘 9.5px。
	const strokeW = 2.0
	// Horizontal center: ~9.5px from the right edge of the border box.
	cx := x + w - 9.5
	cy := y + h/2
	// Use element's text color for arrow, fallback to default.
	col := toGraphicsColor(st.Color)
	if col.A == 0 {
		col = FormControlColors.SelectArrow
	}
	col = applyOpacity(col, op)
	// SVG chevron path (V-shaped polyline), mirroring Chromium's kHTMLSelectArrow
	// (M 3 4.5 L 6 7.5 L 9 4.5 in a 12x12 viewBox). Edge raster fitted (see header):
	// 黑核心 y84..88 5 行、臂端黑 2px、汇合 6px、尖端 2px + y89 灰。
	// pts 臂端 y=cy-1、尖端 y=cy+2（V 路径高 3px，不对称），stroke 2px round cap/join。
	// Edge 实测（白底黑核心）：y84=cy-2 臂 cap 圆顶 2px、y85=cy-1 臂端、y86=cy
	// 汇合 6px、y87=cy+1 4px、y88=cy+2 尖端 2px、y89=cy+3 灰。
	pts := []graphics.Point{
		{X: cx - 3, Y: cy - 1},
		{X: cx, Y: cy + 2},
		{X: cx + 3, Y: cy - 1},
	}
	c.StrokePath(pts, strokeW, col, "round", "round")
}

// --- helpers ---

// applyOpacity is a local alias for ApplyOpacityToColor (defined in animation.go) so the
// form-control painters read concisely. It multiplies a color's alpha by the given
// opacity (0..1).
func applyOpacity(col graphics.Color, op float64) graphics.Color {
	return ApplyOpacityToColor(col, op)
}

// rangeBounds returns the min/max for a range input, defaulting to 0/100 when not set.
func rangeBounds(in html5.HTMLInputElement) (min, max float64) {
	min = parseFloatOr(in.Min(), 0)
	max = parseFloatOr(in.Max(), 100)
	if max <= min {
		max = min + 1
	}
	return min, max
}

// parseFloatOr parses a float string, returning the default on error.
func parseFloatOr(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

// parseHexColor parses a CSS hex color string (#rgb / #rrggbb / #rrggbbaa) into a
// graphics.Color. Returns black on error.
func parseHexColor(s string) graphics.Color {
	s = strings.TrimSpace(s)
	if s == "" {
		return graphics.Color{R: 0, G: 0, B: 0, A: 255}
	}
	// Strip leading #.
	if s[0] == '#' {
		s = s[1:]
	}
	switch len(s) {
	case 3:
		r := hexDigit(s[0])
		g := hexDigit(s[1])
		b := hexDigit(s[2])
		return graphics.Color{R: r * 17, G: g * 17, B: b * 17, A: 255}
	case 6:
		r := hexDigit(s[0])*16 + hexDigit(s[1])
		g := hexDigit(s[2])*16 + hexDigit(s[3])
		b := hexDigit(s[4])*16 + hexDigit(s[5])
		return graphics.Color{R: r, G: g, B: b, A: 255}
	case 8:
		r := hexDigit(s[0])*16 + hexDigit(s[1])
		g := hexDigit(s[2])*16 + hexDigit(s[3])
		b := hexDigit(s[4])*16 + hexDigit(s[5])
		a := hexDigit(s[6])*16 + hexDigit(s[7])
		return graphics.Color{R: r, G: g, B: b, A: a}
	}
	return graphics.Color{R: 0, G: 0, B: 0, A: 255}
}

// hexDigit converts a hex character to its 0-15 numeric value.
func hexDigit(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
