// Translation of: CodeMirror 6 — packages/view/src/view.ts
//                  https://github.com/codemirror/view/blob/main/src/view.ts
//
// Completeness: 40%
// Differences from CM6:
//   - CM6 renders to a real DOM tree; this port renders to a Skia Canvas
//     via the graphics package, and optionally maintains a dom.Element for
//     integration with wb-ui's event system.
//   - CM6 uses a contentEditable div for input; this port handles input
//     directly via keyboard/mouse events routed from the host.
//   - The extension/plugin system is simplified (no DOM lifecycle).
//   - Methods use PascalCase (Go convention).
//
// EditorView is the view-layer of the editor. It holds the current
// EditorState, applies transactions, and renders the document to a Skia
// Canvas. It also handles input (mouse, keyboard) and maintains an
// undo/redo history.

package editor

import (
	"wb-ui/platform/graphics"
)

// EditorViewConfig holds the configuration for creating an EditorView.
type EditorViewConfig struct {
	// State is the initial editor state.
	State EditorState
	// Decorations is the initial set of decorations (e.g. syntax
	// highlighting).
	Decorations DecorationSet
	// Font is the font used for rendering text.
	Font graphics.Font
	// Theme is the highlight style for syntax coloring.
	Theme *HighlightStyle
	// Lang is the language tokenizer (nil for plain text).
	Lang *MonarchLanguage
	// ShowLineNumbers enables the line number gutter.
	ShowLineNumbers bool
	// Width and Height are the content area dimensions in pixels.
	Width  float64
	Height float64
}

// EditorView is the main view structure for the code editor.
type EditorView struct {
	// State is the current editor state (immutable; replaced on update).
	state EditorState
	// decorations holds the current decoration set.
	decorations DecorationSet
	// layout holds the geometric properties (recalculated on font/doc change).
	layout EditorLayout
	// viewport describes the visible range of the document.
	viewport Viewport

	// Font is the font used for text rendering.
	font graphics.Font
	// theme is the highlight style for syntax coloring.
	theme *HighlightStyle
	// lang is the language tokenizer (nil for plain text).
	lang *MonarchLanguage
	// showLineNumbers controls the line number gutter.
	showLineNumbers bool

	// width/height are the content area dimensions.
	width  float64
	height float64

	// scrollX/scrollY are the scroll offsets (for virtual scrolling).
	scrollX float64
	scrollY float64

	// history holds the undo/redo stacks.
	history *History

	// keymap holds the key bindings.
	keymap []KeyBinding

	// cachedTokens holds the tokenized lines (cache invalidated on doc change).
	cachedTokens []TokenLine
	// tokenCacheValid tracks whether the token cache is valid.
	tokenCacheValid bool
}

// NewEditorView creates an EditorView from the given configuration.
func NewEditorView(config EditorViewConfig) *EditorView {
	v := &EditorView{
		state:           config.State,
		decorations:     config.Decorations,
		font:            config.Font,
		theme:           config.Theme,
		lang:            config.Lang,
		showLineNumbers: config.ShowLineNumbers,
		width:           config.Width,
		height:          config.Height,
		history:         NewHistory(),
		tokenCacheValid: false,
	}

	// Set default font if not specified.
	if v.font.Size <= 0 {
		v.font.Size = 14
		v.font.Family = "Consolas"
	}

	// Calculate initial layout.
	v.recalculateLayout()

	// Initialize viewport to cover the entire visible area.
	v.updateViewport()

	return v
}

// State returns the current editor state.
func (v *EditorView) State() EditorState { return v.state }

// Decorations returns the current decoration set.
func (v *EditorView) Decorations() DecorationSet { return v.decorations }

// Layout returns the current layout properties.
func (v *EditorView) Layout() EditorLayout { return v.layout }

// Viewport returns the current viewport.
func (v *EditorView) Viewport() Viewport { return v.viewport }

// Font returns the current font.
func (v *EditorView) Font() graphics.Font { return v.font }

// SetFont changes the font and recalculates the layout.
func (v *EditorView) SetFont(font graphics.Font) {
	v.font = font
	v.recalculateLayout()
	v.updateViewport()
}

// SetSize updates the content area dimensions.
func (v *EditorView) SetSize(width, height float64) {
	v.width = width
	v.height = height
	v.updateViewport()
}

// SetScroll updates the scroll offset.
func (v *EditorView) SetScroll(scrollX, scrollY float64) {
	v.scrollX = scrollX
	v.scrollY = scrollY
	v.updateViewport()
}

// ScrollY returns the vertical scroll offset.
func (v *EditorView) ScrollY() float64 { return v.scrollY }

