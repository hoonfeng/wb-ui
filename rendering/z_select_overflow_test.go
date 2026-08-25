package rendering

import (
	"strings"
	"testing"

	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// helperRenderSelectPopup renders a <select>-popup clone: a fixed-position
// container (150x100, overflow-y:auto) holding 10 nowrap long-text option
// rows — exactly the shape webkit/interact.go's handleSelectClick creates.
// Returns the canvas (200x200) and the popup container's page rect.
func helperRenderSelectPopup(t *testing.T) *graphics.Canvas {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	var rows strings.Builder
	for i := 0; i < 10; i++ {
		rows.WriteString(`<div style="display:block;padding:4px 10px;font-size:12px;line-height:12px;color:#e8eaf0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">`)
		rows.WriteString("窗口标题示例序号" + string(rune('0'+i)) + " 这是一个非常长的窗口标题用于测试溢出裁剪行为")
		rows.WriteString(`</div>`)
	}

	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>
		<div id="popup" style="position:fixed;left:20px;top:20px;width:150px;height:100px;overflow-y:auto;background:#1c2333;">` +
		rows.String() + `</div>
	</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	builder := NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetViewportSize(200, 200)
	state := layout.NewLayoutState(200, 200)
	rv.Layout(state)
	rv.SetCursorPos(100, 100)

	canvas := graphics.NewCanvas(200, 200)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 200, Height: 200})
	return canvas
}

// TestSelectPopupClipsRows: overflow-y:auto must clip overflowing option rows
// to the popup box (row 9/10 must NOT paint below popup bottom edge y=120).
func TestSelectPopupClipsRows(t *testing.T) {
	canvas := helperRenderSelectPopup(t)
	defer canvas.Release()
	// Scan rows below the popup (y 122..198): any non-bg pixel = leak.
	leak := 0
	for y := 122; y < 199; y++ {
		for x := 10; x < 190; x++ {
			px := canvas.PixelAt(x, y)
			if px.A > 10 {
				leak++
			}
		}
	}
	if leak > 0 {
		t.Fatalf("popup rows leaked outside container: %d px below y=120", leak)
	}
}

// TestSelectPopupClipsLongText: option text must be clipped (nowrap +
// overflow:hidden) to popup right edge x=170; no text pixels beyond it.
func TestSelectPopupClipsLongText(t *testing.T) {
	canvas := helperRenderSelectPopup(t)
	defer canvas.Release()
	leak := 0
	for y := 21; y < 120; y++ {
		for x := 172; x < 199; x++ {
			px := canvas.PixelAt(x, y)
			if px.A > 10 {
				leak++
			}
		}
	}
	if leak > 0 {
		t.Fatalf("option text leaked right of popup: %d px beyond x=170", leak)
	}
}
