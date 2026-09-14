//go:build ignore

// Command static_test benchmarks wb-ui layout + paint against a reference HTML.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

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
	wd := "."
	fmt.Printf("Running static_test in %s\n", wd)

	htmlPath := filepath.Join(wd, "dev", "suites", "static_test", "ide_static.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read %s: %v\n", htmlPath, err)
		os.Exit(1)
	}
	htmlStr := string(data)
	fmt.Printf("HTML: %d bytes\n", len(htmlStr))

	// Bridge Skia font metrics to layout engine (same as desktop app/host.go)
	layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	// ─── 1. Parse HTML ───

	doc, err := html.Parse(htmlStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse HTML: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Parsed: %d elements\n", countElems(doc))

	// ─── 2. Collect CSS ───
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)
	fmt.Printf("CSS: stylesheets collected\n")

	// ─── 3. Build render tree ───
	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		fmt.Fprintln(os.Stderr, "ERROR: RenderView is nil")
		os.Exit(1)
	}

	// ─── 3b. Dump file creation (before diagnostics) ───
	dumpPath := filepath.Join(wd, "dev", "suites", "static_test", "ide_vue_diag.txt")
	f, err := os.Create(dumpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create diag: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// ─── 3c. Resolved style diagnostic for key elements ───
	fmt.Fprintln(f, "=== RESOLVED STYLES (key elements) ===")
	dumpResolvedStyles(doc, resolver, f)

	// ─── 4. Layout + Sync ───
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	// ─── 4b. Dump flex container info ───
	lb := rv.LayoutBox()
	if lb != nil {
		dumpFlexInfo(lb, f)
	}

	// ─── 5. Diagnostic dump (text/layout) ───
	if lb == nil {
		fmt.Fprintln(f, "=== LAYOUT BOX IS NIL ===")
		fmt.Fprintln(os.Stderr, "ERROR: layout box is nil!")
		os.Exit(1)
	}

	// 5a. Text segments
	total, withSegs, zeroW := countTextStats(lb)
	fmt.Fprintf(f, "=== TEXT: %d text nodes, %d with segments, %d zero-width ===\n", total, withSegs, zeroW)

	// 5b. Key element positions
	fmt.Fprintln(f, "\n=== KEY ELEMENT POSITIONS ===")
	dumpKeyBoxes(f, lb)

	// 5c. Full layout tree
	fmt.Fprintln(f, "\n=== LAYOUT TREE ===")
	dumpLayout(f, lb, 0)

	// ─── 6. Paint to PNG ───
	canvas := graphics.NewCanvas(1280, 800)
	canvas.Clear(graphics.Color{R: 0, G: 0, B: 0, A: 0}) // transparent to see actual painted areas
	rendering.Paint(rv, canvas, rendering.Rect{Width: 1280, Height: 800})

	outPath := filepath.Join(wd, "dev", "suites", "static_test", "ide_vue_output.png")
	
	savePNG(canvas, outPath)

	coverage := calcCoverage(canvas)
	fmt.Fprintf(f, "\n=== COVERAGE: %.1f%% ===\n", coverage)

	fmt.Printf("Done!\n  PNG: %s\n  Diag: %s\n  Coverage: %.1f%%\n  Texts: %d/%d zero-width=%d\n",
		outPath, dumpPath, coverage, withSegs, total, zeroW)
}

// ─── helpers ───

func countElems(doc *dom.Document) int {
	n := 0
	var walk func(*dom.Element)
	walk = func(el *dom.Element) {
		n++
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok {
				walk(e)
			}
		}
	}
	if root := doc.DocumentElement(); root != nil {
		walk(root)
	}
	return n
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

func countTextStats(box *layout.LayoutBox) (total, withSegs, zeroW int) {
	if box.Type == layout.BoxTextRun {
		total++
		if len(box.TextSegments) > 0 {
			withSegs++
			for _, seg := range box.TextSegments {
				if seg.Width <= 0 {
					zeroW++
				}
			}
		}
	}
	for _, c := range box.Children {
		t, s, z := countTextStats(c)
		total += t
		withSegs += s
		zeroW += z
	}
	return
}

func dumpKeyBoxes(f *os.File, root *layout.LayoutBox) {
	targets := []struct {
		desc   string
		class  string
	}{
		{"actbar", "actbar"},
		{"sidebar", "sidebar"},
		{"main", "main"},
		{"right-panel", "right-panel"},
		{"statusbar", "statusbar"},
		{"tabs", "tabs"},
		{"editor", "editor"},
	}

	for _, tgt := range targets {
		box := findByClass(root, tgt.class)
		if box != nil {
			disp := "?"
			if box.Style != nil {
				disp = fmtDisplay(box.Style.Display)
			}
			txtInfo := ""
			if len(box.TextSegments) > 0 {
				for i, seg := range box.TextSegments {
					txtInfo += fmt.Sprintf(" T%d[W=%.0f]", i, seg.Width)
				}
			}
			fmt.Fprintf(f, "  %-15s x=%.0f y=%.0f w=%.0f h=%.0f disp=%s%s\n",
				tgt.desc, box.Rect.X, box.Rect.Y, box.Rect.Width, box.Rect.Height, disp, txtInfo)
		} else {
			fmt.Fprintf(f, "  %-15s NOT FOUND\n", tgt.desc)
		}
	}
}

func findByClass(root *layout.LayoutBox, class string) *layout.LayoutBox {
	if root.Element != nil && (root.Element.GetAttribute("class") == class || root.Element.GetAttribute("id") == class) {
		return root
	}
	for _, c := range root.Children {
		if b := findByClass(c, class); b != nil {
			return b
		}
	}
	return nil
}

