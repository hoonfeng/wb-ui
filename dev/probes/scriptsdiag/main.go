// Command scriptsdiag loads one fixture through wb-ui's WebView environment and
// reports (a) what its script printed, (b) which platform APIs the script can
// see, and (c) the fixture's own self-reported verdict.
//
// cssprobe tells you that a script-driven fixture did not reach its expected
// picture; this tool tells you *why*: which API is missing, and where the script
// stopped. Several fixtures only mark themselves as passed at the very end of
// the script, so a missing API earlier (e.g. document.currentScript) aborts the
// whole script and the page silently keeps its initial markup.
//
// Usage:
//
//	go run ./dev/probes/scriptsdiag -file dev/suites/cssprobe/fixtures/modern-streams.html
package main

import (
	"flag"
	"fmt"
	"os"

	"wb-ui/jsc"
	"wb-ui/webkit"
)

// apiProbe asks the page's own JS engine what the fixture script can reach. It
// only uses syntax that always parses, so a completely empty environment still
// produces a report instead of an exception.
const apiProbe = `JSON.stringify({
  fixtureState: {
    parserScriptRuns: typeof globalThis.__parserScriptRuns,
    hydrationContracts: typeof globalThis.__modernHydrationContracts,
    streamContracts: typeof globalThis.__modernStreamContracts,
    streamError: (typeof globalThis.__modernStreamError === "string") ? globalThis.__modernStreamError : null
  },
  dom: {
    createElement: typeof document.createElement,
    currentScript: typeof document.currentScript,
    getElementById: typeof document.getElementById,
    querySelector: typeof document.querySelector
  },
  elementAPIs: {
    hasAttributes: typeof document.createElement("div").hasAttributes,
    removeAttributeNode: typeof document.createElement("div").removeAttributeNode,
    attributes: typeof document.createElement("div").attributes,
    scrollTo: typeof document.createElement("div").scrollTo,
    scrollBy: typeof document.createElement("div").scrollBy,
    scrollLeft: typeof document.createElement("div").scrollLeft,
    scrollTop: typeof document.createElement("div").scrollTop
  },
  platform: {
    NamedNodeMap: typeof NamedNodeMap,
    EventTarget: typeof EventTarget,
    Event: typeof Event,
    ReadableStream: typeof ReadableStream,
    TransformStream: typeof TransformStream,
    TextEncoderStream: typeof TextEncoderStream,
    TextDecoder: typeof TextDecoder,
    CSSsupports: (typeof CSS === "object" && CSS) ? typeof CSS.supports : "no CSS object",
    visualViewport: typeof visualViewport,
    trackElement: typeof document.createElement("track").track,
    videoTextTracks: typeof document.createElement("video").textTracks
  }
})`

func main() {
	file := flag.String("file", "", "fixture HTML file to load")
	evalExpr := flag.String("eval", "", "JS expression to evaluate instead of the default API probe (may be multi-statement / IIFE)")
	w := flag.Int("w", 900, "viewport width")
	h := flag.Int("h", 1000, "viewport height")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "scriptsdiag: -file is required")
		os.Exit(2)
	}
	htmlText, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scriptsdiag: read fixture: %v\n", err)
		os.Exit(1)
	}

	wv := webkit.NewWebView()
	defer wv.Destroy()
	logger := &jsc.BufferLogger{}
	wv.SetConsoleLogger(logger)
	wv.Resize(*w, *h)
	if err := wv.LoadHTML(string(htmlText)); err != nil {
		fmt.Fprintf(os.Stderr, "scriptsdiag: LoadHTML: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("fixture %s @ %dx%d\n", *file, *w, *h)
	if s := logger.String(); s != "" {
		fmt.Println("--- console / script output ---")
		fmt.Println(s)
	}
	expr := apiProbe
	if *evalExpr != "" {
		expr = *evalExpr
	}
	v, err := wv.EvalJS(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scriptsdiag: probe failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("--- api probe ---")
	fmt.Println(v.ToString())
}
