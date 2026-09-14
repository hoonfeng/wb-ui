// Command fontmetric 打印注入布局引擎的字体度量（ascent/descent/lineGap）
// 及它们构成的行高，用于与真实浏览器对照：font-metric-line-height 夹具
// 的期望值就是浏览器 grid-fit（每个度量四舍五入到整数像素）后的行高，
// 例如 Arial 12px → round(11.13)+round(2.55) = 14（而不是 13.68）。
//
// 用法：
//
//	go run ./dev/probes/fontmetric            # 默认字族/字号矩阵
//	go run ./dev/probes/fontmetric Arial 12   # 指定字族与字号
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"wb-ui/engine/platform/graphics"
)

func main() {
	families := []string{"Arial", "sans-serif", "Times New Roman", "serif", "Courier New", "monospace", ""}
	sizes := []float64{9.3333, 9, 12, 13, 16, 20}
	if len(os.Args) >= 3 {
		families = []string{os.Args[1]}
		if v, err := strconv.ParseFloat(os.Args[2], 64); err == nil {
			sizes = []float64{v}
		}
	}
	fmt.Printf("%-16s %9s %9s %9s %9s %9s %9s\n", "family", "size", "ascent", "descent", "lineGap", "sum", "gridfit")
	for _, fam := range families {
		for _, size := range sizes {
			f := graphics.Font{Family: fam, Size: size}
			a := graphics.GlobalFontAscent(f)
			d := graphics.GlobalFontDescent(f)
			lg := graphics.GlobalFontLineGap(f)
			fit := math.Round(a) + math.Round(d) + math.Round(lg)
			fmt.Printf("%-16s %9.4f %9.3f %9.3f %9.3f %9.3f %9.3f\n", fam, size, a, d, lg, a+d+lg, fit)
		}
	}
}
