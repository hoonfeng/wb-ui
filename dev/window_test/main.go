// Command window_test loads an HTML file in a wb-ui window for interactive testing.
// Usage: go run ./dev/window_test --html PATH
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"wb-ui/app"
	"wb-ui/rendering"
	"wb-ui/webkit"
)

func main() {
	htmlFile := flag.String("html", "", "HTML file path")
	width := flag.Int("w", 800, "window width")
	height := flag.Int("h", 1000, "window height")
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

	// Dump render tree
	if rv := wv.RenderView(); rv != nil {
		fmt.Println("=== RENDER TREE ===")
		dumpRO(rv, 0)
	}

	host, err := app.NewHost(wv, *width, *height, "wb-ui: "+*htmlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: window: %v\n", err)
		os.Exit(1)
	}

	wv.EnsureLayout()
	fmt.Println("Window opened. Close the window to exit.")
	host.Run()
	fmt.Println("Done.")
}

func dumpRO(ro rendering.RenderObject, depth int) {
	if ro == nil {
		return
	}
	pad := ""
	for i := 0; i < depth; i++ {
		pad += "  "
	}
	name := ro.RenderName()
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
		bg = fmt.Sprintf(" bg=#%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
	}
	fmt.Printf("%s%s (%.0f,%.0f) %.0fx%.0f%s\n", pad, name, x, y, w, h, bg)
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		dumpRO(c, depth+1)
	}
}
