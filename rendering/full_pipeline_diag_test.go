package rendering

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestFullPipelineDiag loads the desktop frontend (HTML + external CSS)
// and dumps the full rendering pipeline, exactly as desktop main.go does.
func TestFullPipelineDiag(t *testing.T) {
	distDir := findDistDir(t)
	if distDir == "" {
		t.Skip("skipping: cannot find desktop dist directory")
	}
	t.Logf("distDir: %s", distDir)

	// Load HTML
	htmlPath := filepath.Join(distDir, "index.html")
	htmlData, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	src := string(htmlData)
	src = strings.Replace(src, `type="module"`, "", 1)
	src = strings.ReplaceAll(src, `crossorigin`, "")

	t.Logf("=== HTML: %d bytes ===", len(src))

	// Parse HTML → DOM
	doc, err := html.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	removeScripts(doc)

	elCount := countElements(doc)
	t.Logf("=== DOM: %d elements ===", elCount)

	// Collect CSS (inline + external)
	resolver := style.NewResolver()
	inlineCount, extCount := collectAllCSS(doc, distDir, resolver, t)
	t.Logf("=== CSS: %d inline rules, %d external rules ===", inlineCount, extCount)

	// Build render tree
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	roCount := countRenderObjects(rv)
	t.Logf("=== Render Tree: %d objects ===", roCount)

	// Layout
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)
	t.Logf("=== Layout: done ===")

	// Dump CSS variable resolution
	dumpCSSVarResolve(doc, resolver, t)

	// Paint
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	canvas.Clear(graphics.Color{R: 255, G: 0, B: 0, A: 255}) // red = unpainted areas
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	outPath := filepath.Join(distDir, "..", "..", "..", "screenshots", "full_pipeline_diag.png")
	outPath, _ = filepath.Abs(outPath)
	os.MkdirAll(filepath.Dir(outPath), 0755)
	savePNGDiag(canvas, outPath)
	t.Logf("=== PNG: %s ===", outPath)

	// Pixel grid
	t.Log("\n=== PIXEL GRID (5x5) ===")
	dumpPixelGrid(canvas, t)

	// Key pixel checks
	t.Log("\n=== KEY PIXELS ===")
	checkKeyPixels(canvas, t)

	// Render tree dump (top 3 levels)
	t.Log("\n=== RENDER TREE (top 3 levels) ===")
	dumpRT3Levels(rv, 0, t)
}

func findDistDir(t *testing.T) string {
	abs := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
		return abs
	}
	wd, _ := os.Getwd()
	for _, c := range []string{
		"../../gou-ide/cmd/desktop/web-ui/dist",
		"../../../gou-ide/cmd/desktop/web-ui/dist",
	} {
		p := filepath.Join(wd, c)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

var dataVRe = regexp.MustCompile(`\[data-v-[a-f0-9]+\]`)

func collectAllCSS(root dom.Node, distDir string, resolver *style.Resolver, t *testing.T) (inlineCount, extCount int) {
	collectCSSWalk(root, distDir, resolver, t, &inlineCount, &extCount)
	return
}

func collectCSSWalk(n dom.Node, distDir string, resolver *style.Resolver, t *testing.T, inline, ext *int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok {
			tag := strings.ToLower(el.TagName())
			if tag == "style" {
				text := el.TextContent()
				sheet := css.NewCSSStyleSheet()
				css.NewParser(text).ParseStyleSheetInto(sheet)
				resolver.AddStyleSheet(sheet)
				*inline += len(sheet.Rules())
			} else if tag == "link" {
				rel := strings.ToLower(el.GetAttribute("rel"))
				href := el.GetAttribute("href")
				if strings.Contains(rel, "stylesheet") && href != "" {
					cssPath := filepath.Join(distDir, href)
					data, err := os.ReadFile(cssPath)
					if err != nil {
						t.Logf("  [CSS] read external %s: %v", href, err)
					} else {
						cleaned := dataVRe.ReplaceAllString(string(data), "")
						sheet := css.NewCSSStyleSheet()
						css.NewParser(cleaned).ParseStyleSheetInto(sheet)
						resolver.AddStyleSheet(sheet)
						*ext += len(sheet.Rules())
						t.Logf("  [CSS] loaded %s: %d rules (%d bytes)", href, len(sheet.Rules()), len(cleaned))
					}
				}
			}
			collectCSSWalk(c, distDir, resolver, t, inline, ext)
		}
	}
}

func removeScripts(n dom.Node) {
	var toRemove []dom.Node
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok {
			tag := strings.ToLower(el.TagName())
			if tag == "script" || tag == "noscript" {
				toRemove = append(toRemove, c)
			} else {
				removeScripts(c)
			}
		}
	}
	for _, r := range toRemove {
		n.RemoveChild(r)
	}
}

