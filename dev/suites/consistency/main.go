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
//	go run ./dev/suites/consistency -case layout_block
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
	"sort"
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
	dump := flag.Bool("dump", false, "print both sides' full element snapshots (debugging a failing field)")
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
		if *dump {
			dumpSnapshots(r)
		}
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

// dumpSnapshots prints both sides' element snapshots side by side (union of
// keys, sorted) so a failing field can be traced back to the exact values each
// renderer produced — the report only lists fields that DIFFER, which is not
// enough to tell e.g. "which earlier element's width shifted this X".
func dumpSnapshots(r CaseResult) {
	key := func(s ElementSnapshot) string {
		if s.ID != "" {
			return s.Tag + "#" + s.ID
		}
		return fmt.Sprintf("%s?%s", s.Tag, s.Class)
	}
	edge := map[string]ElementSnapshot{}
	for _, s := range r.Edge {
		edge[key(s)] = s
	}
	wb := map[string]ElementSnapshot{}
	for _, s := range r.WBUi {
		wb[key(s)] = s
	}
	keys := map[string]bool{}
	for k := range edge {
		keys[k] = true
	}
	for k := range wb {
		keys[k] = true
	}
	var list []string
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	fmt.Printf("    %-22s | %-30s | %s\n", "KEY", "EDGE  x,y wxh  bg", "WBUI  x,y wxh  bg")
	for _, k := range list {
		e, we := edge[k]
		w, ww := wb[k]
		es, ws := "(absent)", "(absent)"
		if we {
			es = fmt.Sprintf("%6.0f,%-4.0f %3.0fx%-3.0f %s", e.X, e.Y, e.W, e.H, e.BG)
		}
		if ww {
			ws = fmt.Sprintf("%6.0f,%-4.0f %3.0fx%-3.0f %s", w.X, w.Y, w.W, w.H, w.BG)
		}
		fmt.Printf("    %-22s | %-30s | %s\n", k, es, ws)
	}
}

func caseNames(cases []TestCase) string {
	var names []string
	for _, c := range cases {
		names = append(names, c.Name)
	}
	return strings.Join(names, ", ")
}
