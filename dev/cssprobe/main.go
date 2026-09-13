// Command cssprobe runs the deterministic CSS render fixtures through wb-ui's
// own pipeline and asserts the geometry of their solid-color boxes.
//
// The fixture set and the oracle are borrowed from obscura's render-repros
// (crates/obscura-render + render-repros/check.py, Apache-2.0): every fixture is
// a tiny page whose colored boxes encode one CSS behavior, and checks.json
// states the expected box geometry in *rendered pixel* space. Matching is by
// exact solid color with a ±1px tolerance, exactly like check.py, so font
// anti-aliasing cannot decide pass/fail.
//
// Why pixel components instead of the render tree: a property that parses and
// computes but never reaches paint (or never reserves layout space) produces a
// correct-looking ComputedStyle and a wrong picture. Asserting the painted
// component verifies layout AND paint in one shot — which is what "the property
// is actually usable" means for an embedder.
//
// Usage:
//
//	go run ./dev/cssprobe                     # every fixture
//	go run ./dev/cssprobe -filter float       # fixtures matching a regexp
//	go run ./dev/cssprobe -v                  # also list passing checks
//	go run ./dev/cssprobe -dump /tmp/out      # write rendered PNGs
//	go run ./dev/cssprobe -json report.json   # machine-readable results
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

// The reference viewport matches obscura's run.sh (OBSCURA_SHOT_W/H) and
// capture_chromium.py's defaults, so fixture expectations are comparable
// verbatim.
const (
	viewportW = 900
	viewportH = 1000
	minArea   = 20 // check.py: components smaller than this are noise
	tolerance = 1  // check.py: |actual - expected| <= 1
)

// Check is one expected solid-color component (checks.json entry).
type Check struct {
	Name   string `json:"name"`
	Color  string `json:"color"`
	X      *int   `json:"x,omitempty"`
	Y      *int   `json:"y,omitempty"`
	Width  *int   `json:"width,omitempty"`
	Height *int   `json:"height,omitempty"`
	Count  int    `json:"count,omitempty"`
}

// Component is one connected area of an exact solid color in the rendered page.
type Component struct {
	X, Y, W, H, Area int
}

func (c Component) String() string {
	return fmt.Sprintf("%dx%d at (%d,%d) area=%d", c.W, c.H, c.X, c.Y, c.Area)
}

// CheckResult is the verdict for a single expected component.
type CheckResult struct {
	Name     string      `json:"name"`
	Color    string      `json:"color"`
	Expected string      `json:"expected"`
	Passed   bool        `json:"passed"`
	Found    int         `json:"found"`
	Actual   []Component `json:"actual,omitempty"`
	// Near explains a miss: how many pixels came close to the wanted color and
	// what the closest pixel actually was. Distinguishes "painted in a slightly
	// different color" (a paint/compositing difference) from "never painted"
	// (a missing implementation), which the reports must not conflate.
	Near *NearColor `json:"near,omitempty"`
}

// NearColor summarizes the closest pixels to an expected color.
type NearColor struct {
	WithinTolerance int    `json:"within_tolerance"` // pixels with every channel within nearTolerance
	Closest         string `json:"closest"`          // hex of the closest pixel found
	ClosestDelta    int    `json:"closest_delta"`    // max channel distance of that pixel
	ClosestX        int    `json:"closest_x"`
	ClosestY        int    `json:"closest_y"`
}

// FixtureResult aggregates one fixture's outcome.
type FixtureResult struct {
	Name    string        `json:"name"`
	Checks  []CheckResult `json:"checks"`
	Error   string        `json:"error,omitempty"`
	Failed  int           `json:"failed"`
	Blank   bool          `json:"blank,omitempty"` // nothing but the white canvas
	Painted int           `json:"painted_pixels,omitempty"`
}

