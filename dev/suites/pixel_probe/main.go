// Command pixel_probe renders HTML through wb-ui full pipeline and reads pixels.
//
// Usage: go run ./dev/suites/pixel_probe --html FILE [--css FILE] [--w 1280] [--h 800] [--out PNG]
//
// Environment: CGO_ENABLED=1 (required for goskia)

package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
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

type ProbePoint struct {
	X, Y      int
	Label     string
	ExpR, ExpG, ExpB, ExpA byte
	Tolerance int
}

func main() {
	htmlFile := flag.String("html", "", "HTML file path")
	cssFile := flag.String("css", "", "CSS file path")
	width := flag.Int("w", 1280, "viewport width")
	height := flag.Int("h", 800, "viewport height")
	outPng := flag.String("out", "", "output PNG path")
	flag.Parse()

	if *htmlFile == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --html FILE is required")
		os.Exit(1)
	}
	data, err := os.ReadFile(*htmlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: read HTML: %v\n", err)
		os.Exit(1)
	}
	htmlSrc := string(data)
	if strings.TrimSpace(htmlSrc) == "" {
		fmt.Fprintln(os.Stderr, "ERROR: empty HTML")
		os.Exit(1)
	}

	// Setup font metrics
	mgr := graphics.InitFontManager("")
	if mgr != nil {
		mgr.LoadSystemFonts()
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	// Parse HTML
	doc, err := html.Parse(htmlSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse HTML: %v\n", err)
		os.Exit(1)
	}

	// Setup resolver
	resolver := style.NewResolver()

	// Load UA default stylesheet (required for form control defaults)
	resolver.AddStyleSheet(html5.NewUAStyleSheet())

	// Extract inline styles
	if doc.DocumentElement() != nil {
		extractStyles(doc.DocumentElement(), resolver)
	}

	// Add external CSS
	if *cssFile != "" {
		cd, err := os.ReadFile(*cssFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: read CSS: %v\n", err)
			os.Exit(1)
		}
		sheet := css.NewCSSStyleSheet()
		css.NewParser(string(cd)).ParseStyleSheetInto(sheet)
		resolver.AddStyleSheet(sheet)
	}

	// Build render tree
	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		fmt.Fprintln(os.Stderr, "ERROR: RenderView is nil")
		os.Exit(1)
	}
	rv.SetViewportSize(float64(*width), float64(*height))
	rv.Layout(nil)

	// DEBUG: Dump first 4 items positioned at x=0, y>30 (activity-bar items)
	if ls := rv.LayoutState(); ls != nil {
		rootEb := rv.LayoutBox()
		if rootEb != nil {
			var walk func(box *layout.ElementBox, depth int)
			count := 0
			walk = func(box *layout.ElementBox, depth int) {
				for _, child := range box.Children() {
					if count >= 4 { return }
					if childEb, ok := child.(*layout.ElementBox); ok {
						g := ls.GeometryForBox(childEb)
						if g.Left() >= -0.5 && g.Left() <= 0.5 && g.Top() >= 30 && g.Top() < 200 {
							fmt.Printf("[ACTBAR-item-%d] y=%.1f h=%.1f bw=%.1f bh=%.1f contH=%.1f marTop=%s\n",
								count, g.Top(), g.BorderBoxHeight(),
								g.BorderBoxWidth(), g.BorderBoxHeight(), g.ContentHeight(),
								childEb.Style().MarginTop.String())
							count++
						}
						walk(childEb, depth+1)
					}
				}
			}
			walk(rootEb, 0)
		}
	}

	// Paint
	canvas := graphics.NewCanvas(*width, *height)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: float64(*width), Height: float64(*height)})

	fmt.Println("\n=== TEXT-OVERFLOW ELLIPSIS SCAN (y=645..652, x=150..200 every pixel) ===")
	for y := 645; y <= 652; y++ {
		line := fmt.Sprintf("y=%3d |", y)
		for x := 150; x <= 200; x++ {
			p := canvas.PixelAt(x, y)
			if p.R > 20 || p.G > 20 || p.B > 20 {
				line += fmt.Sprintf("(%d,%02x%02x%02x)", x, p.R, p.G, p.B)
			} else {
				line += "."
			}
		}
		fmt.Println(line)
	}
	fmt.Println()

	// Save PNG
	if *outPng != "" {
		savePNG(canvas, *outPng)
	}

	// Dump layout tree
	fmt.Println("=== LAYOUT TREE ===")
	dumpRO(rv, 0)

	// Paint grid
	fmt.Printf("\n=== PAINT GRID (8x8, viewport=%dx%d) ===\n", *width, *height)
	for sy := 0; sy < 8; sy++ {
		cy := sy * *height / 8
		line := fmt.Sprintf("y=%d |", cy)
		for sx := 0; sx < 8; sx++ {
			cx := sx * *width / 8
			p := canvas.PixelAt(cx, cy)
			line += fmt.Sprintf(" (%d,#%02x%02x%02x)", cx, p.R, p.G, p.B)
		}
		fmt.Println(line)
	}

	// Probes
	fmt.Println("\n=== PIXEL PROBES (9-point sweep) ===")
	probes := []ProbePoint{
		{0, 0, "TL", 0, 0, 0, 0, 0},
		{*width / 2, 0, "TC", 0, 0, 0, 0, 0},
		{*width - 1, 0, "TR", 0, 0, 0, 0, 0},
		{0, *height / 2, "ML", 0, 0, 0, 0, 0},
		{*width / 2, *height / 2, "MC", 0, 0, 0, 0, 0},
		{*width - 1, *height / 2, "MR", 0, 0, 0, 0, 0},
		{0, *height - 1, "BL", 0, 0, 0, 0, 0},
		{*width / 2, *height / 2, "BC", 0, 0, 0, 0, 0},
		{*width - 1, *height - 1, "BR", 0, 0, 0, 0, 0},
	}
	for _, p := range probes {
		got := canvas.PixelAt(p.X, p.Y)
		fmt.Printf("  %s (%d,%d) = #%02x%02x%02x%02x\n",
			p.Label, p.X, p.Y, got.R, got.G, got.B, got.A)
	}

	// Fine titlebar scan: every pixel across the titlebar y=0..29
	fmt.Println("\n=== TITLEBAR SCAN (y=0..29, sample every 2px) ===")
	for y := 0; y < 30; y += 2 {
		line := fmt.Sprintf("y=%2d |", y)
		for x := 0; x < 60; x += 2 {
			p := canvas.PixelAt(x, y)
			if p.R > 50 || p.G > 50 || p.B > 200 {
				line += fmt.Sprintf("(%d,%02x%02x%02x)", x, p.R, p.G, p.B)
			} else if p.R > 20 {
				line += "*"
			} else {
				line += "."
			}
		}
		fmt.Println(line)
	}

	// Text segments debug
	fmt.Println("\n=== TEXT SEGMENTS ===")
	walkRenderText(rv, 0)

	// Horizontal scan at y=16 (center of app-logo area), x=0..48
	fmt.Println("\n=== HORIZONTAL SCAN y=16 ===")
	horiz := "y=16 |"
	for x := 0; x < 48; x++ {
		p := canvas.PixelAt(x, 16)
		if p.R > 20 {
			horiz += fmt.Sprintf("(%d#%02x%02x%02x)", x, p.R, p.G, p.B)
		} else if p.A > 0 {
			horiz += "."
		} else {
			horiz += " "
		}
	}
	fmt.Println(horiz)

	// === ACTIVITY BUTTON SCAN ===
	// === ACTIVITY BAR FULL SCAN (y=30..90, x=0..47) ===
	fmt.Println("\n=== ACTIVITY BAR FULL SCAN (y=30..90, x=0..47) ===")
	for y := 30; y <= 90; y++ {
		line := fmt.Sprintf("y=%3d |", y)
		for x := 0; x <= 47; x++ {
			p := canvas.PixelAt(x, y)
			if p.B > 200 && p.R > 50 {
				line += fmt.Sprintf("(%d,%02x%02x%02x)", x, p.R, p.G, p.B)
			} else if p.R > 30 || p.G > 30 {
				line += "*"
			} else {
				line += "."
			}
		}
		fmt.Println(line)
	}

	// === EDITOR TAB X-BTN SCAN ===
	fmt.Println("\n=== EDITOR TAB X-BTN SCAN (x=365..380, y=40..52) ===")
	for y := 40; y <= 52; y++ {
		line := fmt.Sprintf("y=%3d |", y)
		for x := 365; x <= 380; x++ {
			p := canvas.PixelAt(x, y)
			if p.R > 10 || p.G > 10 || p.B > 10 {
				line += fmt.Sprintf("(%d,%02x%02x%02x)", x, p.R, p.G, p.B)
			} else {
				line += "."
			}
		}
		fmt.Println(line)
	}

	fmt.Println("\n=== SIDEBAR VERTICAL SCAN: x=100, y=770..800 ===")
	for y := 770; y <= 800; y++ {
		p := canvas.PixelAt(100, y)
		fmt.Printf("  y=%3d = #%02x%02x%02x%02x", y, p.R, p.G, p.B, p.A)
		if p.R == 22 && p.G == 22 && p.B == 34 {
			fmt.Print(" <= SIDEBAR (#161b22)")
		}
		if p.R == 33 && p.G == 38 && p.B == 45 {
			fmt.Print(" <= STATUSBAR (#21262d)")
		}
		if p.R == 13 && p.G == 17 && p.B == 23 {
			fmt.Print(" <= ACTIVITY (#0d1117)")
		}
		fmt.Println()
	}

	fmt.Println("\n=== ACTIVITY BAR SCAN: x=10, y=30..800 ===")
	for y := 30; y <= 800; y += 40 {
		p := canvas.PixelAt(10, y)
		fmt.Printf("  y=%3d = #%02x%02x%02x%02x\n", y, p.R, p.G, p.B, p.A)
	}

	// Font metrics debug
	fmt.Println("\n=== FONT METRICS ===")
	printFontMetrics(rv)
}

