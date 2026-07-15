// Translation of: Source/WebCore/platform/graphics/FontCache.h
//                  Source/WebCore/platform/graphics/FontCache.cpp
// Completeness: 30%
//
// FontCache (FontManager) manages Typefaces loaded from font files on disk.
// In real WebKit the platform FontCache queries the OS font manager and
// handles cross-process font registration; this port loads a fixed set of
// .ttf/.otf files from a resource directory (shipped from WebKit's test
// font collection) and resolves CSS family/weight/style to a Typeface.

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
}

// FontManager is the Go translation of WebCore::FontCache. It owns the set of
// Typefaces loaded from disk and resolves CSS font descriptions to Typefaces.
// A single global instance is initialized once via InitFontManager.
type FontManager struct {
	mu        sync.RWMutex
	fonts     []loadedFont
	defaultTF *skia.Typeface // default sans-serif (prefers CJK coverage)
	sansTF    *skia.Typeface  // generic sans-serif
	monoTF    *skia.Typeface  // generic monospace
	serifTF   *skia.Typeface  // generic serif
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
		m := &FontManager{}
		m.loadDir(fontDir)
		globalFontMgr = m
	})
	return globalFontMgr
}

// GetFontManager returns the global FontManager, or nil if InitFontManager has
// not been called yet.
func GetFontManager() *FontManager { return globalFontMgr }

// loadDir reads every font file under dir and registers it.
func (m *FontManager) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fontmgr] read dir %s failed: %v\n", dir, err)
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

// LoadSystemFonts loads fonts from the Windows system font directory
// (C:\Windows\Fonts), mirroring the GWui font strategy:
//   - Microsoft YaHei (msyh.ttc) as the default proportional CJK font
//   - Microsoft YaHei Bold (msyhbd.ttf) as the real bold variant
//   - Consolas (consola.ttf) for ASCII monospace
//   - NSimSun (simsun.ttc index 1) for CJK monospace
//
// Using real bold typefaces (instead of synthetic SetEmbolden) produces
// correct font-weight rendering, matching how GWui loads separate bold files.
func (m *FontManager) LoadSystemFonts() {
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

// classifyFont inspects the font file name to derive its CSS metadata. Real
// WebKit reads the font's name table; this port uses filename conventions
// matching the WebKit test font collection (e.g. DejaVuSans-Bold.ttf).
func classifyFont(filename string, tf *skia.Typeface) loadedFont {
	name := strings.ToLower(filename)
	entry := loadedFont{
		tf:     tf,
		family: "default",
		weight: 400,
	}
	if strings.Contains(name, "kochi") || strings.Contains(name, "cjk") {
		entry.cjk = true
	}
	if strings.Contains(name, "emoji") || strings.Contains(name, "coloremoji") {
		entry.emoji = true
	}
	switch {
	case strings.HasPrefix(name, "msyhbd"):
		// Microsoft YaHei Bold — real bold variant (not synthetic embolden).
		entry.family = "microsoft yahei"
		entry.weight = 700
		entry.cjk = true
	case strings.HasPrefix(name, "msyh"):
		// Microsoft YaHei Regular — default proportional CJK font (like GWui).
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
		// index 1 of simsun.ttc is NSimSun (新宋体), a CJK monospace face.
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
	// monospace: prefer NSimSun (CJK + ASCII monospace), then Consolas, then
	// LiberationMono/DejaVuSansMono.
	m.monoTF = m.findBest("nsimsun", 400, false)
	if m.monoTF == nil {
		m.monoTF = m.findBest("consolas", 400, false)
	}
	if m.monoTF == nil {
		m.monoTF = m.findBest("liberation mono", 400, false)
	}
	if m.monoTF == nil {
		m.monoTF = m.findBest("dejavu sans mono", 400, false)
	}
	// sans-serif: prefer Microsoft YaHei (proportional CJK, like GWui), then
	// fall back to other sans-serif families.
	m.sansTF = m.findBest("microsoft yahei", 400, false)
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
	// serif: prefer CJK-capable serif (kochi mincho), fall back to DejaVuSerif
	m.serifTF = m.findBest("kochi mincho", 400, false)
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
	// Split the comma-separated list and try each family in order.
	families := splitFontFamily(raw)

	// resolveFamily maps a CSS family keyword to a loaded family name.
	// Returns "" if the family is a generic keyword that should always
	// resolve (terminating the fallback chain).
	resolveFamily := func(f string) (string, bool) {
		switch f {
		case "", "sans-serif", "default", "system-ui", "ui-sans-serif":
			return "microsoft yahei", true // generic — always available
		case "serif", "times", "times new roman", "ui-serif":
			return "kochi mincho", true // generic — always available
		case "monospace", "mono", "courier", "courier new", "consolas", "ui-monospace":
			return "nsimsun", true // generic — always available
		case "arial", "helvetica":
			return "microsoft yahei", false // alias, not generic
		case "microsoft yahei", "微软雅黑", "microsoft yahei ui", "segoe ui":
			return "microsoft yahei", false
		case "roboto":
			return "roboto", false
		}
		return f, false
	}

	for _, fam := range families {
		targetFamily, isGeneric := resolveFamily(fam)
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
			case "monospace", "mono", "courier", "courier new", "consolas", "ui-monospace":
				return m.monoTF
			}
		}
	}
	// Last resort: default sans-serif (CJK-capable) so text is never blank.
	return m.defaultTF
}

// splitFontFamily splits a CSS font-family value into individual family names,
// trimming whitespace and surrounding quotes. e.g. '"Segoe UI", sans-serif' →
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

// GetGlobalDebugLogger is a hook reserved for debug logging; returns nil when
// no logger is installed. Kept here so the package compiles without a logger
// dependency.
func GetGlobalDebugLogger() interface{} { return nil }
