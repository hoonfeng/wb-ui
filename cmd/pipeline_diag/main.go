// Command pipeline-diag parses [PIPELINE:Stage] logs from wb-ui verbose mode
// and detects anomalies in the render pipeline.
//
// Usage:
//   set WB_VERBOSE=1 && desktop.exe 2>desktop.log
//   pipeline-diag desktop.log
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

type scriptInfo struct {
	src  string
	size int
}

type analysis struct {
	entries []entry

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

	scriptTotal     int
	scriptOK        int
	scriptFail      int
	scripts         []scriptInfo
	totalScriptSize int

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
		fmt.Println("!! No [PIPELINE:...] entries found. Did you set WB_VERBOSE=1?")
		os.Exit(0)
	}

	a.parse()
	a.report()
}

func (a *analysis) parse() {
	for _, e := range a.entries {
		switch e.stage {
		case "LoadHTML":
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

		case "ScriptLoad":
			if strings.HasPrefix(e.content, "start: scripts=") {
				a.scriptTotal = parseIntAfter(e.content, "scripts=")
			}
			if strings.HasSuffix(e.content, " OK") {
				a.scriptOK++
			}
			if strings.Contains(e.content, "FAIL") {
				a.scriptFail++
			}
			if strings.Contains(e.content, "exec:") && strings.Contains(e.content, "len=") {
				parts := strings.Split(e.content, "len=")
				if len(parts) == 2 {
					si := scriptInfo{size: parseInt(parts[1])}
					srcParts := strings.SplitN(e.content, "src=", 2)
					if len(srcParts) == 2 {
						si.src = strings.SplitN(srcParts[1], " ", 2)[0]
						si.src = strings.Trim(si.src, `"`)
					}
					a.scripts = append(a.scripts, si)
					a.totalScriptSize += si.size
				}
			}
		}
	}

	a.detectAnomalies()
}

func (a *analysis) detectAnomalies() {
	if a.firstRenderObjectCount > 10 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("First render tree objects=%d (expected <10)", a.firstRenderObjectCount))
	} else if a.firstRenderObjectCount == 0 {
		a.anomalies = append(a.anomalies, "No first render tree build")
	}

	if a.secondRenderObjectCount > 0 && a.secondRenderObjectCount < 50 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("Second render tree objects=%d (Vue may not have mounted)", a.secondRenderObjectCount))
	}

	if a.externalCSSFiles == 0 {
		a.anomalies = append(a.anomalies, "No external CSS files loaded")
	}

	if a.layoutCount == 0 {
		a.anomalies = append(a.anomalies, "No Layout pass detected")
	}

	if a.contentHeight == 0 && a.viewportHeight > 0 {
		a.anomalies = append(a.anomalies, "Content height is 0 -- nothing visible")
	}

	if a.styleSheetsLoaded < 2 {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("Style sheets=%d (expected >=2)", a.styleSheetsLoaded))
	}

	if a.scriptTotal > 0 && a.scriptOK < a.scriptTotal {
		a.anomalies = append(a.anomalies,
			fmt.Sprintf("%d/%d scripts OK, %d FAILED", a.scriptOK, a.scriptTotal, a.scriptFail))
	}

	if a.scriptTotal == 0 {
		a.anomalies = append(a.anomalies, "No scripts loaded")
	}
}

func (a *analysis) report() {
	fmt.Println("========================================")
	fmt.Println("  wb-ui Pipeline Diagnostics Report")
	fmt.Println("========================================")
	fmt.Printf("  Pipeline entries: %d\n", len(a.entries))
	fmt.Println()

	fmt.Println("  --- HTML / CSS ---")
	fmt.Printf("  First render tree objects:  %d\n", a.firstRenderObjectCount)
	fmt.Printf("  Second render tree objects: %d\n", a.secondRenderObjectCount)
	fmt.Printf("  Style sheets:               %d", a.styleSheetsLoaded)
	if a.styleSheetsLoaded >= 2 {
		fmt.Print(" (UA + author)")
	}
	fmt.Println()
	fmt.Printf("  Inline <style> elements:    %d\n", a.styleElementCount)
	fmt.Printf("  External CSS files:         %d (%d bytes)\n", a.externalCSSFiles, a.externalCSSSize)
	fmt.Println()

	fmt.Println("  --- Layout ---")
	fmt.Printf("  Layout passes: %d\n", a.layoutCount)
	fmt.Printf("  Viewport:      %dx%d\n", a.viewportWidth, a.viewportHeight)
	fmt.Printf("  Content size:  %dx%d\n", a.contentWidth, a.contentHeight)
	fmt.Println()

	fmt.Println("  --- Script Loading ---")
	if a.scriptTotal > 0 {
		fmt.Printf("  Scripts found:         %d\n", a.scriptTotal)
		fmt.Printf("  Successfully executed: %d\n", a.scriptOK)
		fmt.Printf("  Failed:                %d\n", a.scriptFail)
		fmt.Printf("  Total JS bytes:        %d\n", a.totalScriptSize)
		fmt.Println()
		for i, si := range a.scripts {
			hasFail := false
			for _, ec := range a.entriesContaining("ScriptLoad", si.src) {
				if strings.Contains(ec, "FAIL") {
					hasFail = true
				}
			}
			status := "[OK]"
			if hasFail {
				status = "[FAIL]"
			}
			fmt.Printf("  [%d] %-45s %10d bytes  %s\n", i, si.src, si.size, status)
		}
	} else {
		fmt.Println("  No scripts loaded")
	}
	fmt.Println()

	fmt.Println("  --- Stage Timeline ---")
	for _, e := range a.entries {
		fmt.Printf("  [L%4d] %-30s %s\n", e.line, "["+e.stage+"]", e.content)
	}
	fmt.Println()

	if len(a.anomalies) > 0 {
		fmt.Println("  --- Anomalies ---")
		for _, an := range a.anomalies {
			fmt.Println("  ** " + an)
		}
		fmt.Println()
	} else {
		fmt.Println("  --- No anomalies detected ---")
		fmt.Println()
	}
}

func (a *analysis) entriesContaining(stage, substr string) []string {
	var res []string
	for _, e := range a.entries {
		if e.stage == stage && strings.Contains(e.content, substr) {
			res = append(res, e.content)
		}
	}
	return res
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
