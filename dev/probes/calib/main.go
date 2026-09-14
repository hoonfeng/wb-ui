// calib 把同一个 HTML 夹具分别交给 wb-ui 与 obscura（Rust 参考实现）渲染，
// 量化两者的像素差异——cssprobe 的期望值是「参照实现的渲染结果」，当某项期望
// 看起来与标准 CSS 矛盾时（例如相邻边框是否 45° 斜切），用本工具可以直接看出
// 差异到底在哪、有多大，而不是盯着单个检查项的 want 猜。
//
//	go run ./dev/probes/calib -fixture dev/suites/cssprobe/fixtures/logical-borders.html
//	go run ./dev/probes/calib -fixture dev/suites/cssprobe/fixtures/tables.html -out /tmp/diff.png
//
// obscura 侧走它的离线绘制入口 crates/obscura-render/src/bin/paint_file.rs
// （不依赖 V8，可在 v8 静态库损坏时照常使用）；-obscura 可指定其他二进制路径。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

func main() {
	fixture := flag.String("fixture", "", "HTML fixture to render with both engines")
	obscura := flag.String("obscura", "", "obscura paint_file executable (the reference renderer)")
	w := flag.Int("w", 900, "viewport width")
	h := flag.Int("h", 1000, "viewport height")
	out := flag.String("out", "", "write a diff PNG (differing pixels in red) here")
	block := flag.Int("block", 32, "region size used to rank the largest differences")
	top := flag.Int("top", 6, "print the N largest differing regions")
	flag.Parse()
	if *fixture == "" {
		fmt.Fprintln(os.Stderr, "calib: -fixture is required")
		os.Exit(2)
	}
	if *obscura == "" {
		fmt.Fprintln(os.Stderr, "calib: -obscura is required (path to obscura's paint_file binary)")
		os.Exit(2)
	}
	htmlText, err := os.ReadFile(*fixture)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: read fixture: %v\n", err)
		os.Exit(1)
	}

	ours, err := renderOurs(string(htmlText), *w, *h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: wb-ui render: %v\n", err)
		os.Exit(1)
	}
	theirs, err := renderObscura(*obscura, *fixture, *w, *h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calib: obscura render: %v\n", err)
		os.Exit(1)
	}
	if ours.Bounds() != theirs.Bounds() {
		fmt.Fprintf(os.Stderr, "calib: size mismatch: wb-ui %v vs obscura %v\n", ours.Bounds(), theirs.Bounds())
		os.Exit(1)
	}

	diffPix, minX, minY, maxX, maxY := 0, 1<<30, 1<<30, -1, -1
	blocks := map[[2]int]int{}
	for y := 0; y < *h; y++ {
		for x := 0; x < *w; x++ {
			ao, bo, co, _ := ours.At(x, y).RGBA()
			at, bt, ct, _ := theirs.At(x, y).RGBA()
			if ao == at && bo == bt && co == ct {
				continue
			}
			diffPix++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			blocks[[2]int{x / *block, y / *block}]++
		}
	}

	total := *w * *h
	fmt.Printf("fixture %s  %dx%d\n", filepath.Base(*fixture), *w, *h)
	fmt.Printf("differing pixels: %d / %d (%.3f%%)\n", diffPix, total, 100*float64(diffPix)/float64(total))
	if diffPix == 0 {
		fmt.Println("wb-ui and obscura render identically")
		return
	}
	fmt.Printf("difference bounds: (%d,%d)-(%d,%d)\n", minX, minY, maxX, maxY)

	type region struct {
		x, y, n int
	}
	regions := make([]region, 0, len(blocks))
	for k, n := range blocks {
		regions = append(regions, region{k[0] * *block, k[1] * *block, n})
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].n > regions[j].n })
	area := *block * *block
	for i, r := range regions {
		if i >= *top {
			break
		}
		fmt.Printf("  region (%d,%d) %dx%d: %d differing px (%.0f%%)\n",
			r.x, r.y, *block, *block, r.n, 100*float64(r.n)/float64(area))
	}

	if *out != "" {
		dim := image.NewRGBA(image.Rect(0, 0, *w, *h))
		for y := 0; y < *h; y++ {
			for x := 0; x < *w; x++ {
				ao, bo, co, _ := ours.At(x, y).RGBA()
				at, bt, ct, _ := theirs.At(x, y).RGBA()
				if ao == at && bo == bt && co == ct {
					g := uint8(ao >> 8)
					dim.SetRGBA(x, y, color.RGBA{R: g, G: g, B: g, A: 255})
					continue
				}
				// 差异像素：ours=绿、obscura=红，便于一眼看出谁多了/少了。
				dim.SetRGBA(x, y, color.RGBA{R: uint8(at >> 8), G: uint8(ao >> 8), B: 0, A: 255})
			}
		}
		f, err := os.Create(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "calib: create diff png: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		if err := png.Encode(f, dim); err != nil {
			fmt.Fprintf(os.Stderr, "calib: encode diff png: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("diff written to %s (green=wb-ui only, red=obscura only)\n", *out)
	}
}

func renderOurs(htmlText string, w, h int) (*image.RGBA, error) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, st, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: st}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, st string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: st}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	layout.XHeightFunc = func(family string, size float64, weight int, st string) float64 {
		return graphics.GlobalFontXHeight(graphics.Font{Family: family, Size: size, Weight: weight, Style: st})
	}

	doc, err := html.Parse(htmlText)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	applyDocumentCSS(doc, resolver)
	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		return nil, fmt.Errorf("render tree build failed")
	}
	rv.SetResolver(resolver)
	rv.SetViewportSize(float64(w), float64(h))
	state := layout.NewLayoutState(float64(w), float64(h))
	rv.Layout(state)

	canvas := graphics.NewCanvas(w, h)
	defer canvas.Release()
	canvas.Clear(graphics.Color{R: 255, G: 255, B: 255, A: 255})
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	pix := canvas.Pixels()
	if len(pix) == len(img.Pix) {
		copy(img.Pix, pix)
		return img, nil
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := canvas.PixelAt(x, y)
			img.SetRGBA(x, y, color.RGBA{R: p.R, G: p.G, B: p.B, A: p.A})
		}
	}
	return img, nil
}

func renderObscura(bin, fixture string, w, h int) (*image.RGBA, error) {
	abs, err := filepath.Abs(fixture)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "calib-*.png")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	cmd := exec.Command(bin, filepath.ToSlash(abs), tmp.Name(), strconv.Itoa(w), strconv.Itoa(h))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run obscura: %w", err)
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode obscura png: %w", err)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			rgba.SetRGBA(x-b.Min.X, y-b.Min.Y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)})
		}
	}
	return rgba, nil
}

func applyDocumentCSS(doc *dom.Document, resolver *style.Resolver) {
	var sheets []string
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					sheets = append(sheets, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc)
	for _, text := range sheets {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(text)
		p.SetOrigin(css.OriginAuthor)
		for _, rule := range p.ParseStyleSheet() {
			sheet.AppendRule(rule)
		}
		resolver.AddStyleSheet(sheet)
	}
}