func main() {
	filter := flag.String("filter", "", "regexp: only fixtures whose name matches")
	dump := flag.String("dump", "", "directory to write rendered PNGs into")
	verbose := flag.Bool("v", false, "also print passing checks")
	tree := flag.Bool("tree", false, "print the laid-out render tree for each fixture")
	jsonOut := flag.String("json", "", "write machine-readable results to this file")
	dir := flag.String("fixtures", defaultFixtureDir(), "fixture directory (HTML + checks.json)")
	flag.Parse()

	var filterRE *regexp.Regexp
	if *filter != "" {
		re, err := regexp.Compile(*filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cssprobe: bad -filter %q: %v\n", *filter, err)
			os.Exit(2)
		}
		filterRE = re
	}

	raw, err := os.ReadFile(filepath.Join(*dir, "checks.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cssprobe: read checks.json: %v\n", err)
		os.Exit(2)
	}
	var checks map[string][]Check
	if err := json.Unmarshal(raw, &checks); err != nil {
		fmt.Fprintf(os.Stderr, "cssprobe: parse checks.json: %v\n", err)
		os.Exit(2)
	}

	names := make([]string, 0, len(checks))
	for name := range checks {
		if filterRE != nil && !filterRE.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "cssprobe: no fixtures selected")
		os.Exit(2)
	}

	if *dump != "" {
		if err := os.MkdirAll(*dump, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "cssprobe: mkdir %s: %v\n", *dump, err)
			os.Exit(2)
		}
	}

	var (
		results     []FixtureResult
		totalPass   int
		totalChecks int
		failedFix   []string
	)
	for _, name := range names {
		src, err := os.ReadFile(filepath.Join(*dir, name+".html"))
		if err != nil {
			results = append(results, FixtureResult{Name: name, Error: err.Error()})
			failedFix = append(failedFix, name)
			fmt.Printf("%-34s ERROR reading fixture: %v\n", name, err)
			continue
		}

		dumpPath := ""
		if *dump != "" {
			dumpPath = filepath.Join(*dump, name+".wbui.png")
		}
		var treeWriter io.Writer
		if *tree {
			treeWriter = os.Stdout
			fmt.Printf("--- %s render tree ---\n", name)
		}
		res := runFixture(name, string(src), checks[name], dumpPath, treeWriter)
		results = append(results, res)

		status := "PASS"
		if res.Error != "" {
			status = "ERROR"
			failedFix = append(failedFix, name)
		} else if res.Failed > 0 {
			status = fmt.Sprintf("FAIL %d/%d", res.Failed, len(res.Checks))
			failedFix = append(failedFix, name)
		} else if res.Blank {
			status = "EMPTY"
			failedFix = append(failedFix, name)
		}
		fmt.Printf("%-34s %s\n", name, status)

		for _, c := range res.Checks {
			totalChecks++
			if c.Passed {
				totalPass++
			}
			if !c.Passed || *verbose {
				mark := "ok  "
				if !c.Passed {
					mark = "FAIL"
				}
				detail := joinComponents(c.Actual)
				if c.Near != nil {
					detail = fmt.Sprintf("closest=%s Δ=%d @(%d,%d) within±%d=%dpx",
						c.Near.Closest, c.Near.ClosestDelta, c.Near.ClosestX, c.Near.ClosestY,
						nearTolerance, c.Near.WithinTolerance)
				}
				fmt.Printf("    %s %-46s want %s (count>=%d) got %d: %s\n",
					mark, truncate(c.Name, 46), c.Expected, max(1, countOf(checks[name], c.Name)), c.Found, detail)
			}
		}
		if res.Error != "" {
			fmt.Printf("    ERROR %s\n", res.Error)
		}
	}

	fmt.Printf("\n%d/%d fixtures clean, %d/%d component checks passed\n",
		len(names)-len(failedFix), len(names), totalPass, totalChecks)
	if len(failedFix) > 0 {
		fmt.Printf("failing fixtures: %s\n", strings.Join(failedFix, " "))
	}

	if *jsonOut != "" {
		payload, _ := json.MarshalIndent(map[string]any{
			"fixtures":      results,
			"checks_passed": totalPass,
			"checks_total":  totalChecks,
			"failed":        failedFix,
		}, "", "  ")
		if err := os.WriteFile(*jsonOut, append(payload, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "cssprobe: write %s: %v\n", *jsonOut, err)
			os.Exit(2)
		}
	}

	if len(failedFix) > 0 {
		os.Exit(1)
	}
}

