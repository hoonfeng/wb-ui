// fontprobe：跨平台字体/CJK 渲染自检探针。
//
// 目的：
//  1. 确认 FontManager 在当前平台（Windows/macOS/Linux）能拿到真正的 CJK
//     Typeface（CJKTypeface() != nil），而不是只剩无 CJK 覆盖的西文字体；
//  2. 确认「中文测试」四个字在 CJK Typeface 里有真实字形（非 .notdef=0）；
//  3. 打印实际选中的 CJK 族名（供 CI grep `[fontmgr] cjk=<族名>`）与
//     中文文本的度量宽度（0 宽度说明字体没加载上）。
//
// 运行（需 CGO）：
//
//	set CGO_ENABLED=1 && set PATH=F:\syproject\goskia\bin;%PATH%
//	go run ./dev/probes/fontprobe
//
// 退出码：0 = 全部通过；1 = 有断言失败（CI 直接判红）。
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/hoonfeng/goskia/skia"

	"wb-ui/engine/platform/graphics"
)

const sample = "中文测试"

func main() {
	fail := false
	fmt.Printf("[fontprobe] GOOS=%s GOARCH=%s\n", runtime.GOOS, runtime.GOARCH)

	// 1) 初始化 + 加载系统字体（与真实运行时同一路径）。
	mgr := graphics.InitFontManager("")
	if mgr == nil {
		fmt.Println("FAIL: InitFontManager 返回 nil")
		os.Exit(1)
	}
	mgr.LoadSystemFonts()

	// 2) CJK 默认 Typeface 必须存在。
	cjk := mgr.CJKTypeface()
	if cjk == nil {
		fmt.Println("FAIL: CJKTypeface() == nil（没有可用 CJK 字体 → 中文必然方块）")
		fail = true
	} else {
		fmt.Printf("[fontprobe] CJKTypeface != nil ✓\n")
		// 3) 逐字检查字形 id：0 表示 .notdef（豆腐块）。
		for _, r := range sample {
			g := cjk.UnicharToGlyph(r)
			status := "✓"
			if g == 0 {
				status = "✗ .notdef"
				fail = true
			}
			fmt.Printf("[fontprobe]   U+%04X %c -> glyph %d %s\n", r, r, g, status)
		}
	}

	// 4) 文本度量：宽度 <= 0 说明字体没参与排版。
	w := graphics.MeasureText(graphics.Font{Family: "sans-serif", Size: 16, Weight: 400, Style: "normal"}, sample)
	fmt.Printf("[fontprobe] MeasureText(16px, %q) = %.2f\n", sample, w)
	if w <= 0 {
		fmt.Println("FAIL: 中文文本宽度 <= 0（字体未生效）")
		fail = true
	}

	// 5) 拉丁字体 + 中文（真实 UI 的组合）：Arial/DejaVu 等无 CJK 字形，
	//    绘制层必须走 CJKTypeface 回退；这里验证回退目标本身可用。
	fallback := mgr.LookupTypeface("sans-serif", 400, "normal")
	if fallback == nil {
		fmt.Println("WARN: LookupTypeface(sans-serif) == nil")
	} else {
		hasCJK := false
		for _, r := range sample {
			if fallback.UnicharToGlyph(r) != 0 {
				hasCJK = true
				break
			}
		}
		fmt.Printf("[fontprobe] sans-serif fallback hasCJK=%v\n", hasCJK)
	}

	// 6) TTC 多 face 能力自检：同一次读取用 index 0/1 构造两个 Typeface，
	//    字符覆盖/字重若不同则说明 sk_typeface_create_from_data 的 index 生效
	//    （Linux/macOS 的 NotoSansCJK*.ttc 靠它拿到 SC face）。
	checkTTCIndex()

	if fail {
		fmt.Println("[fontprobe] RESULT=FAIL")
		os.Exit(1)
	}
	fmt.Println("[fontprobe] RESULT=PASS")
}

// checkTTCIndex 打印本机可用 TTC 的 index 0/1 对照（尽力而为，不判失败）。
func checkTTCIndex() {
	paths := []string{
		`C:\Windows\Fonts\simsun.ttc`,
		`C:\Windows\Fonts\msyh.ttc`,
		"/System/Library/Fonts/PingFang.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		tf0 := skia.NewTypefaceFromData(data, 0)
		tf1 := skia.NewTypefaceFromData(data, 1)
		g0, g1 := uint16(0), uint16(0)
		if tf0 != nil {
			g0 = tf0.UnicharToGlyph('中')
		}
		if tf1 != nil {
			g1 = tf1.UnicharToGlyph('中')
		}
		fmt.Printf("[fontprobe] TTC %s: index0!=nil=%v(glyph中=%d) index1!=nil=%v(glyph中=%d)\n",
			p, tf0 != nil, g0, tf1 != nil, g1)
		return
	}
	fmt.Println("[fontprobe] TTC 对照：本机未找到已知 TTC 路径（跳过）")
}
