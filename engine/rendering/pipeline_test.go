package rendering

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestFullPipeline_DarkThemeBodyBackground runs the full pipeline
// (DOM → CSS → style → render-tree → layout → paint) and verifies
// pixel output against expected browser rendering.
func TestFullPipeline_DarkThemeBodyBackground(t *testing.T) {
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
	headerEl := dom.NewElement(doc, "div")
	headerEl.SetClassName("menubar")
	appEl.AppendChild(headerEl)
	headerEl.AppendChild(doc.CreateTextNode("File Edit View"))

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
:root, .theme-dark {
  --bg-primary: #0d1117; --bg-secondary: #161b22;
  --bg-tertiary: #21262d; --text-primary: #e6edf3;
  --text-secondary: #8b949e; --accent: #58a6ff;
  --border-color: #30363d;
  --font-ui: 'Inter', system-ui, sans-serif;
  --font-size-base: 13px;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { width: 100%; height: 100%; overflow: hidden; }
body {
  font-family: var(--font-ui);
  font-size: var(--font-size-base);
  color: var(--text-primary);
  background-color: var(--bg-primary);
}
#app { background-color: var(--bg-primary); }
.menubar {
  display: flex; flex-direction: row; align-items: center;
  height: 32px; background: var(--bg-secondary);
  color: var(--text-secondary); font-size: 13px; padding: 0 12px;
}
`)
	htmlEl.AppendChild(styleEl)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	t.Logf("Saved to: %s", savePNG(canvas, "wbui_headless.png"))

	// -- RENDER TREE DUMP --
	t.Log("")
	t.Log("━━━ RENDER TREE ━━━")
	dumpRenderTree(rv, 0, t)

	// -- PAINT GRID --
	t.Log("")
	t.Log("━━━ PAINT GRID (5x5 samples) ━━━")
	cw, ch := canvas.Width(), canvas.Height()
	for sy := 0; sy < 5; sy++ {
		y := sy * ch / 5
		line := ""
		for sx := 0; sx < 5; sx++ {
			x := sx * cw / 5
			p := canvas.PixelAt(x, y)
			line += fmt.Sprintf("  (%4d,%4d)=#%02x%02x%02x a=%3d", x, y, p.R, p.G, p.B, p.A)
		}
		t.Log(line)
	}

	// -- EXPECTED vs ACTUAL --
	t.Log("")
	t.Log("━━━ BROWSER vs WB-UI ━━━")
	t.Log("BROWSER: viewport ≈#0d1117  menubar ≈#161b22")

	bgPixel := canvas.PixelAt(10, 40)
	mbPixel := canvas.PixelAt(10, 10)
	btmPixel := canvas.PixelAt(10, 780)
	t.Logf("WB-UI:   body  (10,40)  = #%02x%02x%02x a=%d", bgPixel.R, bgPixel.G, bgPixel.B, bgPixel.A)
	t.Logf("WB-UI:   menu  (10,10)  = #%02x%02x%02x a=%d", mbPixel.R, mbPixel.G, mbPixel.B, mbPixel.A)
	t.Logf("WB-UI:   btm   (10,780) = #%02x%02x%02x a=%d", btmPixel.R, btmPixel.G, btmPixel.B, btmPixel.A)

	failures := 0
	if bgPixel.A == 0 {
		t.Error("❌ CRITICAL: body area TRANSPARENT → blue-cyan screen")
		failures++
	}
	if bgPixel.R == 0 && bgPixel.G == 0 && bgPixel.B == 0 && bgPixel.A == 0xFF {
		t.Error("❌ CRITICAL: body area SOLID BLACK → bg-color not applied")
		failures++
	}
	if mbPixel.A == 0 {
		t.Error("❌ menubar TRANSPARENT")
		failures++
	}
	if btmPixel.A == 0 {
		t.Error("❌ viewport bottom TRANSPARENT")
		failures++
	}
	if failures == 0 {
		t.Log("✅ ALL PAINT CHECKS PASSED")
	}
}

// TestRenderStyleDumpResolution dumps ALL computed style properties
// for html/body/#app — compare against browser DevTools Computed panel.
func TestRenderStyleDumpResolution(t *testing.T) {
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

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
:root, .theme-dark {
  --bg-primary: #0d1117; --bg-secondary: #161b22;
  --text-primary: #e6edf3; --text-secondary: #8b949e;
  --accent: #58a6ff; --border-color: #30363d;
  --shadow-sm: 0 1px 3px rgba(0,0,0,0.25);
  --font-ui: 'Inter', system-ui, sans-serif;
  --font-size-base: 13px; --border-radius: 4px;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body, #app { width: 100%; height: 100%; overflow: hidden; }
body {
  font-family: var(--font-ui); font-size: var(--font-size-base);
  color: var(--text-primary); background-color: var(--bg-primary);
}
#app { background-color: var(--bg-primary); }
`)
	htmlEl.AppendChild(styleEl)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	styleMap := resolver.ResolveDocument(doc)

	t.Log("")
	t.Log("══════════════════════════════════════════════")
	t.Log("  COMPLETE COMPUTED STYLE DUMP")
	t.Log("  (Compare with browser DevTools → Computed)")
	t.Log("══════════════════════════════════════════════")

	for _, label := range []string{"html", "body", "#app"} {
		var el *dom.Element
		switch label {
		case "html":
			el = htmlEl
		case "body":
			el = bodyEl
		case "#app":
			el = appEl
		}
		cs := styleMap[el]
		if cs == nil {
			t.Logf("❌ %s: NO COMPUTED STYLE", label)
			continue
		}
		t.Logf("")
		t.Logf("─── %s ───", label)
		dumpStyleProps(t, cs)
	}
}

