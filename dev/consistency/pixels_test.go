package main

import (
	"fmt"
	"testing"
)

// Pixel-level rendering comparison: Edge headless screenshot vs wb-ui
// rendered canvas. Geometry is already verified at ±2px by the GEO suite
// (main.go, 13 cases); these tests go one level deeper — every pixel.
//
// Cases avoid text (font anti-aliasing differs between DirectWrite and
// wb-ui's rasterizer) and use flat shapes. Strict cases (blocks, hard
// shadows, gradients) must match pixel-for-pixel; shape cases with
// anti-aliased edges (border-radius, corner bevels) allow a small diff
// rate for the 1px anti-aliasing transition band.

func pixelCompare(t *testing.T, name, body string, maxDiffRate float64) {
	t.Helper()
	c := TestCase{Name: name, ViewportW: 400, ViewportH: 300, HTML: baseDoc(body, "")}
	shotPath, sw, sh, err := edgeShot(c)
	if err != nil {
		t.Fatalf("[%s] edgeShot: %v", name, err)
	}
	got, err := wbuiRenderPNG(c, sw, sh)
	if err != nil {
		t.Fatalf("[%s] wbuiRenderPNG: %v", name, err)
	}
	exp, err := loadPNG(shotPath)
	if err != nil {
		t.Fatalf("[%s] loadPNG: %v", name, err)
	}
	r := pixelDiff(exp, got, 3)
	t.Logf("[%s] %s", name, formatDiff(r))
	if r.DiffRate > maxDiffRate {
		t.Errorf("[%s] diff rate %.3f%% exceeds limit %.3f%% (%d px)",
			name, r.DiffRate*100, maxDiffRate*100, r.DiffPixels)
	}
}

// ── Strict: must match pixel-for-pixel ──────────────────────────────

// TestPxBlock: flat color blocks (no anti-aliasing involved).
func TestPxBlock(t *testing.T) {
	pixelCompare(t, "px_block", `
		<div id="a" style="width:100px;height:80px;background:#ff0000;margin:10px"></div>
		<div id="b" style="width:100px;height:80px;background:#00ff00;margin-left:20px"></div>
		<div id="c" style="width:100px;height:80px;background:#0000ff;margin:10px"></div>
	`, 0.0)
}

// TestPxShadow: hard box-shadow (no blur), offset 5px.
func TestPxShadow(t *testing.T) {
	pixelCompare(t, "px_shadow", `
		<div id="s" style="width:120px;height:70px;margin:20px 10px;background:#eeeeee;box-shadow:5px 5px 0px 0px #aa0000"></div>
	`, 0.0)
}

// TestPxGradient: linear-gradient(to right), two stops.
func TestPxGradient(t *testing.T) {
	pixelCompare(t, "px_gradient", `
		<div id="g" style="width:160px;height:60px;margin:10px;background:linear-gradient(to right,#ff0000,#0000ff)"></div>
	`, 0.0)
}

// ── Anti-aliased edges: small diff-rate budget ─────────────────────

// TestPxBorder: distinct border colors per side; corners use 45° bevels
// which Edge anti-aliases over ~1px.
func TestPxBorder(t *testing.T) {
	pixelCompare(t, "px_border", `
		<div id="b" style="width:140px;height:60px;margin:10px;border-top:4px solid #ff0000;border-right:6px solid #00ff00;border-bottom:4px solid #0000ff;border-left:6px solid #ffff00;background:#888888"></div>
	`, 0.003)
}

// TestPxRadius: border-radius + border, rounded-corner anti-aliasing.
func TestPxRadius(t *testing.T) {
	pixelCompare(t, "px_radius", `
		<div id="r" style="width:120px;height:80px;background:#ff8800;border-radius:12px;border:3px solid #2244aa;margin:10px"></div>
	`, 0.005)
}

// TestPxInsetShadow: inset shadow offset strip.
func TestPxInsetShadow(t *testing.T) {
	pixelCompare(t, "px_inset", `
		<div id="i" style="width:140px;height:70px;margin:10px;background:#eeeeee;box-shadow:inset 6px 0px 0px 0px #0055aa"></div>
	`, 0.001)
}

// TestPxMultiStopGradient: three stops with explicit positions.
func TestPxMultiStopGradient(t *testing.T) {
	pixelCompare(t, "px_grad3", `
		<div id="g" style="width:200px;height:50px;margin:10px;background:linear-gradient(to right,#ff0000 0%,#00ff00 50%,#0000ff 100%)"></div>
	`, 0.002)
}

func init() { _ = fmt.Sprintf }