// History returns the undo/redo history.
func (v *EditorView) History() *History { return v.history }

// SetDecorations updates the decoration set.
func (v *EditorView) SetDecorations(ds DecorationSet) {
	v.decorations = ds
}

// SetTheme updates the highlight style.
func (v *EditorView) SetTheme(theme *HighlightStyle) {
	v.theme = theme
}

// SetLanguage updates the language tokenizer.
func (v *EditorView) SetLanguage(lang *MonarchLanguage) {
	v.lang = lang
	v.tokenCacheValid = false
}

// recalculateLayout recalculates the EditorLayout based on the current
// font and document.
func (v *EditorView) recalculateLayout() {
	lineCount := v.state.Doc.Lines()
	if lineCount < 1 {
		lineCount = 1
	}
	v.layout = MeasureLayout(v.font, lineCount, v.showLineNumbers)
}

// updateViewport recalculates the visible document range based on the
// scroll offset and content area height.
func (v *EditorView) updateViewport() {
	if v.layout.LineHeight <= 0 || v.height <= 0 {
		v.viewport = Viewport{From: 0, To: v.state.Doc.Length()}
		return
	}

	// Calculate the first visible line from the scroll offset.
	firstLine := int(v.scrollY / v.layout.LineHeight)
	if firstLine < 0 {
		firstLine = 0
	}

	// Calculate the number of visible lines.
	visibleLines := int(v.height / v.layout.LineHeight) + 1 // +1 for partial lines
	lastLine := firstLine + visibleLines

	totalLines := v.state.Doc.Lines()
	if lastLine >= totalLines {
		lastLine = totalLines - 1
		if lastLine < 0 {
			lastLine = 0
		}
	}

	// Convert line numbers to rune offsets.
	doc := v.state.Doc
	fromPos := 0
	toPos := doc.Length()
	if firstLine > 0 {
		line := doc.LineN(firstLine + 1) // 1-based
		fromPos = line.From
	}
	if lastLine >= 0 && lastLine < totalLines {
		line := doc.LineN(lastLine + 1) // 1-based
		toPos = line.To
		if toPos < doc.Length() {
			toPos++ // include the trailing newline
		}
	}

	v.viewport = Viewport{From: fromPos, To: toPos}
}

// Dispatch applies a transaction spec to the view. This is the main entry
// point for state updates. It:
//  1. Applies the spec to produce a new state.
//  2. Maps decorations through the change set.
//  3. Records the transaction in history (if it's a user edit).
//  4. Recalculates layout and viewport.
func (v *EditorView) Dispatch(spec TransactionSpec) {
	// Apply the transaction to get the new state and transaction.
	newState, tr := v.state.Update(spec)

	// Map decorations through the changes.
	if !tr.Changes().Empty() {
		v.decorations = v.decorations.Map(tr.Changes())
	}

	// Record in history.
	if tr.UserEvent() != "" {
		v.history.Record(tr)
	}

	// Update state.
	v.state = newState

	// Invalidate token cache.
	v.tokenCacheValid = false

	// Recalculate layout and viewport.
	v.recalculateLayout()
	v.updateViewport()
}

// Tokens returns the tokenized lines for the current document. The result
// is cached until the document changes.
func (v *EditorView) Tokens() []TokenLine {
	if v.tokenCacheValid {
		return v.cachedTokens
	}
	if v.lang != nil {
		v.cachedTokens = TokenizeLines(v.lang, v.state.Doc.String())
	} else {
		// No language; create empty token lines.
		v.cachedTokens = nil
	}
	v.tokenCacheValid = true
	return v.cachedTokens
}

// PosToXY converts a rune offset to pixel coordinates within the editor
// content area (accounting for scroll offset).
func (v *EditorView) PosToXY(pos int) (x, y float64) {
	x, y = PosToXY(v.layout, v.state.Doc, pos)
	// Adjust for scroll.
	x -= v.scrollX
	y -= v.scrollY
	// Add content offset.
	x += v.layout.ContentX
	y += v.layout.ContentY
	return x, y
}

// XYToPos converts pixel coordinates within the editor content area to a
// rune offset (accounting for scroll offset).
func (v *EditorView) XYToPos(x, y float64) int {
	// Adjust for content offset.
	x -= v.layout.ContentX
	y -= v.layout.ContentY
	// Adjust for scroll.
	x += v.scrollX
	y += v.scrollY
	return XYToPos(v.layout, v.state.Doc, x, y)
}

// TotalContentHeight returns the total height of all lines (for scrollbar
// calculation).
func (v *EditorView) TotalContentHeight() float64 {
	return TotalHeight(v.layout)
}