func countElements(n dom.Node) int {
	cnt := 0
	if _, ok := n.(*dom.Element); ok && n != nil {
		cnt++
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		cnt += countElements(c)
	}
	return cnt
}

func countRenderObjects(ro RenderObject) int {
	if ro == nil {
		return 0
	}
	cnt := 1
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		cnt += countRenderObjects(c)
	}
	return cnt
}

func dumpCSSVarResolve(doc *dom.Document, resolver *style.Resolver, t *testing.T) {
	body := doc.Body()
	if body == nil {
		t.Log("  body: nil")
		return
	}
	cs := resolver.ResolveElement(body)
	if cs == nil {
		t.Log("  body CS: nil")
		return
	}
	t.Logf("  body bg=#%02x%02x%02x a=%d fg=#%02x%02x%02x a=%d fs=%s ff=%q",
		cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, cs.BackgroundColor.A,
		cs.Color.R, cs.Color.G, cs.Color.B, cs.Color.A,
		cs.FontSize.String(), cs.FontFamily)

	htmlEl := doc.DocumentElement()
	if htmlEl != nil {
		hcs := resolver.ResolveElement(htmlEl)
		if hcs != nil && len(hcs.CustomProperties) > 0 {
			t.Logf("  :root CSS vars: %d", len(hcs.CustomProperties))
			for k, v := range hcs.CustomProperties {
				vals := make([]string, len(v))
				for i, tok := range v {
					vals[i] = tok.Value
				}
				if len(vals) > 3 {
					vals = vals[:3]
				}
				t.Logf("    %s = %v", k, vals)
			}
		} else {
			t.Log("  :root CSS vars: NONE!")
		}
	}
}

func dumpRT3Levels(ro RenderObject, depth int, t *testing.T) {
	if ro == nil || depth > 3 {
		return
	}
	prefix := strings.Repeat("  ", depth)
	name := ro.RenderName()

	frStr := ""
	if box, ok := ro.(*RenderBox); ok {
		fr := box.FrameRect()
		cs := box.Style()
		bgStr := ""
		dispStr := ""
		if cs != nil {
			if cs.BackgroundColor.A > 0 {
				bgStr = fmt.Sprintf(" bg=#%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
			}
			dispStr = fmt.Sprintf(" d=%d", cs.Display)
		}
		frStr = fmt.Sprintf(" (%5.0f,%5.0f %5.0fx%5.0f) vis=%v%s%s",
			fr.X, fr.Y, fr.Width, fr.Height, box.IsVisible(), dispStr, bgStr)
	}
	if txt, ok := ro.(*RenderText); ok {
		data := txt.Text()
		if len(data) > 30 {
			data = data[:27] + "..."
		}
		frStr = fmt.Sprintf(" \"%s\"", data)
	}

	t.Logf("%s%s%s", prefix, name, frStr)

	if depth < 3 {
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			dumpRT3Levels(c, depth+1, t)
		}
	}
}

func checkKeyPixels(canvas *graphics.Canvas, t *testing.T) {
	checks := []struct {
		x, y int
		desc string
	}{
		{10, 10, "menubar (top-left, should be #161b22 dark)"},
		{10, 40, "below menubar (should be #0d1117 bg)"},
		{200, 400, "center (should have content)"},
		{10, 790, "bottom-left (should be #0d1117 bg)"},
	}
	allGood := true
	for _, ck := range checks {
		p := canvas.PixelAt(ck.x, ck.y)
		status := "OK"
		if p.A == 0 {
			status = "TRANSPARENT!"
			allGood = false
		} else if p.R == 255 && p.G == 0 && p.B == 0 {
			status = "RED (unpainted)!"
			allGood = false
		} else if p.R == 0 && p.G == 0 && p.B == 0 && p.A == 255 {
			status = "BLACK!"
			allGood = false
		}
		t.Logf("  (%3d,%3d) #%02x%02x%02x a=%d  %-20s  %s",
			ck.x, ck.y, p.R, p.G, p.B, p.A, status, ck.desc)
	}
	if allGood {
		t.Log("  All key pixels painted correctly")
	}
}

func dumpPixelGrid(canvas *graphics.Canvas, t *testing.T) {
	w, h := canvas.Width(), canvas.Height()
	for sy := 0; sy < 5; sy++ {
		y := sy * h / 5
		line := ""
		for sx := 0; sx < 5; sx++ {
			x := sx * w / 5
			p := canvas.PixelAt(x, y)
			line += fmt.Sprintf("  (%4d,%4d)=#%02x%02x%02x a=%3d", x, y, p.R, p.G, p.B, p.A)
		}
		t.Log(line)
	}
}

func savePNGDiag(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	png.Encode(f, img)
}
