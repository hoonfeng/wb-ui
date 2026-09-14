package graphics

import (
	"fmt"
	"testing"
)

func TestDiagSymbolU2304(t *testing.T) {
	mgr := GetFontManager()
	if mgr == nil {
		_ = InitFontManager("")
		mgr = GetFontManager()
	}
	if mgr == nil {
		t.Skip("no font manager")
	}
	mgr.LoadSystemFonts()
	sym := mgr.SymbolTypeface()
	if sym == nil {
		t.Skip("no symbol typeface")
	}
	for _, r := range []rune{'⌄', '›', '▸', '▾'} {
		g := sym.UnicharToGlyph(r)
		fmt.Printf("Symbol U+%04X %q glyph=%d\n", r, string(r), g)
	}
	mono := mgr.LookupTypeface("Consolas", 400, "normal")
	if mono != nil {
		for _, r := range []rune{'⌄', '›'} {
			g := mono.UnicharToGlyph(r)
			fmt.Printf("Consolas U+%04X %q glyph=%d\n", r, string(r), g)
		}
	}
}
