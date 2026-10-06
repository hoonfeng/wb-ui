// Command webshot 是「任意页面 → 位图」的通用无头渲染器，用于视觉自查与
// 与真实浏览器的像素对照。
//
// 与 pixel_probe（只跑 HTML/CSS 不跑 JS 的布局像素探针）的差别：本工具走
// webkit.WebView 完整管线（HTML 解析 → 样式 → 布局 → JS 执行 → 绘制），
// 因而能做：
//   - 加载本地 HTML（相对资源按文档目录解析：同目录 <script src>/<link> 可引用）
//   - 加载后执行 JS 并读回表达式（-js / -eval）
//   - 多帧驱动（事件循环 + rAF/定时器 + 重排重绘），把动画渲染成序列
//   - 由多帧合成 GIF（-gif）或逐帧 PNG（-framedir），肉眼自查时序
//
// 用法（仓库根，CGO 环境）：
//
//	go run ./dev/probes/webshot -html page.html -out dev/output/page.png
//	go run ./dev/probes/webshot -html anim.html -frames 24 -step 16 \
//	    -gif dev/output/anim.gif -framedir dev/output/anim
//	go run ./dev/probes/webshot -html app.html -js "document.title" -eval "document.title"
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

func main() {
	htmlPath := flag.String("html", "", "HTML 文件路径（与 -url 二选一）")
	urlArg := flag.String("url", "", "直接加载的 URL（http(s)/file/data）")
	w := flag.Int("w", 1280, "视口宽")
	h := flag.Int("h", 800, "视口高")
	out := flag.String("out", "", "PNG 输出路径（默认取最后一帧）")
	frames := flag.Int("frames", 1, "渲染帧数（>1 用于动画/异步）")
	step := flag.Int("step", 0, "帧间等待毫秒（真实时间，供 setTimeout/transition 推进）")
	framedir := flag.String("framedir", "", "逐帧 PNG 输出目录（配合 -frames）")
	gifPath := flag.String("gif", "", "多帧合成 GIF 输出路径")
	gifDelay := flag.Int("gifdelay", 8, "GIF 每帧延迟（单位 1/100 秒）")
	jsCode := flag.String("js", "", "加载完成后执行的 JS（只执行一次）")
	evalExpr := flag.String("eval", "", "每帧渲染后读回并打印的 JS 表达式")
	waitMS := flag.Int("wait", 0, "加载完成后等待毫秒（给定时器/Promise 落地时间）")
	transparent := flag.Bool("transparent", false, "保留透明通道（默认合成到白底）")
	scale := flag.Int("scale", 1, "整数倍放大输出（便于肉眼检查细节）")
	quiet := flag.Bool("quiet", false, "只输出错误")
	flag.Parse()

	fail := func(format string, a ...any) {
		fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", a...)
		os.Exit(1)
	}
	if *htmlPath == "" && *urlArg == "" {
		fail("必须指定 -html 或 -url")
	}
	if *frames < 1 {
		*frames = 1
	}

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	logger := &jsc.BufferLogger{}
	wv.SetConsoleLogger(logger)

	if *htmlPath != "" {
		data, err := os.ReadFile(*htmlPath)
		if err != nil {
			fail("读取 HTML: %v", err)
		}
		abs, _ := filepath.Abs(*htmlPath)
		base := "file:///" + strings.ReplaceAll(filepath.ToSlash(filepath.Dir(abs)), " ", "%20") + "/"
		if err := wv.LoadHTMLWithBaseURL(string(data), base); err != nil {
			fail("LoadHTML: %v", err)
		}
	} else {
		if err := wv.LoadURL(*urlArg); err != nil {
			fail("LoadURL: %v", err)
		}
	}

	if *waitMS > 0 {
		time.Sleep(time.Duration(*waitMS) * time.Millisecond)
		pumpFrames(wv, 4, 0)
	}

	if *jsCode != "" {
		v, err := wv.EvalJS(*jsCode)
		if err != nil {
			fmt.Printf("[js] ERROR: %v\n", err)
		} else if !*quiet {
			fmt.Printf("[js] => %s\n", summarizeJS(v.ToString()))
		}
	}

	animStart = time.Now()
	var frameImages []*image.RGBA
	for i := 0; i < *frames; i++ {
		if *step > 0 && i > 0 {
			time.Sleep(time.Duration(*step) * time.Millisecond)
		}
		pumpFrames(wv, 1, 0)
		pix, err := wv.Render()
		if err != nil {
			fail("Render 第 %d 帧: %v", i, err)
		}
		img := composite(pix, *w, *h, *transparent, *scale)
		frameImages = append(frameImages, img)
		if *framedir != "" {
			p := filepath.Join(*framedir, fmt.Sprintf("frame-%03d.png", i))
			if err := writePNG(img, p); err != nil {
				fail("写帧 PNG: %v", err)
			}
		}
		if *evalExpr != "" {
			if v, err := wv.EvalJS(*evalExpr); err == nil {
				if !*quiet {
					fmt.Printf("[frame %d] %s => %s\n", i, *evalExpr, summarizeJS(v.ToString()))
				}
			} else if !*quiet {
				fmt.Printf("[frame %d] eval ERROR: %v\n", i, err)
			}
		}
	}

	if *gifPath != "" {
		if err := writeGIF(frameImages, *gifPath, *gifDelay); err != nil {
			fail("写 GIF: %v", err)
		}
		if !*quiet {
			fmt.Printf("GIF %s（%d 帧，%dx%d，delay=%d）\n", *gifPath, len(frameImages), (*w)*(*scale), (*h)*(*scale), *gifDelay)
		}
	}
	if *out != "" && len(frameImages) > 0 {
		if err := writePNG(frameImages[len(frameImages)-1], *out); err != nil {
			fail("写 PNG: %v", err)
		}
		if !*quiet {
			fmt.Printf("PNG %s（%dx%d）\n", *out, (*w)*(*scale), (*h)*(*scale))
		}
	}

	if s := strings.TrimSpace(logger.String()); s != "" && !*quiet {
		fmt.Println("--- console ---")
		fmt.Println(s)
	}
}

