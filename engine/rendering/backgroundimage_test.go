package rendering

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// smallRedPNG returns a base64 data URI for an 8x8 solid red PNG.
func smallRedPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	red := color.RGBA{R: 255, A: 255}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, red)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestParseBackgroundURL(t *testing.T) {
	if u, ok := parseBackgroundURL(`url(images/bg.png)`); !ok || u != "images/bg.png" {
		t.Fatalf("url(images/bg.png) = %q %v", u, ok)
	}
	if u, ok := parseBackgroundURL(`url("data:image/png;base64,AAAA")`); !ok || u != "data:image/png;base64,AAAA" {
		t.Fatalf("quoted data uri = %q %v", u, ok)
	}
	if _, ok := parseBackgroundURL("linear-gradient(red,blue)"); ok {
		t.Fatalf("gradient should not parse as url")
	}
}

func TestDecodeDataURI(t *testing.T) {
	data, ok := decodeDataURI("data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("abc")))
	if !ok || !bytes.Equal(data, []byte("abc")) {
		t.Fatalf("decode data uri = %q %v", data, ok)
	}
	if _, ok := decodeDataURI("http://x/y.png"); ok {
		t.Fatalf("http url should not decode as data uri")
	}
}

func TestComputeBackgroundDest(t *testing.T) {
	// box 200x100, img 8x8, cover: scale=25 → 200x200, centered-ish position.
	dx, dy, dw, dh := computeBackgroundDest(0, 0, 200, 100, "cover", "center", 8, 8)
	if dw != 200 || dh != 200 {
		t.Fatalf("cover = %vx%v, want 200x200", dw, dh)
	}
	// contain: scale=12.5 → 100x100
	dx, dy, dw, dh = computeBackgroundDest(0, 0, 200, 100, "contain", "center", 8, 8)
	if dw != 100 || dh != 100 {
		t.Fatalf("contain = %vx%v, want 100x100", dw, dh)
	}
	// explicit 50% 50% → 100x50
	dx, dy, dw, dh = computeBackgroundDest(0, 0, 200, 100, "50% 50%", "left top", 8, 8)
	if dw != 100 || dh != 50 {
		t.Fatalf("50%% 50%% = %vx%v, want 100x50", dw, dh)
	}
	// position: 50% with delta (200-100)=100 → x=50
	dx, dy, dw, dh = computeBackgroundDest(0, 0, 200, 100, "50% 50%", "50% 50%", 8, 8)
	if dx != 50 || dy != 25 {
		t.Fatalf("center position = (%v,%v), want (50,25)", dx, dy)
	}
	// auto: original 8x8 at top-left
	dx, dy, dw, dh = computeBackgroundDest(10, 20, 200, 100, "auto", "", 8, 8)
	if dw != 8 || dh != 8 || dx != 10 || dy != 20 {
		t.Fatalf("auto = (%v,%v %vx%v)", dx, dy, dw, dh)
	}
	// explicit px: 40x30 at bottom-right (position 100% 100%)
	dx, dy, dw, dh = computeBackgroundDest(0, 0, 200, 100, "40px 30px", "100% 100%", 8, 8)
	if dw != 40 || dh != 30 || dx != 160 || dy != 70 {
		t.Fatalf("px bottom-right = (%v,%v %vx%v), want (160,70 40x30)", dx, dy, dw, dh)
	}
	_ = dx
}

func TestParseBackgroundSize(t *testing.T) {
	mode, wp, wpx, hp, hpx := parseBackgroundSize("cover")
	if mode != bgSizeCover {
		t.Fatalf("cover mode = %v", mode)
	}
	mode, wp, wpx, hp, hpx = parseBackgroundSize("100% 50%")
	if mode != bgSizeExplicit || wp != 100 || hp != 50 {
		t.Fatalf("100%% 50%% = %v %v %v %v %v", mode, wp, wpx, hp, hpx)
	}
	mode, wp, wpx, hp, hpx = parseBackgroundSize("70px")
	if mode != bgSizeExplicit || wpx != 70 || hp != 0 {
		t.Fatalf("70px = %v %v %v %v %v", mode, wp, wpx, hp, hpx)
	}
}

// TestBackgroundImageDataURI: end-to-end paint of a data-URI background.
func TestBackgroundImageDataURI(t *testing.T) {
	img := loadBackgroundImage(smallRedPNG(t), "")
	if img == nil || !img.Loaded() {
		t.Fatalf("loadBackgroundImage failed")
	}
	if img.Width() != 8 || img.Height() != 8 {
		t.Fatalf("size = %dx%d, want 8x8", img.Width(), img.Height())
	}
	if !strings.HasPrefix(smallRedPNG(t), "data:image/png;base64,") {
		t.Fatalf("data uri prefix")
	}
}
