// Command window_test loads an HTML file in a wb-ui window for interactive testing.
// Usage: go run ./dev/probes/window_test --html PATH
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/platform/window"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

func main() {
	htmlFile := flag.String("html", "", "HTML file path")
	width := flag.Int("w", 800, "window width")
	height := flag.Int("h", 1000, "window height")
	autoClick := flag.String("autoclick", "", "auto-click coords after startup (e.g. '80,370,44,609')")
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

	// Init font manager (required for Skia font metrics)
	fontDir := locateFontDir()
	graphics.InitFontManager(fontDir)
	if mgr := graphics.GetFontManager(); mgr != nil {
		mgr.LoadSystemFonts()
	}

	// Hook Skia text measurement to layout pipeline so the IFC uses real
	// glyph widths rather than monospace estimation.
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	wv := webkit.NewWebView()

	// Setup loaders for local file resolution
	absPath, _ := filepath.Abs(*htmlFile)
	dir := filepath.Dir(absPath)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.ScriptLoader = func(src string) (string, error) {
				clean := src
				if len(clean) > 0 && clean[0] == '/' {
					// absolute
				} else {
					clean = filepath.Join(dir, clean)
				}
				d, err := os.ReadFile(clean)
				return string(d), err
			}
			fr.StyleSheetLoader = func(href string) (string, error) {
				clean := href
				if len(clean) > 0 && clean[0] == '/' {
					// absolute
				} else {
					clean = filepath.Join(dir, clean)
				}
				d, err := os.ReadFile(clean)
				return string(d), err
			}
		}
	}

	err = wv.LoadHTML(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: LoadHTML: %v\n", err)
		os.Exit(1)
	}

	// Set viewport to match the requested window size BEFORE first layout,
	// otherwise the FrameView defaults to 1280x800 (set in NewFrame).
	wv.Resize(*width, *height)
	wv.EnsureLayout()

	// Dump render tree after layout
	if rv := wv.RenderView(); rv != nil {
		fmt.Println("=== RENDER TREE ===")
		dumpRO(rv, 0)
	}

	host, err := app.NewHost(wv, *width, *height, "wb-ui: "+*htmlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: window: %v\n", err)
		os.Exit(1)
	}

	f, _ := os.Create("resize_dump_log.txt")
	if f != nil {
		defer f.Close()
		if rv2 := wv.RenderView(); rv2 != nil {
			fmt.Fprintf(f, "=== RENDER TREE (start) ===\n")
			dumpROTo(f, rv2, 0)
		}
	}

	fmt.Println("Window opened. Close the window to exit.\n\n=== RESIZE DUMP ACTIVE ===\nMaximize the window to trigger re-layout dump.")
	app.DumpRTCallback = func(rv *rendering.RenderView) {
		if f != nil {
			fmt.Fprintf(f, "\n=== RENDER TREE (after resize) viewport=%.0fx%.0f ===\n",
				rv.ViewWidth(), rv.ViewHeight())
			dumpROTo(f, rv, 0)
			f.Sync()
		}
	}
	// Parse auto-click coordinates.
	if coords := parseAutoClick(*autoClick); len(coords) > 0 {
		go injectAutoClicks(host, coords)
	}
	host.Run()
	fmt.Println("Done.")
}

// injectAutoClicks posts synthetic mouse click events to the host's window
// at the given (x,y) coordinates after a short delay to allow the window and
// render loop to start.
func injectAutoClicks(h *app.Host, coords []struct{ x, y int }) {
	if h == nil || len(coords) == 0 {
		return
	}
	time.Sleep(500 * time.Millisecond)
	win := h.Window()
	if win == nil {
		return
	}
	for _, c := range coords {
		fmt.Printf("[autoclick] clicking at (%d,%d)\n", c.x, c.y)
		win.PostEvent(window.Event{
			Type:   window.EventMouseButton,
			X:      float64(c.x),
			Y:      float64(c.y),
			Button: 0, // left button
			Action: 1, // glfw.Press
		})
		time.Sleep(100 * time.Millisecond)
		win.PostEvent(window.Event{
			Type:   window.EventMouseButton,
			X:      float64(c.x),
			Y:      float64(c.y),
			Button: 0,
			Action: 0, // glfw.Release
		})
		time.Sleep(500 * time.Millisecond)
	}
}

func parseAutoClick(s string) []struct{ x, y int } {
	if s == "" {
		return nil
	}
	var result []struct{ x, y int }
	parts := strings.Split(s, ",")
	for i := 0; i+1 < len(parts); i += 2 {
		x, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		y, _ := strconv.Atoi(strings.TrimSpace(parts[i+1]))
		if x > 0 && y > 0 {
			result = append(result, struct{ x, y int }{x, y})
		}
	}
	return result
}

func dumpRO(ro rendering.RenderObject, depth int) {
	dumpROTo(nil, ro, depth)
}

func dumpROTo(wrt io.Writer, ro rendering.RenderObject, depth int) {
	if wrt == nil {
		wrt = os.Stdout
	}
	if ro == nil {
		return
	}
	pad := ""
	for i := 0; i < depth; i++ {
		pad += "  "
	}
	name := ro.RenderName()
	var xx, yy, ww, hh float64
	if lb := ro.LayoutBox(); lb != nil {
		if rv := ro.View(); rv != nil {
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(lb)
				xx, yy, ww, hh = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			}
		}
	}
	bg := ""
	if cs := ro.Style(); cs != nil {
		bg = fmt.Sprintf(" bg=#%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
	}
	fmt.Fprintf(wrt, "%s%s (%.0f,%.0f) %.0fx%.0f%s\n", pad, name, xx, yy, ww, hh, bg)
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpROTo(wrt, c, depth+1)
	}
}

func locateFontDir() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "resources", "fonts"),
			filepath.Join(dir, "..", "..", "resources", "fonts"),
		)
	}
	candidates = append(candidates,
		filepath.Join("resources", "fonts"),
		filepath.Join("..", "..", "resources", "fonts"),
		`f:\syproject\wb-ui\resources\fonts`,
	)
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return filepath.Join("resources", "fonts")
}
