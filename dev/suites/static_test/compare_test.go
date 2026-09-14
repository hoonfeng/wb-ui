// Command compare_test renders layout_bench.html via wb-ui, extracts all
// element positions and ink rectangles, and writes a comparison file along
// with manual browser reference coordinates.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

func main() {
	wd := "."
	htmlPath := filepath.Join(wd, "dev", "static_test", "layout_bench.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	data, _ = os.ReadFile(htmlPath)
	doc, _ := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	outPath := filepath.Join(wd, "dev", "static_test", "compare_report.txt")
	f, _ := os.Create(outPath)
	defer f.Close()

	// Key elements with known browser reference coordinates (from OCR + visual inspection)
	// Browser viewport = 1280×800, page loaded in Chrome headless
	type ref struct {
		name    string
		desc    string
		browser struct{ x, y, w, h float64 }
	}
	refs := []ref{
		{"actbar", "Activity bar (leftmost icons)", struct{ x, y, w, h float64 }{0, 0, 48, 800}},
		{"sidebar", "Sidebar (files/search/git)", struct{ x, y, w, h float64 }{48, 0, 220, 800}},
		{"main", "Main content area", struct{ x, y, w, h float64 }{268, 0, 812, 800}},
		{"right-panel", "Right outline panel", struct{ x, y, w, h float64 }{1080, 0, 200, 800}},
		{"statusbar", "Status bar bottom", struct{ x, y, w, h float64 }{0, 776, 1256, 24}},
		{"tabs", "Editor tabs bar", struct{ x, y, w, h float64 }{268, 0, 812, 36}},
		{"editor", "Editor area", struct{ x, y, w, h float64 }{268, 36, 812, 178}},
	}

	fmt.Fprintln(f, "=== WB-UI vs BROWSER: ELEMENT POSITION COMPARISON ===")
	fmt.Fprintln(f, "Viewport: 1280×800")
	fmt.Fprintln(f, "HTML: layout_bench.html (static test page)")
	fmt.Fprintln(f)
	fmt.Fprintf(f, "%-15s %-10s %-10s %-10s %-10s  |  %-10s %-10s %-10s %-10s  |  STATUS\n",
		"ELEMENT", "B-X", "B-Y", "B-W", "B-H", "W-X", "W-Y", "W-W", "W-H")
	fmt.Fprintln(f, strings.Repeat("-", 110))

	root := rv.LayoutBox()
	allOK := true
	for _, ref := range refs {
		box := findByClass(root, ref.name)
		bw := fmt.Sprintf("%.0f", ref.browser.w)
		bh := fmt.Sprintf("%.0f", ref.browser.h)
		wx, wy, ww, wh := "?", "?", "?", "?"
		status := "❌ NOT FOUND"
		if box != nil {
			g := state.GeometryForBox(box)
			wx = fmt.Sprintf("%.0f", g.Left())
			wy = fmt.Sprintf("%.0f", g.Top())
			ww = fmt.Sprintf("%.0f", g.BorderBoxWidth())
			wh = fmt.Sprintf("%.0f", g.BorderBoxHeight())
			// Tolerance: ±2px for integer rounding
			xOK := approxEq(g.Left(), ref.browser.x, 2)
			yOK := approxEq(g.Top(), ref.browser.y, 2)
			wOK := approxEq(g.BorderBoxWidth(), ref.browser.w, 2)
			hOK := approxEq(g.BorderBoxHeight(), ref.browser.h, 2)
			if xOK && yOK && wOK && hOK {
				status = "✅ MATCH"
			} else {
				status = "❌ DIFF"
				allOK = false
			}
			fmt.Fprintf(f, "%-15s %-10.0f %-10.0f %-10s %-10s  |  %-10s %-10s %-10s %-10s  |  %s\n",
				ref.name, ref.browser.x, ref.browser.y, bw, bh, wx, wy, ww, wh, status)
		} else {
			fmt.Fprintf(f, "%-15s %-10.0f %-10.0f %-10s %-10s  |  %-10s %-10s %-10s %-10s  |  %s\n",
				ref.name, ref.browser.x, ref.browser.y, bw, bh, wx, wy, ww, wh, status)
			allOK = false
		}
	}

	// Text position comparison
	fmt.Fprintln(f)
	fmt.Fprintln(f, "=== TEXT SEGMENT COMPARISON ===")
	fmt.Fprintf(f, "%-20s %-10s %-10s %-15s  |  %-10s %-10s %-15s  |  %s\n",
		"TEXT", "B-Y-TOP", "B-Y-BOT", "B-RANGE", "W-Y-TOP", "W-Y-BOT", "W-RANGE", "GAP")
	fmt.Fprintln(f, strings.Repeat("-", 100))
	dumpTextCompare(f, root, state, &allOK)

	// Coverage
	canvas := graphics.NewCanvas(1280, 800)
	canvas.Clear(graphics.Color{R: 0, G: 0, B: 0, A: 0})
	rendering.Paint(rv, canvas, rendering.Rect{Width: 1280, Height: 800})
	pngPath := filepath.Join(wd, "dev", "static_test", "wbui_compare.png")
	savePNG(canvas, pngPath)
	cov := calcCoverage(canvas)

	fmt.Fprintf(f, "\n=== COVERAGE: %.1f%% ===\n", cov)
	fmt.Fprintf(f, "PNG: %s\n", pngPath)
	if allOK {
		fmt.Fprintln(f, "\n✅ ALL ELEMENTS MATCH BROWSER REFERENCE")
	} else {
		fmt.Fprintln(f, "\n❌ SOME ELEMENTS DIFFER FROM BROWSER REFERENCE")
	}

	fmt.Printf("Done! Report: %s\n", outPath)
}

func approxEq(a, b, tolerance float64) bool {
	d := a - b
	return d >= -tolerance && d <= tolerance
}

func dumpTextCompare(f *os.File, box *layout.ElementBox, state *layout.LayoutState, allOK *bool) {
	if len(box.TextSegments) > 0 {
		for i, seg := range box.TextSegments {
			textTop := seg.Y
			textBot := seg.Y + seg.Height
			lineTop := seg.LineY
			lineBot := seg.LineY + seg.LineHeight
			gapTop := textTop - lineTop
			gapBot := lineBot - textBot
			label := fmt.Sprintf("seg[%d]", i)
			fmt.Fprintf(f, "%-20s %.0f       %.0f       (%.0f-%.0f)  |  %.0f       %.0f       (%.0f-%.0f)  |  ▲=%.0f ▼=%.0f\n",
				label, lineTop, lineBot, textTop, textBot, textTop, textBot, lineTop, lineBot, gapTop, gapBot)
			if gapTop < 0 || gapBot < 0 {
				*allOK = false
			}
		}
	}
	// Text runs live in InlineTextBox children (or nested ElementBoxes).
	for _, child := range box.Children() {
		switch c := child.(type) {
		case *layout.InlineTextBox:
			if len(c.TextSegments) > 0 {
				for i, seg := range c.TextSegments {
					textTop := seg.Y
					textBot := seg.Y + seg.Height
					lineTop := seg.LineY
					lineBot := seg.LineY + seg.LineHeight
					gapTop := textTop - lineTop
					gapBot := lineBot - textBot
					label := fmt.Sprintf("%s[%d]", trunc(c.Text(), 15), i)
					fmt.Fprintf(f, "%-20s %.0f       %.0f       (%.0f-%.0f)  |  %.0f       %.0f       (%.0f-%.0f)  |  ▲=%.0f ▼=%.0f\n",
						label, lineTop, lineBot, textTop, textBot, textTop, textBot, lineTop, lineBot, gapTop, gapBot)
					if gapTop < 0 || gapBot < 0 {
						*allOK = false
					}
				}
			}
		case *layout.ElementBox:
			dumpTextCompare(f, c, state, allOK)
		}
	}
}

func findByClass(root *layout.ElementBox, class string) *layout.ElementBox {
	if root.Element() != nil && root.Element().GetAttribute("class") == class {
		return root
	}
	for _, c := range root.Children() {
		if eb, ok := c.(*layout.ElementBox); ok {
			if b := findByClass(eb, class); b != nil {
				return b
			}
		}
	}
	return nil
}

func mergeCSSFromDOM(doc *dom.Document, resolver *style.Resolver) {
	var walk func(el *dom.Element)
	walk = func(el *dom.Element) {
		if strings.EqualFold(el.LocalName(), "style") {
			text := textContent(el)
			if strings.TrimSpace(text) != "" {
				sheet := css.NewCSSStyleSheet()
				sheet.SetOwnerNode(el)
				css.NewParser(text).ParseStyleSheetInto(sheet)
				resolver.AddStyleSheet(sheet)
			}
		}
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok {
				walk(e)
			}
		}
	}
	if head := doc.Head(); head != nil {
		walk(head)
	}
	if body := doc.Body(); body != nil {
		walk(body)
	}
}

func textContent(el *dom.Element) string {
	var sb strings.Builder
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*dom.Text); ok {
			sb.WriteString(t.Data())
		}
	}
	return sb.String()
}

func savePNG(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 4
			c := color.NRGBA{R: pixels[off], G: pixels[off+1], B: pixels[off+2], A: pixels[off+3]}
			img.Set(x, y, c)
		}
	}
	f2, _ := os.Create(path)
	defer f2.Close()
	png.Encode(f2, img)
}

func calcCoverage(canvas *graphics.Canvas) float64 {
	pixels := canvas.Pixels()
	if len(pixels) == 0 {
		return 0
	}
	visible := 0
	total := len(pixels) / 4
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] > 0 {
			visible++
		}
	}
	return float64(visible) / float64(total) * 100
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
