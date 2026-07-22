// Command pipeline-diag parses [PIPELINE:Stage] logs from wb-ui verbose mode
// and detects anomalies in the render pipeline.
//
// Usage:
//   set WB_VERBOSE=1 && desktop.exe 2>desktop.log
//   pipeline-diag desktop.log
//
// Or pipe:
//   type desktop.log | pipeline-diag
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type entry struct {
	stage   string
	content string
	line    int
}

type analysis struct {
	entries []entry

	// Parsed metrics
	firstRenderObjectCount int
	secondRenderObjectCount int
	viewportWidth          int
	viewportHeight         int
	styleElementCount      int
	externalCSSSize        int
	externalCSSFiles       int
	layoutCount            int
	contentWidth           int
	contentHeight          int
	styleSheetsLoaded      int

	// Anomalies
	anomalies []string
}

func main() {
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		analyze(f)
	} else {
		analyze(os.Stdin)
	}
}

func analyze(r *os.File) {
	a := &analysis{}
	scanner := bufio.NewScanner(r)
	lineNum := 0

	pipeRe := regexp.MustCompile(`^\[PIPELINE:(\w+)\]\s+(.*)$`)

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		m := pipeRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		e := entry{
			stage:   m[1],
			content: m[2],
			line:    lineNum,
		}
		a.entries = append(a.entries, e)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}

	if len(a.entries) == 0 {
		fmt.Println("⚠️  No [PIPELINE:...] entries found. Did you set WB_VERBOSE=1?")
		os.Exit(0)
	}

	a.parse()
	a.report()
}

func (a *analysis) parse() {
	for _, e := range a.entries {
		switch e.stage {
		case "LoadHTML":
			if strings.HasPrefix(e.content, "input_size=") {
				// ignore size
			}
			if strings.HasPrefix(e.content, "parsed tagCount=") {
				// element stats already logged
			}

		case "SetDocument":
			if strings.Contains(e.content, "renderObjectCount=") {
				parts := strings.SplitN(e.content, "=", 2)
				v := parseInt(parts[1])
				if a.firstRenderObjectCount == 0 {
					a.firstRenderObjectCount = v
				} else {
					a.secondRenderObjectCount = v
				}
			}

		case "RebuildRenderTree":
			if strings.Contains(e.content, "renderObjectCount=") {
				parts := strings.SplitN(e.content, "=", 2)
				v := parseInt(parts[1])
				if a.firstRenderObjectCount == 0 {
					a.firstRenderObjectCount = v
				} else if a.secondRenderObjectCount == 0 {
					a.secondRenderObjectCount = v
				} else {
					// multiple rebuilds
					a.secondRenderObjectCount = v
				}
			}

		case "extractAndAddStyles":
			if strings.HasPrefix(e.content, "styleElementCount=") {
				a.styleElementCount = parseIntAfter(e.content, "=")
			}
			if strings.Contains(e.content, "sync loaded len=") {
				parts := strings.SplitN(e.content, "len=", 2)
				if len(parts) == 2 {
					a.externalCSSSize += parseInt(parts[1])
					a.externalCSSFiles++
				}
			}
			if strings.HasPrefix(e.content, "done: totalStyleSheets=") {
				parts := strings.SplitN(e.content, "=", 2)
				if len(parts) == 2 {
					sheetParts := strings.SplitN(parts[1], " ", 2)
					a.styleSheetsLoaded = parseInt(sheetParts[0])
				}
			}

		case "Layout":
			if strings.HasPrefix(e.content, "start viewport=") {
				// Parse viewport: "1280x800 needsLayout=true"
				parts := strings.Split(e.content, " ")
				vpParts := strings.SplitN(strings.TrimPrefix(parts[0], "viewport="), "x", 2)
				if len(vpParts) == 2 {
					a.viewportWidth = parseInt(vpParts[0])
					a.viewportHeight = parseInt(vpParts[1])
				}
				a.layoutCount++
			}
			if strings.HasPrefix(e.content, "done contentSize=") {
				cs := strings.TrimPrefix(e.content, "done contentSize=")
				csParts := strings.SplitN(cs, "x", 2)
				if len(csParts) == 2 {
					a.contentWidth = parseInt(csParts[0])
					a.contentHeight = parseInt(csParts[1])
				}
			}
		}
	}

	// Detect anomalies
	a.detectAnomalies()
}

