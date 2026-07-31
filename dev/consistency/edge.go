// Edge headless reference collector. Launches the real browser on a temp
// HTML file with the injected collector script, reads document.title back
// through --dump-dom, and parses the "GEO:" payload into ElementSnapshots.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// edgeCollect runs the reference browser and returns per-element snapshots.
func edgeCollect(c TestCase) ([]ElementSnapshot, error) {
	// The HTML from baseDoc() already embeds the collector script (cases.go
	// baseDoc appends collectorSnippet before </body>). Write it as-is.
	fname := filepath.Join(TempDir, c.Name+".html")
	html := c.HTML
	if err := os.WriteFile(fname, []byte(html), 0644); err != nil {
		return nil, fmt.Errorf("write html: %w", err)
	}

	url := "file:///" + strings.ReplaceAll(fname, "\\", "/")
	// No --user-data-dir: Edge headless manages a temp profile itself. A
	// shared fixed profile dir accumulates SingletonLock files across runs
	// (crashed instances leave them), which makes subsequent launches hang.
	// Each --dump-dom run is one-shot, so the default temp profile is fine.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, EdgePath,
		"--headless", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
		"--window-size="+fmt.Sprintf("%d,%d", c.ViewportW, c.ViewportH),
		"--dump-dom", url)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("edge run: %v (stderr: %s)", err, strings.TrimSpace(stderr.String())[:min(300, len(strings.TrimSpace(stderr.String())))])
	}

	payload := extractGeo(stdout.String())
	if payload == "" {
		// Try reading from stderr too (some headless builds emit to stderr).
		payload = extractGeo(stderr.String())
	}
	if payload == "" {
		return nil, fmt.Errorf("no GEO payload in edge output (title not serialized); output len=%d", stdout.Len())
	}
	// Parse the VP:WxH; prefix to learn the actual viewport the browser used
	// (headless window-size is larger than innerWidth due to window chrome).
	actualW, actualH, geo := splitVP(payload)
	if actualW > 0 {
		edgeViewportW, edgeViewportH = actualW, actualH
	}
	return parseGeo(geo), nil
}

// edgeViewportW/H holds the actual browser inner viewport, used by wbuiCollect
// to render with the same width so geometry is directly comparable.
var edgeViewportW, edgeViewportH int

// splitVP separates "WxH;GEO:..." → (W, H, payload-after-GEO:).
func splitVP(p string) (int, int, string) {
	if !strings.HasPrefix(p, "VP:") {
		return 0, 0, p
	}
	rest := p[3:]
	idx := strings.Index(rest, ";")
	if idx < 0 {
		return 0, 0, rest
	}
	vp := rest[:idx]
	geo := rest[idx+1:]
	w, h := 0, 0
	if i := strings.Index(vp, "x"); i > 0 {
		w = atoi(vp[:i])
		h = atoi(vp[i+1:])
	}
	if strings.HasPrefix(geo, "GEO:") {
		geo = geo[4:]
	}
	return w, h, geo
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

// extractGeo finds the "VP:...GEO:" payload inside the dumped DOM and returns
// the full payload (both the viewport prefix and the element records).
// The payload is the document.title content; it ends at the closing </title>
// tag in the serialized DOM. TextContent within the payload may contain
// newlines, so scanning stops only at the HTML tag boundary.
func extractGeo(s string) string {
	idx := strings.Index(s, "VP:")
	if idx < 0 {
		idx = strings.Index(s, "GEO:")
	}
	if idx < 0 {
		return ""
	}
	// The title text ends at the first '<' that starts a tag after the payload.
	end := strings.Index(s[idx:], "</title>")
	if end >= 0 {
		return s[idx : idx+end]
	}
	// Fallback: scan until a '<' (start of any tag).
	end2 := strings.IndexByte(s[idx:], '<')
	if end2 < 0 {
		return s[idx:]
	}
	return s[idx : idx+end2]
}

// parseGeo converts "tag|id|class|x|y|w|h|display|color|bg|font|text|checked|value|count;..." into snapshots.
func parseGeo(payload string) []ElementSnapshot {
	var snaps []ElementSnapshot
	for _, rec := range strings.Split(payload, ";") {
		parts := strings.Split(rec, "|")
		if len(parts) < 14 {
			continue
		}
		s := ElementSnapshot{
			Tag:   parts[0],
			ID:    parts[1],
			Class: parts[2],
		}
		s.X = atof(parts[3])
		s.Y = atof(parts[4])
		s.W = atof(parts[5])
		s.H = atof(parts[6])
		s.Display = parts[7]
		s.Color = parts[8]
		s.BG = parts[9]
		s.FontSz = parts[10]
		s.TextContent = parts[11]
		s.Checked = parts[12]
		s.Value = parts[13]
		snaps = append(snaps, s)
	}
	return snaps
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