func dumpStyleProps(t *testing.T, cs *style.ComputedStyle) {
	t.Helper()
	t.Logf("  Display                    %d", cs.Display)
	t.Logf("  Position                   %d", cs.Position)
	t.Logf("  Color                      #%02x%02x%02x a=%d", cs.Color.R, cs.Color.G, cs.Color.B, cs.Color.A)
	t.Logf("  Background-Color           #%02x%02x%02x a=%d", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, cs.BackgroundColor.A)
	t.Logf("  Width                      %s", cs.Width.String())
	t.Logf("  Height                     %s", cs.Height.String())
	t.Logf("  Overflow-X/Y               %d / %d", cs.OverflowX, cs.OverflowY)
	t.Logf("  Font-Family                %q", cs.FontFamily)
	t.Logf("  Font-Size                  %s", cs.FontSize.String())
	t.Logf("  Font-Weight                %q", cs.FontWeight)
	t.Logf("  Line-Height                %s", cs.LineHeight.String())
	t.Logf("  Opacity                    %.2f", cs.Opacity)
	t.Logf("  Z-Index                    %d", cs.ZIndex)
	t.Logf("  Box-Sizing                 %q", cs.BoxSizing)
	t.Logf("  Border-Radius              %s", cs.BorderRadius.String())
	t.Logf("  Box-Shadow                 %q", cs.BoxShadow)
	t.Logf("  Margin  (T,R,B,L)         (%s,%s,%s,%s)", cs.MarginTop, cs.MarginRight, cs.MarginBottom, cs.MarginLeft)
	t.Logf("  Flex-Dir/Wrap/Align/Jstfy  %q/%q/%q/%q", cs.FlexDirection, cs.FlexWrap, cs.AlignItems, cs.JustifyContent)
	t.Logf("  Gap                        %s", cs.Gap.String())
	t.Logf("  Direction                  %q", cs.Direction)
	if len(cs.CustomProperties) > 0 {
		t.Logf("  ── CSS Variables ──")
		for k, v := range cs.CustomProperties {
			var vals []string
			for _, tok := range v {
				vals = append(vals, tok.Value)
			}
			t.Logf("  %-24s %v", k, vals)
		}
	}
}

func dumpRenderTree(ro RenderObject, depth int, t *testing.T) {
	t.Helper()
	if ro == nil {
		return
	}
	padding := ""
	for i := 0; i < depth; i++ {
		padding += "  "
	}
	name := ro.RenderName()
	bgInfo := ""
	if box, ok := ro.(*RenderBox); ok {
		fr := box.FrameRect()
		cs := box.Style()
		if cs != nil {
			bgInfo = fmt.Sprintf(" @ (%d,%d) %d×%d bg=#%02x%02x%02x a=%d vis=%v",
				int(fr.X), int(fr.Y), int(fr.Width), int(fr.Height),
				cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B,
				cs.BackgroundColor.A, box.IsVisible())
		}
	}
	t.Logf("%s%s%s", padding, name, bgInfo)
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpRenderTree(c, depth+1, t)
	}
}

// savePNG 把画布写成 PNG 并返回实际写入路径。
// 输出目录取环境变量 WBUI_TEST_OUT，缺省为系统临时目录——测试因此既不依赖
// 开发机上的绝对路径，又能在需要看图时把产物固定到指定目录。
func savePNG(canvas *graphics.Canvas, name string) string {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		return ""
	}
	dir := os.Getenv("WBUI_TEST_OUT")
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, filepath.Base(name))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	png.Encode(f, img)
	return path
}