func (a *analysis) detectAnomalies() {
	// 1. First build should have few objects (just HTML parse)
	if a.firstRenderObjectCount > 10 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("⚠️  First render tree build has %d objects (>10 expected). CSS may apply before JS runs.",
				a.firstRenderObjectCount))
	} else if a.firstRenderObjectCount == 0 {
		a.anomalies = append(a.anomalies, "⚠️  No first render tree build detected.")
	}

	// 2. Second build should have many objects (after Vue renders)
	if a.secondRenderObjectCount > 0 && a.secondRenderObjectCount < 50 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("⚠️  Second render tree has only %d objects. Vue may not have been mounted.",
				a.secondRenderObjectCount))
	}

	// 3. External CSS should be loaded
	if a.externalCSSFiles == 0 {
		a.anomalies = append(a.anomalies, "⚠️  No external CSS files loaded. <link> elements may not work.")
	}
	if a.externalCSSSize == 0 {
		a.anomalies = append(a.anomalies, "⚠️  External CSS size is 0. Stylesheets may not be loaded.")
	}

	// 4. Layout should run
	if a.layoutCount == 0 {
		a.anomalies = append(a.anomalies, "⚠️  No Layout pass detected. FrameView.Layout may not be called.")
	}

	// 5. Content size should match viewport
	if a.contentWidth > a.viewportWidth && a.viewportWidth > 0 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("⚠️  Content width %d exceeds viewport %d. Overflow not scrollable.",
				a.contentWidth, a.viewportWidth))
	}
	if a.contentHeight == 0 && a.viewportHeight > 0 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("⚠️  Content height is 0. Nothing may be visible (viewport=%d).",
				a.viewportHeight))
	}

	// 6. Style sheets
	if a.styleSheetsLoaded < 2 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("⚠️  Only %d style sheets loaded (expected ≥2: UA + author). Author CSS may be missing.",
				a.styleSheetsLoaded))
	}
}

func (a *analysis) report() {
	fmt.Println("═══════════════════════════════════════")
	fmt.Println("  wb-ui Pipeline Diagnostics Report")
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  Entries parsed: %d\n", len(a.entries))
	fmt.Println()

	fmt.Println("─── Pipeline Metrics ─────────────────")
	fmt.Printf("  First render tree objects:  %d\n", a.firstRenderObjectCount)
	fmt.Printf("  Second render tree objects: %d\n", a.secondRenderObjectCount)
	fmt.Printf("  Viewport:                   %dx%d\n", a.viewportWidth, a.viewportHeight)
	fmt.Printf("  Content size:               %dx%d\n", a.contentWidth, a.contentHeight)
	fmt.Printf("  Layout passes:              %d\n", a.layoutCount)
	fmt.Printf("  <style> elements:           %d\n", a.styleElementCount)
	fmt.Printf("  External CSS files:         %d (%d bytes)\n", a.externalCSSFiles, a.externalCSSSize)
	fmt.Printf("  Style sheets loaded:        %d\n", a.styleSheetsLoaded)
	fmt.Println()

	fmt.Println("─── Stage Timeline ───────────────────")
	for _, e := range a.entries {
		fmt.Printf("  [L%4d] %-30s %s\n", e.line, "["+e.stage+"]", e.content)
	}
	fmt.Println()

	if len(a.anomalies) > 0 {
		fmt.Println("─── Anomalies ─────────────────────────")
		for _, an := range a.anomalies {
			fmt.Println("  " + an)
		}
		fmt.Println()
	} else {
		fmt.Println("─── No anomalies detected ✅ ──────────")
		fmt.Println()
	}
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}

func parseIntAfter(s, sep string) int {
	parts := strings.SplitN(s, sep, 2)
	if len(parts) != 2 {
		return 0
	}
	return parseInt(parts[1])
}
