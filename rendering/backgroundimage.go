// Background-image painting: url(...) images with background-size and
// background-position. Complements the gradient painting in painter.go.
// Mirrors Source/WebCore/rendering/BackgroundPainter.cpp's image path
// (BackgroundImageGeometry).

package rendering

import (
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"sync"
)

// parseBackgroundURL extracts the URL inside a url(...) token.
// Returns ("", false) when s is not a url(...) value.
func parseBackgroundURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "url(") {
		return "", false
	}
	inner := s[4:]
	if i := strings.LastIndex(inner, ")"); i >= 0 {
		inner = inner[:i]
	}
	inner = strings.TrimSpace(inner)
	// Strip optional quotes.
	if len(inner) >= 2 && (inner[0] == '"' || inner[0] == '\'') {
		inner = inner[1 : len(inner)-1]
	}
	return inner, true
}

// decodeDataURI decodes a data: URI (data:image/png;base64,XXXX) to bytes.
// Returns (data, true) for base64 data URIs; (nil, false) otherwise.
func decodeDataURI(uri string) ([]byte, bool) {
	low := strings.ToLower(uri)
	if !strings.HasPrefix(low, "data:") {
		return nil, false
	}
	if !strings.Contains(low, ";base64,") {
		return nil, false
	}
	_, raw, _ := strings.Cut(uri, ";base64,")
	raw = strings.TrimSpace(raw)
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	return data, true
}

// backgroundImageCache caches decoded background images by URL string.
//
// ⚠️ Single-WebView global: see package doc.
var backgroundImageCache = struct {
	mu   sync.Mutex
	imgs map[string]*DecodedImage
}{imgs: map[string]*DecodedImage{}}

// loadBackgroundImage resolves and decodes a background-image URL.
// data: URIs are decoded inline; file paths are read relative to baseDir
// (empty = current working directory). Results are cached; Release is not
// called on cached images (they live for the process).
func loadBackgroundImage(url, baseDir string) *DecodedImage {
	backgroundImageCache.mu.Lock()
	defer backgroundImageCache.mu.Unlock()
	if img, ok := backgroundImageCache.imgs[url]; ok {
		return img
	}
	var data []byte
	if b, ok := decodeDataURI(url); ok {
		data = b
	} else if !strings.Contains(url, ":") { // not a scheme, treat as file
		p := url
		if baseDir != "" && !strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "\\") {
			p = baseDir + string(os.PathSeparator) + url
		}
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
		}
	}
	if len(data) == 0 {
		return nil
	}
	img := NewDecodedImage(data)
	if img == nil {
		return nil
	}
	backgroundImageCache.imgs[url] = img
	return img
}

// bgSizeMode describes how background-size scales the image.
type bgSizeMode int

const (
	bgSizeAuto    bgSizeMode = iota // original pixel size
	bgSizeCover                     // scale to cover the box (crop overflow)
	bgSizeContain                   // scale to fit inside the box (letterbox)
	bgSizeExplicit                  // width/height lengths or percentages
)

// parseBackgroundSize parses a background-size value. Returns the mode and,
// for bgSizeExplicit, the horizontal/vertical sizes (percentages are 0..100
// with px>0, or absolute px values with unit "px").
func parseBackgroundSize(s string) (mode bgSizeMode, wPct, wPx, hPct, hPx float64) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "auto" {
		return bgSizeAuto, 0, 0, 0, 0
	}
	if s == "cover" {
		return bgSizeCover, 0, 0, 0, 0
	}
	if s == "contain" {
		return bgSizeContain, 0, 0, 0, 0
	}
	// <width> [<height>] — lengths/percentages; single value => auto height.
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return bgSizeAuto, 0, 0, 0, 0
	}
	mode = bgSizeExplicit
	wPct, wPx = parseBgSizePart(parts[0])
	if len(parts) > 1 {
		hPct, hPx = parseBgSizePart(parts[1])
	}
	return
}

func parseBgSizePart(v string) (pct, px float64) {
	if strings.HasSuffix(v, "%") {
		if n, ok := parseFloatS(strings.TrimSuffix(v, "%")); ok {
			return n, 0
		}
		return 0, 0
	}
	// px / em / etc: strip the unit; non-px units are treated as px-scale
	// (simplification: background-size supports px and % in practice).
	v = strings.TrimSuffix(v, "px")
	if n, ok := parseFloatS(v); ok {
		return 0, n
	}
	return 0, 0
}

func parseFloatS(s string) (float64, bool) {
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}

// computeBackgroundDest computes the destination rect for a background image
// inside a (x,y,w,h) box, honoring background-size and background-position.
func computeBackgroundDest(x, y, w, h float64, size, position string, imgW, imgH int) (dx, dy, dw, dh float64) {
	mode, wPct, wPx, hPct, hPx := parseBackgroundSize(size)
	iw, ih := float64(imgW), float64(imgH)
	switch mode {
	case bgSizeCover:
		scale := maxF(w/iw, h/ih)
		dw, dh = iw*scale, ih*scale
	case bgSizeContain:
		scale := minF(w/iw, h/ih)
		dw, dh = iw*scale, ih*scale
	case bgSizeExplicit:
		aw, ah := wPx, hPx
		if wPct > 0 {
			aw = w * wPct / 100
		}
		if hPct > 0 {
			ah = h * hPct / 100
		}
		dw, dh = aw, ah
		// auto height preserves aspect ratio.
		if dw <= 0 && dh > 0 {
			dw = dh * iw / ih
		} else if dh <= 0 && dw > 0 {
			dh = dw * ih / iw
		}
		if dw <= 0 {
			dw = w
		}
		if dh <= 0 {
			dh = h
		}
	default: // auto: original size
		dw, dh = iw, ih
	}
	// Position: default 0% 0% (top-left). Percentages offset by
	// (box - image) so 50% centers, 100% aligns bottom-right.
	px, py := parseBackgroundPosition(position, w-dw, h-dh)
	dx, dy = x+px, y+py
	return
}

// parseBackgroundPosition parses background-position. Percentages and
// keywords (left/center/right, top/middle/bottom) resolve against the
// (box - image) delta; px values are absolute offsets.
func parseBackgroundPosition(s string, deltaW, deltaH float64) (px, py float64) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, 0
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, 0
	}
	// Two-value form, or a single value that implies the other axis is center.
	xv, yv := parts[0], "center"
	if len(parts) >= 2 {
		yv = parts[1]
	}
	if len(parts) == 1 && (xv == "top" || xv == "bottom") {
		xv, yv = "center", parts[0]
	}
	px = resolveBgPosAxis(xv, deltaW)
	py = resolveBgPosAxis(yv, deltaH)
	return
}

func resolveBgPosAxis(v string, delta float64) float64 {
	switch v {
	case "left", "top":
		return 0
	case "center", "middle":
		return delta / 2
	case "right", "bottom":
		return delta
	}
	if strings.HasSuffix(v, "%") {
		if n, ok := parseFloatS(strings.TrimSuffix(v, "%")); ok {
			return delta * n / 100
		}
		return 0
	}
	if n, ok := parseFloatS(v); ok {
		return n
	}
	return 0
}
