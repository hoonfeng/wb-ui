// Package main 综合测试 wb-ui：创建窗口，加载 HTML 页面，测试：
//  1. CSS 布局（flex / block / inline-block）
//  2. CSS 样式（背景色、边框、圆角、阴影）
//  3. CSS 动画（@keyframes）
//  4. Go 函数（bindings.RegisterGoFunction，JS 中 go.xxx() 调用）
//  5. JS 函数（<script> 中定义，onclick="js:..." 调用）
//  6. 标签直接指定 Go 函数的事件处理（onclick="goHandler"）
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/app"
	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/editor"
	"wb-ui/jsc"
	"wb-ui/layout"
	"wb-ui/markdown"
	"wb-ui/platform/graphics"
	"wb-ui/platform/ime"
	"wb-ui/webkit"
)

// 全局状态
var (
	clickCount = 0
	itemCount  = 0
	jsToggleOn = false
)

// GoHandler 是 onclick 标签直接指定的 Go 事件处理函数签名。
type GoHandler func(doc *dom.Document)

func main() {
	// ===== 0. 加载字体 =====
	fontDir := locateFontDir()
	graphics.InitFontManager(fontDir)
	if mgr := graphics.GetFontManager(); mgr != nil {
		mgr.LoadSystemFonts()
	}

	// 将布局层的文本测量/字体度量 hook 桥接到 Skia，使内联格式化上下文
	// 使用真实字形宽度而非等宽估算。
	layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	// ===== 1. 创建 WebView 并加载 HTML =====
	htmlSrc, htmlErr := loadTestHTML()
	if htmlErr != nil {
		fmt.Fprintf(os.Stderr, "加载 test.html 失败: %v\n", htmlErr)
		os.Exit(1)
	}
	wv := webkit.NewWebView()
	wv.Resize(720, 900)
	if err := wv.LoadHTML(htmlSrc); err != nil {
		fmt.Fprintf(os.Stderr, "LoadHTML 失败: %v\n", err)
		os.Exit(1)
	}

	// ===== 2. 注册自定义元素（编辑器 + Markdown） =====
	editor.RegisterEditorElement()
	markdown.RegisterMarkdownElement()

	// ===== 3. 注册 Go 函数供 JS 调用（go.namespace） =====
	interp := wv.JSInterpreter()
	bindings.RegisterDOMBindings(interp, wv.Document())
	bindings.RegisterGoFunction(interp, "getGreeting", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.StringValue("Hello from Go! 🎉"), nil
	})
	bindings.RegisterGoFunction(interp, "computeProduct", func(args []jsc.JSValue) (jsc.JSValue, error) {
		a, b := 6, 7
		return jsc.NumberValue(float64(a * b)), nil
	})
	bindings.RegisterGoFunction(interp, "getTime", func(args []jsc.JSValue) (jsc.JSValue, error) {
		return jsc.StringValue(fmt.Sprintf("Go 时间: %s", "2026-07-08")), nil
	})

	// ===== 3. 注册 onclick 标签直接指定的 Go 事件处理函数 =====
	handlers := map[string]GoHandler{
		"goIncrement": func(doc *dom.Document) {
			clickCount++
			el := doc.GetElementById("countDisplay")
			if el != nil {
				el.SetTextContent(fmt.Sprintf("Go 计数: %d", clickCount))
			}
		},
		"goReset": func(doc *dom.Document) {
			clickCount = 0
			el := doc.GetElementById("countDisplay")
			if el != nil {
				el.SetTextContent("Go 计数: 0")
			}
		},
		"goToggleBg": func(doc *dom.Document) {
			el := doc.GetElementById("toggleBox")
			if el != nil {
				if clickCount%2 == 0 {
					el.SetAttribute("style", "width:200px;height:60px;border-radius:8px;background-color:#e53935;display:block;color:#fff;text-align:center;line-height:60px;font-size:14px;margin-top:8px;")
				} else {
					el.SetAttribute("style", "width:200px;height:60px;border-radius:8px;background-color:#1a73e8;display:block;color:#fff;text-align:center;line-height:60px;font-size:14px;margin-top:8px;")
				}
			}
			clickCount++
		},
		"goAddItem": func(doc *dom.Document) {
			itemCount++
			list := doc.GetElementById("itemList")
			if list != nil {
				item := doc.CreateElement("div")
				item.SetAttribute("style", "padding:6px 12px;margin:4px 0;background-color:#e8f0fe;border-radius:4px;font-size:13px;color:#333;")
				item.SetTextContent(fmt.Sprintf("Go 创建的列表项 #%d", itemCount))
				list.AppendChild(item)
			}
		},
		"goGetTime": func(doc *dom.Document) {
			el := doc.GetElementById("timeDisplay")
			if el != nil {
				el.SetTextContent("Go 时间: 2026-07-08 (由 Go 事件处理函数更新)")
			}
		},
	}

	// ===== 4. 执行页面中的 <script> =====
	runScripts(wv)
	// 初始化自定义元素（编辑器 + Markdown），替换 <wb-editor>/<wb-markdown> 占位节点。
	doc := wv.Document()
	if doc != nil {
		editor.InitEditorElements(doc)
		markdown.InitMarkdownElements(doc)
	}
	fmt.Println("=== wb-ui 综合测试窗口已启动 ===")
	fmt.Println("测试项: CSS布局 / CSS样式 / CSS动画 / Go函数 / JS函数 / 标签事件处理 / IME输入法 / 代码编辑器 / Markdown渲染")
	fmt.Println("JS 控制台输出:")
	fmt.Println(wv.ConsoleOutput())

	// ===== 5. 创建 Host（封装窗口 + GPU 渲染循环 + 事件分发） =====
	host, err := app.NewHost(wv, 720, 900, "wb-ui 综合测试 - 布局/动画/Go/JS/事件")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 Host 失败: %v\n", err)
		os.Exit(1)
	}
	defer host.Window().Close()

	// ===== 6. 设置点击处理器（Go handler 分发 + IME 焦点管理） =====
	host.SetClickHandler(func(el *dom.Element, onclick string, clickX, clickY float64) {
		// IME 焦点管理：点击输入框 → 聚焦；点击其他位置 → 取消焦点
		if el != nil && el.GetId() == "imeInput" {
			host.FocusElement(el)
			// Position the IME candidate window at the click location (window
			// CSS coordinates); clickY is window-relative, not document-relative.
			host.SetIMECompositionPos(clickX, clickY)
			updateIMEStatus(wv.Document(), "已聚焦，请输入")
			return
		}
		if host.FocusedElement() != nil {
			host.Unfocus()
			updateIMEStatus(wv.Document(), "等待焦点")
		}
		// Go handler 分发
		if onclick == "" {
			return
		}
		name := strings.TrimSuffix(onclick, "()")
		if handler, ok := handlers[name]; ok {
			handler(wv.Document())
		}
	})

	// ===== 7. 设置 IME 处理器（更新状态显示） =====
	host.SetIMEHandler(func(events []ime.Event) {
		if host.FocusedElement() == nil {
			return
		}
		for _, ev := range events {
			switch ev.Kind {
			case ime.EventCompositionUpdate:
				updateIMEStatus(wv.Document(), "组合中: "+ev.Composition)
			case ime.EventCharInput:
				// For <input>/<textarea>, read value attribute; for other elements,
				// read textContent. The host already writes to the right place.
				val := host.FocusedElement().GetAttribute("value")
				if val == "" {
					val = host.FocusedElement().TextContent()
				}
				updateIMEStatus(wv.Document(), "输入: "+val)
			case ime.EventCompositionEnd:
				updateIMEStatus(wv.Document(), "组合结束")
			}
		}
	})

	// ===== 8. 运行主循环（阻塞，直到窗口关闭） =====
	host.Run()
	fmt.Println("窗口已关闭")
}

