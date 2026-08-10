package graphics

import (
	"fmt"
	"testing"
)

// TestDiagCJKMeasure 验证中文字符的 MeasureText 宽度（行3 注释「释」被推进
// 89px 疑测量错误——中英混排对齐异常根因）。
func TestDiagCJKMeasure(t *testing.T) {
	mgr := GetFontManager()
	if mgr == nil {
		_ = InitFontManager("")
		mgr = GetFontManager()
	}
	if mgr == nil {
		t.Skip("no font manager")
	}
	mgr.LoadSystemFonts()
	// 模拟 CM6 编辑器的 font-family（JetBrains Mono 系）
	f := Font{Family: `"JetBrains Mono", "Cascadia Code", "Fira Code", monospace`, Size: 13}
	for _, r := range []rune{'函', '数', '处', '理', '中', '文', '注', '释', '测', '试', 'f', 'n', '1'} {
		w := MeasureText(f, string(r))
		fmt.Printf("MeasureText(%q) = %.2f\n", r, w)
	}
	// 对比 CJK fallback 字体（雅黑）
	f2 := Font{Family: "Microsoft YaHei", Size: 13}
	fmt.Println("--- 雅黑 ---")
	for _, r := range []rune{'释', '测', '试', '函'} {
		w := MeasureText(f2, string(r))
		fmt.Printf("MeasureText(%q) = %.2f\n", r, w)
	}
}