// animStart 是本工具充当 embedder 时的动画时钟起点。
//
// ★ 引擎把 @keyframes / transition 的补间放在**绘制期**按
// rendering.AnimationTime（全局秒）计算（engine/rendering/animation.go），
// 且必须由宿主每帧调用 rendering.ApplyAnimations(rv) 才会生效
// （app/host.go:1445-1460）。不驱动这两者时，所有 CSS 动画都停在 0%
// （实测：红色 @keyframes 块 24 帧纹丝不动、transition 直接跳变）——
// 那是宿主侧缺失，不是引擎缺陷。
var animStart = time.Now()

// pumpFrames 驱动 n 轮「事件循环 → 微任务 → 动画 → 布局 → 渲染」。
// WebView 的定时器/微任务不会自己跑，必须显式推进（与真实宿主的帧回调等价）。
func pumpFrames(wv *webkit.WebView, n int, gap time.Duration) {
	for i := 0; i < n; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		// 动画驱动（与 app.Host 的帧循环同序）：推进时钟 → 应用补间 →
		// 影响布局的动画需要 relayout。
		rendering.AnimationTime = time.Since(animStart).Seconds()
		if rv := wv.RenderView(); rv != nil {
			if rendering.ApplyAnimations(rv) && rendering.AnimationsAffectLayout(rv) {
				if mf := wv.MainFrame(); mf != nil {
					if fr := mf.Frame(); fr != nil {
						fr.SetNeedsLayout(true)
					}
				}
			}
		}
		wv.EnsureLayout()
		rendering.AnimationTime = time.Since(animStart).Seconds()
		_, _ = wv.Render()
		if gap > 0 {
			time.Sleep(gap)
		}
	}
}

// composite 把渲染缓冲（预乘 RGBA）转成可直接看的图像：
// 默认合成到白底（页面的默认背景），-transparent 时还原为非预乘直通 alpha。
func composite(pix []byte, w, h int, transparent bool, scale int) *image.RGBA {
	ow, oh := w*scale, h*scale
	img := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			r, g, b, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
			var c color.RGBA
			if transparent {
				// 还原非预乘：C = C_pre * 255 / A
				if a == 0 {
					c = color.RGBA{}
				} else {
					c = color.RGBA{
						R: uint8(int(r) * 255 / int(a)),
						G: uint8(int(g) * 255 / int(a)),
						B: uint8(int(b) * 255 / int(a)),
						A: a,
					}
				}
			} else {
				// 白底合成：C + 255*(1-a)
				inv := 255 - int(a)
				c = color.RGBA{
					R: uint8(min(255, int(r)+inv)),
					G: uint8(min(255, int(g)+inv)),
					B: uint8(min(255, int(b)+inv)),
					A: 255,
				}
			}
			if scale == 1 {
				img.SetRGBA(x, y, c)
			} else {
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						img.SetRGBA(x*scale+dx, y*scale+dy, c)
					}
				}
			}
		}
	}
	return img
}

func writePNG(img *image.RGBA, path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writeGIF 把帧序列编码成 GIF（逐帧 256 色调色板量化：无需外部依赖）。
func writeGIF(frames []*image.RGBA, path string, delay int) error {
	if len(frames) == 0 {
		return fmt.Errorf("无帧")
	}
	if delay <= 0 {
		delay = 8
	}
	g := &gif.GIF{LoopCount: 0}
	for _, f := range frames {
		p := image.NewPaletted(f.Bounds(), palette256(f))
		for y := f.Bounds().Min.Y; y < f.Bounds().Max.Y; y++ {
			for x := f.Bounds().Min.X; x < f.Bounds().Max.X; x++ {
				p.Set(x, y, f.At(x, y))
			}
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, delay)
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return gif.EncodeAll(fh, g)
}

// palette256 用 6x7x6 的均匀 Web 风格调色板近似（够了：自查看的是结构与时序，
// 不是颜色精度；GIF 本身也只有 256 色）。
func palette256(img *image.RGBA) color.Palette {
	p := make(color.Palette, 0, 256)
	steps := []int{0, 51, 102, 153, 204, 255}
	for _, r := range steps {
		for _, g := range steps {
			for _, b := range steps {
				if len(p) >= 255 {
					break
				}
				p = append(p, color.RGBA{uint8(r), uint8(g), uint8(b), 255})
			}
		}
	}
	p = append(p, color.RGBA{0, 0, 0, 0})
	return p
}

func summarizeJS(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