func extractStyles(el *dom.Element, resolver *style.Resolver) {
	if strings.EqualFold(el.LocalName(), "style") {
		text := el.TextContent()
		if strings.TrimSpace(text) != "" {
			sheet := css.NewCSSStyleSheet()
			css.NewParser(text).ParseStyleSheetInto(sheet)
			resolver.AddStyleSheet(sheet)
		}
	}
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok {
			extractStyles(e, resolver)
		}
	}
}

func dumpRO(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	pad := strings.Repeat("  ", depth)
	name := ro.RenderName()
	nodeInfo := ""
	if el, ok := ro.Node().(*dom.Element); ok {
		tag := el.LocalName()
		id := el.GetId()
		cls := el.GetClassName()
		nodeInfo = fmt.Sprintf("<%s", tag)
		if id != "" {
			nodeInfo += "#" + id
		}
		if cls != "" {
			nodeInfo += "." + cls
		}
		nodeInfo += ">"
	}
	var x, y, w, h float64
	if lb := ro.LayoutBox(); lb != nil {
		if rv := ro.View(); rv != nil {
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(lb)
				x, y, w, h = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			}
		}
	}
	bg := ""
	if cs := ro.Style(); cs != nil {
		bg = fmt.Sprintf("bg=#%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
	}
	fmt.Printf("%s%s %s (%.0f,%.0f) %.0fx%.0f %s\n", pad, name, nodeInfo, x, y, w, h, bg)
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpRO(c, depth+1)
	}
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
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create PNG: %v\n", err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
	fmt.Printf("PNG: %s\n", path)
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