func dumpLayout(f *os.File, box *layout.LayoutBox, depth int) {
	if box == nil {
		return
	}
	prefix := strings.Repeat("  ", depth)
	typeName := typeStr(box.Type)
	disp := "?"
	if box.Style != nil {
		disp = fmtDisplay(box.Style.Display)
	}
	info := fmt.Sprintf("%s x=%6.0f y=%6.0f w=%6.0f h=%6.0f disp=%s", typeName, box.Rect.X, box.Rect.Y, box.Rect.Width, box.Rect.Height, disp)
	if box.Text != "" {
		info += fmt.Sprintf(" text=%q", trunc(box.Text, 30))
	}
	for i, seg := range box.TextSegments {
		info += fmt.Sprintf(" s%d[x=%.0f,y=%.0f,h=%.0f,w=%.0f]", i, seg.X, seg.Y, seg.Height, seg.Width)
	}
	if box.Text != "" && box.Rect.Height > 0 && len(box.TextSegments) > 0 {
		seg := box.TextSegments[0]
		topGap := seg.Y - box.Rect.Y
		botGap := (box.Rect.Y + box.Rect.Height) - (seg.Y + seg.Height)
		info += fmt.Sprintf(" ▲=%.0f ▼=%.0f", topGap, botGap)
	}
	fmt.Fprintf(f, "%s%s\n", prefix, info)
	for _, c := range box.Children {
		dumpLayout(f, c, depth+1)
	}
}

func typeStr(t layout.BoxType) string {
	switch t {
	case 0:
		return "BoxBlock"
	case 1:
		return "BoxInline"
	case 2:
		return "BoxAnonymous"
	case 3:
		return "BoxTextRun"
	case 4:
		return "BoxInlineBlock"
	default:
		return fmt.Sprintf("Type%d", t)
	}
}

func fmtDisplay(d style.DisplayType) string {
	switch d {
	case style.DisplayNone:
		return "none"
	case style.DisplayInline:
		return "inline"
	case style.DisplayBlock:
		return "block"
	case style.DisplayInlineBlock:
		return "inl-block"
	case style.DisplayFlex:
		return "flex"
	case style.DisplayInlineFlex:
		return "inl-flex"
	default:
		return fmt.Sprintf("d%d", d)
	}
}

func savePNG(canvas *graphics.Canvas, path string) {
	pixels := canvas.Pixels()
	w, h := canvas.Width(), canvas.Height()
	if len(pixels) < w*h*4 {
		fmt.Fprintf(os.Stderr, "ERROR: not enough pixels for %dx%d\n", w, h)
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: create %s: %v\n", path, err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
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

func dumpResolvedStyles(doc *dom.Document, resolver *style.Resolver, f *os.File) {
	body := doc.Body()
	if body == nil {
		return
	}
	// Dump styles for specific target elements
	var walk func(el *dom.Element, depth int)
	walk = func(el *dom.Element, depth int) {
		if depth > 8 {
			return
		}
		cs := resolver.ResolveElement(el)
		id := el.GetAttribute("id")
		cls := el.GetAttribute("class")
		sty := el.GetAttribute("style")
		if len(sty) > 60 {
			sty = sty[:60] + "..."
		}
		// Only dump elements with explicit styles or ids
		if id != "" || cls != "" || sty != "" {
			prefix := strings.Repeat("  ", depth)
			fmt.Fprintf(f, "%s<%s> id=%q cls=%q color=%s bg=%s alignItems=%q justifyContent=%q disp=%s flexGrow=%.1f style=%q\n",
				prefix, el.LocalName(), id, cls,
				cs.Color.String(), cs.BackgroundColor.String(),
				cs.AlignItems, cs.JustifyContent, fmtDisplay(cs.Display), cs.FlexGrow, sty)
		}
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok {
				walk(e, depth+1)
			}
		}
	}
	walk(body, 0)
}

func dumpFlexInfo(root *layout.LayoutBox, f *os.File) {
	fmt.Fprintln(f, "\n=== FLEX GROW INFO ===")
	var walk func(box *layout.LayoutBox)
	walk = func(box *layout.LayoutBox) {
		if box.Style != nil && box.Style.Display == style.DisplayFlex {
			// Only dump if this container has flex-grow children
			hasGrow := false
			for _, c := range box.Children {
				if c.Style != nil && c.Style.FlexGrow > 0 {
					hasGrow = true
					break
				}
			}
			if hasGrow {
				cw := box.Rect.ContentWidth()
				ch := box.Rect.ContentHeight()
				fmt.Fprintf(f, "FLEX_CONTAINER x=%.0f y=%.0f w=%.0f h=%.0f cw=%.0f ch=%.0f dir=%q\n",
					box.Rect.X, box.Rect.Y, box.Rect.Width, box.Rect.Height,
					cw, ch, box.Style.FlexDirection)
				for _, c := range box.Children {
					fmt.Fprintf(f, "  CHILD x=%.0f y=%.0f w=%.0f h=%.0f grow=%.1f shrink=%.1f basis=%s %s\n",
						c.Rect.X, c.Rect.Y, c.Rect.Width, c.Rect.Height,
						c.Style.FlexGrow, c.Style.FlexShrink, c.Style.FlexBasis.String(),
						typeStr(c.Type))
				}
			}
		}
		for _, c := range box.Children {
			walk(c)
		}
	}
	walk(root)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