// runFixture renders one fixture and evaluates every expected component. When
// dumpPath is non-empty the rendered image is written there for visual review.
func runFixture(name, htmlText string, checks []Check, dumpPath string, tree io.Writer) FixtureResult {
	res := FixtureResult{Name: name}
	img, err := renderFixture(htmlText, tree)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if dumpPath != "" {
		if err := dumpPNG(dumpPath, img); err != nil {
			res.Error = "dump png: " + err.Error()
			return res
		}
	}

	painted := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 || img.Pix[i+1] != 255 || img.Pix[i+2] != 255 {
			painted++
		}
	}
	res.Painted = painted
	res.Blank = painted == 0

	mask := newColorMask(viewportW, viewportH)
	for _, chk := range checks {
		cr := CheckResult{Name: chk.Name, Color: chk.Color, Expected: describeExpectation(chk)}
		want, ok := parseHex(chk.Color)
		if !ok {
			cr.Expected = "bad color " + chk.Color
			res.Checks = append(res.Checks, cr)
			res.Failed++
			continue
		}
		found := mask.components(img, want, minArea)
		matches := 0
		for _, comp := range found {
			if matchesCheck(comp, chk) {
				matches++
			}
		}
		cr.Found = matches
		cr.Actual = found
		cr.Passed = matches >= max(1, chk.Count)
		if !cr.Passed {
			res.Failed++
			cr.Near = nearestColor(img, want)
		}
		res.Checks = append(res.Checks, cr)
	}
	return res
}

// buildLayout parses the fixture and lays it out at the reference viewport,
// returning the laid-out render tree.
func buildLayout(htmlText string) (*rendering.RenderView, error) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	// ex 单位用字体真实 x-height（Skia FontMetrics().XHeight）。
	layout.XHeightFunc = func(family string, size float64, weight int, style2 string) float64 {
		return graphics.GlobalFontXHeight(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2})
	}

	doc, err := html.Parse(htmlText)
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
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
	rv.SetViewportSize(viewportW, viewportH)
	state := layout.NewLayoutState(viewportW, viewportH)
	rv.Layout(state)
	return rv, nil
}

// renderFixture runs the fixture through wb-ui's pipeline at the reference
// viewport and returns the composited RGBA image. When tree is non-nil the
// laid-out render tree is written there for diagnosis.
func renderFixture(htmlText string, tree io.Writer) (*image.RGBA, error) {
	rv, err := buildLayout(htmlText)
	if err != nil {
		return nil, err
	}
	if tree != nil {
		dumpTree(tree, rv, 0)
	}
	canvas := graphics.NewCanvas(viewportW, viewportH)
	defer canvas.Release()
	// Chromium (and obscura) composite onto an opaque white canvas; wb-ui's
	// raster surface starts transparent. Clearing first makes every pixel
	// opaque, which also makes the premultiplied read-back exactly linear and
	// keeps the strict color comparison meaningful.
	canvas.Clear(graphics.Color{R: 255, G: 255, B: 255, A: 255})
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: viewportW, Height: viewportH})

	img := image.NewRGBA(image.Rect(0, 0, viewportW, viewportH))
	pix := canvas.Pixels()
	if len(pix) == len(img.Pix) {
		copy(img.Pix, pix)
		return img, nil
	}
	for y := 0; y < viewportH; y++ {
		for x := 0; x < viewportW; x++ {
			p := canvas.PixelAt(x, y)
			img.SetRGBA(x, y, color.RGBA{R: p.R, G: p.G, B: p.B, A: p.A})
		}
	}
	return img, nil
}

