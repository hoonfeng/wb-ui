// Background-image painting: url(...) images with background-size and
// background-position. Complements the gradient painting in painter.go.
// Mirrors Source/WebCore/rendering/BackgroundPainter.cpp's image path
// (BackgroundImageGeometry).

package rendering

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/platform/graphics"
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

// httpGet fetches a URL's body with a short timeout. Returns nil on error.
func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap
}

// svgBackgroundCache caches parsed SVG background documents by URL string.
var svgBackgroundCache = struct {
	mu   sync.Mutex
	docs map[string]*svgDocument
}{docs: map[string]*svgDocument{}}

// loadBackgroundSVG resolves and parses a background-image SVG (data:
// image/svg+xml URIs or file paths). Returns nil when the URL is not an SVG
// or cannot be parsed. Results are cached by URL.
func loadBackgroundSVG(url string) *svgDocument {
	svgBackgroundCache.mu.Lock()
	defer svgBackgroundCache.mu.Unlock()
	if d, ok := svgBackgroundCache.docs[url]; ok {
		return d
	}
	var text string
	low := strings.ToLower(url)
	if strings.HasPrefix(low, "data:image/svg+xml") {
		if strings.Contains(low, ";base64,") {
			if b, ok := decodeDataURI(url); ok {
				text = string(b)
			}
		} else if i := strings.Index(url, ","); i >= 0 {
			raw := url[i+1:]
			if dec, err := neturl.QueryUnescape(raw); err == nil {
				text = dec
			} else {
				text = raw
			}
		}
	} else if !strings.Contains(url, ":") { // file path
		if b, err := os.ReadFile(url); err == nil {
			text = string(b)
		}
	}
	if text == "" {
		return nil
	}
	doc := parseSVGText(text)
	if doc == nil {
		return nil
	}
	svgBackgroundCache.docs[url] = doc
	return doc
}

// parseSVGText parses an SVG document string and builds the svgDocument for
// painting. Uses the HTML parser (which folds SVG foreign content).
func parseSVGText(text string) *svgDocument {
	d, err := html.Parse(text)
	if err != nil {
		return nil
	}
	var svgEl *dom.Element
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if svgEl != nil {
			return
		}
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "svg" {
			svgEl = el
			return
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for c := d.FirstChild(); c != nil; c = c.NextSibling() {
		walk(c)
	}
	if svgEl == nil {
		return nil
	}
	return buildSVGDocument(svgEl)
}

// paintSVGScaled paints an svgDocument scaled into the destination rect.
// The SVG's intrinsic width/height define the scale; missing intrinsic size
// falls back to painting at (x, y) at natural shape coordinates.
func paintSVGScaled(canvas *graphics.Canvas, svg *svgDocument, x, y, w, h float64) {
	if svg == nil || canvas == nil {
		return
	}
	if svg.width > 0 && svg.height > 0 && w > 0 && h > 0 {
		canvas.Save()
		canvas.Translate(x, y)
		canvas.Scale(w/svg.width, h/svg.height)
		paintSVG(canvas, svg, 0, 0, graphics.Color{})
		canvas.Restore()
	} else {
		paintSVG(canvas, svg, x, y, graphics.Color{})
	}
}

// paintBackgroundImageTiled draws a decoded background image into the box
// honoring background-repeat. The image's destination (dx,dy,dw,dh) is the
// first tile; repeat (default) tiles in both axes, repeat-x/repeat-y tile in
// one axis, no-repeat draws a single tile. Tiles that start before the box
// edge start at the tile's own offset so the pattern stays aligned with the
// position origin (matching CSS: the position defines the first tile's
// location).
func paintBackgroundImageTiled(canvas *graphics.Canvas, img *DecodedImage,
	x, y, w, h, dx, dy, dw, dh float64, repeat string) {
	if canvas == nil || img == nil || !img.Loaded() || dw <= 0 || dh <= 0 {
		return
	}
	rep := strings.ToLower(strings.TrimSpace(repeat))
	repX := rep != "no-repeat" && rep != "repeat-y"
	repY := rep != "no-repeat" && rep != "repeat-x"
	startX := dx
	startY := dy
	for ty := startY; ty < y+h; ty += dh {
		for tx := startX; tx < x+w; tx += dw {
			if repX && tx+dw < x {
				continue
			}
			if repY && ty+dh < y {
				continue
			}
			img.Draw(canvas, tx, ty, dw, dh)
			if !repX {
				break
			}
		}
		if !repY {
			break
		}
	}
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
	mu      sync.Mutex
	imgs    map[string]*DecodedImage
	loading map[string]bool // http(s) URLs currently being fetched
}{imgs: map[string]*DecodedImage{}, loading: map[string]bool{}}

// bgImageLoadedCallback, when set, is invoked after an async http(s)
// background image finishes loading (success or failure). Hosts use it to
// schedule a repaint so the image appears without waiting for the next
// frame-driven paint.
var bgImageLoadedCallback func(url string)

// SetBackgroundImageLoadedCallback registers the callback fired after an
// async background image load completes. Pass nil to clear.
func SetBackgroundImageLoadedCallback(cb func(url string)) {
	backgroundImageCache.mu.Lock()
	bgImageLoadedCallback = cb
	backgroundImageCache.mu.Unlock()
}

// loadBackgroundImage resolves and decodes a background-image URL.
// data: URIs and file paths decode synchronously (local, fast). http(s)
// URLs fetch asynchronously: the first call spawns a goroutine and returns
// nil; subsequent paints pick the image from the cache once loaded. This
// keeps the render thread unblocked by network latency.
func loadBackgroundImage(url, baseDir string) *DecodedImage {
	backgroundImageCache.mu.Lock()
	defer backgroundImageCache.mu.Unlock()
	if img, ok := backgroundImageCache.imgs[url]; ok {
		return img
	}
	var data []byte
	if b, ok := decodeDataURI(url); ok {
		data = b
	} else if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		// Remote image: async fetch once per URL. Return nil now; the
		// goroutine fills the cache and fires the loaded callback.
		if !backgroundImageCache.loading[url] {
			backgroundImageCache.loading[url] = true
			go fetchBackgroundImageAsync(url)
		}
		return nil
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

// fetchBackgroundImageAsync downloads an http(s) image off-thread and stores
// the decoded result in the cache, then fires the loaded callback.
func fetchBackgroundImageAsync(url string) {
	data, err := httpGet(url)
	backgroundImageCache.mu.Lock()
	delete(backgroundImageCache.loading, url)
	if err == nil && len(data) > 0 {
		if img := NewDecodedImage(data); img != nil {
			backgroundImageCache.imgs[url] = img
		}
	}
	cb := bgImageLoadedCallback
	backgroundImageCache.mu.Unlock()
	if cb != nil {
		cb(url)
	}
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

// computeGradientDest computes the destination rect for a gradient layer
// inside a box. Gradients have no intrinsic size, so background-size:
// auto/cover/contain all mean "fill the box"; explicit lengths/percentages
// confine the gradient to a sub-rect, offset by background-position.
func computeGradientDest(x, y, w, h float64, size, pos string) (dx, dy, dw, dh float64) {
	mode, wPct, wPx, hPct, hPx := parseBackgroundSize(size)
	dw, dh = w, h
	if mode == bgSizeExplicit {
		if wPct > 0 {
			dw = w * wPct / 100
		} else if wPx > 0 {
			dw = wPx
		}
		if hPct > 0 {
			dh = h * hPct / 100
		} else if hPx > 0 {
			dh = hPx
		}
	}
	px, py := parseBackgroundPosition(pos, w-dw, h-dh)
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
