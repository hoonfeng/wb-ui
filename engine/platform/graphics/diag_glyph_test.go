package graphics

import (
	"fmt"
	"os"
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestDiagSkiaDirect 用底层 skia API 直接渲染 ▸（绕过 wb-ui fallback）。
func TestDiagSkiaDirect(t *testing.T) {
	if GetFontManager() == nil {
		InitFontManager("")
	}
	mgr := GetFontManager()
	mgr.LoadSystemFonts()

	// 渲染一串相似字符，对比字形是否正确映射
	const chars = "▸▶▹▻◆◇□■●▲→›»"

	// 额外测试：微软雅黑（默认 UI 字体）渲染 ▸ —— 模拟实际 UI 字体链
	fmt.Printf("== microsoft yahei (sansTF) ==\n")
	if mgr.sansTF != nil {
		sf0 := skia.NewFont(mgr.sansTF, 40)
		fmt.Printf("UnicharToGlyph: '▸'=%d '▾'=%d '▶'=%d 'A'=%d\n",
			sf0.UnicharToGlyph('▸'), sf0.UnicharToGlyph('▾'), sf0.UnicharToGlyph('▶'), sf0.UnicharToGlyph('A'))
		fmt.Printf("SymbolTypeface nil? %v\n", mgr.SymbolTypeface() == nil)
		if st := mgr.SymbolTypeface(); st != nil {
			sfSym := skia.NewFont(st, 40)
			fmt.Printf("Symbol UnicharToGlyph: '▸'=%d '▾'=%d\n", sfSym.UnicharToGlyph('▸'), sfSym.UnicharToGlyph('▾'))
		}
		// 用 Canvas.DrawText（带 fallback 的入口）渲染 ▸ —— 验证 fallback 路径
		c2 := NewCanvas(60, 60)
		c2.DrawText(12, 44, "▸▾", Font{Family: "Microsoft YaHei", Size: 40}, Color{R: 255, G: 255, B: 255, A: 255})
		shape2 := ""
		for y := 4; y < 56; y++ {
			row := ""
			for x := 6; x < 54; x++ {
				a := c2.PixelAt(x, y).A
				if a > 40 {
					row += "#"
				} else if a > 0 {
					row += "."
				} else {
					row += " "
				}
			}
			shape2 += row + "\n"
		}
		c2.Release()
		fmt.Printf("--- Canvas.DrawText fallback '▸▾' ---\n%s\n", shape2)
		for _, r := range "▸▾▶" {
			c := NewCanvas(60, 60)
			sf := skia.NewFont(mgr.sansTF, 40)
			sf.SetEdging(skia.FontEdgingAntialias)
			sf.SetSubpixel(true)
			paint := skia.NewPaint()
			paint.SetColor(skia.RGBA(255, 255, 255, 255))
			c.canvas.DrawText(string(r), 12, 44, sf, paint)
			shape := ""
			for y := 4; y < 56; y++ {
				row := ""
				for x := 6; x < 54; x++ {
					a := c.PixelAt(x, y).A
					if a > 40 {
						row += "#"
					} else if a > 0 {
						row += "."
					} else {
						row += " "
					}
				}
				shape += row + "\n"
			}
			c.Release()
			fmt.Printf("--- %q ---\n%s\n", r, shape)
		}
	} else {
		fmt.Println("sansTF is nil!")
	}

	// TTC 数据 typeface 的 UnicharToGlyph 行为（关键疑点：TTC 加载的字体可能对缺失字符返回非 0）
	if data, err := os.ReadFile(`C:\Windows\Fonts\msyh.ttc`); err == nil {
		if tfTTC := skia.NewTypefaceFromData(data, 0); tfTTC != nil {
			sfTTC := skia.NewFont(tfTTC, 40)
			fmt.Printf("msyh.ttc[0] UnicharToGlyph: '▸'=%d '▾'=%d 'A'=%d '中'=%d\n",
				sfTTC.UnicharToGlyph('▸'), sfTTC.UnicharToGlyph('▾'), sfTTC.UnicharToGlyph('A'), sfTTC.UnicharToGlyph('中'))
		} else {
			fmt.Println("msyh.ttc[0] NewTypefaceFromData returned nil")
		}
	}
	if data, err := os.ReadFile(`C:\Windows\Fonts\simsun.ttc`); err == nil {
		if tfTTC := skia.NewTypefaceFromData(data, 1); tfTTC != nil {
			sfTTC := skia.NewFont(tfTTC, 40)
			fmt.Printf("simsun.ttc[1] UnicharToGlyph: '▸'=%d '▾'=%d 'A'=%d '中'=%d\n",
				sfTTC.UnicharToGlyph('▸'), sfTTC.UnicharToGlyph('▾'), sfTTC.UnicharToGlyph('A'), sfTTC.UnicharToGlyph('中'))
		} else {
			fmt.Println("simsun.ttc[1] NewTypefaceFromData returned nil")
		}
	}
	if tfOS := skia.NewTypeface("Microsoft YaHei", skia.FontStyle{Weight: 400, Width: 5, Slant: 0}); tfOS != nil {
		sfOS := skia.NewFont(tfOS, 40)
		fmt.Printf("OS Microsoft YaHei UnicharToGlyph: '▸'=%d '▾'=%d '中'=%d\n",
			sfOS.UnicharToGlyph('▸'), sfOS.UnicharToGlyph('▾'), sfOS.UnicharToGlyph('中'))
	} else {
		fmt.Println("OS Microsoft YaHei NewTypeface returned nil")
	}

	for _, fam := range []string{"seguisym", "symbol", "msyh"} {
		var tf *skia.Typeface
		for _, f := range mgr.fonts {
			if f.family == fam {
				tf = f.tf
				break
			}
		}
		if tf == nil {
			fmt.Printf("== %s: not loaded ==\n", fam)
			continue
		}
		fmt.Printf("== %s ==\n", fam)
		for _, r := range chars {
			c := NewCanvas(60, 60)
			sf := skia.NewFont(tf, 40)
			sf.SetEdging(skia.FontEdgingAntialias)
			sf.SetSubpixel(true)
			paint := skia.NewPaint()
			paint.SetColor(skia.RGBA(255, 255, 255, 255))
			c.canvas.DrawText(string(r), 12, 44, sf, paint)
			shape := ""
			for y := 4; y < 56; y++ {
				row := ""
				for x := 6; x < 54; x++ {
					a := c.PixelAt(x, y).A
					if a > 40 {
						row += "#"
					} else if a > 0 {
						row += "."
					} else {
						row += " "
					}
				}
				shape += row + "\n"
			}
			c.Release()
			fmt.Printf("--- %q ---\n%s\n", r, shape)
		}
	}
}