// runScripts 提取页面中所有 <script> 标签的内容并执行。
func runScripts(wv *webkit.WebView) {
	doc := wv.Document()
	if doc == nil {
		return
	}
	scripts := doc.GetElementsByTagName("script")
	for _, s := range scripts {
		code := s.TextContent()
		if code == "" {
			continue
		}
		_, err := wv.EvalJS(code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "脚本执行错误: %v\n", err)
		}
	}
}

// updateIMEStatus 更新页面上的 IME 状态显示。
func updateIMEStatus(doc *dom.Document, status string) {
	el := doc.GetElementById("imeStatus")
	if el != nil {
		el.SetTextContent("IME 状态: " + status)
	}
}

// locateFontDir 查找 resources/fonts 目录。先尝试可执行文件同目录，再尝试
// 当前工作目录的 resources/fonts，最后尝试项目根目录（wb-ui/resources/fonts）。
func locateFontDir() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "resources", "fonts"),
			filepath.Join(dir, "..", "..", "resources", "fonts"),
		)
	}
	candidates = append(candidates,
		filepath.Join("resources", "fonts"),
		filepath.Join("..", "..", "resources", "fonts"),
		`f:\syproject\wb-ui\resources\fonts`,
	)
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return filepath.Join("resources", "fonts")
}

// loadTestHTML 查找并读取 test.html 文件。查找顺序：
//  1. 可执行文件同目录
//  2. 当前工作目录
//  3. 项目源码目录（examples/comprehensive_test/test.html）
func loadTestHTML() (string, error) {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "test.html"))
	}
	candidates = append(candidates,
		filepath.Join("test.html"),
		filepath.Join("examples", "comprehensive_test", "test.html"),
		`f:\syproject\wb-ui\examples\comprehensive_test\test.html`,
	)
	for _, c := range candidates {
		if data, err := os.ReadFile(c); err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("test.html 未找到，已搜索: %v", candidates)
}
