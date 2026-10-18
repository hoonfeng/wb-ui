// Translation of: Source/WebCore/platform/graphics/FontCache.h
//                  Source/WebCore/platform/graphics/FontCache.cpp
// Completeness: 80%
//
// FontCache (FontManager) manages Typefaces loaded from font files on disk.
// In real WebKit the platform FontCache queries the OS font manager and
// handles cross-process font registration; this port loads a fixed set of
// .ttf/.otf files from a resource directory (shipped from WebKit's test
// font collection) and resolves CSS family/weight/style to a Typeface.
//
// Platform support:
//   - Windows: C:\Windows\Fonts (Microsoft YaHei, Consolas, SimSun)
//   - macOS:   /System/Library/Fonts, ~/Library/Fonts, /Library/Fonts
//   - Linux:   /usr/share/fonts, /usr/local/share/fonts

package graphics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/hoonfeng/goskia/skia"
)

// loadedFont describes a single Typeface loaded from a font file, together
// with the CSS-facing metadata (family name, weight, italic, mono/serif/CJK)
// used by LookupTypeface to pick the best match.
type loadedFont struct {
	tf     *skia.Typeface
	family string // normalized lowercase family name, e.g. "dejavu sans"
	weight int    // 400 = normal, 700 = bold
	italic bool
	mono   bool
	serif  bool
	cjk    bool
	emoji  bool
	symbol bool // symbol font (geometric shapes, arrows, etc.)
}

// CSSFontWeightName maps CSS numeric font-weight values to their standard
// human-readable names, as used in font file naming conventions (e.g. "Thin",
// "Light", "Regular", "Bold"). Useful when resolving font files by weight when
// the Typeface metadata does not expose weight directly.
var CSSFontWeightName = map[int]string{
	100: "Thin",
	200: "ExtraLight",
	300: "Light",
	400: "Regular",
	500: "Medium",
	600: "SemiBold",
	700: "Bold",
	800: "ExtraBold",
	900: "Black",
	950: "ExtraBlack",
}

// WeightName returns the CSS weight name for a numeric value (e.g. 400 �?"Regular").
// Returns "Unknown" for weights outside the 100�?50 range.
func WeightName(weight int) string {
	if name, ok := CSSFontWeightName[weight]; ok {
		return name
	}
	return "Unknown"
}

// FontManager is the Go translation of WebCore::FontCache. It owns the set of
// Typefaces loaded from disk and resolves CSS font descriptions to Typefaces.
// A single global instance is initialized once via InitFontManager.
type FontManager struct {
	mu        sync.RWMutex
	fonts     []loadedFont
	customTF  map[string]*skia.Typeface // @font-face registered fonts, keyed by lowercased family name
	defaultTF *skia.Typeface            // default sans-serif (prefers CJK coverage)
	sansTF    *skia.Typeface            // generic sans-serif
	monoTF    *skia.Typeface            // generic monospace
	serifTF   *skia.Typeface            // generic serif
	emojiTF   *skia.Typeface            // emoji font (Segoe UI Emoji / Noto Color Emoji)
	symbolTF  *skia.Typeface            // symbol font (Segoe UI Symbol) for geometric shapes/arrows
	cjkTF     *skia.Typeface            // CJK fallback for primary faces without CJK glyphs
	cjkFamily string                    // 实际选中的 CJK 族名（日志/CI 断言用，如 PingFang SC）

	// systemFontsLoaded guards LoadSystemFonts against duplicate scans
	// (ensureFonts / NewHost may both call it; fonts must load only once).
	systemFontsLoaded bool

	// loadedKeys 按「文件名#faceIndex」去重：TTC 会按 index 加载多个 face，
	// 且 InitFontManager(dir) 与 LoadSystemFonts() 可能扫到同一文件。
	loadedKeys map[string]bool
}

var (
	globalFontMgr     *FontManager
	globalFontMgrOnce sync.Once
)

// InitFontManager initializes the global FontManager by loading every .ttf/.otf
// file under fontDir. It is safe to call multiple times; only the first call
// performs the load. Returns the global manager (may be non-nil even if no
// fonts were loaded).
func InitFontManager(fontDir string) *FontManager {
	globalFontMgrOnce.Do(func() {
		m := &FontManager{
			customTF: make(map[string]*skia.Typeface),
		}
		m.loadDir(fontDir)
		globalFontMgr = m
	})
	return globalFontMgr
}

// GetFontManager returns the global FontManager, or nil if InitFontManager has
// not been called yet.
func GetFontManager() *FontManager { return globalFontMgr }

// RegisterCustomFont creates a Typeface from raw font file data (TTF/OTF/WOFF)
// and registers it under the given CSS family name, making it available via
// subsequent LookupTypeface calls. This is the primary integration point for
// @font-face rules: when a style sheet declares @font-face { font-family: X;
// src: url(Y); }, the loaded font data should be passed to this method so that
// the painter can resolve text with the correct typeface.
//
// The font data must be a raw TTF, OTF or WOFF (Skia handles WOFF->SFNT
// conversion internally). index is 0 for single-font files (the common case);
// TTC collections may use a non-zero index.
func (m *FontManager) RegisterCustomFont(family string, data []byte, index int) error {
	if m == nil {
		return fmt.Errorf("fontmgr: FontManager is nil")
	}
	if len(data) == 0 {
		return fmt.Errorf("fontmgr: empty font data for %q", family)
	}
	tf := skia.NewTypefaceFromData(data, index)
	if tf == nil {
		return fmt.Errorf("fontmgr: failed to create typeface from data for %q", family)
	}
	key := strings.ToLower(strings.TrimSpace(family))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.customTF == nil {
		m.customTF = make(map[string]*skia.Typeface)
	}
	m.customTF[key] = tf
	return nil
}

// loadDir reads every font file under dir and registers it.
func (m *FontManager) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// ★ 空目录（InitFontManager("") 走 OS 字体名查找）不打印错误：
		// 此前每次进程启动都向 stderr 打一行 "[fontmgr] read dir  failed:
		// open : ..."——宿主（widget-overlay 等）未配置字体目录时首屏
		// 日志噪音，且与默认字体按名查找的正常行为无关。仅非空目录
		// 读取失败才提示（路径配置错误值得告警）。
		if dir != "" {
			fmt.Fprintf(os.Stderr, "[fontmgr] read dir %s failed: %v\n", dir, err)
		}
		// 目录不可用（如 InitFontManager("")）时仍执行 selectDefaults：
		// 默认字体通过 skia.NewTypeface 按 OS 字体名查找（Microsoft YaHei
		// / Consolas / SimSun 等），不依赖本目录已加载的文件，保证文本
		// 始终可渲染。
		m.selectDefaults()
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".ttf") &&
			!strings.HasSuffix(lower, ".otf") &&
			!strings.HasSuffix(lower, ".ttc") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fontmgr] read %s failed: %v\n", name, err)
			continue
		}
		if m.addTypefacesFromData(name, data) == 0 {
			fmt.Fprintf(os.Stderr, "[fontmgr] load %s failed (NewTypefaceFromData returned nil)\n", name)
			continue
		}
	}
	m.selectDefaults()
	fmt.Fprintf(os.Stderr, "[fontmgr] loaded %d font(s); default=%v sans=%v serif=%v mono=%v\n",
		len(m.fonts), m.defaultTF != nil, m.sansTF != nil, m.serifTF != nil, m.monoTF != nil)
}

// LoadSystemFonts loads fonts from known system font directories for the
// current platform (Windows, macOS, Linux). On Windows it reads from
// C:\Windows\Fonts with hardcoded targets (Microsoft YaHei, Consolas, SimSun).
// On macOS it scans /System/Library/Fonts, /Library/Fonts, and ~/Library/Fonts.
// On Linux it scans /usr/share/fonts and /usr/local/share/fonts.
func (m *FontManager) LoadSystemFonts() {
	m.mu.Lock()
	if m.systemFontsLoaded {
		m.mu.Unlock()
		return
	}
	m.systemFontsLoaded = true
	m.mu.Unlock()
	loaded := 0
	// Windows
	_ = m.loadSystemFontDir(`C:\Windows\Fonts`, &loaded)
	// macOS
	_ = m.loadSystemFontDir("/System/Library/Fonts", &loaded)
	_ = m.loadSystemFontDir("/Library/Fonts", &loaded)
	if home, err := os.UserHomeDir(); err == nil {
		_ = m.loadSystemFontDir(home+"/Library/Fonts", &loaded)
	}
	// Linux
	_ = m.loadSystemFontDir("/usr/share/fonts", &loaded)
	_ = m.loadSystemFontDir("/usr/local/share/fonts", &loaded)
	if loaded > 0 {
		m.selectDefaults()
		fmt.Fprintf(os.Stderr, "[fontmgr] loaded %d system font(s) total\n", loaded)
	}
}

