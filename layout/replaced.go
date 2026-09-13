// Replaced-element sizing: CSS 2.1 §10.3.2 (width) / §10.6.2 (height) with
// the CSS Images 3 default sizing algorithm.
//
// A replaced element such as <img> does not size from its (empty) children —
// its used size comes from the *resource's* intrinsic size, or from the
// resource's aspect ratio combined with the available width. wb-ui used to
// fall back to the line height for every such element, so a ratio-only SVG
// (markup with just a viewBox, no width/height — the shape every modern icon
// and `data:` SVG image takes) collapsed to 0×18 and its inline-block wrapper
// to 18px tall.
//
// The intrinsic size is discovered from the resource itself (SVG markup or the
// image header for raster formats) and cached per URL, so no layout pass ever
// re-parses the same source.
package layout

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"  // raster intrinsic sizes
	_ "image/jpeg" // raster intrinsic sizes
	_ "image/png"  // raster intrinsic sizes
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// maxResourceProbeBytes bounds how much of a local file is read while probing
// for an intrinsic size: the root element / image header is all that matters.
const maxResourceProbeBytes = 1 << 20

// intrinsicSize is a resource's intrinsic size and aspect ratio. A ratio-only
// resource reports w=h=0 with a non-zero ratio.
type intrinsicSize struct {
	w, h  float64
	ratio float64
}

var (
	intrinsicMu    sync.RWMutex
	intrinsicCache = map[string]intrinsicSize{}
)

// replacedIntrinsicSize returns the intrinsic size of a replaced box's
// resource. The HTML width/height attributes are a presentational hint (a used
// size, not an intrinsic one) and are deliberately not consulted here.
func replacedIntrinsicSize(box *ElementBox) intrinsicSize {
	el := box.Element()
	if el == nil {
		return intrinsicSize{}
	}
	var src string
	switch el.LocalName() {
	case "img", "embed":
		src = el.GetAttribute("src")
	case "video", "audio":
		src = el.GetAttribute("poster")
	case "object":
		src = el.GetAttribute("data")
	default:
		return intrinsicSize{}
	}
	return lookupIntrinsic(src)
}

func lookupIntrinsic(src string) intrinsicSize {
	src = strings.TrimSpace(src)
	if src == "" {
		return intrinsicSize{}
	}
	intrinsicMu.RLock()
	is, ok := intrinsicCache[src]
	intrinsicMu.RUnlock()
	if ok {
		return is
	}
	is = parseIntrinsic(src)
	intrinsicMu.Lock()
	if len(intrinsicCache) < 1024 {
		intrinsicCache[src] = is
	}
	intrinsicMu.Unlock()
	return is
}

func parseIntrinsic(src string) intrinsicSize {
	data, svg, ok := readResource(src)
	if !ok || len(data) == 0 {
		return intrinsicSize{}
	}
	if svg {
		return svgIntrinsic(data)
	}
	if w, h, ok := rasterIntrinsic(data); ok {
		return intrinsicSize{w: w, h: h, ratio: w / h}
	}
	// Resources may be labelled with a generic type (or served as
	// application/octet-stream); snif the markup before giving up.
	return svgIntrinsic(data)
}

// readResource resolves a resource URL to its bytes for probing. Network URLs
// are not fetched: layout must stay synchronous and offline-safe.
func readResource(src string) (data []byte, svg, ok bool) {
	low := strings.ToLower(src)
	switch {
	case strings.HasPrefix(low, "data:"):
		comma := strings.IndexByte(src, ',')
		if comma < 0 {
			return nil, false, false
		}
		meta := low[len("data:"):comma]
		payload := src[comma+1:]
		svg = strings.Contains(meta, "svg")
		if strings.Contains(meta, "base64") {
			b, err := decodeDataBase64(payload)
			if err != nil {
				return nil, svg, false
			}
			return b, svg, true
		}
		// Non-base64 data URIs are percent-encoded text.
		if s, err := url.PathUnescape(payload); err == nil {
			return []byte(s), svg, true
		}
		return []byte(payload), svg, true
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"),
		strings.HasPrefix(low, "//"):
		return nil, false, false
	}
	path := src
	if strings.HasPrefix(low, "file://") {
		path = src[len("file://"):]
		path = strings.TrimPrefix(path, "localhost")
		// Windows drive paths arrive as /C:/dir/file.svg.
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
	}
	f, err := os.Open(filepath.FromSlash(path))
	if err != nil {
		return nil, false, false
	}
	defer f.Close()
	buf := make([]byte, maxResourceProbeBytes)
	n, err := f.Read(buf)
	if n <= 0 && err != nil {
		return nil, false, false
	}
	data = buf[:n]
	return data, strings.HasSuffix(strings.ToLower(path), ".svg") ||
		strings.HasSuffix(strings.ToLower(path), ".svgz") ||
		bytes.Contains(data, []byte("<svg")), true
}

func decodeDataBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawURLEncoding.DecodeString(s)
}

// svgIntrinsic extracts the intrinsic size of an SVG document: an explicit
// width/height pair when both are present, otherwise the viewBox extent as a
// ratio (the default sizing algorithm's "ratio-only" case).
func svgIntrinsic(data []byte) intrinsicSize {
	s := string(data)
	i := strings.Index(s, "<svg")
	if i < 0 {
		return intrinsicSize{}
	}
	rest := s[i:]
	j := strings.IndexByte(rest, '>')
	if j < 0 {
		return intrinsicSize{}
	}
	tag := rest[:j]
	w, h := svgLength(tag, "width"), svgLength(tag, "height")
	if w > 0 && h > 0 {
		return intrinsicSize{w: w, h: h, ratio: w / h}
	}
	if vbW, vbH := svgViewBox(tag); vbW > 0 && vbH > 0 {
		return intrinsicSize{ratio: vbW / vbH}
	}
	return intrinsicSize{}
}

// svgAttr finds attr="…" / attr='…' inside an SVG start tag. The match must
// start at a tag-attribute boundary so that "stroke-width" cannot answer a
// lookup for "width".
func svgAttr(tag, name string) (string, bool) {
	for _, q := range []string{"\"", "'"} {
		key := name + "=" + q
		from := 0
		for {
			i := strings.Index(tag[from:], key)
			if i < 0 {
				break
			}
			i += from
			if i == 0 || isTagSpace(tag[i-1]) {
				start := i + len(key)
				if end := strings.IndexByte(tag[start:], q[0]); end >= 0 {
					return tag[start : start+end], true
				}
				break
			}
			from = i + 1
		}
	}
	return "", false
}

func isTagSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func svgLength(tag, name string) float64 {
	v, ok := svgAttr(tag, name)
	if !ok {
		return 0
	}
	return parseSVGLength(v)
}

// parseSVGLength parses an SVG length in user units. Percentages (which depend
// on the viewport this document is embedded in) and "auto" report 0 = unknown.
func parseSVGLength(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "auto") || strings.Contains(s, "%") {
		return 0
	}
	i := 0
	for ; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '+' || c == '-' {
			continue
		}
		break
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || v <= 0 {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(s[i:])) {
	case "", "px":
		return v
	case "pt":
		return v * (96.0 / 72.0)
	case "pc":
		return v * 16
	case "in":
		return v * 96
	case "cm":
		return v * (96.0 / 2.54)
	case "mm":
		return v * (96.0 / 25.4)
	}
	return 0
}

func svgViewBox(tag string) (float64, float64) {
	v, ok := svgAttr(tag, "viewBox")
	if !ok {
		return 0, 0
	}
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) < 4 {
		return 0, 0
	}
	w, errW := strconv.ParseFloat(fields[2], 64)
	h, errH := strconv.ParseFloat(fields[3], 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

func rasterIntrinsic(data []byte) (float64, float64, bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return float64(cfg.Width), float64(cfg.Height), true
}

// replacedContentSize resolves the used content-box size of a replaced box.
//
// Following CSS 2.1 §10.3.2/§10.6.2, a definite CSS width wins; the other axis
// then comes from the aspect ratio (or the intrinsic extent). When neither axis
// is specified the intrinsic size is used, and — for a ratio-only resource —
// the width fills the available width and the height follows the ratio (the
// behaviour browsers implement for SVG carrying just a viewBox).
//
// ok is false when nothing can be decided (no intrinsic data and no definite
// CSS size): the caller's existing fallback then stays in charge.
func replacedContentSize(box *ElementBox, availableWidth, availableHeight float64) (w, h float64, ok bool) {
	cs := box.Style()
	if cs == nil {
		return 0, 0, false
	}
	is := replacedIntrinsicSize(box)
	fs := fontSizeOf(box)
	specW, hasW := definiteWidth(cs.Width, availableWidth, fs)
	specH, hasH := definiteHeight(cs.Height, availableHeight, fs)
	switch {
	case hasW && hasH:
		return specW, specH, true
	case hasW:
		if is.ratio > 0 {
			return specW, specW / is.ratio, true
		}
		if is.h > 0 {
			return specW, is.h, true
		}
	case hasH:
		if is.ratio > 0 {
			return specH * is.ratio, specH, true
		}
		if is.w > 0 {
			return is.w, specH, true
		}
	default:
		if is.w > 0 && is.h > 0 {
			return is.w, is.h, true
		}
		if is.ratio > 0 && availableWidth > 0 {
			return availableWidth, availableWidth / is.ratio, true
		}
		if is.w > 0 {
			return is.w, is.h, true
		}
	}
	return 0, 0, false
}