func walkRenderText(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	pfx := ""
	for i := 0; i < depth; i++ {
		pfx += "  "
	}
	if rt, ok := ro.(*rendering.RenderText); ok {
		segs := rt.Segments()
		fmt.Printf("%sRenderText %q len=%d segs=%d\n", pfx, rt.OriginalText(), rt.Length(), len(segs))
		for i, seg := range segs {
			fmt.Printf("%s  seg[%d]: start=%d len=%d x=%.1f y=%.1f w=%.1f h=%.1f\n",
				pfx, i, seg.Start, seg.Len, seg.X, seg.Y, seg.Width, seg.Height)
		}
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		walkRenderText(c, depth+1)
	}
}

func printFontMetrics(rv *rendering.RenderView) {
	var walk func(ro rendering.RenderObject)
	walk = func(ro rendering.RenderObject) {
		if ro == nil { return }
		if rt, ok := ro.(*rendering.RenderText); ok {
			st := rt.Style()
			if st == nil { return }
			fs := st.FontSize
			gf := toGraphicsFont(st)
			ascent := graphics.GlobalFontAscent(gf)
			descent := graphics.GlobalFontDescent(gf)
			realLineH := ascent + descent
			layoutLineH := 0.0
			if fs.Unit == "px" { layoutLineH = fs.Value * 1.2 }
			fmt.Printf("  RenderText=%q fontSize=%.1f layoutAscent=%.1f skiaAscent=%.1f layoutDescent=%.1f skiaDescent=%.1f layoutLineH=%.1f skiaLineH=%.1f diff=%.1f\n",
				rt.OriginalText(), pixelFontSize(fs), fs.Value*0.8, ascent, fs.Value*0.2, descent, layoutLineH, realLineH, realLineH-layoutLineH)
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
}

func pixelFontSize(l style.Length) float64 {
	if l.Unit == "px" { return l.Value }
	return 13
}

func toGraphicsFont(st *style.ComputedStyle) graphics.Font {
	sz := 16.0
	if st.FontSize.Unit == "px" { sz = st.FontSize.Value }
	w := 400
	switch strings.ToLower(strings.TrimSpace(st.FontWeight)) {
	case "bold", "bolder": w = 700
	case "lighter": w = 300
	case "", "normal": w = 400
	}
	return graphics.Font{
		Family: st.FontFamily,
		Size:   sz,
		Weight: w,
		Style:  st.FontStyle,
	}
}
