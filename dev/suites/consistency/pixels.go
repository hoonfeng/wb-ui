// Pixel-level rendering comparison: Edge headless screenshot vs wb-ui
// rendered canvas. Geometry is already verified at ±2px by the GEO suite;
// this goes one level deeper — every pixel of the rendered page.
//
// Cases avoid text (font anti-aliasing differs between DirectWrite and
// wb-ui's rasterizer) and focus on shapes: color blocks, borders, radius,
// shadows, gradients.

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"context"

	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

// edgeShot renders c.HTML through Edge headless, saves a screenshot to
// TempDir and returns the PNG path plus its pixel dimensions.
func edgeShot(c TestCase) (string, int, int, error) {
	fname := filepath.Join(TempDir, c.Name+".shot.html")
	if err := os.WriteFile(fname, []byte(c.HTML), 0644); err != nil {
		return "", 0, 0, fmt.Errorf("write html: %w", err)
	}
	out := filepath.Join(TempDir, c.Name+".edge.png")
	os.Remove(out)
	url := "file:///" + strings.ReplaceAll(fname, "\\", "/")
	// No --user-data-dir (crashed profiles leave SingletonLock that hang
	// launches); Edge headless manages a temp profile per run. 30s timeout
	// so a wedged browser fails fast instead of blocking the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, EdgePath,
		"--headless", "--disable-gpu", "--no-sandbox", "--hide-scrollbars",
		"--window-size="+fmt.Sprintf("%d,%d", c.ViewportW, c.ViewportH),
		"--screenshot="+out, url)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", 0, 0, fmt.Errorf("edge screenshot: %v (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	f, err := os.Open(out)
	if err != nil {
		return "", 0, 0, fmt.Errorf("browser produced no screenshot (stderr: %s): %w",
			strings.TrimSpace(stderr.String()), err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return "", 0, 0, fmt.Errorf("decode screenshot %s: %w", out, err)
	}
	b := img.Bounds()
	return out, b.Dx(), b.Dy(), nil
}

// wbuiRenderPNG renders c.HTML through the wb-ui pipeline into an RGBA image.
// w/h are the pixel dimensions of the reference screenshot.
func wbuiRenderPNG(c TestCase, w, h int) (*image.RGBA, error) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	clean := stripCollector(c.HTML)
	doc, err := html.Parse(clean)
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	if rv == nil {
		return nil, fmt.Errorf("render tree build failed")
	}
	rv.SetViewportSize(float64(w), float64(h))
	state := layout.NewLayoutState(float64(w), float64(h))
	rv.Layout(state)

	canvas := graphics.NewCanvas(w, h)
	defer canvas.Release()
	// Paint a white document background first (Edge screenshots have white
	// canvas by default; wb-ui paints transparent).
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := canvas.PixelAt(x, y)
			img.SetRGBA(x, y, color.RGBA{R: p.R, G: p.G, B: p.B, A: p.A})
		}
	}
	// Composite onto white: RGBA pixels with A<255 must blend over white,
	// since the PNG has no alpha channel after decoding the Edge shot.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := img.RGBAAt(x, y)
			if px.A < 255 {
				a := float64(px.A) / 255
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(float64(px.R)*a + 255*(1-a)),
					G: uint8(float64(px.G)*a + 255*(1-a)),
					B: uint8(float64(px.B)*a + 255*(1-a)),
					A: 255,
				})
			}
		}
	}
	return img, nil
}

// pixelDiffReport summarizes differences between two RGBA images.
type pixelDiffReport struct {
	W, H           int
	DiffPixels     int
	DiffRate       float64 // 0..1
	MaxChannelDiff int
	BBox           image.Rectangle // bounding box of all differing pixels
	Sample         []diffPoint     // up to 12 diff samples for diagnosis
}

type diffPoint struct {
	X, Y     int
	Got, Exp color.RGBA
}

// pixelDiff compares a (expected = Edge) and b (got = wb-ui) with per-channel
// tolerance. Pixels equal within tol are considered matching.
func pixelDiff(exp, got *image.RGBA, tol int) pixelDiffReport {
	r := pixelDiffReport{W: exp.Bounds().Dx(), H: exp.Bounds().Dy()}
	if got.Bounds().Dx() != r.W || got.Bounds().Dy() != r.H {
		r.BBox = image.Rect(0, 0, r.W, r.H)
		return r
	}
	first := true
	for y := 0; y < r.H; y++ {
		for x := 0; x < r.W; x++ {
			e := exp.RGBAAt(x, y)
			g := got.RGBAAt(x, y)
			d := max3(abs(int(e.R)-int(g.R)), abs(int(e.G)-int(g.G)), abs(int(e.B)-int(g.B)))
			if d > tol {
				r.DiffPixels++
				if d > r.MaxChannelDiff {
					r.MaxChannelDiff = d
				}
				if first {
					r.BBox = image.Rect(x, y, x+1, y+1)
					first = false
				} else {
					r.BBox = r.BBox.Union(image.Rect(x, y, x+1, y+1))
				}
				if len(r.Sample) < 12 {
					r.Sample = append(r.Sample, diffPoint{X: x, Y: y, Got: g, Exp: e})
				}
			}
		}
	}
	r.DiffRate = float64(r.DiffPixels) / float64(r.W*r.H)
	return r
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// loadPNG reads a PNG file into an RGBA image.
func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba, nil
}

func formatDiff(r pixelDiffReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "size=%dx%d diff=%d px (%.3f%%) maxChannelDiff=%d\n",
		r.W, r.H, r.DiffPixels, r.DiffRate*100, r.MaxChannelDiff)
	if r.DiffPixels > 0 {
		fmt.Fprintf(&sb, "bbox=(%d,%d)-(%d,%d)\n", r.BBox.Min.X, r.BBox.Min.Y, r.BBox.Max.X, r.BBox.Max.Y)
		for _, s := range r.Sample {
			fmt.Fprintf(&sb, "  @(%d,%d) got=#%02x%02x%02x exp=#%02x%02x%02x\n",
				s.X, s.Y, s.Got.R, s.Got.G, s.Got.B, s.Exp.R, s.Exp.G, s.Exp.B)
		}
	}
	return sb.String()
}

// perpixel tolerance helpers for sub-region checks.
func chanDiff(e, g color.RGBA) int {
	return max3(abs(int(e.R)-int(g.R)), abs(int(e.G)-int(g.G)), abs(int(e.B)-int(g.B)))
}

func colorAt(img *image.RGBA, x, y int) color.RGBA {
	return img.RGBAAt(x, y)
}

// inside checks that every pixel of the rect is within tol of the target
// color. Used for assertions on flat color regions.
func regionMatches(img *image.RGBA, rect image.Rectangle, want color.RGBA, tol int) (bool, string) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if chanDiff(want, colorAt(img, x, y)) > tol {
				return false, fmt.Sprintf("@(%d,%d) got=#%02x%02x%02x want=#%02x%02x%02x",
					x, y, colorAt(img, x, y).R, colorAt(img, x, y).G, colorAt(img, x, y).B, want.R, want.G, want.B)
			}
		}
	}
	return true, ""
}
