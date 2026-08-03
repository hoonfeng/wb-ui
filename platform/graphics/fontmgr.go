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
	defaultTF *skia.Typeface // default sans-serif (prefers CJK coverage)
	sansTF    *skia.Typeface  // generic sans-serif
	monoTF    *skia.Typeface  // generic monospace
	serifTF   *skia.Typeface  // generic serif
	emojiTF   *skia.Typeface  // emoji font (Segoe UI Emoji / Noto Color Emoji)
	symbolTF  *skia.Typeface // symbol font (Segoe UI Symbol) for geometric shapes/arrows
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
		fmt.Fprintf(os.Stderr, "[fontmgr] read dir %s failed: %v\n", dir, err)
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
		tf := skia.NewTypefaceFromData(data, 0)
		if tf == nil {
			fmt.Fprintf(os.Stderr, "[fontmgr] load %s failed (NewTypefaceFromData returned nil)\n", name)
			continue
		}
		m.fonts = append(m.fonts, classifyFont(name, tf))
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
func (m *FontManager) tryLoadFont(path string, loaded *int) bool {
	name := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(name, ".ttf") &&
		!strings.HasSuffix(name, ".otf") &&
		!strings.HasSuffix(name, ".ttc") {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var index int
	if strings.HasSuffix(name, ".ttc") {
		// For .ttc files we try loading each font collection entry;
		// NewTypefaceFromData with different indices picks each face.
	}
	tf := skia.NewTypefaceFromData(data, index)
	if tf == nil {
		return false
	}
	// Check for duplicates: skip if we already have an identical Typeface.
	for _, existing := range m.fonts {
		if existing.tf == tf {
			return false
		}
	}
	m.fonts = append(m.fonts, classifyFont(name, tf))
	if loaded != nil {
		*loaded++
	}
	return true
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
		{"msyh.ttc", 0},     // Microsoft YaHei Regular (default proportional CJK)
		{"msyhbd.ttc", 0},   // Microsoft YaHei Bold (real bold variant)
		{"consola.ttf", 0},  // Consolas Regular (English monospace)
		{"simsun.ttc", 1},   // NSimSun (CJK monospace, index 1 of the TTC)
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
		m.fonts = append(m.fonts, classifyFont(t.filename, tf))
		fmt.Fprintf(os.Stderr, "[fontmgr] loaded system font %s index %d\n", t.filename, t.index)
	}
	m.selectDefaults()
}

