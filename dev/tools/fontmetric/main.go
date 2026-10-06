// fontmetric 打印 Skia 的**水平**字体度量（xMin / xMax / avgCharWidth / maxCharWidth）
// 与若干代表字符的 advance width，用于反推并复核 Chromium 表单控件的固有内容宽公式
// （见 engine/layout/formcontrol.go）。
//
// 背景：WebCore/rendering/RenderTextControlSingleLine.cpp 的
// preferredContentLogicalWidth() 是
//
//	content = ceil(avgCharWidth × size) + (maxCharWidth − avgCharWidth)
//
// platform/graphics/skia/FontSkia.cpp 的 initCharWidths() 把 maxCharWidth 定为
// round(fXMax − fXMin)。avgCharWidth 在 Windows 上来自 GDI TEXTMETRIC.tmAveCharWidth，
// 本机 SkiaSharp 后端未填 fAvgCharWidth（恒 0），故这里同时打印 'x' / '0' / 'M' 的
// advance width，以便定出可用的替代量。
//
// 用法（仓库根）：go run ./dev/tools/fontmetric
//
//	go run ./dev/tools/fontmetric -scan   # 垂直度量扫描（CJK 行盒/基线反推用）
package main

import (
	"flag"
	"fmt"
	"math"

	"github.com/hoonfeng/goskia/skia"
)

func main() {
	scan := flag.Bool("scan", false, "扫描 Arial / Noto Sans SC / monospace 在 8..32px 的垂直度量")
	flag.Parse()
	if *scan {
		fmt.Printf("%-14s %9s %9s %9s %9s %9s %9s\n",
			"family", "size", "asc", "desc", "lead", "lineH", "rAsc")
		for _, fam := range []string{"Arial", "Noto Sans SC", "monospace", "sans-serif"} {
			for _, s := range []float32{8, 10, 12, 13.3333, 14, 16, 20, 24, 32} {
				tf := skia.NewTypeface(fam, skia.FontStyleNormal)
				f := skia.NewFont(tf, s)
				m, _ := f.Metrics()
				asc := float64(-m.Ascent)
				desc := float64(m.Descent)
				lead := float64(m.Leading)
				fmt.Printf("%-14s %9g %9.4f %9.4f %9.4f %9.4f %9.4f\n",
					fam, s, asc, desc, lead, asc+desc+lead, math.Round(asc))
			}
		}
		return
	}
	specs := []struct {
		fam  string
		size float32
	}{
		{"Arial", 13.3333}, {"Arial", 16}, {"Arial", 20},
		{"sans-serif", 16}, {"Noto Sans SC", 16}, {"monospace", 16},
		{"Courier New", 16}, {"Times New Roman", 16}, {"Verdana", 16},
		{"Tahoma", 16}, {"Segoe UI", 16},
	}
	p := skia.NewPaint()
	fmt.Printf("%-16s %9s %9s %10s %9s %9s %9s %9s %9s %9s %9s\n",
		"family", "size", "xMin", "xMax", "xM-xm", "round", "x_w", "zero_w", "M_w", "asc", "desc")
	for _, s := range specs {
		tf := skia.NewTypeface(s.fam, skia.FontStyleNormal)
		f := skia.NewFont(tf, s.size)
		m, _ := f.Metrics()
		xw, _ := f.MeasureText("x", p)
		zw, _ := f.MeasureText("0", p)
		mw, _ := f.MeasureText("M", p)
		fmt.Printf("%-16s %9.4f %9.4f %10.4f %9.4f %9.4f %9.4f %9.4f %9.4f %9.4f %9.4f\n",
			s.fam, s.size, m.XMin, m.XMax, m.XMax-m.XMin,
			math.Round(float64(m.XMax-m.XMin)), xw, zw, mw, -m.Ascent, m.Descent)
	}

	fmt.Printf("\n--- 复算 content = ceil(avg×size) + (round(xMax-xMin) − avg)，avg 取 'x' 宽 ---\n")
	fmt.Printf("%-16s %8s %10s %10s %10s %10s\n", "family", "size", "avg_x", "maxc_r", "w20", "w30")
	for _, s := range specs {
		tf := skia.NewTypeface(s.fam, skia.FontStyleNormal)
		f := skia.NewFont(tf, s.size)
		m, _ := f.Metrics()
		xw, _ := f.MeasureText("x", p)
		avg := float64(xw)
		mx := math.Round(float64(m.XMax - m.XMin))
		w20 := math.Ceil(avg*20) + (mx - avg)
		w30 := math.Ceil(avg*30) + (mx - avg)
		fmt.Printf("%-16s %8g %10.4f %10.4f %10.4f %10.4f\n", s.fam, s.size, avg, mx, w20, w30)
	}
}
