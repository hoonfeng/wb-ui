package rendering

import (
	"fmt"
	"strings"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/html"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// buildPillWithFont renders the composer-row-1 pill scenario with a custom
// font-family, mirroring
//   plugins-src/ui-app/src/components/RightPanel.vue  (.input-row1 / .ir-pill)
//   release/PairCode/.pair/assets/runtime/web/index.html  (--fs-xs: 11px)
func buildPillWithFont(t *testing.T, fontFamily string, sizePx float64, w, h int) (*RenderView, *graphics.Canvas) {
	t.Helper()
	cssText := `
:root { --fs-xs: ` + fmt.Sprintf("%gpx", sizePx) + `; --accent: #58a6ff; --bg-active: #2b3350; }
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: 100%; height: 100%; overflow: hidden; }
body { font-family: ` + fontFamily + `; font-size: 16px; background: #101010; padding-top: 8px; }
.input-row1 { display: flex; align-items: center; gap: 8px; height: 24px; }
.ir-pill { display: inline-flex; align-items: center; justify-content: center; height: 24px; padding: 0 10px; border-radius: 999px; font-size: var(--fs-xs); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ir-pill-model { background: var(--bg-active); color: var(--accent); }
`
	htmlSrc := `<html><head><style>` + cssText + `</style></head><body>
<div class="input-row1"><span class="ir-pill ir-pill-model">deepseek-flash</span></div>
</body></html>`
	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil RenderView")
	}
	rv.SetViewportSize(float64(w), float64(h))
	rv.Layout(nil)
	canvas := graphics.NewCanvas(w, h)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})
	return rv, canvas
}

// firstFamilyOf mimics graphics.firstConcreteFamily's candidate selection so the
// reported metrics belong to the family the engine would actually pick.
func firstFamilyOf(list string) string {
	generic := map[string]string{
		"serif": "Times New Roman", "sans-serif": "Arial", "monospace": "Consolas",
		"cursive": "Comic Sans MS", "fantasy": "Impact", "system-ui": "Segoe UI",
		"ui-serif": "Times New Roman", "ui-sans-serif": "Arial", "ui-monospace": "Consolas",
		"ui-rounded": "Arial", "math": "Cambria Math", "emoji": "Segoe UI Emoji",
		"fangsong": "FangSong",
	}
	for _, p := range strings.Split(list, ",") {
		name := strings.Trim(strings.TrimSpace(p), `"'`)
		if name == "" {
			continue
		}
		if mapped, ok := generic[strings.ToLower(name)]; ok {
			return mapped
		}
		return name
	}
	return list
}

// TestPillFontStackScan renders the pill with several font stacks and reports
// the resulting line-box height and the glyph ink distribution. A font whose
// metrics leave the ink off-centre reproduces the "tag text not vertically
// centred" report.
func TestPillFontStackScan(t *testing.T) {
	families := []string{
		"'Inter', system-ui, -apple-system, sans-serif", // 真实页面 body 字体栈
		"Inter",
		"system-ui",
		"Segoe UI",
		"Arial",
		"sans-serif",
		"Microsoft YaHei",
		"Tahoma",
		"Consolas",
	}
	for _, ff := range families {
		t.Run(ff, func(t *testing.T) {
			rv, canvas := buildPillWithFont(t, ff, 11, 320, 40)
			defer canvas.Release()
			g := collectPill(rv)
			if g.h == 0 || len(g.segs) == 0 {
				t.Fatalf("no pill geometry/segments")
			}
			s := g.segs[0]
			pick := firstFamilyOf(ff)
			mf := graphics.Font{Family: pick, Size: 11}
			asc := graphics.GlobalFontAscent(mf)
			desc := graphics.GlobalFontDescent(mf)
			lead := graphics.GlobalFontLineGap(mf)
			t.Logf("family=%-46s first=%q metrics A=%.2f D=%.2f gap=%.2f (sum=%.2f)",
				ff, pick, asc, desc, lead, asc+desc+lead)
			t.Logf("  seg X=%.2f Y=%.2f W=%.2f H=%.2f LineH=%.2f | pill top=%.2f h=%.2f",
				s.X, s.Y, s.Width, s.Height, s.LineHeight, g.top, g.h)

			bgR, bgG, bgB, inkRows := scanPillInk(t, canvas, g)
			top, bot := int(g.top), int(g.top+g.h)
			if len(inkRows) == 0 {
				t.Fatalf("no ink (bg=#%02x%02x%02x)", bgR, bgG, bgB)
			}
			first, last := inkRows[0], inkRows[len(inkRows)-1]
			above, below := first-top, bot-1-last
			verdict := "CENTERED"
			if above > below {
				verdict = fmt.Sprintf("TEXT LOW by %d", above-below)
			} else if below > above {
				verdict = fmt.Sprintf("TEXT HIGH by %d", below-above)
			}
			t.Logf("  ink rows %d..%d: above=%d below=%d (inkH=%d) => %s", first, last, above, below, last-first+1, verdict)
		})
	}
}
