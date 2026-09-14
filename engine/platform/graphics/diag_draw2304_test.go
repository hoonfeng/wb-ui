package graphics

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestDiagDrawU2304 直接用 Canvas.DrawText 画 ⌄（U+2304），验证 symbol
// fallback 是否生效——fold 图标渲染成矩形的根因排查。
func TestDiagDrawU2304(t *testing.T) {
	mgr := GetFontManager()
	if mgr == nil {
		_ = InitFontManager("")
		mgr = GetFontManager()
	}
	if mgr == nil {
		t.Skip("no font manager")
	}
	mgr.LoadSystemFonts()
	c := NewCanvas(200, 90)
	white := Color{R: 255, G: 255, B: 255, A: 255}
	c.Clear(white)
	fonts := []Font{
		{Family: `"JetBrains Mono", "Cascadia Code", "Fira Code", monospace`, Size: 13},
		{Family: "monospace", Size: 13},
		{Family: "Consolas", Size: 13},
	}
	black := Color{R: 0, G: 0, B: 0, A: 255}
	for i, f := range fonts {
		c.DrawText(10, 25+float64(i)*28, "⌄", f, black)
		c.DrawText(40, 25+float64(i)*28, "›", f, black)
	}
	px := c.Pixels()
	// 统计每带黑色像素（字符渲染）
	for i, f := range fonts {
		ymin := 12 + i*28
		ymax := ymin + 26
		rows := 0
		for yy := ymin; yy < ymax; yy++ {
			for xx := 8; xx < 60; xx++ {
				off := (yy*200 + xx) * 4
				if off+3 < len(px) && px[off] < 100 {
					rows++
					break
				}
			}
		}
		t.Logf("font[%d] %q → 有像素行数=%d", i, f.Family, rows)
	}
	out := filepath.Join(os.TempDir(), "diag_u2304.png")
	f, err := os.Create(out)
	if err == nil {
		img := image.NewRGBA(image.Rect(0, 0, 200, 90))
		for y := 0; y < 90; y++ {
			for x := 0; x < 200; x++ {
				off := (y*200 + x) * 4
				if off+3 < len(px) {
					img.SetRGBA(x, y, color.RGBA{R: px[off], G: px[off+1], B: px[off+2], A: px[off+3]})
				}
			}
		}
		_ = png.Encode(f, img)
		f.Close()
		t.Logf("saved %s", out)
	}
}
