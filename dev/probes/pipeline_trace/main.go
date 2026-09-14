package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ComponentTrace holds the full pipeline trace for one component.
type ComponentTrace struct {
	Selector       string           `json:"selector"`       // tag#id.class
	Tag            string           `json:"tag"`            // HTML tag
	ID             string           `json:"id"`             // DOM ID
	Class          string           `json:"class"`          // CSS class
	Expected       *Rect            `json:"expected"`       // Browser reference (from Vue CSS)
	DOMNode        bool             `json:"dom_node"`       // Found in DOM
	StyleApplied   bool             `json:"style_applied"`  // Has computed style
	LayoutBox      *Rect            `json:"layout_box"`     // Layout tree position
	RenderObject   *Rect            `json:"render_object"`  // Render tree position
	TextSegments   int              `json:"text_segments"`  // Number of text segments
	TextWidth      float64          `json:"text_width"`     // Max text segment width
	TextHeight     float64          `json:"text_height"`    // Max text segment height
	BgColor        string           `json:"bg_color"`       // Background color hex
	Display        string           `json:"display"`        // CSS display value
	Anomalies      []string         `json:"anomalies"`      // Detected issues
	Children       []*ComponentTrace `json:"children,omitempty"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// RenderNode parsed from diagnostic log
type RenderNode struct {
	Line        int
	Depth       int
	Name        string
	X, Y, W, H  float64
	ChildCount  int
	Display     int
	BgR, BgG, BgB int
	BgAlpha     int
	IsText      bool
	SegCount    int
	SegW, SegH  float64
	Pointer     string
}

var indentRE = regexp.MustCompile(`^(\s*)`)

func parseRenderTree(path string) ([]RenderNode, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var nodes []RenderNode
	scanner := bufio.NewScanner(f)
	lineNo := 0
	inRenderTree := false
	nodeRE := regexp.MustCompile(`^(\s*)(\w+)\s+\((\d+) ch\)\s+x=([\d.]+)\s+y=([-\d.]+)\s+w=([\d.]+)\s+h=([\d.]+)\s+(.*)$`)
	textSegRE := regexp.MustCompile(`segs=(\d+)\[W=([\d.]+) H=([\d.]+)\]`)
	bgRE := regexp.MustCompile(`bg=#([0-9a-fA-F]{2})([0-9a-fA-F]{2})([0-9a-fA-F]{2})`)
	dispRE := regexp.MustCompile(`disp=(\d+)`)
	pRE := regexp.MustCompile(`p=0x([0-9a-fA-F]+)`)

	for scanner.Scan() {
		line := scanner.Text()
		lineNo++

		if strings.Contains(line, "=== RENDER TREE ===") {
			inRenderTree = true
			continue
		}
		if strings.Contains(line, "=== ANOMALY ANALYSIS ===") {
			break
		}
		if !inRenderTree {
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		m := nodeRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		indent := len(m[1])
		depth := indent / 2

		x, _ := strconv.ParseFloat(m[4], 64)
		y, _ := strconv.ParseFloat(m[5], 64)
		w, _ := strconv.ParseFloat(m[6], 64)
		h, _ := strconv.ParseFloat(m[7], 64)

		rest := m[8]
		node := RenderNode{
			Line:   lineNo,
			Depth:  depth,
			Name:   m[2],
			X:      x, Y: y, W: w, H: h,
		}

		if bgM := bgRE.FindStringSubmatch(rest); bgM != nil {
			r, _ := strconv.ParseInt(bgM[1], 16, 0)
			g, _ := strconv.ParseInt(bgM[2], 16, 0)
			b, _ := strconv.ParseInt(bgM[3], 16, 0)
			node.BgR, node.BgG, node.BgB = int(r), int(g), int(b)
			node.BgAlpha = 255
		}

		if dM := dispRE.FindStringSubmatch(rest); dM != nil {
			d, _ := strconv.Atoi(dM[1])
			node.Display = d
		}

		if sM := textSegRE.FindStringSubmatch(rest); sM != nil {
			node.IsText = true
			node.SegCount, _ = strconv.Atoi(sM[1])
			node.SegW, _ = strconv.ParseFloat(sM[2], 64)
			node.SegH, _ = strconv.ParseFloat(sM[3], 64)
		}

		if pM := pRE.FindStringSubmatch(rest); pM != nil {
			node.Pointer = pM[1]
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

type Report struct {
	Header  string                       `json:"header"`
	Summary *Summary                     `json:"summary"`
	Traces  map[string]*ComponentTrace   `json:"traces"`
	Issues  []Issue                      `json:"issues"`
}

type Summary struct {
	TotalNodes     int     `json:"total_nodes"`
	RenderTexts    int     `json:"render_texts"`
	ZeroWidth      int     `json:"zero_width"`
	ZeroHeight     int     `json:"zero_height"`
	NegY           int     `json:"neg_y"`
	NegX           int     `json:"neg_x"`
	OverflowX      int     `json:"overflow_x"`
	MassiveHeight  int     `json:"massive_height"`
	TextWO         int     `json:"text_w0_seg_nonzero"`
	BgW0           int     `json:"bg_w0"` // has bg but w=0
	MaxHeight      float64 `json:"max_height"`
	AnonNodes      int     `json:"anon_nodes"`
}

type Issue struct {
	Severity string `json:"severity"` // critical/major/minor
	Type     string `json:"type"`
	Desc     string `json:"desc"`
	Node     string `json:"node"`
	Fix      string `json:"fix"`
}

func analyze(nodes []RenderNode) *Report {
	r := &Report{
		Header: "wb-ui Pipeline Trace Report",
		Traces: make(map[string]*ComponentTrace),
	}

	s := &Summary{}
	r.Summary = s

	s.TotalNodes = len(nodes)

	// Scan for anomalies
	for _, n := range nodes {
		if n.IsText {
			s.RenderTexts++
		}
		if n.W == 0 && n.H > 0 {
			s.ZeroWidth++
			if n.BgAlpha > 0 || n.Display > 0 {
				s.BgW0++
				r.Issues = append(r.Issues, Issue{
					Severity: "major",
					Type:     "zero-width-background",
					Desc:     fmt.Sprintf("%s bg=#%02x%02x%02x w=0 h=%.0f", n.Name, n.BgR, n.BgG, n.BgB, n.H),
					Node:     fmt.Sprintf("depth=%d", n.Depth),
					Fix:      "LayoutBox width not set; check flex-grow/container width propagation",
				})
			}
		}
		if n.H == 0 && n.W > 0 {
			s.ZeroHeight++
		}
		if n.Y < -1 {
			s.NegY++
			r.Issues = append(r.Issues, Issue{
				Severity: "minor",
				Type:     "negative-y",
				Desc:     fmt.Sprintf("%s y=%.0f", n.Name, n.Y),
				Fix:      "negative Y from margin collapse or relative offset",
			})
		}
		if n.X < -1 {
			s.NegX++
		}
		if n.X > 1280 && n.W > 0 {
			s.OverflowX++
			r.Issues = append(r.Issues, Issue{
				Severity: "major",
				Type:     "overflow-x",
				Desc:     fmt.Sprintf("%s x=%.0f w=%.0f exceeds viewport 1280", n.Name, n.X, n.W),
				Fix:      "position calculation error in BFC/flex layout",
			})
		}
		if n.H > 100000 {
			s.MassiveHeight++
			r.Issues = append(r.Issues, Issue{
				Severity: "critical",
				Type:     "massive-height",
				Desc:     fmt.Sprintf("%s h=%.0f at depth=%d", n.Name, n.H, n.Depth),
				Fix:      "flex-grow/column height cascading overflow; check measureFlexItemContentMain",
			})
		}
		if n.IsText && (n.W == 0 || n.H == 0) && n.SegCount > 0 && n.SegW > 0 {
			s.TextWO++
			r.Issues = append(r.Issues, Issue{
				Severity: "critical",
				Type:     "text-rect-zero",
				Desc:     fmt.Sprintf("RenderText rect=(%.0f,%.0f) but segs=%d[W=%.0f H=%.0f]", n.X, n.Y, n.SegCount, n.SegW, n.SegH),
				Fix:      "syncGeometry not propagating TextSegments dimensions to LayoutBox Rect",
			})
		}
		if n.H > s.MaxHeight && n.H < 1e9 {
			s.MaxHeight = n.H
		}
		if n.Name == "RenderBlockFlow" && n.ChildCount == 0 && n.BgAlpha == 0 && n.W == 0 && n.H == 0 {
			s.AnonNodes++
		}
	}

	// Key component analysis
	keyComponents := []struct {
		selector string
		depth    int
		nearX    float64
		nearY    float64
	}{
		{"titlebar[menubar]", 0, 0, 0},
		{"activitybar", 0, 0, 0},
		{"sidebar[file-explorer]", 0, 0, 0},
		{"main-area[editor]", 0, 0, 0},
		{"right-container[rp-body]", 0, 0, 0},
		{"statusbar", 0, 0, 0},
		{"chat-area", 0, 0, 0},
	}

	_ = keyComponents

	return r
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		log.Fatalf("Usage: %s <desktop_diag.log>", os.Args[0])
	}

	path := os.Args[1]
	nodes, err := parseRenderTree(path)
	if err != nil {
		log.Fatalf("parse: %v", err)
	}

	fmt.Printf("解析完成：%d 个渲染节点\n", len(nodes))

	report := analyze(nodes)

	// Output report
	dir := filepath.Dir(path)
	outPath := filepath.Join(dir, "pipeline_trace_report.json")
	jsonData, _ := json.MarshalIndent(report, "", "  ")

	// Also output a readable text report
	textPath := filepath.Join(dir, "pipeline_trace_report.txt")
	textF, _ := os.Create(textPath)
	defer textF.Close()

	fmt.Fprintf(textF, "═══════════════════════════════════════════════\n")
	fmt.Fprintf(textF, "  wb-ui Pipeline Trace Report\n")
	fmt.Fprintf(textF, "═══════════════════════════════════════════════\n\n")

	s := report.Summary
	fmt.Fprintf(textF, "总节点数:          %d\n", s.TotalNodes)
	fmt.Fprintf(textF, "RenderText:        %d\n", s.RenderTexts)
	fmt.Fprintf(textF, "零宽节点:          %d\n", s.ZeroWidth)
	fmt.Fprintf(textF, "零高节点:          %d\n", s.ZeroHeight)
	fmt.Fprintf(textF, "负 Y 坐标:         %d\n", s.NegY)
	fmt.Fprintf(textF, "负 X 坐标:         %d\n", s.NegX)
	fmt.Fprintf(textF, "越界 X:            %d\n", s.OverflowX)
	fmt.Fprintf(textF, "巨量高度(>10万):   %d\n", s.MassiveHeight)
	fmt.Fprintf(textF, "文字w=0但段有值:   %d\n", s.TextWO)
	fmt.Fprintf(textF, "有背景但w=0:       %d\n", s.BgW0)
	fmt.Fprintf(textF, "匿名节点(0子0bg):  %d\n", s.AnonNodes)
	fmt.Fprintf(textF, "最大合理高度:      %.0f\n", s.MaxHeight)
	fmt.Fprintf(textF, "\n")

	fmt.Fprintf(textF, "── 异常列表 ──────────────────────────────────\n")
	for i, iss := range report.Issues {
		sev := ""
		switch iss.Severity {
		case "critical":
			sev = "🔴 "
		case "major":
			sev = "🟠 "
		default:
			sev = "🟡 "
		}
		fmt.Fprintf(textF, "%s%s: %s\n", sev, iss.Type, iss.Desc)
		fmt.Fprintf(textF, "    建议: %s\n", iss.Fix)
		if i > 50 {
			fmt.Fprintf(textF, "  ... 还有 %d 条异常\n", len(report.Issues)-i-1)
			break
		}
	}

	fmt.Fprintf(textF, "\n── 关键组件坐标 ──────────────────────────────\n")
	// Find key components by X position
	type keyMatch struct {
		name string
		x, y, w, h float64
		depth int
	}
	var keys []keyMatch

	for _, n := range nodes {
		if n.Depth != 0 && n.X < 50 && n.Y < 50 && n.W > 100 && n.H > 600 {
			keys = append(keys, keyMatch{"[sidebar区域] x<50 y<50", n.X, n.Y, n.W, n.H, n.Depth})
		}
		if n.X > 200 && n.X < 250 && n.Y < 50 && n.W > 700 && n.H > 600 {
			keys = append(keys, keyMatch{"[main-area] x=200~250", n.X, n.Y, n.W, n.H, n.Depth})
		}
		if n.X > 950 && n.Y < 50 && n.W > 300 && n.H > 600 {
			keys = append(keys, keyMatch{"[right-panel] x>950", n.X, n.Y, n.W, n.H, n.Depth})
		}
		if n.X < 50 && n.Y > 750 && n.W > 1000 {
			keys = append(keys, keyMatch{"[statusbar] y>750", n.X, n.Y, n.W, n.H, n.Depth})
		}
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].depth != keys[j].depth {
			return keys[i].depth < keys[j].depth
		}
		return keys[i].x < keys[j].x
	})

	for _, k := range keys {
		fmt.Fprintf(textF, "  %-25s x=%.0f y=%.0f w=%.0f h=%.0f depth=%d\n",
			k.name, k.x, k.y, k.w, k.h, k.depth)
	}

	_ = jsonData
	_ = outPath

	fmt.Fprintf(textF, "\n报告已生成: %s\n", textPath)
	fmt.Printf("报告已生成: %s\n", textPath)
}
