package rendering

import (
	"encoding/base64"
	"net/url"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestParseSVGText: an SVG string parses to a paintable svgDocument.
func TestParseSVGText(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><rect x="0" y="0" width="10" height="10" fill="red"/></svg>`
	doc := parseSVGText(svg)
	if doc == nil {
		t.Fatalf("parseSVGText returned nil")
	}
	if doc.width != 10 || doc.height != 10 {
		t.Fatalf("size = %vx%v, want 10x10", doc.width, doc.height)
	}
	if len(doc.shapes) == 0 {
		t.Fatalf("no shapes parsed")
	}
	if !doc.hasVB {
		t.Fatalf("viewBox not parsed")
	}
}

// TestLoadBackgroundSVGDataURI: data:image/svg+xml;base64 loads and parses.
func TestLoadBackgroundSVGDataURI(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><rect width="8" height="8" fill="blue"/></svg>`
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	doc := loadBackgroundSVG(uri)
	if doc == nil {
		t.Fatalf("loadBackgroundSVG(base64) returned nil")
	}
	if doc.width != 8 {
		t.Fatalf("width = %v, want 8", doc.width)
	}
}

// TestLoadBackgroundSVGURLEncoded: data:image/svg+xml,<encoded> loads.
func TestLoadBackgroundSVGURLEncoded(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4" fill="green"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)
	doc := loadBackgroundSVG(uri)
	if doc == nil {
		t.Fatalf("loadBackgroundSVG(urlencoded) returned nil")
	}
	if len(doc.shapes) == 0 {
		t.Fatalf("no circle shape parsed")
	}
}

// TestDecodeDataURITextKeepsLiteralPlus: data URI（RFC 2397）的载荷是
// **percent-编码文本**，`+` 必须保持字面（不是空格——那是 form-encoding 的
// 约定）。此前用 QueryUnescape，SVG 内容里字面出现的 `+`
// （`transform="translate(+1,2)"`、path 数据里的 `M0+0` 之类）会被吃成空格。
func TestDecodeDataURITextKeepsLiteralPlus(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a+b", "a+b"},         // 字面 + 保持
		{"%3Csvg%3E", "<svg>"}, // percent 解码
		{"%23ff0000", "#ff0000"},
		{"a%20b", "a b"},
		{"%2B", "+"},
	}
	for _, c := range cases {
		if got := decodeDataURIText(c.in); got != c.want {
			t.Errorf("decodeDataURIText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLoadBackgroundSVGPercentEncoded: a percent-encoded data URI (the RFC 2397
// form that browsers/探针 produce, `+` literal) parses完整，不依赖
// form-encoding 回退。
func TestLoadBackgroundSVGPercentEncoded(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="6" ` +
		`viewBox="0 0 12 6"><rect width="12" height="6" fill="#ff0000"/></svg>`
	uri := "data:image/svg+xml," + url.PathEscape(svg)
	doc := loadBackgroundSVG(uri)
	if doc == nil {
		t.Fatalf("loadBackgroundSVG(percent-encoded) returned nil")
	}
	if doc.width != 12 || doc.height != 6 {
		t.Fatalf("size = %vx%v, want 12x6", doc.width, doc.height)
	}
	if len(doc.shapes) == 0 {
		t.Fatalf("no shapes parsed")
	}
}

// TestSVGReferenceIntrinsicSize: SVG 引用的固有尺寸（naturalWidth 语义）——
// width/height 属性优先、viewBox 兜底、两者都没有时按 CSS 默认尺寸算法取
// 300×150；非 SVG 引用（位图、远端）返回 ok=false，由各自的通道负责。
func TestSVGReferenceIntrinsicSize(t *testing.T) {
	withWH := `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24"/></svg>`
	vbOnly := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20"><rect width="40" height="20"/></svg>`
	bare := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`

	cases := []struct {
		name string
		url  string
		w, h float64
		ok   bool
	}{
		{"width/height 优先", "data:image/svg+xml," + url.PathEscape(withWH), 24, 24, true},
		{"viewBox 兜底", "data:image/svg+xml," + url.PathEscape(vbOnly), 40, 20, true},
		{"默认 300x150", "data:image/svg+xml," + url.PathEscape(bare), 300, 150, true},
		{"非 SVG（png data URI）", "data:image/png;base64,iVBORw0KGgo=", 0, 0, false},
		{"远端引用（交异步通道）", "https://example.com/x.svg", 0, 0, false},
	}
	for _, c := range cases {
		w, h, ok := SVGReferenceIntrinsicSize(c.url, nil)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && (w != c.w || h != c.h) {
			t.Errorf("%s: size = %gx%g, want %gx%g", c.name, w, h, c.w, c.h)
		}
	}
}

// TestPaintBackgroundSVG: an SVG data-URI background paints a red rect
// scaled into the box.
func TestPaintBackgroundSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)

	// Step 1: verify parse + shape list.
	sdoc := parseSVGText(svg)
	if sdoc == nil || len(sdoc.shapes) == 0 {
		t.Fatalf("parse failed")
	}
	t.Logf("shapes=%d width=%v height=%v hasVB=%v", len(sdoc.shapes), sdoc.width, sdoc.height, sdoc.hasVB)

	// Step 2: direct paintSVG (no scale) on a 20x20 canvas.
	canvas0 := graphics.NewCanvas(20, 20)
	defer canvas0.Release()
	paintSVG(canvas0, sdoc, 0, 0, graphics.Color{})
	if px := canvas0.PixelAt(5, 5); px.R != 255 {
		t.Logf("direct paintSVG (5,5) = %+v", px)
	} else {
		t.Logf("direct paintSVG (5,5) = red OK")
	}

	// Step 3: direct paintSVGScaled.
	canvas2 := graphics.NewCanvas(80, 40)
	defer canvas2.Release()
	paintSVGScaled(canvas2, sdoc, 20, 10, 40, 20)
	if px := canvas2.PixelAt(40, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("direct paintSVGScaled center = %+v, want red", px)
	}

	canvas := graphics.NewCanvas(80, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 40})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	// Explicit size 40x20 centered in 80x40 → covers (20,10)-(60,30).
	st.BackgroundSize = "40px 20px"
	st.BackgroundPosition = "center"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(80, 40)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(40, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("center (40,20) = %+v, want red", px)
	}
	if px := canvas.PixelAt(4, 4); px.A != 0 {
		t.Fatalf("outside svg (4,4) = %+v, want transparent", px)
	}
}

// TestPaintImageSVG: an <img src="data:image/svg+xml,..."> paints the SVG
// scaled into the content box.
func TestPaintImageSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><circle cx="5" cy="5" r="5" fill="red"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)

	canvas := graphics.NewCanvas(60, 30)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 30})

	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("src", uri)
	st := style.NewComputedStyle()
	box := NewRenderBox(imgEl, st)
	box.SetLocation(0, 0)
	box.SetSize(60, 30)

	if !PaintImage(box, info) {
		t.Fatalf("PaintImage returned false")
	}
	// Center of the SVG circle scaled into the box → red.
	if px := canvas.PixelAt(30, 15); px.R != 255 || px.G != 0 {
		t.Fatalf("center (30,15) = %+v, want red", px)
	}
	// Corner outside the circle stays transparent.
	if px := canvas.PixelAt(2, 2); px.A != 0 {
		t.Fatalf("corner (2,2) = %+v, want transparent", px)
	}
}

var _ = base64.StdEncoding