// applyDocumentCSS feeds <style> text and style="" attributes found in the
// parsed document to the resolver, mirroring the browser-facing path.
// dumpTree prints the laid-out render tree geometry, so a fixture can be
// diagnosed without guessing from pixels.
func dumpTree(w io.Writer, ro rendering.RenderObject, depth int) {
	if ro == nil || depth > 14 {
		return
	}
	indent := strings.Repeat("  ", depth)
	label := "node"
	if n := ro.Node(); n != nil {
		switch v := n.(type) {
		case *dom.Element:
			label = v.LocalName()
			if id := v.GetAttribute("id"); id != "" {
				label += "#" + id
			}
			if cls := v.GetAttribute("class"); cls != "" {
				label += "." + strings.ReplaceAll(strings.TrimSpace(cls), " ", ".")
			}
		case *dom.Text:
			text := strings.TrimSpace(v.Data())
			if text == "" {
				text = "<ws>"
			}
			label = fmt.Sprintf("#text(%s)", truncate(text, 24))
		}
	}
	if x, y, bw, bh, ok := rendering.BoxGeometry(ro); ok {
		link := "unlinked"
		if ro.LayoutBox() != nil {
			link = "linked"
		}
		fmt.Fprintf(w, "%s%s  %.1fx%.1f @(%.1f,%.1f) [%s]\n", indent, label, bw, bh, x, y, link)
	} else {
		fmt.Fprintf(w, "%s%s  (no box)\n", indent, label)
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpTree(w, c, depth+1)
	}
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
		sheet := cssStyleSheet(text)
		if sheet != nil {
			resolver.AddStyleSheet(sheet)
		}
	}
}

// cssStyleSheet parses author-origin CSS text into a stylesheet.
func cssStyleSheet(text string) *css.CSSStyleSheet {
	sheet := css.NewCSSStyleSheet()
	sheet.SetOrigin(css.OriginAuthor)
	p := css.NewParser(text)
	p.SetOrigin(css.OriginAuthor)
	for _, rule := range p.ParseStyleSheet() {
		sheet.AppendRule(rule)
	}
	return sheet
}

// colorMask finds connected areas of one exact color. It reuses its mask buffer
// across checks so scanning hundreds of expectations stays cheap.
type colorMask struct {
	w, h    int
	mask    []bool
	touched []int32
}

func newColorMask(w, h int) *colorMask {
	return &colorMask{w: w, h: h, mask: make([]bool, w*h)}
}

