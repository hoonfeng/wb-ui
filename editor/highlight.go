// HighlightStyle maps token scopes to visual styles (color, bold, italic).
// Based on CodeMirror 6's @codemirror/language HighlightStyle and VS Code's
// TextMate theme system.
//
// A scope like "keyword.control.go" is matched against the style map by
// checking progressively shorter prefixes: first "keyword.control.go",
// then "keyword.control", then "keyword". This mirrors TextMate scope
// matching.

package editor

// Color is an RGBA color value (8 bits per channel).
type Color struct {
	R, G, B, A uint8
}

// RGBA returns the color as a 32-bit packed value (0xRRGGBBAA).
func (c Color) RGBA() uint32 {
	return uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A)
}

// TextStyle describes the visual style for a token: color and font modifiers.
type TextStyle struct {
	// Color is the text color. A zero value (R=G=B=A=0) means "inherit"
	// (use the default text color).
	Color Color
	// Bold makes the text bold.
	Bold bool
	// Italic makes the text italic.
	Italic bool
	// Underline draws an underline under the text.
	Underline bool
}

// HighlightStyle maps scope names to TextStyle values. Scopes are matched
// by prefix: the token ["keyword", "control", "go"] is matched against
// "keyword.control.go", then "keyword.control", then "keyword".
type HighlightStyle struct {
	// styles maps scope strings (dot-separated, e.g. "keyword.control")
	// to TextStyle values.
	styles map[string]TextStyle
	// background is the default background color for the editor.
	background Color
	// foreground is the default text color.
	foreground Color
	// selection is the selection background color.
	selection Color
	// cursor is the caret color.
	cursor Color
	// lineNumber is the line number gutter text color.
	lineNumber Color
	// activeLineNumber is the active line number color.
	activeLineNumber Color
	// activeLine is the active line background highlight.
	activeLine Color
}

// NewHighlightStyle creates an empty HighlightStyle with the given
// theme colors.
func NewHighlightStyle(bg, fg, sel, cur, ln, activeLn, activeLine Color) *HighlightStyle {
	return &HighlightStyle{
		styles:            make(map[string]TextStyle),
		background:        bg,
		foreground:        fg,
		selection:         sel,
		cursor:            cur,
		lineNumber:        ln,
		activeLineNumber: activeLn,
		activeLine:        activeLine,
	}
}

// Set associates a scope string with a TextStyle.
func (h *HighlightStyle) Set(scope string, style TextStyle) {
	h.styles[scope] = style
}

// Get returns the TextStyle for the given scope list, matching by
// progressively shorter prefixes. Returns the foreground color if no
// match is found.
func (h *HighlightStyle) Get(scopes []string) TextStyle {
	// Try progressively shorter scope prefixes.
	for n := len(scopes); n > 0; n-- {
		scope := scopes[0]
		for i := 1; i < n; i++ {
			scope += "." + scopes[i]
		}
		if style, ok := h.styles[scope]; ok {
			return style
		}
	}
	return TextStyle{Color: h.foreground}
}

// GetByString returns the TextStyle for a dot-separated scope string.
func (h *HighlightStyle) GetByString(scope string) TextStyle {
	return h.Get(parseScopes(scope))
}

// Background returns the editor background color.
func (h *HighlightStyle) Background() Color { return h.background }

// Foreground returns the default text color.
func (h *HighlightStyle) Foreground() Color { return h.foreground }

// Selection returns the selection background color.
func (h *HighlightStyle) Selection() Color { return h.selection }

// Cursor returns the caret color.
func (h *HighlightStyle) Cursor() Color { return h.cursor }

// LineNumber returns the line number gutter text color.
func (h *HighlightStyle) LineNumber() Color { return h.lineNumber }

// ActiveLineNumber returns the active line number color.
func (h *HighlightStyle) ActiveLineNumber() Color { return h.activeLineNumber }

// ActiveLine returns the active line background highlight color.
func (h *HighlightStyle) ActiveLine() Color { return h.activeLine }

// ----- Built-in themes -----

