// Command consistency verifies wb-ui rendering consistency against a real
// browser (Edge/Chrome headless). For each test HTML it:
//
//	1. Loads the page in Edge headless and injects a JS collector that
//	   extracts every element's geometry (getBoundingClientRect), computed
//	   styles, animation frames and interaction results into document.title.
//	2. Renders the same HTML through the wb-ui full pipeline (parse → style →
//	   layout → render tree) and extracts the same per-element data.
//	3. Compares the two and writes a structured diff report to
//	   dev/suites/consistency/report/<name>.txt.
//
// Coverage: layout, styles, animations, form-control polymorphism, interaction.
//
// Usage:
//
//	go run ./dev/suites/consistency -case layout_basic
//	go run ./dev/suites/consistency            (run all cases)
//
// Requires Edge (or Chrome) at a configurable path; see EdgePath.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EdgePath is the Chromium binary used as the reference renderer.
// Override with -edge or the WBUI_EDGE env var.
var EdgePath = "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe"

// TempDir holds transient HTML files handed to Edge.
var TempDir = "C:\\Temp\\wbui_consistency"

// ReportDir receives the per-case comparison reports.
// 用 runtime.Caller 定位包目录，使报告始终落在 dev/suites/consistency/report
// （与 .gitignore 规则一致），不受调用者 CWD（go run 自仓库根 / go test 自包目录）影响。
var ReportDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "report")
}()

func main() {
	caseName := flag.String("case", "", "run a single case by name")
	edge := flag.String("edge", envOr("WBUI_EDGE", EdgePath), "Chromium binary path")
	flag.Parse()
	EdgePath = *edge

	if err := os.MkdirAll(TempDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir temp: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(ReportDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir report: %v\n", err)
		os.Exit(1)
	}

	cases := allCases()
	if *caseName != "" {
		found := false
		for _, c := range cases {
			if c.Name == *caseName {
				cases = []TestCase{c}
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "unknown case %q (available: %s)\n", *caseName, caseNames(cases))
			os.Exit(1)
		}
	}

	fmt.Printf("Reference browser: %s\n", EdgePath)
	fmt.Printf("Running %d consistency case(s)\n\n", len(cases))
	failures := 0
	start := time.Now()
	for _, c := range cases {
		fmt.Printf("── %-24s %s\n", c.Name, c.Desc)
		r := runCase(c)
		fmt.Printf("    %s\n", r.Summary())
		if !r.Passed() {
			failures++
		}
	}
	fmt.Printf("\n=== %d/%d passed in %s ===\n", len(cases)-failures, len(cases), time.Since(start).Round(time.Millisecond))
	if failures > 0 {
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func caseNames(cases []TestCase) string {
	var names []string
	for _, c := range cases {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}
