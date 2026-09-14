// Command static_probe renders a real-world page (ide_static.html / the Vue
// IDE page) through the current wb-ui pipeline and writes PNGs for visual
// inspection.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

func mergeCSSFromDOM(doc *dom.Document, resolver *style.Resolver) {
	var collect func(n dom.Node, styles *[]string)
	collect = func(n dom.Node, styles *[]string) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					*styles = append(*styles, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c, styles)
		}
	}
	var styles []string
	collect(doc, &styles)
	for _, s := range styles {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(s)
		p.SetOrigin(css.OriginAuthor)
		for _, r := range p.ParseStyleSheet() {
			sheet.AppendRule(r)
		}
		resolver.AddStyleSheet(sheet)
	}
}

func renderPage(path string, w, h int, out string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	htmlStr := string(data)

	if graphics.GetFontManager() == nil {
		mgr := graphics.InitFontManager("")
		mgr.LoadSystemFonts()
	}

	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	doc, err := html.Parse(htmlStr)
	if err != nil {
		return fmt.Errorf("html parse: %w", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	if rv == nil {
		return fmt.Errorf("render tree build failed")
	}
	rv.SetViewportSize(float64(w), float64(h))
	state := layout.NewLayoutState(float64(w), float64(h))
	rv.Layout(state)

	canvas := graphics.NewCanvas(w, h)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})

	// Composite onto white + write PNG (canvas pixels are premultiplied RGBA).
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := canvas.PixelAt(x, y)
			a := float64(p.A) / 255
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(p.R) * a),
				G: uint8(float64(p.G) * a),
				B: uint8(float64(p.B) * a),
				A: p.A,
			})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	fmt.Printf("rendered %s → %s (%dx%d)\n", filepath.Base(path), out, w, h)
	return nil
}

func main() {
	wd := "dev/static_probe"
	// 1. ide_static.html (real IDE layout, ~636KB)
	if err := renderPage("dev/static_test/ide_static.html", 1280, 800, filepath.Join(wd, "ide_now.png")); err != nil {
		fmt.Fprintln(os.Stderr, "ide_static:", err)
	} else {
		fmt.Println("ide_static OK")
	}
	// 2. grid_app.html (correct grid+flex IDE layout)
	if err := renderPage("dev/static_probe/grid_app.html", 1280, 800, filepath.Join(wd, "grid_app.png")); err != nil {
		fmt.Fprintln(os.Stderr, "grid_app:", err)
	} else {
		fmt.Println("grid_app OK")
	}
	// 3. desc_test.html (descendant selector + background:none)
	if err := renderPage("dev/static_probe/desc_test.html", 1280, 800, filepath.Join(wd, "desc_test.png")); err != nil {
		fmt.Fprintln(os.Stderr, "desc_test:", err)
	} else {
		fmt.Println("desc_test OK")
	}
}
