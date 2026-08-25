package graphics

import (
	"testing"
)

// TestTypefaceWeightSystemLookup 回归（2026-08-24，描边锯齿根因）：
// osLookup（skia.NewTypeface 系统名查找）返回的 Typeface 不在 m.fonts
// 加载集合中，TypefaceWeight 曾一律返回 0 → getSkiaFont 把真 Bold
// （weight 700）误判为非粗体 → SetEmbolden(true) 二次合成加粗 →
// Skia 把字形按位图膨胀，-webkit-text-stroke 描边作用在膨胀后的
// 像素轮廓上 → 严重锯齿/碎屑（用户报告「文字描边锯齿严重」）。
// 修复：m.fonts 未命中时回退 Skia 真实 weight（Typeface.Weight）。
func TestTypefaceWeightSystemLookup(t *testing.T) {
	mgr := InitFontManager("")
	if mgr == nil {
		t.Skip("FontManager 不可用")
	}
	mgr.LoadSystemFonts()

	tf := mgr.LookupTypeface("microsoft yahei", 700, "")
	if tf == nil {
		t.Skip("系统无 Microsoft YaHei")
	}
	w := mgr.TypefaceWeight(tf)
	if w < 600 {
		t.Fatalf("LookupTypeface(yahei,700) 应返回真实 bold weight>=600，实得 %d（旧 bug 返回 0 → 误触发 SetEmbolden → 描边锯齿）", w)
	}
	t.Logf("LookupTypeface(yahei,700).TypefaceWeight = %d ✓（不再误触发 embolden）", w)

	// 常规字重对照：400 请求应 < 600
	tfReg := mgr.LookupTypeface("microsoft yahei", 400, "")
	if tfReg != nil {
		wReg := mgr.TypefaceWeight(tfReg)
		if wReg >= 600 {
			t.Fatalf("LookupTypeface(yahei,400) 不应是 bold（weight=%d）", wReg)
		}
	}
}

// TestTypefaceItalicSystemLookup 同理：osLookup 系统斜体字体
// （如 Segoe UI Italic）不在 m.fonts 中，TypefaceIsItalic 曾返回
// false → getSkiaFont 对真实 italic 再 SetSkewX(-0.2) 双重斜切。
func TestTypefaceItalicSystemLookup(t *testing.T) {
	mgr := InitFontManager("")
	if mgr == nil {
		t.Skip("FontManager 不可用")
	}
	mgr.LoadSystemFonts()
	tf := mgr.LookupTypeface("segoe ui", 400, "italic")
	if tf == nil {
		t.Skip("系统无 Segoe UI Italic")
	}
	if !mgr.TypefaceIsItalic(tf) {
		t.Fatalf("LookupTypeface(segoe ui, italic) 应由 Skia 判定为 italic（旧 bug 返回 false → 双重斜切）")
	}
}