// ThemeDarkPlus returns a HighlightStyle matching VS Code's Dark+ theme.
// Colors are taken from the official VS Code Dark+ color scheme.
func ThemeDarkPlus() *HighlightStyle {
	bg := Color{30, 30, 30, 255}       // #1E1E1E
	fg := Color{212, 212, 212, 255}    // #D4D4D4
	sel := Color{38, 79, 120, 255}     // #264F78
	cur := Color{170, 170, 170, 255}   // #AAAAAA
	ln := Color{133, 133, 133, 255}    // #858585
	activeLn := Color{191, 191, 191, 255} // #BFBFBF
	activeLine := Color{37, 37, 38, 255}  // #252526

	h := NewHighlightStyle(bg, fg, sel, cur, ln, activeLn, activeLine)

	// Keywords.
	h.Set("keyword", TextStyle{Color: Color{86, 156, 214, 255}})      // #569CD6
	h.Set("keyword.control", TextStyle{Color: Color{197, 134, 192, 255}}) // #C586C0
	// Strings.
	h.Set("string", TextStyle{Color: Color{206, 145, 120, 255}})      // #CE9178
	h.Set("string.escape", TextStyle{Color: Color{86, 156, 214, 255}}) // #569CD6
	// Comments.
	h.Set("comment", TextStyle{Color: Color{106, 153, 85, 255}, Italic: true}) // #6A9955
	// Numbers.
	h.Set("constant.numeric", TextStyle{Color: Color{181, 206, 168, 255}}) // #B5CEA8
	h.Set("constant.language", TextStyle{Color: Color{86, 156, 214, 255}})  // #569CD6
	// Functions.
	h.Set("support.function", TextStyle{Color: Color{220, 220, 170, 255}}) // #DCDCAA
	h.Set("entity.name.function", TextStyle{Color: Color{220, 220, 170, 255}})
	// Types.
	h.Set("support.type", TextStyle{Color: Color{78, 201, 176, 255}})     // #4EC9B0
	h.Set("entity.name.type", TextStyle{Color: Color{78, 201, 176, 255}})
	h.Set("entity.name.class", TextStyle{Color: Color{78, 201, 176, 255}})
	// Variables.
	h.Set("variable", TextStyle{Color: Color{156, 220, 254, 255}})       // #9CDCFE
	h.Set("variable.parameter", TextStyle{Color: Color{156, 220, 254, 255}})
	// Identifiers.
	h.Set("identifier", TextStyle{Color: Color{156, 220, 254, 255}})     // #9CDCFE
	// Operators.
	h.Set("keyword.operator", TextStyle{Color: Color{212, 212, 212, 255}}) // #D4D4D4
	// Punctuation.
	h.Set("punctuation", TextStyle{Color: Color{212, 212, 212, 255}})      // #D4D4D4
	// Source (default).
	h.Set("source", TextStyle{Color: Color{212, 212, 212, 255}})

	return h
}

// ThemeLight returns a HighlightStyle matching VS Code's Light+ theme.
func ThemeLight() *HighlightStyle {
	bg := Color{255, 255, 255, 255}       // #FFFFFF
	fg := Color{0, 0, 0, 255}             // #000000
	sel := Color{173, 214, 255, 255}      // #ADD6FF
	cur := Color{0, 0, 0, 255}           // #000000
	ln := Color{102, 102, 102, 255}      // #666666
	activeLn := Color{0, 0, 0, 255}
	activeLine := Color{245, 245, 245, 255} // #F5F5F5

	h := NewHighlightStyle(bg, fg, sel, cur, ln, activeLn, activeLine)

	h.Set("keyword", TextStyle{Color: Color{0, 0, 255, 255}})        // #0000FF
	h.Set("keyword.control", TextStyle{Color: Color{175, 0, 219, 255}}) // #AF00DB
	h.Set("string", TextStyle{Color: Color{163, 21, 21, 255}})        // #A31515
	h.Set("comment", TextStyle{Color: Color{0, 128, 0, 255}, Italic: true}) // #008000
	h.Set("constant.numeric", TextStyle{Color: Color{9, 134, 88, 255}}) // #098658
	h.Set("constant.language", TextStyle{Color: Color{0, 0, 255, 255}})
	h.Set("support.function", TextStyle{Color: Color{95, 95, 95, 255}}) // #5F5F5F
	h.Set("support.type", TextStyle{Color: Color{38, 127, 153, 255}}) // #267F99
	h.Set("variable", TextStyle{Color: Color{0, 16, 128, 255}})
	h.Set("identifier", TextStyle{Color: Color{0, 16, 128, 255}})
	h.Set("keyword.operator", TextStyle{Color: Color{0, 0, 0, 255}})
	h.Set("punctuation", TextStyle{Color: Color{0, 0, 0, 255}})
	h.Set("source", TextStyle{Color: Color{0, 0, 0, 255}})

	return h
}