// loadSystemFontDir loads font files from a single directory. On Windows,
// specific font files are loaded individually; on macOS/Linux, all .ttf/.otf
// files in the directory (and one level of subdirectories) are loaded.
func (m *FontManager) loadSystemFontDir(dir string, loaded *int) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0 // directory doesn't exist on this platform �?not an error
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			// Recurse one level (macOS fonts are often in subdirectories)
			sub, err := os.ReadDir(dir + "/" + e.Name())
			if err != nil {
				continue
			}
			for _, se := range sub {
				if m.tryLoadFont(dir+"/"+e.Name()+"/"+se.Name(), loaded) {
					count++
				}
			}
			continue
		}
		if m.tryLoadFont(dir+"/"+e.Name(), loaded) {
			count++
		}
	}
	return count
}

// tryLoadFont attempts to load a single font file if its extension is .ttf/.otf/.ttc.
// systemFontWhitelist 判定系统字体文件是否值得加载。★ 内存优化
// （2026-08-23）：此前全量 os.ReadFile + Skia 拷贝加载 C:\Windows\Fonts
// 全部 294 个字体（≈526MB 驻留！），挂件/配置界面实际只用常见西文 +
// 中文 + emoji/symbol + 等宽。白名单改为精确文件名列表（~55 个文件
// ≈110MB）：CJK 仅保留微软雅黑（sans）+ 宋体/NSimSun（serif/mono），
// 其他语种（日/韩/繁/其他族系）通过 Skia 系统字体名查找回退
// （skia.NewTypeface），不影响 CSS font-family 解析与试水渲染。
//
// ★ 2026-10-19（多平台中文字体，P0）：白名单**按平台分表**。旧实现只有
// Windows 文件名表（msyh.ttc / simsun.ttc / consola.ttf …），macOS 的
// PingFang.ttc、Hiragino Sans GB.ttc 与 Linux 的 NotoSansCJK*.ttc、
// wqy-zenhei.ttc 全被过滤掉 → selectDefaults 拿不到任何 CJK 字体 →
// 非 Windows 上所有中文渲染成豆腐块（真实用户与 CI headless 同样中招）。
// Windows 表逐项保持原样，行为不变。
func systemFontWhitelist(fileName string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(fileName)))
	switch runtime.GOOS {
	case "windows":
		return windowsFontWhitelist[base]
	case "darwin":
		return darwinFontWhitelist[base]
	default:
		return linuxFontWhitelist[base] || linuxCJKPrefixMatch(base)
	}
}

// windowsFontWhitelist：Windows 精确文件名白名单（既有的 ~55 个文件，
// 只加载挂件/配置 UI 实际会用到：中文 + emoji/symbol + 西文主流）。
var windowsFontWhitelist = map[string]bool{
	"msyh.ttc": true, "msyhbd.ttc": true,
	"simsun.ttc": true, "simhei.ttf": true,
	"seguiemj.ttf": true, "seguisym.ttf": true,
	"segoeui.ttf": true, "segoeuib.ttf": true, "segoeuii.ttf": true, "segoeuiz.ttf": true,
	"seguisb.ttf": true, "seguisbi.ttf": true, "seguili.ttf": true,
	"arial.ttf": true, "arialbd.ttf": true, "ariali.ttf": true, "arialbi.ttf": true,
	"calibri.ttf": true, "calibrib.ttf": true, "calibrii.ttf": true, "calibriz.ttf": true,
	"times.ttf": true, "timesbd.ttf": true, "timesi.ttf": true, "timesbi.ttf": true,
	"georgia.ttf": true, "georgiab.ttf": true, "georgiai.ttf": true, "georgiaz.ttf": true,
	"tahoma.ttf": true, "tahomabd.ttf": true,
	"verdana.ttf": true, "verdanab.ttf": true, "verdanai.ttf": true, "verdanaz.ttf": true,
	"consola.ttf": true, "consolab.ttf": true, "consolai.ttf": true, "consolaz.ttf": true,
	"cour.ttf": true, "courbd.ttf": true, "couri.ttf": true, "courbi.ttf": true,
	"impact.ttf": true, "comic.ttf": true, "comicbd.ttf": true, "comici.ttf": true, "comicz.ttf": true,
	"cambria.ttc": true, "cambriab.ttf": true, "cambriai.ttf": true, "cambriaz.ttf": true,
	"constan.ttf": true, "constanb.ttf": true, "constani.ttf": true, "constanz.ttf": true,
	"corbel.ttf": true, "corbelb.ttf": true, "corbeli.ttf": true, "corbelz.ttf": true,
}

// darwinFontWhitelist：macOS 系统字体（小写文件名），覆盖
// /System/Library/Fonts 与 /System/Library/Fonts/Supplemental：
//   - 中文：PingFang.ttc（SC/TC/HK 合集）、Hiragino Sans GB.ttc、
//     STHeiti Light/Medium.ttc、Songti.ttc、Arial Unicode.ttf
//   - 西文/等宽：Helvetica(Neue)、SFNS、Menlo、Monaco、Arial、Times、
//     Courier New、Geneva、Lucida Grande
//   - emoji：Apple Color Emoji.ttc
var darwinFontWhitelist = map[string]bool{
	"pingfang.ttc":          true,
	"hiragino sans gb.ttc":  true,
	"stheiti light.ttc":     true,
	"stheiti medium.ttc":    true,
	"songti.ttc":            true,
	"songtisc.ttc":          true,
	"arial unicode.ttf":     true,
	"apple symbols.ttf":     true,
	"helvetica.ttc":         true,
	"helveticaneue.ttc":     true,
	"sfns.ttf":              true,
	"sfnsmono.ttf":          true,
	"menlo.ttc":             true,
	"monaco.ttf":            true,
	"arial.ttf":             true,
	"arial bold.ttf":        true,
	"times.ttf":             true,
	"times new roman.ttf":   true,
	"courier new.ttf":       true,
	"geneva.ttf":            true,
	"lucida grande.ttf":     true,
	"apple color emoji.ttc": true,
}

// linuxFontWhitelist：Linux 常见字体精确名（Debian/Ubuntu/Fedora/Arch 的
// 命名差异由 linuxCJKPrefixMatch 兜住）。
var linuxFontWhitelist = map[string]bool{
	"notosanscjk-regular.ttc":      true,
	"notosanscjk-bold.ttc":         true,
	"notoserifcjk-regular.ttc":     true,
	"notoserifcjk-bold.ttc":        true,
	"notosanssc-vf.ttf":            true,
	"notoserifsc-vf.ttf":           true,
	"wqy-zenhei.ttc":               true,
	"wqy-microhei.ttc":             true,
	"droidsansfallback.ttf":        true,
	"droidsansfallbackfull.ttf":    true,
	"dejavusans.ttf":               true,
	"dejavusans-bold.ttf":          true,
	"dejavusansmono.ttf":           true,
	"dejavusansmono-bold.ttf":      true,
	"dejavuserif.ttf":              true,
	"dejavuserif-bold.ttf":         true,
	"liberationsans-regular.ttf":   true,
	"liberationsans-bold.ttf":      true,
	"liberationmono-regular.ttf":   true,
	"liberationmono-bold.ttf":      true,
	"liberationserif-regular.ttf":  true,
	"liberationserif-bold.ttf":     true,
	"freesans.ttf":                 true,
	"freeserif.ttf":                true,
	"freemono.ttf":                 true,
	"ubuntu-r.ttf":                 true,
	"ubuntu-b.ttf":                 true,
	"ubuntu-mono-r.ttf":            true,
	"notosans-regular.ttf":         true,
	"notosans-bold.ttf":            true,
	"notosansmono-regular.ttf":     true,
	"notocoloremoji.ttf":           true,
	"sourcehansanssc-regular.otf":  true,
	"sourcehanserifsc-regular.otf": true,
}

