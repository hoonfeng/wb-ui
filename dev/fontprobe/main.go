// Command fontprobe verifies CJK rendering through every font resolution
// path. Key comparison: OS-name Typeface vs raw-data Typeface (registered via
// RegisterCustomFont) — data faces may lack the system fallback chain,
// producing tofu boxes for CJK.
package main

import (
	"fmt"
	"os"

	"wb-ui/platform/graphics"
)

func canvasInk(family string) int {
	c := graphics.NewCanvas(200, 60)
	defer c.Release()
	c.DrawText(10, 40, "中文", graphics.Font{Family: family, Size: 28, Weight: 400},
		graphics.Color{R: 0, G: 0, B: 0, A: 255})
	ink := 0
	for y := 0; y < 60; y++ {
		for x := 0; x < 200; x++ {
			if p := c.PixelAt(x, y); p.A > 0 {
				ink++
			}
		}
	}
	return ink
}

func main() {
	mgr := graphics.InitFontManager("")
	mgr.LoadSystemFonts()

	fmt.Println("-- FontManager resolution paths --")
	for _, fam := range []string{"", "Arial", "Microsoft YaHei", "SimSun", "Segoe UI", "serif", "monospace"} {
		fmt.Printf("  family=%-18q cjk_ink=%d\n", fam, canvasInk(fam))
	}

	// Register raw data faces under distinct families to test the data path.
	register := func(family, path string, idx int) {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("  read %s: %v\n", path, err)
			return
		}
		if err := mgr.RegisterCustomFont(family, raw, idx); err != nil {
			fmt.Printf("  register %s: %v\n", family, err)
			return
		}
		fmt.Printf("  registered %-12s cjk_ink=%d\n", family, canvasInk(family))
	}
	register("msyhdata0", `C:\Windows\Fonts\msyh.ttc`, 0)
	register("msyhdata1", `C:\Windows\Fonts\msyh.ttc`, 1)
	register("simsundata0", `C:\Windows\Fonts\simsun.ttc`, 0)
	register("simsundata1", `C:\Windows\Fonts\simsun.ttc`, 1)
}
