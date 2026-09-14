// Command uitoolkit —— 把 wb-ui 当「UI 库」用的最小示例（不需要窗口）。
//
// 与 examples/minibrowser（嵌入浏览器：加载 URL/HTML、页面脚本驱动）对照：
//   - 这里不写任何页面 HTML，界面完全由 Go 代码构建（基础方式）
//   - 同一棵树里混入一段 HTML 片段（web 方式），由引擎解析后接入
//   - 组件注册表里同时有「基础（native）」与「web」两种来源的组件
//   - 事件回调写在 Go 里（点击按钮 → Go 函数被调用）
//   - 最后渲染成 PNG，宿主拿到的就是 RGBA 像素
//
// 运行（需要 CGO + Skia DLL，见 Makefile）：
//
//	go run ./examples/uitoolkit
//	go run ./examples/uitoolkit -mode browser -out out.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"wb-ui/engine/dom"
	"wb-ui/ui"
	"wb-ui/webkit"
)

const demoStyles = `
#panel{width:420px;background:#1e2228;color:#e6e6e6;
	font-family:'Segoe UI','Microsoft YaHei',sans-serif;padding:12px;box-sizing:border-box}
.title{font-size:15px;font-weight:600;margin-bottom:10px}
.row{display:flex;align-items:center;gap:8px;margin-bottom:8px}
.hint{color:#9aa4b2;font-size:12px}
button{padding:4px 10px;font-size:12px;background:#2d7ef7;color:#fff;border:0;border-radius:4px}
button.wbtn{background:#3aa76d}
.badge{display:inline-block;padding:1px 6px;border-radius:9px;background:#2d7ef7;color:#fff;font-size:11px}
.chip{display:inline-block;padding:1px 6px;border-radius:9px;background:#3aa76d;color:#fff;font-size:11px}
`