// linuxCJKPrefixMatch 覆盖各发行版对 Noto CJK / 思源黑体的命名差异
// （NotoSansCJKsc-Regular.otf、NotoSansCJKJP-Regular.otf、
// SourceHanSansSC-Regular.otf …）。只收 Regular/Bold 一类字重：
// 每个 NotoSansCJK*.ttc ≈19MB，把 Light/Thin/Black 全收进来会白吃上百 MB。
func linuxCJKPrefixMatch(base string) bool {
	if strings.Contains(base, "light") || strings.Contains(base, "thin") ||
		strings.Contains(base, "black") || strings.Contains(base, "medium") ||
		strings.Contains(base, "demi") || strings.Contains(base, "semi") {
		return false
	}
	if !strings.HasSuffix(base, ".ttc") && !strings.HasSuffix(base, ".otf") && !strings.HasSuffix(base, ".ttf") {
		return false
	}
	for _, p := range []string{"notosanscjk", "notoserifcjk", "sourcehansans", "sourcehanserif"} {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

func (m *FontManager) tryLoadFont(path string, loaded *int) bool {
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(name, ".ttf") &&
		!strings.HasSuffix(name, ".otf") &&
		!strings.HasSuffix(name, ".ttc") {
		return false
	}
	// ★ 白名单过滤（内存优化：跳过非必要字体文件）
	if !systemFontWhitelist(filepath.Base(path)) {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	n := m.addTypefacesFromData(name, data)
	if n == 0 {
		return false
	}
	if loaded != nil {
		*loaded += n
	}
	return true
}

// maxTTCFaces 是单个 TTC 最多尝试的 face 数。Noto Sans CJK 合集有 5 个
// face（JP/KR/SC/TC/HK），PingFang 类似；8 足够覆盖，且避免恶意/异常
// 文件导致的长循环。
const maxTTCFaces = 8

// isCJKCapable reports whether tf actually maps 中 to a real glyph — the
// only reliable "this face can render Chinese" test available through the
// Skia C API (glyph 0 is .notdef = 豆腐块).
func isCJKCapable(tf *skia.Typeface) bool {
	return tf != nil && tf.UnicharToGlyph('中') != 0
}

// addTypefacesFromData registers the face(s) of one font file and returns how
// many were added.
//
// ★ 2026-10-19（TTC 多 face，P0）：旧实现对 .ttc 固定 index=0。
//   - NotoSansCJK-Regular.ttc 的 index 0 是 **JP** face，中文字形是日式写法；
//   - 某些合集首个 face 甚至不含 CJK 覆盖（Cambria.ttc 的 index 0 是拉丁
//     Cambria，index 1 才是 Cambria Math）。
//   现在逐个 index 尝试，取第一个真正能画「中」的 face；整包都没有 CJK
//   覆盖时退回 index 0（拉丁字体正常加载，中文交给 CJKTypeface 回退）。
//   face 必须是「CJK 优先」的，因为它决定了 Linux/macOS 上中文的落点。
func (m *FontManager) addTypefacesFromData(fileName string, data []byte) int {
	lower := strings.ToLower(filepath.Base(fileName))
	if !strings.HasSuffix(lower, ".ttc") {
		tf := skia.NewTypefaceFromData(data, 0)
		if tf == nil {
			return 0
		}
		return m.addFace(fileName, 0, tf)
	}
	added := 0
	chosenIdx := -1
	// 1) 已知合集按约定顺序优先（Noto CJK 的 SC face 是 index 2，
	//    直接选 index 0 会拿到日文 JP face：字形能画但写法是日式）。
	for _, idx := range preferredTTCFaces(lower) {
		tf := skia.NewTypefaceFromData(data, idx)
		if tf == nil {
			continue
		}
		if isCJKCapable(tf) {
			chosenIdx = idx
			added += m.addFace(fileName, idx, tf)
			break
		}
		tf.Release()
	}
	// 2) 通用路径：逐个 index 尝试，取第一个 CJK face；整包无 CJK 覆盖
	//    （Cambria.ttc 等）则退回 index 0。
	var first *skia.Typeface
	for idx := 0; idx < maxTTCFaces; idx++ {
		if chosenIdx >= 0 {
			break
		}
		tf := skia.NewTypefaceFromData(data, idx)
		if tf == nil {
			break // 越过最后一个 face
		}
		if idx == 0 {
			first = tf
			if isCJKCapable(tf) {
				chosenIdx = 0
				added += m.addFace(fileName, 0, tf)
				break
			}
			continue
		}
		if isCJKCapable(tf) {
			chosenIdx = idx
			added += m.addFace(fileName, idx, tf)
			break
		}
		// 未选中的中间 face 立即释放（每 face 持有一份字体数据副本）。
		tf.Release()
	}
	if chosenIdx < 0 && first != nil {
		added += m.addFace(fileName, 0, first)
	}
	// 3) 合集里另有必须保留的 face（simsun.ttc 的 index 1 = NSimSun 等宽）。
	for _, idx := range ttcExtraFaces(lower) {
		tf := skia.NewTypefaceFromData(data, idx)
		if tf != nil {
			added += m.addFace(fileName, idx, tf)
		}
	}
	return added
}

// preferredTTCFaces 返回已知 TTC 合集的 face 优选顺序。
// Noto Sans/Serif CJK 的标准合集顺序：0=JP 1=KR 2=SC 3=TC 4=HK；
// UI 默认族要的是 SC，所以先试 2，再退回 0 兜底。
func preferredTTCFaces(base string) []int {
	if strings.HasPrefix(base, "notosanscjk") || strings.HasPrefix(base, "notoserifcjk") {
		return []int{2, 0, 1, 3, 4}
	}
	return nil
}

// ttcExtraFaces 返回除主 CJK face 外还需额外加载的 face index。
// simsun.ttc：index 1 = NSimSun（新宋体，CJK 等宽），mono 回退要用。
func ttcExtraFaces(base string) []int {
	if base == "simsun.ttc" {
		return []int{1}
	}
	return nil
}

// ttcFaceFamily 按文件名 + face index 推断 TTC 合集里各 face 的 CSS 族名。
// 返回空串表示不是已知的 TTC 合集，调用方按文件名走默认规则。
//
// 为什么必须按 index 命名：NotoSansCJK-Regular.ttc 的 5 个 face 是
// JP/KR/SC/TC/HK，字形不同但都覆盖「中」。旧实现只有 index 0（JP），
// Linux 上中文用的是日式写法；按族名 "noto sans cjk sc" 查找也永远 miss。
func ttcFaceFamily(base string, index int) string {
	switch {
	case strings.HasPrefix(base, "notosanscjk"):
		return cjkFaceName("noto sans cjk", index)
	case strings.HasPrefix(base, "notoserifcjk"):
		return cjkFaceName("noto serif cjk", index)
	case strings.HasPrefix(base, "notosanscjksc"), strings.HasPrefix(base, "notosanssc"):
		return "noto sans cjk sc"
	case strings.HasPrefix(base, "notoserifcjksc"), strings.HasPrefix(base, "notoserifsc"):
		return "noto serif cjk sc"
	case strings.HasPrefix(base, "pingfang"):
		// PingFang.ttc：SC/TC/HK 各字重的合集，首个 face 即 PingFang SC。
		return "pingfang sc"
	case strings.HasPrefix(base, "hiragino"):
		return "hiragino sans gb"
	case strings.HasPrefix(base, "stheiti"), strings.HasPrefix(base, "heiti"):
		return "heiti sc"
	case strings.HasPrefix(base, "songti"):
		return "songti sc"
	case strings.HasPrefix(base, "wqy-zenhei"):
		return "wenquanyi zen hei"
	case strings.HasPrefix(base, "wqy-microhei"):
		return "wenquanyi micro hei"
	case strings.HasPrefix(base, "sourcehansans"):
		return "source han sans sc"
	case strings.HasPrefix(base, "sourcehanserif"):
		return "source han serif sc"
	}
	return ""
}

// cjkFaceName 把 Noto CJK 合集的 index 映射为族名（SC/TC/JP…）。
func cjkFaceName(prefix string, index int) string {
	names := []string{"jp", "kr", "sc", "tc", "hk"}
	if index >= 0 && index < len(names) {
		return prefix + " " + names[index]
	}
	return prefix
}

// addFace registers one face, deduped by「文件名#faceIndex」.
func (m *FontManager) addFace(fileName string, index int, tf *skia.Typeface) int {
	key := strings.ToLower(filepath.Base(fileName)) + "#" + strconv.Itoa(index)
	if m.loadedKeys == nil {
		m.loadedKeys = make(map[string]bool)
	}
	if m.loadedKeys[key] {
		tf.Release()
		return 0
	}
	m.loadedKeys[key] = true
	m.fonts = append(m.fonts, classifyFont(fileName, index, tf))
	return 1
}

// Legacy LoadSystemFonts kept for backward compatibility.
// Deprecated: Use the new LoadSystemFonts which auto-detects the platform.
func (m *FontManager) LoadSystemFontsLegacy() {
	winFontDir := `C:\Windows\Fonts`
	type sysFont struct {
		filename string
		index    int
	}
	targets := []sysFont{
		{"msyh.ttc", 0},    // Microsoft YaHei Regular (default proportional CJK)
		{"msyhbd.ttc", 0},  // Microsoft YaHei Bold (real bold variant)
		{"consola.ttf", 0}, // Consolas Regular (English monospace)
		{"simsun.ttc", 1},  // NSimSun (CJK monospace, index 1 of the TTC)
	}
	for _, t := range targets {
		path := filepath.Join(winFontDir, t.filename)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fontmgr] system font %s: %v\n", t.filename, err)
			continue
		}
		tf := skia.NewTypefaceFromData(data, t.index)
		if tf == nil {
			fmt.Fprintf(os.Stderr, "[fontmgr] system font %s index %d: NewTypefaceFromData returned nil\n", t.filename, t.index)
			continue
		}
		m.fonts = append(m.fonts, classifyFont(t.filename, t.index, tf))
		fmt.Fprintf(os.Stderr, "[fontmgr] loaded system font %s index %d\n", t.filename, t.index)
	}
	m.selectDefaults()
}

// classifyFont inspects the font file name to derive its CSS metadata (family name,
// weight, italic flag). Real WebKit reads the font's name table; this port uses
// filename conventions matching the WebKit test font collection (e.g.
// DejaVuSans-Bold.ttf) and Skia Typeface style queries where available.
// classifyFont inspects the font file name (and TTC face index) to derive its
// CSS metadata (family name, weight, italic flag). Real WebKit reads the font's
// name table; this port uses filename conventions plus the known face order of
// well-known collections (Noto CJK / PingFang).
func classifyFont(filename string, index int, tf *skia.Typeface) loadedFont {
	name := strings.ToLower(filepath.Base(filename))
	entry := loadedFont{
		tf:     tf,
		family: "default",
		weight: 400,
	}
	// Region/script detection.
	// ★ 2026-10-19：补 macOS/Linux 的中文字体特征（pingfang / hiragino /
	// heiti / songti / noto sans cjk / wqy / source han / droid fallback）。
	// 此前只认 kochi/cjk/noto/msyh/simhei/simsun，导致即使文件被白名单放行，
	// selectDefaults 也不会把它们当 CJK 回退候选。
	if strings.Contains(name, "cjk") || strings.Contains(name, "noto") ||
		strings.Contains(name, "pingfang") || strings.Contains(name, "hiragino") ||
		strings.Contains(name, "heiti") || strings.Contains(name, "songti") ||
		strings.Contains(name, "wqy") || strings.Contains(name, "wenquanyi") ||
		strings.Contains(name, "droidsansfallback") || strings.Contains(name, "sourcehan") ||
		strings.Contains(name, "simhei") || strings.Contains(name, "simsun") ||
		strings.Contains(name, "msyh") || strings.Contains(name, "kochi") {
		entry.cjk = true
	}
	if strings.Contains(name, "emoji") || strings.Contains(name, "coloremoji") || strings.Contains(name, "seguiemj") {
		entry.emoji = true
	}
	if strings.Contains(name, "symbol") || strings.Contains(name, "seguisym") {
		entry.symbol = true
	}
	// ★ 2026-10-19：TTC 合集按 face index 命名（Noto CJK 标准顺序为
	// 0=JP 1=KR 2=SC 3=TC 4=HK）。此前一律落回文件名，Linux 上
	// NotoSansCJK-Regular.ttc 会被记成 "notosanscjk-regular"，
	// 按族名 "noto sans cjk sc" 查找永远 miss → 中文走不到该 face。
	if fam := ttcFaceFamily(name, index); fam != "" {
		entry.family = fam
		entry.cjk = true
		if strings.Contains(name, "mono") {
			entry.mono = true
		}
		if strings.Contains(name, "serif") || strings.Contains(name, "songti") {
			entry.serif = true
		}
		if strings.Contains(name, "bold") {
			entry.weight = 700
		}
		if strings.Contains(name, "oblique") || strings.Contains(name, "italic") {
			entry.italic = true
		}
		return entry
	}
	// Weight override from filename for common patterns.
	switch {
	case strings.HasPrefix(name, "msyhbd"):
		// Microsoft YaHei Bold �?real bold variant (not synthetic embolden).
		entry.family = "microsoft yahei"
		entry.weight = 700
		entry.cjk = true
	case strings.HasPrefix(name, "msyh"):
		// Microsoft YaHei Regular �?default proportional CJK font (like GWui).
		entry.family = "microsoft yahei"
		entry.weight = 400
		entry.cjk = true
	case strings.HasPrefix(name, "dejavusansmono"):
		entry.family = "dejavu sans mono"
		entry.mono = true
	case strings.HasPrefix(name, "dejavusans"):
		entry.family = "dejavu sans"
	case strings.HasPrefix(name, "dejavuserif"):
		entry.family = "dejavu serif"
		entry.serif = true
	case strings.HasPrefix(name, "liberationmono"):
		entry.family = "liberation mono"
		entry.mono = true
	case strings.HasPrefix(name, "liberationsans"):
		entry.family = "liberation sans"
	case strings.HasPrefix(name, "liberationserif"):
		entry.family = "liberation serif"
		entry.serif = true
	case strings.HasPrefix(name, "roboto"):
		entry.family = "roboto"
	case strings.HasPrefix(name, "consola"):
		entry.family = "consolas"
		entry.mono = true
	case name == "simsun.ttc" && index == 1:
		// index 1 of simsun.ttc is NSimSun (新宋�?, a CJK monospace face.
		entry.family = "nsimsun"
		entry.mono = true
		entry.cjk = true
	case strings.Contains(name, "kochi-mincho"):
		entry.family = "kochi mincho"
		entry.serif = true
	case strings.Contains(name, "kochi-gothic"):
		entry.family = "kochi gothic"
	case strings.Contains(name, "notocoloremoji") || strings.Contains(name, "notocolor"):
		entry.family = "noto color emoji"
		entry.emoji = true
	default:
		entry.family = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if strings.Contains(name, "bold") {
		entry.weight = 700
	}
	if strings.Contains(name, "oblique") || strings.Contains(name, "italic") {
		entry.italic = true
	}
	return entry
}

// selectDefaults picks the generic-family Typefaces used to resolve
// sans-serif/serif/monospace. Following GWui's strategy, we prefer Microsoft
// YaHei (a proportional CJK font) as the default sans-serif so UI text looks
// natural. NSimSun is reserved for the monospace generic family only.
// fontCand 描述一个默认字体候选：
//   - os:     平台字体名，走 skia.NewTypeface（保留系统 fallback 链）
//   - family: 已加载字体集合里的族名，走 findBest（按文件加载的 face）
type fontCand struct {
	os     string
	family string
}

// pickFont 按候选顺序返回第一个可用的 Typeface 与其标签（族名）。
func (m *FontManager) pickFont(cands []fontCand, weight int) (*skia.Typeface, string) {
	for _, c := range cands {
		if c.os != "" {
			if tf := skia.NewTypeface(c.os, skia.FontStyle{Weight: weight, Width: 5, Slant: 0}); tf != nil {
				return tf, c.os
			}
		}
		if c.family != "" {
			if tf := m.findBest(c.family, weight, false); tf != nil {
				return tf, c.family
			}
		}
	}
	return nil, ""
}

// monoCandidates 返回当前平台的等宽候选（首个可用者胜出）。
func monoCandidates() []fontCand {
	switch runtime.GOOS {
	case "darwin":
		return []fontCand{
			{os: "Menlo"}, {family: "menlo"},
			{os: "Monaco"}, {os: "SF Mono"},
			{os: "Consolas"}, {os: "Courier New"},
			{family: "dejavu sans mono"}, {family: "liberation mono"},
		}
	case "windows":
		// Windows 保持既有行为：Consolas（与 Edge 度量一致），NSimSun 作 CJK 兜底。
		return []fontCand{
			{os: "Consolas"}, {family: "consolas"},
			{os: "NSimSun"}, {family: "nsimsun"},
			{family: "liberation mono"}, {family: "dejavu sans mono"},
		}
	default:
		return []fontCand{
			{os: "DejaVu Sans Mono"}, {family: "dejavu sans mono"},
			{os: "Liberation Mono"}, {family: "liberation mono"},
			{os: "Noto Sans Mono CJK SC"}, {os: "Noto Sans Mono"},
			{os: "Consolas"},
		}
	}
}

// sansCandidates 返回当前平台的无衬线候选。
// ★ 非 Windows 必须包含真正的 CJK 字体（PingFang SC / Noto Sans CJK SC）：
// 旧实现只有 Microsoft YaHei / Segoe UI / Arial，macOS/Linux 全部 miss →
// sansTF 退化成不含 CJK 的西文字体 → 中文全部豆腐块。
func sansCandidates() []fontCand {
	switch runtime.GOOS {
	case "darwin":
		return []fontCand{
			{os: "PingFang SC"}, {family: "pingfang sc"},
			{os: "Hiragino Sans GB"}, {family: "hiragino sans gb"},
			{os: "Heiti SC"}, {family: "heiti sc"},
			{os: "Helvetica Neue"}, {os: "Helvetica"}, {os: "Arial"},
			{family: "dejavu sans"}, {family: "liberation sans"},
		}
	case "windows":
		return []fontCand{
			{os: "Microsoft YaHei"}, {family: "microsoft yahei"},
			{os: "Segoe UI"}, {os: "Arial"}, {os: "Times New Roman"},
			{family: "kochi gothic"}, {family: "dejavu sans"}, {family: "liberation sans"},
		}
	default:
		return []fontCand{
			{os: "Noto Sans CJK SC"}, {family: "noto sans cjk sc"},
			{os: "Noto Sans SC"}, {family: "noto sans sc"},
			{os: "WenQuanYi Zen Hei"}, {family: "wenquanyi zen hei"},
			{os: "Droid Sans Fallback"}, {family: "droidsansfallback"},
			{os: "DejaVu Sans"}, {os: "Liberation Sans"}, {os: "FreeSans"},
		}
	}
}

// serifCandidates 返回当前平台的衬线候选。
func serifCandidates() []fontCand {
	switch runtime.GOOS {
	case "darwin":
		return []fontCand{
			{os: "Songti SC"}, {family: "songti sc"},
			{os: "Times New Roman"}, {os: "Times"},
			{family: "dejavu serif"}, {family: "liberation serif"},
		}
	case "windows":
		return []fontCand{
			{os: "SimSun"}, {family: "kochi mincho"},
			{family: "dejavu serif"}, {family: "liberation serif"},
		}
	default:
		return []fontCand{
			{os: "Noto Serif CJK SC"}, {family: "noto serif cjk sc"},
			{os: "Noto Serif SC"}, {family: "noto serif sc"},
			{os: "DejaVu Serif"}, {os: "Liberation Serif"}, {os: "FreeSerif"},
		}
	}
}

// cjkCandidates 返回 CJK 回退候选（按平台）。
func cjkCandidates() []fontCand {
	switch runtime.GOOS {
	case "darwin":
		return []fontCand{
			{os: "PingFang SC"}, {family: "pingfang sc"},
			{os: "Hiragino Sans GB"}, {family: "hiragino sans gb"},
			{os: "Heiti SC"}, {family: "heiti sc"},
			{os: "STHeiti"}, {os: "Arial Unicode MS"},
		}
	case "windows":
		// 与 Edge 对齐：Windows 上「拉丁主字体 + 中文」回退到 Noto Sans SC
		//（系统装有 NotoSansSC-VF.ttf），取不到再回落雅黑。
		return []fontCand{
			{os: "Noto Sans SC"}, {family: "noto sans sc"},
			{os: "Microsoft YaHei"}, {family: "microsoft yahei"},
		}
	default:
		return []fontCand{
			{os: "Noto Sans CJK SC"}, {family: "noto sans cjk sc"},
			{os: "WenQuanYi Zen Hei"}, {family: "wenquanyi zen hei"},
			{os: "Noto Sans SC"}, {family: "noto sans sc"},
			{os: "Droid Sans Fallback"}, {family: "droidsansfallback"},
		}
	}
}

// selectDefaults picks the generic-family Typefaces used to resolve
// sans-serif/serif/monospace.
//
// ★ 2026-10-19（多平台中文字体，P0）：候选按 runtime.GOOS 分表。
// 旧实现只认识 Microsoft YaHei / Segoe UI / Arial / SimSun / Consolas，
// macOS（PingFang/Hiragino）与 Linux（Noto CJK/WQY）上一律 miss，
// cjkTF 最终退化成无 CJK 覆盖的西文字体 → 中文全是豆腐块。
// 现在非 Windows 的 cjkTF **必须**通过 isCJKCapable（能画「中」）校验，
// 找不到就保持 nil，而不是塞一个画不出中文的西文字体。
func (m *FontManager) selectDefaults() {
	m.monoTF, _ = m.pickFont(monoCandidates(), 400)
	m.sansTF, _ = m.pickFont(sansCandidates(), 400)
	if m.sansTF == nil && len(m.fonts) > 0 {
		m.sansTF = m.fonts[0].tf
	}
	m.serifTF, _ = m.pickFont(serifCandidates(), 400)
	if m.serifTF == nil {
		m.serifTF = m.sansTF
	}
	// default = sans-serif（比例字体），与浏览器默认族一致。
	m.defaultTF = m.sansTF
	if m.defaultTF == nil {
		m.defaultTF = m.monoTF
	}
	// CJK 回退（主字体无 CJK 字形时使用，见 CJKTypeface）。
	m.cjkTF, m.cjkFamily = m.pickCJK()
	// CI 断言点（release.yml grep `[fontmgr] cjk=`）：
	// 非 Windows 上必须是非 "(none)" 的真实 CJK 族名。
	fmt.Fprintf(os.Stderr, "[fontmgr] cjk=%s fonts=%d\n", m.cjkLabel(), len(m.fonts))
	// emoji: find the first font with emoji flag set (e.g. Segoe UI Emoji
	// on Windows, Noto Color Emoji on Linux/macOS).
	m.emojiTF = nil
	for _, f := range m.fonts {
		if f.emoji {
			m.emojiTF = f.tf
			break
		}
	}
	// Fallback: try to find by family name if no font was flagged as emoji.
	// Use OS-name lookup (skia.NewTypeface) rather than findBest so emoji /
	// symbol faces are found even when no font files were pre-loaded
	// (m.fonts empty — e.g. the webkit.ensureFonts path calls
	// InitFontManager("") without LoadSystemFonts; findBest would always
	// miss and symbols/emoji would render as .notdef tofu boxes).
	if m.emojiTF == nil {
		for _, name := range []string{"Segoe UI Emoji", "Noto Color Emoji", "Apple Color Emoji"} {
			m.emojiTF = skia.NewTypeface(name, skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
			if m.emojiTF != nil {
				break
			}
		}
	}
	// symbol: find the first font with symbol flag set (e.g. Segoe UI Symbol).
	m.symbolTF = nil
	for _, f := range m.fonts {
		if f.symbol {
			m.symbolTF = f.tf
			break
		}
	}
	// Fallback: try common symbol font family names. OS-name lookup (see
	// comment above) so symbol faces resolve even with an empty m.fonts.
	if m.symbolTF == nil {
		for _, name := range []string{"Segoe UI Symbol", "Segoe UI", "Arial"} {
			m.symbolTF = skia.NewTypeface(name, skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
			if m.symbolTF != nil {
				break
			}
		}
	}
}

// pickCJK 选择 CJK 回退 Typeface，并返回其族名标签。
//
// 校验顺序：
//  1. 平台 CJK 候选（os 名查找 → 文件族名查找），每个都必须 isCJKCapable；
//  2. 已加载文件集合里带 CJK 标记且真能画「中」的 face；
//  3. Windows 额外容许回落 sansTF（雅黑本身就是 CJK 字体）；
//  4. 否则返回 nil —— 宁可不画，也不回落到画不出中文的西文字体
//     （旧实现正是这么把「Noto Sans SC miss → 雅黑/Arial」写成了方块）。
func (m *FontManager) pickCJK() (*skia.Typeface, string) {
	for _, c := range cjkCandidates() {
		if c.os != "" {
			if tf := skia.NewTypeface(c.os, skia.FontStyle{Weight: 400, Width: 5, Slant: 0}); isCJKCapable(tf) {
				return tf, c.os
			}
		}
		if c.family != "" {
			if tf := m.findBest(c.family, 400, false); isCJKCapable(tf) {
				return tf, c.family
			}
		}
	}
	for _, f := range m.fonts {
		if f.cjk && isCJKCapable(f.tf) {
			return f.tf, f.family
		}
	}
	if runtime.GOOS == "windows" && isCJKCapable(m.sansTF) {
		return m.sansTF, "microsoft yahei"
	}
	return nil, ""
}

// cjkLabel 返回日志用的 CJK 族名（未命中时为 "(none)"）。
func (m *FontManager) cjkLabel() string {
	if m.cjkFamily != "" {
		return m.cjkFamily
	}
	return "(none)"
}

// findBest returns the Typeface for family whose weight/italic most closely
// match the request. weight/italic matching uses a small score: exact match
// wins, otherwise same weight class (bold/non-bold) and same italic win partial.
// Per CSS spec, weights 600+ are "bold class" and weights < 600 are "normal
// class", so font-weight: 600 (semibold) matches the bold (700) face when no
// exact 600 face is loaded.
func (m *FontManager) findBest(family string, weight int, italic bool) *skia.Typeface {
	var best *skia.Typeface
	bestScore := -1
	for _, f := range m.fonts {
		if f.family != family {
			continue
		}
		score := 0
		if f.weight == weight {
			score += 2
		} else if (weight >= 600 && f.weight >= 600) || (weight < 600 && f.weight < 600) {
			score += 1
		}
		if f.italic == italic {
			score += 2
		}
		if score > bestScore {
			bestScore = score
			best = f.tf
		}
	}
	return best
}

// LookupTypeface resolves a CSS font description (family name + weight + style)
// to a Typeface. Generic families (sans-serif/serif/monospace) and common
// aliases (Arial, Times, Courier, Consolas, etc.) map to the loaded defaults.
// Weight is respected so that font-weight: bold picks a real bold Typeface when
// one is loaded (e.g. Microsoft YaHei Bold). Unknown named families fall back
// to the default sans-serif Typeface so that text always renders (mirroring
// WebKit's last-resort fallback).
//
// CSS font-family is a comma-separated list of family names; the browser tries
// each in order and uses the first that has a matching Typeface. Generic
// families (sans-serif, serif, monospace) are always "available" so they
// terminate the fallback chain.
func (m *FontManager) LookupTypeface(family string, weight int, style string) *skia.Typeface {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	italic := style == "italic" || style == "oblique"
	raw := strings.ToLower(strings.TrimSpace(family))

	// Check @font-face custom fonts first. If the exact family matches
	// a registered custom font, return it immediately.
	if len(m.customTF) > 0 {
		if tf, ok := m.customTF[raw]; ok {
			// Check if there's a custom font with a weight match.
			// Exact lookup by raw family name.
			_ = italic // weight matching omitted for now; single-face @font-face
			return tf
		}
		// Also check individual families from the font-family list.
		for _, fam := range splitFontFamily(raw) {
			if tf, ok := m.customTF[fam]; ok {
				return tf
			}
		}
	}

	// Split the comma-separated list and try each family in order.
	families := splitFontFamily(raw)

	// resolveFamily maps a CSS family keyword to a loaded family name.
	// Returns "" if the family is a generic keyword that should always
	// resolve (terminating the fallback chain).
	resolveFamily := func(f string) (string, bool) {
		switch f {
		case "", "sans-serif", "default", "ui-sans-serif", "-apple-system":
			// ★ 2026-09 D7「默认族」修复（第二轮）：以 Edge 基准实测为准
			//   （headless=new + --dump-dom 探针，24px 下 10 个 M 的宽度）：
			//     无声明 / sans-serif / ui-sans-serif / -apple-system
			//       → "Noto Sans SC"（194.88px）——浏览器这四个语义**同族**
			//     system-ui → "Microsoft YaHei"（234.49px，与 sans-serif 不同）
			//     Arial     → Arial（199.92px）
			//   上一轮把这一组映射到 "arial" 属**对齐错目标**：浏览器
			//   sans-serif 通用族解析到 Noto Sans SC（194.88），既非 Arial，
			//   也解释了未声明 font-family 的文档默认族同样是它。
			//   isGeneric=true 保证 Noto Sans SC 缺失时仍落到 m.sansTF
			//   （雅黑，黑体无衬线）兜底，绝不回落到衬线族。
			return "noto sans sc", true // generic → always available
		case "system-ui":
			// ★ 2026-10（system-ui 收口）：Edge 的 system-ui 解析到
			//   **Microsoft YaHei UI**（msyh.ttc 的 UI face），**不是** Microsoft YaHei。
			//   两者从同一 ttf 出，字形宽度完全相同（旧依据「24px M×10 = 234.49」
			//   因此区分不了它们），差异只在**垂直度量**。指纹矩阵
			//   （dev 夹具 systemui_matrix：12/14/16/20/24/32px ×
			//   Hamburgefonstiv / REC / 录制中 / REC 录制）实测行盒高：
			//     Microsoft YaHei     16 / 19 / 21 / 26 / 31 / 42   ← 旧映射落点
			//     Microsoft YaHei UI  15 / 18 / 20 / 25 / 30 / 41   ← Edge 的 system-ui（逐项吻合）
			//   即：MediaStudio #btn-record 的高 37（Edge 36）由此而来。
			//   见 dev/audit/WORKITEMS.md §25 的 A/B 表。
			return platformSystemUIFamily(), true // generic → always available
		case "serif", "ui-serif":
			// ★ Edge 基准实测（24px 三维指纹 M10/I10/line-height:normal 行盒高）：
			//   serif → Noto Serif SC（234 / 97.2 / 35）。此前映射到 kochi
			//   mincho（未加载）→ 最终落到 serifTF = SimSun（120 / 120 / 27），
			//   拉丁字形宽度只有 Edge 的一半（120 vs 234），整行文本全部重排。
			return platformSerifFamily(), true // generic → always available
		case "times", "times new roman":
			// 显式族名 → 真 Times New Roman（Edge 指纹 213.4 / 79.92 / 27）。
			// 此前并入 serif 组 → 落到宋体（120 / 120 / 27），M 宽差 78%。
			return "times new roman", false
		case "monospace", "mono", "ui-monospace":
			// ★ Edge 基准实测：monospace → SimSun（120 / 120 / 27），**不是**
			//   Consolas（131.95 / 131.95 / 28）—— 中文 Windows 的等宽默认字体
			//   是宋体，每字符宽 0.5em 而非 Consolas 的 0.5498em，长行会累积整行错位。
			return platformMonoFamily(), true // generic → always available
		case "arial", "helvetica":
			// ★ 不再硬别名到微软雅黑（D7 根因）：雅黑的拉丁字形显著宽于 Arial
			//   —— 实测 10 个 `M` @24px：雅黑 234.5px，浏览器 Arial 197px
			//   （+19%）。凡声明 `font-family: Arial, Helvetica, sans-serif`
			//   的页面（Bootstrap/绝大多数站点），文字度量、换行位置与行数都会
			//   与浏览器不一致；Vue/React 夹具那 5% 的像素差异主要来自这里。
			//   Helvetica 在 Windows 上通常缺失，浏览器按度量兼容替换为 Arial，
			//   这里同样落到 arial（白名单已加载 arial.ttf，classifyFont 记为
			//   "arial"）。中文仍由绘制层的 CJK 字体回退处理（见 canvas.go 的
			//   getCJKSkiaFont），不会出现豆腐块。
			return "arial", false
		case "segoe ui":
			// 白名单加载 segoeui*.ttf，classifyFont 按文件名记为 "segoeui"。
			return "segoeui", false
		case "segoe ui symbol":
			return "seguisym", false
		case "segoe ui emoji":
			return "seguiemj", false
		case "microsoft yahei", "微软雅黑",
			"pingfang sc", "pingfang", "hiragino sans gb", "hiragino", "simhei",
			"heiti", "stheiti", "songti", "wenquanyi zen hei", "wenquanyi", "noto sans cjk":
			// ★ 2026-10-19：按平台落到各自真实 CJK 族。旧实现一律返回
			// "microsoft yahei"，macOS/Linux 上 osLookup 与 findBest 全 miss
			// → 最终连兜底都拿不到 CJK 字体 → 中文方块。
			return platformCJKFamily(f), false
		case "microsoft yahei ui":
			// 显式声明 UI face：与 system-ui 同落点（见上）。此前并入上面的雅黑组，
			// 导致显式 `font-family: "Microsoft YaHei UI"` 也拿到雅黑度量（+1px）。
			return "microsoft yahei ui", false
		case "simsun", "nsimsun":
			// ★ 宋体/新宋体是**具体的衬线族**，不是雅黑的别名：实测把两者并入
			//   雅黑组后，`font-family: SimSun` 的拉丁 advance 变成 234.49
			//   （雅黑），而 Edge 为 120（宋体西文半宽），行盒高 31 vs 27。
			//   osLookup 可把两者解析到真 face（SimSun / NSimSun）。
			return "simsun", false
		case "noto sans sc", "noto sans", "notosanssc":
			// 浏览器基准的默认无衬线族（见上）：显式声明同样解析到此目标。
			// 非 Windows 上 Linux 装的是 Noto Sans CJK SC 合集（无独立
			// NotoSansSC-VF），按平台映射过去。
			if runtime.GOOS == "windows" {
				return "noto sans sc", false
			}
			return platformCJKFamily("noto sans cjk"), false
		case "roboto":
			return "roboto", false
		}
		return f, false
	}

	for _, fam := range families {
		targetFamily, isGeneric := resolveFamily(fam)
		// OS 名查找优先（skia.NewTypeface）：
		//   - CJK 族：TTC faces loaded via NewTypefaceFromData may fail to
		//     render CJK glyphs (tofu); OS faces keep Skia's system fallback
		//     chain, so Chinese text renders instead of showing boxes.
		//   - 西文族（Arial / Segoe UI …）：从文件加载的 face 没有斜体/粗体
		//     变体，italic 请求会拿到 regular face 并触发双重斜切
		//     （typeface_weight_test 的 TestTypefaceItalicSystemLookup 正是
		//     这个回归）——交给 Skia 按 FontStyle 匹配正确的 face。
		// 表外的族返回 nil，继续走下面的 findBest，不截断 CSS font-family
		// 的回退链。
		if tf := m.osLookup(targetFamily, weight, italic); tf != nil {
			return tf
		}
		// Try exact family + weight/italic match first.
		if tf := m.findBest(targetFamily, weight, italic); tf != nil {
			return tf
		}
		// Try ignoring weight/italic and just pick any face of that family.
		for _, f := range m.fonts {
			if f.family == targetFamily {
				return f.tf
			}
		}
		// Generic families always resolve (they terminate the fallback chain).
		if isGeneric {
			switch fam {
			case "", "sans-serif", "default", "ui-sans-serif", "-apple-system":
				return m.sansTF
			case "system-ui":
				return m.sansTF
			case "serif", "times", "times new roman", "ui-serif":
				return m.serifTF
			case "monospace", "mono", "ui-monospace":
				return m.monoTF
			}
		}
	}
	// Last resort: default sans-serif (CJK-capable) so text is never blank.
	return m.defaultTF
}

// osLookup resolves a family via the platform font name (skia.NewTypeface),
// which keeps Skia's system fallback chain — unlike NewTypefaceFromData TTC
// faces that may fail to render CJK glyphs.
func (m *FontManager) osLookup(targetFamily string, weight int, italic bool) *skia.Typeface {
	var osName string
	switch targetFamily {
	case "microsoft yahei", "微软雅黑":
		osName = "Microsoft YaHei"
	case "microsoft yahei ui":
		// msyh.ttc 除 Microsoft YaHei 外还含 UI face（注册表名
		// 「Microsoft YaHei & Microsoft YaHei UI」），DirectWrite 按族名可区分，
		// 其 hhea 等垂直度量不同于 YaHei（行盒少 1px @12px，见 resolveFamily 注释）。
		osName = "Microsoft YaHei UI"
	case "nsimsun":
		osName = "NSimSun"
	case "simsun":
		osName = "SimSun"
	case "consolas":
		osName = "Consolas"
	case "arial":
		osName = "Arial"
	case "noto sans sc":
		// 系统装有 NotoSansSC-VF.ttf（Windows Fonts）；DirectWrite 名查找可用
		// （探针实测 skia.NewTypeface("Noto Sans SC") != nil）。取不到时
		// 上层 isGeneric 分支回落 m.sansTF。
		osName = "Noto Sans SC"
	case "noto serif sc":
		// 系统装有 NotoSerifSC-VF.ttf；Edge 的 serif 通用族实测解析到它
		// （24px 指纹 234 / 97.2 / 35）。取不到时回落 m.serifTF。
		osName = "Noto Serif SC"
	case "times new roman":
		// times.ttf（Windows Fonts）；Edge 的显式 Times New Roman 指纹
		// 213.4 / 79.92 / 27。
		osName = "Times New Roman"
	case "courier new":
		// cour.ttf（Windows Fonts）；Edge 指纹 144.02 / 144.02 / 27。
		// 缺此分支时落到默认衬线/无衬线兜底（本机实测退化为雅黑 234.49 /
		// 70.55 / 31），等宽文本宽度差 62%。
		osName = "Courier New"
	case "segoeui":
		osName = "Segoe UI"
	case "seguisym":
		osName = "Segoe UI Symbol"
	case "seguiemj":
		osName = "Segoe UI Emoji"
	// ── ★ 2026-10-19：macOS / Linux 平台族名（CoreText / fontconfig 名）──
	// 缺这些分支时，非 Windows 上 osLookup 一律返回 nil，CJK 族只能靠
	// findBest 命中已加载文件；一旦白名单/族名有一处对不上就退化成方块。
	case "pingfang sc", "pingfang":
		osName = "PingFang SC"
	case "hiragino sans gb", "hiragino":
		osName = "Hiragino Sans GB"
	case "heiti sc", "stheiti":
		osName = "Heiti SC"
	case "songti sc", "songti":
		osName = "Songti SC"
	case "arial unicode ms":
		osName = "Arial Unicode MS"
	case "noto sans cjk sc":
		osName = "Noto Sans CJK SC"
	case "noto sans cjk jp":
		osName = "Noto Sans CJK JP"
	case "noto sans cjk kr":
		osName = "Noto Sans CJK KR"
	case "noto sans cjk tc":
		osName = "Noto Sans CJK TC"
	case "noto sans cjk hk":
		osName = "Noto Sans CJK HK"
	case "noto sans cjk":
		osName = "Noto Sans CJK SC"
	case "noto serif cjk sc":
		osName = "Noto Serif CJK SC"
	case "noto serif cjk":
		osName = "Noto Serif CJK SC"
	case "wenquanyi zen hei":
		osName = "WenQuanYi Zen Hei"
	case "wenquanyi micro hei":
		osName = "WenQuanYi Micro Hei"
	case "droidsansfallback":
		osName = "Droid Sans Fallback"
	case "source han sans sc":
		osName = "Source Han Sans SC"
	case "dejavu sans":
		osName = "DejaVu Sans"
	case "dejavu sans mono":
		osName = "DejaVu Sans Mono"
	case "dejavu serif":
		osName = "DejaVu Serif"
	case "liberation sans":
		osName = "Liberation Sans"
	case "liberation mono":
		osName = "Liberation Mono"
	case "liberation serif":
		osName = "Liberation Serif"
	case "menlo":
		osName = "Menlo"
	case "monaco":
		osName = "Monaco"
	case "helvetica":
		osName = "Helvetica"
	case "helvetica neue":
		osName = "Helvetica Neue"
	case "sf mono":
		osName = "SF Mono"
	case "freesans":
		osName = "FreeSans"
	case "freeserif":
		osName = "FreeSerif"
	case "freemono":
		osName = "FreeMono"
	case "apple color emoji":
		osName = "Apple Color Emoji"
	case "apple symbols":
		osName = "Apple Symbols"
	default:
		return nil
	}
	slant := skia.FontSlantUpright
	if italic {
		slant = skia.FontSlantItalic
	}
	return skia.NewTypeface(osName, skia.FontStyle{Weight: weight, Width: 5, Slant: slant})
}

// platformSystemUIFamily 返回当前平台 system-ui 的落点族名。
// Windows 保持既有对齐：Microsoft YaHei UI（msyh.ttc 的 UI face）。
func platformSystemUIFamily() string {
	switch runtime.GOOS {
	case "windows":
		return "microsoft yahei ui"
	case "darwin":
		return "pingfang sc"
	default:
		return "noto sans cjk sc"
	}
}

// platformSerifFamily 返回当前平台 serif 通用族的落点族名。
func platformSerifFamily() string {
	switch runtime.GOOS {
	case "windows":
		return "noto serif sc"
	case "darwin":
		return "songti sc"
	default:
		return "noto serif cjk sc"
	}
}

// platformMonoFamily 返回当前平台 monospace 通用族的落点族名。
func platformMonoFamily() string {
	switch runtime.GOOS {
	case "windows":
		return "simsun"
	case "darwin":
		return "menlo"
	default:
		return "dejavu sans mono"
	}
}

// platformCJKFamily 把跨平台 CJK 别名映射到当前平台的真实族名。
// 旧实现不分平台一律 "microsoft yahei"：macOS/Linux 上没有该族，
// osLookup/findBest 双双 miss，CJKTypeface 最终退化成西文字体。
func platformCJKFamily(alias string) string {
	if runtime.GOOS == "windows" {
		return "microsoft yahei"
	}
	a := strings.ToLower(alias)
	switch {
	case strings.Contains(a, "pingfang"), strings.Contains(a, "hiragino"),
		strings.Contains(a, "heiti"), strings.Contains(a, "songti"):
		if runtime.GOOS == "darwin" {
			switch {
			case strings.Contains(a, "hiragino"):
				return "hiragino sans gb"
			case strings.Contains(a, "songti"):
				return "songti sc"
			default:
				return "pingfang sc"
			}
		}
		return "noto sans cjk sc"
	case strings.Contains(a, "wenquanyi"):
		if runtime.GOOS == "darwin" {
			return "pingfang sc"
		}
		return "wenquanyi zen hei"
	case strings.Contains(a, "noto"):
		if runtime.GOOS == "darwin" {
			return "pingfang sc"
		}
		return "noto sans cjk sc"
	case strings.Contains(a, "simhei"):
		if runtime.GOOS == "darwin" {
			return "pingfang sc"
		}
		return "noto sans cjk sc"
	}
	if runtime.GOOS == "darwin" {
		return "pingfang sc"
	}
	return "noto sans cjk sc"
}

// splitFontFamily splits a CSS font-family value into individual family names,
// trimming whitespace and surrounding quotes. e.g. '"Segoe UI", sans-serif' �?
// ["segoe ui", "sans-serif"].
func splitFontFamily(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"'")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// DefaultTypeface returns the default sans-serif Typeface (prefers CJK
// coverage), mirroring FontCache::getCachedFontPlatformData for the default
// font description.
func (m *FontManager) DefaultTypeface() *skia.Typeface {
	if m == nil {
		return nil
	}
	return m.defaultTF
}

// EmojiTypeface returns the loaded emoji Typeface (e.g. Segoe UI Emoji), or
// nil if no emoji font is available.
func (m *FontManager) EmojiTypeface() *skia.Typeface {
	if m == nil {
		return nil
	}
	return m.emojiTF
}

// SymbolTypeface returns the loaded symbol Typeface (e.g. Segoe UI Symbol), or
// nil if no symbol font is available. Used to render geometric shapes, arrows,
// and other symbols that the primary font may lack.
func (m *FontManager) SymbolTypeface() *skia.Typeface {
	if m == nil {
		return nil
	}
	return m.symbolTF
}

// CJKTypeface returns the CJK fallback Typeface used to render
// Chinese/Japanese/Korean characters that the primary font lacks
// (e.g. Consolas, Arial, DejaVu Sans).
//
// ★ 2026-10-19（多平台中文字体，P0）：旧实现找不到 cjkTF 时无条件回落
// sansTF → 在 macOS/Linux 上 sansTF 是无 CJK 覆盖的西文字体（Arial /
// DejaVu Sans），中文因此全部渲染成豆腐块。现在每次回落都必须通过
// isCJKCapable（能画「中」）校验；确实没有 CJK 字体时返回 nil，
// 让调用方显式看到「缺字体」，而不是拿到一个画不出中文的字体。
func (m *FontManager) CJKTypeface() *skia.Typeface {
	if m == nil {
		return nil
	}
	if isCJKCapable(m.cjkTF) {
		return m.cjkTF
	}
	if isCJKCapable(m.sansTF) {
		return m.sansTF
	}
	if isCJKCapable(m.defaultTF) {
		return m.defaultTF
	}
	for _, f := range m.fonts {
		if isCJKCapable(f.tf) {
			return f.tf
		}
	}
	return nil
}

// CJKFamilyName 返回当前选中的 CJK 族名（诊断/日志用，如 "PingFang SC"）。
// 未命中 CJK 字体时返回空串。
func (m *FontManager) CJKFamilyName() string {
	if m == nil {
		return ""
	}
	if m.cjkFamily != "" {
		return m.cjkFamily
	}
	if isCJKCapable(m.cjkTF) {
		return "(unknown)"
	}
	return ""
}

// TypefaceWeight returns the CSS weight of the loaded Typeface (400 for regular,
// 700 for bold), or 0 if the Typeface is not in the loaded set. Callers use this
// to decide whether synthetic embolden (SetEmbolden) is needed: if the returned
// weight is already >= 600 the Typeface is a real bold face and no faux bold
// should be applied.
//
// ★ 2026-08-24 修复（描边锯齿）：osLookup（skia.NewTypeface 系统名查找，
// 用于 Microsoft YaHei/Segoe UI 等）返回的 Typeface 不在 m.fonts 集合中，
// 此前一律返回 0 → 真 Bold（weight 700）被误判为非粗体 → getSkiaFont
// 叠加 SetEmbolden(true) 二次合成加粗 → Skia 把字形按位图膨胀，glyph
// 描边（-webkit-text-stroke）作用在膨胀后的像素轮廓上 → 严重锯齿/碎屑。
// 现在回退查询 Skia 的真实 weight（sk_typeface_get_font_weight）。
func (m *FontManager) TypefaceWeight(tf *skia.Typeface) int {
	if m == nil || tf == nil {
		return 0
	}
	m.mu.RLock()
	for _, f := range m.fonts {
		if f.tf == tf {
			m.mu.RUnlock()
			return f.weight
		}
	}
	m.mu.RUnlock()
	// 不在加载集合中（平台名查找/回退字体）：用 Skia 的真实 weight。
	return tf.Weight()
}

// TypefaceIsItalic reports whether the typeface is an italic/oblique variant.
// Returns false when the typeface is not found in the manager.
//
// ★ 2026-08-24：与 TypefaceWeight 同理，osLookup 系统字体不在 m.fonts 中，
// 回退用 Skia 真实 slant 判断（避免真实 italic 字体被 SetSkewX 二次斜切）。
func (m *FontManager) TypefaceIsItalic(tf *skia.Typeface) bool {
	if m == nil || tf == nil {
		return false
	}
	m.mu.RLock()
	for _, f := range m.fonts {
		if f.tf == tf {
			m.mu.RUnlock()
			return f.italic
		}
	}
	m.mu.RUnlock()
	return tf.Slant() != skia.FontSlantUpright
}

// GetGlobalDebugLogger is a hook reserved for debug logging; returns nil when
// no logger is installed. Kept here so the package compiles without a logger
// dependency.
func GetGlobalDebugLogger() interface{} { return nil }