// components returns every connected (4-neighbour, like scipy.ndimage.label)
// area of exactly the wanted color, keeping areas of at least minArea pixels.
func (m *colorMask) components(img *image.RGBA, want color.RGBA, minArea int) []Component {
	for _, i := range m.touched {
		m.mask[i] = false
	}
	m.touched = m.touched[:0]

	pix := img.Pix
	for y := 0; y < m.h; y++ {
		row := y * img.Stride
		for x := 0; x < m.w; x++ {
			o := row + x*4
			if pix[o] == want.R && pix[o+1] == want.G && pix[o+2] == want.B {
				i := int32(y*m.w + x)
				m.mask[i] = true
				m.touched = append(m.touched, i)
			}
		}
	}

	var out []Component
	for _, start := range m.touched {
		if !m.mask[start] {
			continue
		}
		comp := m.flood(start)
		if comp.Area >= minArea {
			out = append(out, comp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

// flood consumes one 4-connected component starting at start (which must be
// marked) and returns its bounding box and area.
func (m *colorMask) flood(start int32) Component {
	stack := []int32{start}
	m.mask[start] = false
	minX, minY := m.w, m.h
	maxX, maxY := -1, -1
	area := 0
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := int(i)%m.w, int(i)/m.w
		area++
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
		if x > 0 {
			if j := i - 1; m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if x < m.w-1 {
			if j := i + 1; m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if y > 0 {
			if j := i - int32(m.w); m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
		if y < m.h-1 {
			if j := i + int32(m.w); m.mask[j] {
				m.mask[j] = false
				stack = append(stack, j)
			}
		}
	}
	return Component{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1, Area: area}
}

// nearTolerance is the per-channel window used by nearestColor to decide
// whether an off-color area is "almost" the wanted color.
const nearTolerance = 8

// nearestColor reports how close the rendered page came to a color that was
// expected but not found exactly.
func nearestColor(img *image.RGBA, want color.RGBA) *NearColor {
	best := NearColor{ClosestDelta: 1 << 30, Closest: "#000000"}
	for y := 0; y < viewportH; y++ {
		row := y * img.Stride
		for x := 0; x < viewportW; x++ {
			o := row + x*4
			r, g, b := img.Pix[o], img.Pix[o+1], img.Pix[o+2]
			d := max3(abs(int(r)-int(want.R)), abs(int(g)-int(want.G)), abs(int(b)-int(want.B)))
			if d <= nearTolerance {
				best.WithinTolerance++
			}
			if d < best.ClosestDelta {
				best.ClosestDelta = d
				best.Closest = fmt.Sprintf("#%02x%02x%02x", r, g, b)
				best.ClosestX, best.ClosestY = x, y
			}
		}
	}
	return &best
}

func max3(a, b, c int) int {
	return max(max(a, b), c)
}

// matchesCheck applies check.py's per-field ±1px tolerance. Fields absent from
// the expectation are not constrained (check.py: only present keys are tested).
func matchesCheck(comp Component, chk Check) bool {
	if chk.X != nil && abs(comp.X-*chk.X) > tolerance {
		return false
	}
	if chk.Y != nil && abs(comp.Y-*chk.Y) > tolerance {
		return false
	}
	if chk.Width != nil && abs(comp.W-*chk.Width) > tolerance {
		return false
	}
	if chk.Height != nil && abs(comp.H-*chk.Height) > tolerance {
		return false
	}
	return true
}

func describeExpectation(chk Check) string {
	parts := []string{}
	if chk.X != nil {
		parts = append(parts, fmt.Sprintf("x=%d", *chk.X))
	}
	if chk.Y != nil {
		parts = append(parts, fmt.Sprintf("y=%d", *chk.Y))
	}
	if chk.Width != nil {
		parts = append(parts, fmt.Sprintf("w=%d", *chk.Width))
	}
	if chk.Height != nil {
		parts = append(parts, fmt.Sprintf("h=%d", *chk.Height))
	}
	if len(parts) == 0 {
		return "exists"
	}
	return strings.Join(parts, " ")
}

func joinComponents(comps []Component) string {
	if len(comps) == 0 {
		return "(none)"
	}
	limit := len(comps)
	if limit > 4 {
		limit = 4
	}
	parts := make([]string, 0, limit+1)
	for _, c := range comps[:limit] {
		parts = append(parts, c.String())
	}
	if len(comps) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(comps)-limit))
	}
	return strings.Join(parts, ", ")
}

func parseHex(value string) (color.RGBA, bool) {
	s := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(s) != 6 {
		return color.RGBA{}, false
	}
	var v [3]uint8
	for i := 0; i < 3; i++ {
		var b int
		if _, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &b); err != nil {
			return color.RGBA{}, false
		}
		v[i] = uint8(b)
	}
	return color.RGBA{R: v[0], G: v[1], B: v[2], A: 255}, true
}

func countOf(checks []Check, name string) int {
	for _, c := range checks {
		if c.Name == name {
			return c.Count
		}
	}
	return 1
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// defaultFixtureDir locates fixtures/ next to this source file so the command
// works regardless of the working directory.
func defaultFixtureDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "fixtures"
	}
	return filepath.Join(filepath.Dir(file), "fixtures")
}

// dumpPNG writes an image for visual inspection.
func dumpPNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