func main() {
	modeFlag := flag.String("mode", "toolkit", "运行模式：toolkit（UI 库，默认）| browser（嵌入浏览器）")
	out := flag.String("out", "_temp/ui-toolkit-demo.png", "PNG 输出路径（空字符串 = 不写文件）")
	flag.Parse()

	mode := webkit.ModeToolkit
	switch *modeFlag {
	case "toolkit":
		mode = webkit.ModeToolkit
	case "browser":
		mode = webkit.ModeBrowser
	default:
		fatalf("未知模式 %q（可选 toolkit / browser）", *modeFlag)
	}

	wv := webkit.NewWebViewWithMode(mode)
	defer wv.Destroy()
	wv.Resize(420, 200)

	// ── 1. 建立 UI 库视图（宿主不写 HTML）────────────────────
	view, err := ui.New(wv)
	if err != nil {
		fatalf("ui.New: %v", err)
	}
	view.Style(demoStyles)

	// ── 2. 基础方式：Go 构建界面 ────────────────────────────
	clicks := 0
	view.Div().ID("panel").Append(
		view.Div().Class("title").Text("wb-ui · UI 库模式（"+mode.String()+"）"),
		view.Div().Class("row").
			Append(
				view.Button("Go 按钮", func(dom.Event) { clicks++ }).ID("go-btn"),
				view.Span().Class("hint").Text("点击回调在 Go 里（clicks 计数）"),
			),
	)

	// ── 3. web 方式：同一棵树里混入 HTML 片段 ───────────────
	view.Web(`<div class="row" id="web-row">` +
		`<button id="web-btn" class="wbtn">web 按钮</button>` +
		`<span class="hint">（HTML 片段，由引擎解析后接入同一棵树）</span>` +
		`</div>`)

	// ── 4. 组件来源可切换：基础方式 + web 方式 ──────────────
	reg := view.Registry()
	if err := reg.RegisterNative("badge", func(v *ui.View, p ui.Props) *ui.Node {
		// 基础方式：Go 逐节点构建（无 HTML 解析）
		return v.El("span").Class("badge").Text(p.String("text"))
	}); err != nil {
		fatalf("register badge: %v", err)
	}
	if err := reg.RegisterWeb("chip", func(p ui.Props) string {
		// web 方式：返回 HTML 片段，由引擎解析
		return `<span class="chip">` + p.String("text") + `</span>`
	}); err != nil {
		fatalf("register chip: %v", err)
	}
	row := view.Div().Class("row")
	for _, name := range []string{"badge", "chip"} {
		node, err := row.Mount(name, ui.Props{"text": name})
		if err != nil {
			fatalf("mount %s: %v", name, err)
		}
		src, _ := reg.Source(name)
		fmt.Printf("组件 %-6s 来源=%s 节点=%s\n", name, src, node.Element().TagName())
	}

	// ── 5. 事件：模拟一次点击（引擎命中测试 → Go 回调）──────
	wv.EnsureHitTestReady()
	simulateClick(wv, "go-btn")
	fmt.Printf("Go 按钮点击回调次数 = %d\n", clicks)
	if clicks != 1 {
		fatalf("点击回调未生效（clicks=%d）", clicks)
	}

	// ── 6. 模式能力面（两种模式的差异直接可见）──────────────
	for _, probe := range []string{"fetch", "XMLHttpRequest", "Worker", "WebSocket", "document"} {
		fmt.Printf("typeof %-16s = %s\n", probe, evalString(wv, "typeof "+probe))
	}

	// ── 7. 渲染输出 ─────────────────────────────────────────
	cssW := evalString(wv, `String(document.getElementById("go-btn").getBoundingClientRect().width)`)
	fmt.Printf("Go 按钮渲染宽度 = %s px\n", cssW)

	pixels, err := wv.Render()
	if err != nil {
		fatalf("Render: %v", err)
	}
	if *out != "" {
		// 默认写到 _temp/（.gitignore 已忽略）：示例不该往仓库根丢产物。
		if dir := filepath.Dir(*out); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "创建输出目录失败：%v\n", err)
				os.Exit(1)
			}
		}
		if err := writePNG(*out, wv.Width(), wv.Height(), pixels); err != nil {
			fatalf("写 PNG: %v", err)
		}
		fmt.Printf("已写出 %s（%dx%d）\n", *out, wv.Width(), wv.Height())
	}
}

// simulateClick 在元素中心模拟一次按下+释放（无窗口宿主的交互入口）。
func simulateClick(wv *webkit.WebView, id string) {
	raw := evalString(wv, `(function(){var el=document.getElementById("`+id+`");`+
		`if(!el)return"";var r=el.getBoundingClientRect();`+
		`return (r.left+r.width/2)+","+(r.top+r.height/2);})()`)
	var x, y float64
	if _, err := fmt.Sscanf(raw, "%f,%f", &x, &y); err != nil {
		fatalf("取 #%s 中心失败：%q", id, raw)
	}
	wv.HandleMouseMove(x, y)
	wv.HandleMouseButton(x, y, 0, 0)
	wv.HandleMouseButton(x, y, 0, 1)
}

func evalString(wv *webkit.WebView, script string) string {
	val, err := wv.EvalJS(script)
	if err != nil {
		fatalf("EvalJS(%q): %v", script, err)
	}
	return val.ToString()
}

// writePNG 把引擎的 RGBA8888 像素（预乘 alpha）写成 PNG（非预乘）。
func writePNG(path string, w, h int, pixels []byte) error {
	if len(pixels) < w*h*4 {
		return fmt.Errorf("像素缓冲 %d 字节，小于 %dx%d", len(pixels), w, h)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		if a == 0 || a == 255 {
			continue
		}
		// 反预乘：源是预乘 RGBA，PNG 要求非预乘
		for c := 0; c < 3; c++ {
			v := int(img.Pix[i+c]) * 255 / int(a)
			if v > 255 {
				v = 255
			}
			img.Pix[i+c] = uint8(v)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "uitoolkit: "+format+"\n", args...)
	os.Exit(1)
}