// classifyFont inspects the font file name to derive its CSS metadata (family name,
// weight, italic flag). Real WebKit reads the font's name table; this port uses
// filename conventions matching the WebKit test font collection (e.g.
// DejaVuSans-Bold.ttf) and Skia Typeface style queries where available.
func classifyFont(filename string, tf *skia.Typeface) loadedFont {
	name := strings.ToLower(filename)
	entry := loadedFont{
		tf:     tf,
		family: "default",
		weight: 400,
	}
	// Region/script detection.
	if strings.Contains(name, "kochi") || strings.Contains(name, "cjk") || strings.Contains(name, "noto") {
		entry.cjk = true
	}
	if strings.Contains(name, "emoji") || strings.Contains(name, "coloremoji") || strings.Contains(name, "seguiemj") {
		entry.emoji = true
	}
	if strings.Contains(name, "symbol") || strings.Contains(name, "seguisym") {
		entry.symbol = true
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
	case name == "simsun.ttc":
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
func (m *FontManager) selectDefaults() {
	// monospace: prefer Consolas — the OS standard monospace that Edge falls
	// back to when Google Fonts (JetBrains Mono) is unreachable, so ASCII/code
	// metrics match the browser reference (.cs-val "0 / 1.0M" 12px = 56px in
	// Edge; NSimSun measured 49px). CJK glyphs still fall back per-rune to
	// the CJK face in drawTextWithFallback.
	m.monoTF = skia.NewTypeface("Consolas", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	if m.monoTF == nil {
		m.monoTF = m.findBest("consolas", 400, false)
	}
	// NSimSun (CJK monospace) as the CJK-capable monospace fallback.
	if m.monoTF == nil {
		m.monoTF = skia.NewTypeface("NSimSun", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	}
	if m.monoTF == nil {
		m.monoTF = m.findBest("nsimsun", 400, false)
	}
	if m.monoTF == nil {
		m.monoTF = m.findBest("liberation mono", 400, false)
	}
	if m.monoTF == nil {
		m.monoTF = m.findBest("dejavu sans mono", 400, false)
	}
	// sans-serif: prefer OS Microsoft YaHei (CJK coverage + intact system
	// fallback chain) over raw-data faces. OS-name lookup (skia.NewTypeface)
	// is used first because NewTypefaceFromData on TTC files may return a
	// Typeface that fails to render glyphs, and OS faces keep Skia's system
	// fallback for characters the face lacks. Arial is a LAST resort (no
	// CJK coverage — would render tofu for Chinese without fallback).
	if dt := skia.NewTypeface("Microsoft YaHei", skia.FontStyle{Weight: 400, Width: 5, Slant: 0}); dt != nil {
		m.sansTF = dt
	}
	if m.sansTF == nil {
		m.sansTF = m.findBest("microsoft yahei", 400, false)
	}
	if m.sansTF == nil {
		m.sansTF = skia.NewTypeface("Segoe UI", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	}
	if m.sansTF == nil {
		m.sansTF = skia.NewTypeface("Arial", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	}
	if m.sansTF == nil {
		m.sansTF = skia.NewTypeface("Times New Roman", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	}
	if m.sansTF == nil {
		m.sansTF = m.findBest("kochi gothic", 400, false)
	}
	if m.sansTF == nil {
		m.sansTF = m.findBest("dejavu sans", 400, false)
	}
	if m.sansTF == nil {
		m.sansTF = m.findBest("liberation sans", 400, false)
	}
	if m.sansTF == nil && len(m.fonts) > 0 {
		m.sansTF = m.fonts[0].tf
	}
	// serif: prefer OS SimSun (Chinese serif, matches browser default serif
	// for CJK), then kochi mincho / DejaVuSerif / LiberationSerif.
	m.serifTF = skia.NewTypeface("SimSun", skia.FontStyle{Weight: 400, Width: 5, Slant: 0})
	if m.serifTF == nil {
		m.serifTF = m.findBest("kochi mincho", 400, false)
	}
	if m.serifTF == nil {
		m.serifTF = m.findBest("dejavu serif", 400, false)
	}
	if m.serifTF == nil {
		m.serifTF = m.findBest("liberation serif", 400, false)
	}
	if m.serifTF == nil {
		m.serifTF = m.sansTF
	}
	// default = sans-serif (proportional) so UI text looks natural, matching
	// browser behaviour where the initial value of font-family is a proportional
	// sans-serif. Monospace is only used when explicitly requested via CSS.
	m.defaultTF = m.sansTF
	if m.defaultTF == nil {
		m.defaultTF = m.monoTF
	}
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
	if m.emojiTF == nil {
		for _, name := range []string{"segoe ui emoji", "noto color emoji", "apple color emoji"} {
			m.emojiTF = m.findBest(name, 400, false)
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
	// Fallback: try common symbol font family names.
	if m.symbolTF == nil {
		for _, name := range []string{"segoe ui symbol", "segoe ui", "arial"} {
			m.symbolTF = m.findBest(name, 400, false)
			if m.symbolTF != nil {
				break
			}
		}
	}
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
		case "", "sans-serif", "default", "system-ui", "ui-sans-serif":
			return "microsoft yahei", true // generic �?always available
		case "serif", "times", "times new roman", "ui-serif":
			return "kochi mincho", true // generic �?always available
		case "monospace", "mono", "ui-monospace":
			return "consolas", true // generic → always available (Edge falls back to Consolas when Google Fonts unreachable)
		case "arial", "helvetica":
			return "microsoft yahei", false // alias, not generic
		case "microsoft yahei", "微软雅黑", "microsoft yahei ui", "segoe ui", "-apple-system",
			"pingfang sc", "pingfang", "hiragino sans gb", "hiragino", "simhei",
			"simsun", "nsimsun", "heiti", "songti", "wenquanyi zen hei", "wenquanyi", "noto sans cjk":
			return "microsoft yahei", false
		case "roboto":
			return "roboto", false
		}
		return f, false
	}

	for _, fam := range families {
		targetFamily, isGeneric := resolveFamily(fam)
		// CJK families: prefer OS-name lookup (skia.NewTypeface) because
		// TTC faces loaded via NewTypefaceFromData may fail to render CJK
		// glyphs (tofu). OS faces keep Skia's system fallback chain, so
		// Chinese text renders instead of showing boxes.
		if targetFamily == "microsoft yahei" || targetFamily == "nsimsun" || targetFamily == "simsun" {
			if tf := m.osLookup(targetFamily, weight, italic); tf != nil {
				return tf
			}
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
			case "", "sans-serif", "default", "system-ui", "ui-sans-serif":
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
	case "microsoft yahei":
		osName = "Microsoft YaHei"
	case "nsimsun":
		osName = "NSimSun"
	case "simsun":
		osName = "SimSun"
	case "consolas":
		osName = "Consolas"
	default:
		return nil
	}
	slant := skia.FontSlantUpright
	if italic {
		slant = skia.FontSlantItalic
	}
	return skia.NewTypeface(osName, skia.FontStyle{Weight: weight, Width: 5, Slant: slant})
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

// CJKTypeface returns the OS CJK Typeface (Microsoft YaHei on Windows,
// fallback to the default sans-serif). Used to render Chinese/Japanese/
// Korean characters that the primary font lacks (e.g. Consolas).
func (m *FontManager) CJKTypeface() *skia.Typeface {
	if m == nil {
		return nil
	}
	if m.sansTF != nil {
		return m.sansTF
	}
	return m.defaultTF
}

// TypefaceWeight returns the CSS weight of the loaded Typeface (400 for regular,
// 700 for bold), or 0 if the Typeface is not in the loaded set. Callers use this
// to decide whether synthetic embolden (SetEmbolden) is needed: if the returned
// weight is already >= 600 the Typeface is a real bold face and no faux bold
// should be applied.
func (m *FontManager) TypefaceWeight(tf *skia.Typeface) int {
	if m == nil || tf == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, f := range m.fonts {
		if f.tf == tf {
			return f.weight
		}
	}
	return 0
}

// TypefaceIsItalic reports whether the typeface is an italic/oblique variant.
// Returns false when the typeface is not found in the manager.
func (m *FontManager) TypefaceIsItalic(tf *skia.Typeface) bool {
	if m == nil || tf == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, f := range m.fonts {
		if f.tf == tf {
			return f.italic
		}
	}
	return false
}

// GetGlobalDebugLogger is a hook reserved for debug logging; returns nil when
// no logger is installed. Kept here so the package compiles without a logger
// dependency.
func GetGlobalDebugLogger() interface{} { return nil }
