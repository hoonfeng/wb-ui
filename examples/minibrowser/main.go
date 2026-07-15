// Command minibrowser is a minimal end-to-end demo of the wb-ui browser engine.
//
// It mirrors the role of WebKit's MiniBrowser helper app: it creates a WebView,
// loads an HTML document, runs the layout + paint pipeline, and surfaces the
// resulting pixel buffer. Because wb-ui has no windowing system, the "window" is
// the terminal: the program prints the viewport dimensions, document title and
// pixel-buffer size. An optional -o flag writes the rendered pixels to a PPM
// (Portable PixMap) image so the result can be inspected with any image viewer.
//
// Usage:
//
//	go run ./examples/minibrowser                       # use built-in sample page
//	go run ./examples/minibrowser -html page.html        # load a file from disk
//	go run ./examples/minibrowser -o out.ppm             # save rendered pixels
//	go run ./examples/minibrowser -w 1024 -h 768         # custom viewport
//
// The demo exercises the full pipeline translated across Phase 0-12:
// html.Parse -> dom -> style.Resolve -> layout -> rendering.Paint -> RGBA pixels.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"wb-ui/webkit"
)

// defaultHTML is the sample page used when no -html flag is supplied. It contains
// the heading / paragraph / list structure the task asks the minibrowser to render.
const defaultHTML = `<!DOCTYPE html>
<html>
  <head>
    <title>wb-ui MiniBrowser Sample</title>
    <style>
      body { background-color: rgb(245, 245, 235); margin: 16px; }
      h1 { color: rgb(20, 60, 160); }
      p  { color: rgb(40, 40, 40); }
      ul { color: rgb(60, 100, 60); }
    </style>
  </head>
  <body>
    <h1>Hello from wb-ui</h1>
    <p>This page was parsed, laid out and painted entirely in Go.</p>
    <ul>
      <li>HTML parsing via wb-ui/html</li>
      <li>Style resolution via wb-ui/style</li>
      <li>Block layout via wb-ui/layout</li>
      <li>Software raster via wb-ui/rendering</li>
    </ul>
  </body>
</html>`

func main() {
	// Flags mirror the most useful MiniBrowser command-line switches.
	htmlPath := flag.String("html", "", "path to an HTML file to load (default: built-in sample page)")
	outPath := flag.String("o", "", "write rendered pixels to this PPM (P6) file")
	width := flag.Int("w", webkit.DefaultWebViewWidth, "viewport width in CSS pixels")
	height := flag.Int("h", webkit.DefaultWebViewHeight, "viewport height in CSS pixels")
	flag.Parse()

	// Step 1: create a WebView with default settings and an 800x600 viewport,
	// mirroring WebView::create on Windows.
	wv := webkit.NewWebView()
	wv.Resize(*width, *height)

	// Step 2: load HTML. Either read a file supplied on the command line or use
	// the built-in sample page. We read the file into memory and call LoadHTML
	// (rather than LoadURL("file://...")) to keep path handling cross-platform.
	src := defaultHTML
	if *htmlPath != "" {
		b, err := os.ReadFile(*htmlPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "minibrowser: read %s: %v\n", *htmlPath, err)
			os.Exit(1)
		}
		src = string(b)
	}
	if err := wv.LoadHTML(src); err != nil {
		fmt.Fprintf(os.Stderr, "minibrowser: LoadHTML failed: %v\n", err)
		os.Exit(1)
	}

	// Step 3: run the layout + paint pipeline into a fresh RGBA canvas, mirroring
	// the WebView::onPaintEvent -> DrawingAreaProxy::paint -> drawRect path.
	pixels, err := wv.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "minibrowser: Render failed: %v\n", err)
		os.Exit(1)
	}

	// Step 4: report the result to the terminal (the "window" for this port).
	doc := wv.MainFrame().Document()
	title := ""
	if doc != nil {
		title = doc.Title()
	}
	fmt.Println("=== wb-ui MiniBrowser ===")
	fmt.Printf("Viewport:    %d x %d CSS pixels\n", wv.Width(), wv.Height())
	fmt.Printf("Pixel buffer: %d bytes (%d x %d x 4 RGBA)\n", len(pixels), wv.Width(), wv.Height())
	fmt.Printf("Document:    title=%q\n", title)

	// A quick pixel sanity check: sample the top-left pixel so the user can see
	// the paint actually produced colour data rather than a zero buffer.
	if len(pixels) >= 4 {
		r, g, b, a := pixels[0], pixels[1], pixels[2], pixels[3]
		fmt.Printf("Top-left pixel: rgba(%d, %d, %d, %d)\n", r, g, b, a)
	}

	// Step 5 (optional): persist the pixels to a PPM P6 file. PPM is chosen
	// because it needs no external library: the header is plain ASCII and the
	// body is raw RGB bytes. We drop the alpha channel (PPM has none).
	if *outPath != "" {
		if err := writePPM(*outPath, wv.Width(), wv.Height(), pixels); err != nil {
			fmt.Fprintf(os.Stderr, "minibrowser: write %s: %v\n", *outPath, err)
			os.Exit(1)
		}
		fmt.Printf("Saved rendered image to %s\n", *outPath)
	}
}

// writePPM writes an RGBA pixel buffer to a PPM (P6) file. The P6 format is:
//
//	"P6\n<width> <height>\n255\n" followed by raw RGB bytes (alpha dropped).
//
// It is the simplest portable image format that requires no third-party code.
func writePPM(path string, width, height int, rgba []byte) error {
	var b strings.Builder
	fmt.Fprintf(&b, "P6\n%d %d\n255\n", width, height)
	header := []byte(b.String())

	// Convert RGBA -> RGB by keeping every 4th byte triple and skipping alpha.
	rgb := make([]byte, width*height*3)
	for i := 0; i < width*height; i++ {
		rgb[i*3+0] = rgba[i*4+0]
		rgb[i*3+1] = rgba[i*4+1]
		rgb[i*3+2] = rgba[i*4+2]
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(header); err != nil {
		return err
	}
	if _, err := f.Write(rgb); err != nil {
		return err
	}
	return nil
}
