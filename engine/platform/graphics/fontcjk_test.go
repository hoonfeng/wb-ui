package graphics

import (
	"runtime"
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestCJKTypefaceAvailable 是「多平台中文字体」的防回归断言：
//
//  1. FontManager 在当前平台必须能选出 CJK Typeface（CJKTypeface() != nil）；
//  2. 「中文测试」四个字在该 Typeface 里必须有真实字形（glyph id != 0，
//     0 就是 .notdef = 豆腐块）；
//  3. 日志里的族名非空（release.yml 会 grep `[fontmgr] cjk=`）。
//
// 旧实现在 macOS/Linux 上会把 cjkTF 退化成 Arial / DejaVu Sans（无 CJK
// 覆盖），这三个断言全部失败 —— 正是 CI headless 截图里满屏方块的原因。
func TestCJKTypefaceAvailable(t *testing.T) {
	mgr := InitFontManager("")
	if mgr == nil {
		t.Fatal("InitFontManager 返回 nil")
	}
	mgr.LoadSystemFonts()

	cjk := mgr.CJKTypeface()
	if cjk == nil {
		t.Fatalf("GOOS=%s 上 CJKTypeface() == nil：没有可用中文字体（中文必然渲染成方块）", runtime.GOOS)
	}
	for _, r := range "中文测试" {
		if g := cjk.UnicharToGlyph(r); g == 0 {
			t.Fatalf("GOOS=%s 的 CJK 字体缺字形 U+%04X %c（.notdef → 豆腐块）", runtime.GOOS, r, r)
		}
	}
	if fam := mgr.CJKFamilyName(); fam == "" || fam == "(none)" {
		t.Fatalf("CJK 族名为空（%q）：CI 无法断言字体落点", fam)
	} else {
		t.Logf("GOOS=%s CJK 字体 = %q ✓（中文测试 4 字形均有覆盖）", runtime.GOOS, fam)
	}
}

// TestCJKTypefaceDoesNotFallBackToWestern 回归「cjkTF 回落西文字体」：
// 没有 CJK 覆盖的 sans 字体（Arial / DejaVu Sans）绝不能被 CJKTypeface
// 当成中文回退，否则中文全部是方块。旧实现无条件 `return m.sansTF`。
func TestCJKTypefaceDoesNotFallBackToWestern(t *testing.T) {
	western := skia.NewTypeface("Arial", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	if western == nil {
		t.Skip("本机无 Arial，跳过")
	}
	if isCJKCapable(western) {
		t.Skip("Arial 覆盖中文（异常环境），跳过")
	}
	m := &FontManager{}
	m.sansTF = western
	m.defaultTF = western
	if got := m.CJKTypeface(); got != nil {
		t.Fatalf("无 CJK 字体时 CJKTypeface() 不应回落到西文字体，实得 %v", got)
	}
}

// TestClassifyTTCCJKFace 断言 TTC 合集按 face index 分类：
// Noto CJK 合集的标准顺序是 0=JP 1=KR 2=SC 3=TC 4=HK，
// 直接取 index 0 会让 Linux 上中文用日文字形，且 "noto sans cjk sc"
// 按族名查永远 miss。
func TestClassifyTTCCJKFace(t *testing.T) {
	cases := []struct {
		file  string
		index int
		want  string
	}{
		{"NotoSansCJK-Regular.ttc", 0, "noto sans cjk jp"},
		{"NotoSansCJK-Regular.ttc", 2, "noto sans cjk sc"},
		{"NotoSansCJK-Bold.ttc", 2, "noto sans cjk sc"},
		{"NotoSerifCJK-Regular.ttc", 2, "noto serif cjk sc"},
		{"PingFang.ttc", 0, "pingfang sc"},
		{"Hiragino Sans GB.ttc", 0, "hiragino sans gb"},
		{"STHeiti Light.ttc", 0, "heiti sc"},
		{"Songti.ttc", 0, "songti sc"},
		{"wqy-zenhei.ttc", 0, "wenquanyi zen hei"},
		{"wqy-microhei.ttc", 0, "wenquanyi micro hei"},
	}
	for _, c := range cases {
		got := classifyFont(c.file, c.index, nil)
		if got.family != c.want {
			t.Errorf("classifyFont(%q, %d).family = %q，want %q", c.file, c.index, got.family, c.want)
		}
		if !got.cjk {
			t.Errorf("classifyFont(%q, %d) 未标记 cjk（selectDefaults 不会把它当 CJK 候选）", c.file, c.index)
		}
	}
}

// TestFontWhitelistPlatform 断言白名单按 runtime.GOOS 分表：
// 当前平台的主流 CJK 字体必须放行；Windows 专属字体在非 Windows 上
// 不应被放行（旧实现的 Windows 表会让 Linux/macOS 一个 CJK 都加载不到）。
func TestFontWhitelistPlatform(t *testing.T) {
	expect := map[string][]string{
		"windows": {"msyh.ttc", "msyhbd.ttc", "simsun.ttc", "consola.ttf"},
		"darwin":  {"PingFang.ttc", "Hiragino Sans GB.ttc", "STHeiti Light.ttc", "Arial Unicode.ttf", "Menlo.ttc"},
		"linux":   {"NotoSansCJK-Regular.ttc", "NotoSansCJK-Bold.ttc", "NotoSansSC-VF.ttf", "wqy-zenhei.ttc", "DejaVuSans.ttf", "DejaVuSansMono.ttf", "LiberationSans-Regular.ttf"},
	}
	for _, name := range expect[runtime.GOOS] {
		if !systemFontWhitelist(name) {
			t.Errorf("GOOS=%s 白名单未放行 %q（该平台中文/西文主力字体）", runtime.GOOS, name)
		}
	}
	if runtime.GOOS != "windows" {
		for _, name := range []string{"msyh.ttc", "simsun.ttc", "consola.ttf"} {
			if systemFontWhitelist(name) {
				t.Errorf("GOOS=%s 不应放行 Windows 字体 %q", runtime.GOOS, name)
			}
		}
	}
}

// TestPreferredTTCFacesPickSC 断言 Noto CJK 合集优先选 SC face（index 2），
// 而不是默认 index 0（JP）。
func TestPreferredTTCFacesPickSC(t *testing.T) {
	got := preferredTTCFaces("notosanscjk-regular.ttc")
	if len(got) == 0 || got[0] != 2 {
		t.Fatalf("preferredTTCFaces(notosanscjk) = %v，期望首项为 2（SC face）", got)
	}
	if got := preferredTTCFaces("cambria.ttc"); got != nil {
		t.Fatalf("非已知合集不应有优选顺序，实得 %v", got)
	}
}
