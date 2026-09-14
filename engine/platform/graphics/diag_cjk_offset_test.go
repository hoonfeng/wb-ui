package graphics

import (
	"fmt"
	"testing"
)

// TestDiagCJKOffset 验证 DrawText 画单个中文的像素位置（行3「测」绘制
// 日志 @555 但像素 @568，疑 fallback 内部偏移——中英对齐异常根因）。
func TestDiagCJKOffset(t *testing.T) {
	mgr := GetFontManager()
	if mgr == nil {
		_ = InitFontManager("")
		mgr = GetFontManager()
	}
	if mgr == nil {
		t.Skip("no font manager")
	}
	mgr.LoadSystemFonts()
	c := NewCanvas(400, 60)
	white := Color{R: 255, G: 255, B: 255, A: 255}
	c.Clear(white)
	black := Color{R: 0, G: 0, B: 0, A: 255}
	font := Font{Family: `"JetBrains Mono", "Cascadia Code", "Fira Code", monospace`, Size: 13}
	// 在 x=100 画每个字符
	for i, r := range []rune{'函', '测', '试', '释', 'A'} {
		c.DrawText(100, 45, string(r), font, black)
		// 找该字符像素范围（y=35-55）
		px := c.Pixels()
		minX, maxX := -1, -1
		for y := 30; y < 56; y++ {
			for x := 80; x < 200; x++ {
				off := (y*400 + x) * 4
				if off+3 < len(px) && px[off] < 100 {
					if minX == -1 || x < minX {
						minX = x
					}
					if x > maxX {
						maxX = x
					}
				}
			}
		}
		fmt.Printf("'%c' DrawText@100 → 像素 %d-%d (偏移 %d)\n", r, minX, maxX, minX-100)
		_ = i
	}
}
