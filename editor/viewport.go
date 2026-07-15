// Translation of: CodeMirror 6 — packages/view/src/viewport.ts
//                  https://github.com/codemirror/view/blob/main/src/viewport.ts
//
// Completeness: 50%
// Differences from CM6:
//   - CM6 tracks line blocks with a height map for variable-height content;
//     this port assumes uniform line height (sufficient for code editors
//     with monospace fonts).
//   - Methods use PascalCase (Go convention).
//
// The viewport describes which part of the document is currently visible.
// The EditorView uses it to implement virtual scrolling — only lines within
// the viewport (plus a small buffer) are rendered.

package editor

// Viewport describes the visible range of the document.
type Viewport struct {
	// From is the start of the visible range (rune offset, inclusive).
	From int
	// To is the end of the visible range (rune offset, exclusive).
	To int
}

// BlockInfo describes a line block's geometry for virtual scrolling.
// In CM6, blocks can have variable heights (e.g. wrapped lines, widgets).
// This port assumes uniform height, so BlockInfo is simplified.
type BlockInfo struct {
	// Top is the Y coordinate of the block's top edge (relative to the
	// content area).
	Top float64
	// Height is the block's height in pixels.
	Height float64
	// LineFrom is the first line number in this block (0-based).
	LineFrom int
	// LineTo is the last line number in this block (exclusive).
	LineTo int
}

// NewViewport creates a Viewport covering the given range.
func NewViewport(from, to int) Viewport {
	return Viewport{From: from, To: to}
}

// Empty reports whether the viewport covers no content.
func (vp Viewport) Empty() bool { return vp.From >= vp.To }

// Contains reports whether the given position is within the viewport.
func (vp Viewport) Contains(pos int) bool {
	return pos >= vp.From && pos < vp.To
}
