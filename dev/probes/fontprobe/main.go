// fontprobe：D7「默认族」验收用的字体可得性/度量探针。
//
// 目的：确认 wb-ui 引擎能否拿到浏览器基准（Edge）实际使用的默认族
// 「Noto Sans SC」，以及其度量是否与 Edge 一致（24px 下 10 个 M = 194.88）。
//
// 运行（需 CGO）：
//
//	set CGO_ENABLED=1 && set PATH=F:\syproject\goskia\bin;%PATH%
//	go run ./dev/probes/fontprobe
package main

import (
	"os"

	"fmt"

	"github.com/hoonfeng/goskia/skia"

	"wb-ui/engine/platform/graphics"
)

func main() {
	graphics.InitFontManager("")

	fmt.Println("=== skia.NewTypeface（OS 名查找）===")
	for _, n := range []string{
		"Noto Sans SC", "NotoSansSC", "Noto Sans", "Arial",
		"Microsoft YaHei", "SimSun", "Times New Roman", "Consolas",
	} {
		tf := skia.NewTypeface(n, skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
		fmt.Printf("  NewTypeface(%-18q) != nil : %v\n", n, tf != nil)
	}

	fmt.Println("=== MeasureText(24px, \"MMMMMMMMMM\") —— Edge 基准：Noto Sans SC=194.88, Arial=199.92, YaHei=234.49, serif=234 ===")
	for _, fam := range []string{
		"", "Noto Sans SC", "sans-serif", "ui-sans-serif", "system-ui",
		"serif", "Arial", "Microsoft YaHei", "monospace",
	} {
		w := graphics.MeasureText(graphics.Font{Family: fam, Size: 24, Weight: 400, Style: "normal"}, "MMMMMMMMMM")
		fmt.Printf("  MeasureText(%-16q) = %.2f\n", fam, w)
	}

	fmt.Println("=== 直接加载 C:\\Windows\\Fonts\\NotoSansSC-VF.ttf ===")
	data, err := os.ReadFile(`C:\Windows\Fonts\NotoSansSC-VF.ttf`)
	if err != nil {
		fmt.Println("  read failed:", err)
		return
	}
	fmt.Printf("  file size = %d bytes\n", len(data))
	tf := skia.NewTypefaceFromData(data, 0)
	fmt.Printf("  NewTypefaceFromData != nil : %v\n", tf != nil)
}
