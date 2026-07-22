package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

func TestLayout_FlexColumnAppFix(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	htmlEl.SetClassName("theme-dark")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	bodyEl.SetClassName("theme-dark")
	htmlEl.AppendChild(bodyEl)
	appEl := dom.NewElement(doc, "div")
	appEl.SetId("app")
	bodyEl.AppendChild(appEl)

	menubar := dom.NewElement(doc, "div")
	menubar.SetClassName("menubar")
	appEl.AppendChild(menubar)

	mainContent := dom.NewElement(doc, "div")
	mainContent.SetAttribute("style", "display:flex; flex-direction:row; flex:1; overflow:hidden;")
	appEl.AppendChild(mainContent)

	activityBar := dom.NewElement(doc, "div")
	activityBar.SetClassName("activity-bar")
	activityBar.SetAttribute("style", "width:48px; display:flex; flex-direction:column; background:var(--activity-bar-bg);")
	mainContent.AppendChild(activityBar)

	sidebar := dom.NewElement(doc, "div")
	sidebar.SetClassName("sidebar")
	sidebar.SetAttribute("style", "width:260px; display:flex; flex-direction:column; background:var(--sidebar-bg); border-right:1px solid var(--border-color);")
	mainContent.AppendChild(sidebar)

	editorArea := dom.NewElement(doc, "div")
	editorArea.SetAttribute("style", "flex:1; display:flex; flex-direction:column; overflow:hidden; background:var(--bg-primary);")
	mainContent.AppendChild(editorArea)

	baseCSS := `
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { width: 100%; height: 100%; overflow: hidden; }
:root, .theme-dark {
  --bg-primary: #0d1117; --bg-secondary: #161b22;
  --text-primary: #e6edf3; --accent: #58a6ff;
  --border-color: #30363d; --activity-bar-bg: #0d1117;
  --sidebar-bg: #161b22;
}
body { color: var(--text-primary); background-color: var(--bg-primary); }
`

	t.Run("block_app_bug", func(t *testing.T) {
		css := baseCSS + `
#app { background-color: var(--bg-primary); }
.menubar { display: flex; height: 32px; background: var(--bg-secondary); }
`
		rv := buildAndLayout(doc, css, 1280, 800)
		ab := findByClass(rv, "activity-bar")
		if ab != nil {
			_, _, _, h := layoutInfo(ab)
			t.Logf("BUG: activity-bar H=%.0f (should be 768, is 0 due to block #app)", h)
		}
	})

	t.Run("flex_app_fix", func(t *testing.T) {
		css := baseCSS + `
#app { display: flex; flex-direction: column; background-color: var(--bg-primary); }
.menubar { display: flex; height: 32px; background: var(--bg-secondary); }
`
		rv := buildAndLayout(doc, css, 1280, 800)
		dumpLayoutTree(rv, 0, t)

		mb := findByClass(rv, "menubar")
		ab := findByClass(rv, "activity-bar")
		sb := findByClass(rv, "sidebar")

		if mb != nil {
			_, _, _, h := layoutInfo(mb)
			t.Logf("menubar H=%.0f (expect 32)", h)
			if h < 25 {
				t.Error("menubar too short")
			}
		}

		if ab != nil {
			_, _, _, h := layoutInfo(ab)
			t.Logf("activity-bar H=%.0f (expect 768)", h)
			if h < 700 {
				t.Errorf("activity-bar height is %.0f, expected 768", h)
			}
		}

		if sb != nil {
			_, _, _, h := layoutInfo(sb)
			t.Logf("sidebar H=%.0f (expect 768)", h)
			if h < 700 {
				t.Errorf("sidebar height is %.0f, expected 768", h)
			}
		}

		canvas := graphics.NewCanvas(1280, 800)
		defer canvas.Release()
		Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})
		savePNG(canvas, "F:\\syproject\\gou-ide\\screenshots\\wbui_flex_fix.png")

		t.Log("")
		t.Log("=== ALL BOXES ===")
		walkAllLayoutBoxes(rv, t)
	})
}

func buildAndLayout(doc *dom.Document, cssText string, vw, vh int) *RenderView {
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		return nil
	}
	rv.SetViewportSize(float64(vw), float64(vh))
	rv.Layout(nil)
	return rv
}
