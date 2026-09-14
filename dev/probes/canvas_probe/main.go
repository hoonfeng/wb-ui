// Command canvas_probe — WebView 级 canvas 2D 端到端冒烟：
// LoadHTML（JS 绘 canvas）→ Render → 像素断言（PaintCanvas 管线验证）。
package main

import (
	"fmt"
	"os"

	"wb-ui/webkit"
)

func main() {
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(320, 240)

	htmlSrc := `<html><body style="margin:0">
<canvas id="cv" width="200" height="100" style="display:block"></canvas>
<script>
var c = document.getElementById('cv');
var ctx = c.getContext('2d');
ctx.fillStyle = '#ff0000';
ctx.fillRect(0, 0, 100, 100);
ctx.fillStyle = '#00ff00';
ctx.fillRect(100, 0, 100, 50);
ctx.font = '16px sans-serif';
ctx.fillStyle = '#0000ff';
ctx.fillText('WB', 10, 80);
</script></body></html>`
	if err := wv.LoadHTML(htmlSrc); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML: %v\n", err)
		os.Exit(1)
	}
	pix, err := wv.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Render: %v\n", err)
		os.Exit(1)
	}
	W, H := wv.Width(), wv.Height()
	if len(pix) != W*H*4 {
		fmt.Fprintf(os.Stderr, "pixels len=%d want %d\n", len(pix), W*H*4)
		os.Exit(1)
	}
	at := func(x, y int) (r, g, b, a byte) {
		i := (y*W + x) * 4
		return pix[i], pix[i+1], pix[i+2], pix[i+3]
	}
	r, g, b, a := at(50, 50) // canvas 左上红色区
	if !(r > 200 && g < 60 && a > 200) {
		fmt.Fprintf(os.Stderr, "FAIL red px (50,50)=#%02x%02x%02x%02x want ~red\n", r, g, b, a)
		os.Exit(1)
	}
	r, g, b, a = at(150, 25) // 绿色区
	if !(g > 200 && r < 60) {
		fmt.Fprintf(os.Stderr, "FAIL green px (150,25)=#%02x%02x%02x%02x want ~green\n", r, g, b, a)
		os.Exit(1)
	}
	if os.Getenv("WB_CANVAS_PROBE_DUMP") != "" {
		// 输出一行颜色采样便于人工核对
		for _, p := range [][2]int{{50, 50}, {150, 25}, {20, 85}, {250, 20}, {320 - 5, 240 - 5}} {
			r, g, b, a := at(p[0], p[1])
			fmt.Printf("px(%d,%d)=#%02x%02x%02x%02x\n", p[0], p[1], r, g, b, a)
		}
	}
	fmt.Println("CANVAS PROBE OK: canvas 2D draws through WebView paint pipeline")
}
