//go:build ignore

// xheight_probe 验证字体的 x-height 度量（ex 单位的解析基础），
// 并对比「未初始化字体管理器」（cssprobe 的情形）与「已初始化」的差别。
package main

import (
	"fmt"

	"wb-ui/platform/graphics"
)

func printMetrics(tag string) {
	for _, fam := range []string{"Arial", "Arial, sans-serif", "Arial, Helvetica, sans-serif", "sans-serif", ""} {
		f := graphics.Font{Family: fam, Size: 16}
		fmt.Printf("[%s] %-16q ascent=%6.2f descent=%6.2f lineGap=%5.2f xheight=%6.2f\n",
			tag, fam,
			graphics.GlobalFontAscent(f),
			graphics.GlobalFontDescent(f),
			graphics.GlobalFontLineGap(f),
			graphics.GlobalFontXHeight(f))
	}
}

func main() {
	printMetrics("no-mgr")
	_ = graphics.InitFontManager(`C:\Windows\Fonts`)
	if mgr := graphics.GetFontManager(); mgr != nil {
		mgr.LoadSystemFonts()
	}
	printMetrics("mgr")
}
