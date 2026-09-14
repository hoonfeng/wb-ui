// Command mixed is a Go+HTML+CSS+JS interop demo for the wb-ui engine.
//
// It mirrors how a real browser loads a page that references external CSS and JS
// resources: the Go host reads index.html, style.css and app.js from disk,
// inlines the stylesheet into the HTML (wb-ui has no network layer, so sub-
// resources must be resolved by the embedder), loads the combined document into
// a WebView, then drives the JavaScript runtime with the DOM bindings and a set
// of Go functions exposed to JS as go.GetTime() / go.Calculate().
//
// The demo exercises both directions of the Go<->JS<->DOM bridge:
//   - JS -> Go: app.js calls go.GetTime() and go.Calculate(a, b) via the
//     functions registered with bindings.RegisterGoFunction.
//   - Go -> DOM: after the script runs, Go updates an element directly through
//     the DOM (dom.Document.GetElementById), proving the host can mutate the
//     page that JS sees.
//
// Run from the examples/mixed directory so the resource files are found:
//
//	go run ./examples/mixed
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

func main() {
	// --- Step 1: locate and read the page resources ---------------------------
	// The three files live alongside this source. Resolve them against a few
	// candidate directories so the demo works whether run via `go run` from the
	// package dir or as a built binary from elsewhere.
	resources := []string{"index.html", "style.css", "app.js"}
	contents := make(map[string]string, len(resources))
	for _, name := range resources {
		path, ok := resolveResource(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "mixed: cannot find resource %q (run from the examples/mixed directory)\n", name)
			os.Exit(1)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mixed: read %s: %v\n", path, err)
			os.Exit(1)
		}
		contents[name] = string(b)
	}

	// --- Step 2: inline the stylesheet and strip the external <script> --------
	// wb-ui's HTML parser does not fetch external resources, so the host acts as
	// the resource loader: the <link> tag is replaced with an inline <style>,
	// and the <script src> tag is removed (app.js is executed explicitly below).
	combinedHTML := contents["index.html"]
	combinedHTML = strings.Replace(combinedHTML,
		`<link rel="stylesheet" href="style.css">`,
		"<style>\n"+contents["style.css"]+"\n</style>", 1)
	combinedHTML = strings.Replace(combinedHTML,
		`<script src="app.js"></script>`, "", 1)

	// --- Step 3: create a WebView and load the combined document ---------------
	// LoadHTML drives html.Parse -> dom.Document -> style resolve -> render tree.
	wv := webkit.NewWebView()
	wv.Resize(640, 200)
	if err := wv.LoadHTML(combinedHTML); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: LoadHTML failed: %v\n", err)
		os.Exit(1)
	}
	doc := wv.MainFrame().Document()
	if doc == nil {
		fmt.Fprintln(os.Stderr, "mixed: no document after LoadHTML")
		os.Exit(1)
	}

	// --- Step 4: build the JS runtime with DOM bindings -----------------------
	// The WebView keeps its own interpreter for EvalJS, but to register host
	// functions before running the script we drive the bindings package directly
	// with a dedicated interpreter. This is the same layering WebKit uses: the
	// embedder (UIProcess) registers host objects on the scripting context.
	interp := jsc.NewInterpreter()
	logger := &jsc.BufferLogger{}
	interp.SetupGlobal(logger)
	bindings.RegisterDOMBindings(interp, doc)

	// --- Step 5: expose Go functions to JS as go.GetTime / go.Calculate -------
	// Each GoCallback receives JS arguments as []jsc.JSValue and returns a
	// JSValue. A non-nil error surfaces as undefined to JS (this port has no
	// throw path), so we return nil on success.
	bindings.RegisterGoFunction(interp, "GetTime", func(args []jsc.JSValue) (jsc.JSValue, error) {
		// No arguments; return the wall-clock time formatted as a string.
		return jsc.StringValue(time.Now().Format("2006-01-02 15:04:05")), nil
	})
	bindings.RegisterGoFunction(interp, "Calculate", func(args []jsc.JSValue) (jsc.JSValue, error) {
		// Expect two numeric arguments; multiply them on the Go side and return
		// the product. Missing args default to 0, mirroring JS's undefined -> 0.
		var a, b float64
		if len(args) > 0 {
			a = args[0].ToNumber()
		}
		if len(args) > 1 {
			b = args[1].ToNumber()
		}
		return jsc.NumberValue(a * b), nil
	})

	// --- Step 6: run the page script ------------------------------------------
	// app.js calls go.GetTime() and go.Calculate(6, 7) and writes the results
	// into #time-display and #calc-result via document.getElementById.
	if _, err := interp.Run(contents["app.js"]); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: app.js failed: %v\n", err)
		os.Exit(1)
	}

	// --- Step 7: report what JS produced --------------------------------------
	fmt.Println("=== wb-ui Mixed Go+HTML+CSS+JS Demo ===")
	fmt.Println("--- console output (JS -> Go calls) ---")
	out := strings.TrimSpace(logger.String())
	if out == "" {
		fmt.Println("(no console output)")
	} else {
		fmt.Println(out)
	}

	// Read back the DOM state that JS mutated, going through the bindings bridge
	// by evaluating a small read-only script. This shows Go observing the result
	// of JS execution through the same DOM the script used.
	fmt.Println("--- DOM state after JS (read through bindings) ---")
	if _, err := interp.Run(`console.log("time-display:", document.getElementById("time-display").textContent);`); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: read-back failed: %v\n", err)
	}
	if _, err := interp.Run(`console.log("calc-result:", document.getElementById("calc-result").textContent);`); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: read-back failed: %v\n", err)
	}
	fmt.Println(strings.TrimSpace(logger.String()))

	// --- Step 8: Go -> DOM mutation (the reverse direction) -------------------
	// The host updates #status directly through the Go DOM API. This is the
	// same object graph the JS bindings wrap, so a subsequent script would see
	// the change. We then confirm it by reading textContent back via JS.
	statusEl := doc.GetElementById("status")
	if statusEl != nil {
		_ = statusEl.SetTextContent("updated by Go at " + time.Now().Format("15:04:05"))
	}
	logger.Lines = nil // reset so the read-back below only shows fresh output
	if _, err := interp.Run(`console.log("status:", document.getElementById("status").textContent);`); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: Go->DOM read-back failed: %v\n", err)
	}
	fmt.Println("--- DOM state after Go mutation (Go -> DOM -> JS read-back) ---")
	fmt.Println(strings.TrimSpace(logger.String()))

	// --- Step 9: render the final page and save it ----------------------------
	// After all mutations, run the paint pipeline so the saved image reflects
	// every change from both JS and Go.
	pixels, err := wv.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mixed: Render failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("--- render result ---")
	fmt.Printf("Viewport %dx%d, pixel buffer %d bytes\n", wv.Width(), wv.Height(), len(pixels))
	ppmPath := "mixed-output.ppm"
	if err := writePPM(ppmPath, wv.Width(), wv.Height(), pixels); err != nil {
		fmt.Fprintf(os.Stderr, "mixed: write %s: %v\n", ppmPath, err)
		os.Exit(1)
	}
	fmt.Printf("Saved rendered image to %s\n", ppmPath)
}

// resolveResource searches a few candidate directories for name and returns the
// first match. Candidates: the current working directory, the directory of the
// running executable, and an "examples/mixed" subdirectory of the CWD (so the
// demo works whether run from the repo root or the package directory).
func resolveResource(name string) (string, bool) {
	candidates := []string{name, filepath.Join("examples", "mixed", name)}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "mixed", name))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

// writePPM writes an RGBA buffer to a PPM (P6) file: ASCII header followed by
// raw RGB bytes (alpha dropped). No external image library is required.
func writePPM(path string, width, height int, rgba []byte) error {
	var b strings.Builder
	fmt.Fprintf(&b, "P6\n%d %d\n255\n", width, height)
	header := []byte(b.String())
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
