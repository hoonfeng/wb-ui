// Translation of: Source/WebCore/rendering/RenderText.h
//                  Source/WebCore/rendering/RenderText.cpp
//                  Source/WebCore/rendering/RenderTextFragment.h
// Completeness: 50%
// Simplifications:
//   - text rendering uses the raw string as a single segment; the line-box split that
//     WebKit tracks via InlineTextBox lists is omitted (the inline formatting context
//     handles breaking internally)
//   - text-transform (uppercase/lowercase/capitalize) is applied lazily on read rather
//     than pre-computed
//   - secure (password masking) text rendering is omitted
//   - combined text (text-combine-upright) is omitted
//   - text autosizing is omitted

package rendering

import (
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/layout"
	"wb-ui/engine/style"
)

// InlineTextBox represents a segment of text that has been laid out within a single
// line box. It mirrors the geometry-bearing portion of WebCore::InlineTextBox. In this
// simplified port, segments are produced lazily by the inline formatting context and
// stored on the RenderText for later retrieval by the paint pipeline.
type InlineTextBox struct {
	// Start and Len are the rune offset and length of this segment within the original
	// text.
	Start int
	Len   int
	// X / Y / Width / Height are the geometry of the text fragment (ascent +
	// descent height) in the containing block's content coordinate space.
	X, Y, Width, Height float64
	// LineY / LineHeight describe the containing line box (RootInlineBox),
	// used for hit-testing and selection so that clicks in inter-line
	// whitespace register and selection highlights cover the full line height.
	LineY, LineHeight float64
}

// RenderText is the Go translation of WebCore::RenderText. It holds the text content of
// a DOM Text node and the list of inline boxes (line segments) that the inline
// formatting context produced when laying it out. RenderText does not generate a CSS
// box; it is always a leaf in the render tree.
type RenderText struct {
	renderObjectBase
	text      string
	transform style.WhiteSpaceType
	segments  []InlineTextBox
}

// NewRenderText constructs a RenderText for the given DOM Text node with the given
// style. The text content is read from the node's data.
func NewRenderText(node dom.Node, st *style.ComputedStyle) *RenderText {
	rt := &RenderText{}
	rt.initBase(rt, node, st)
	if st != nil {
		rt.transform = st.WhiteSpace
	}
	if t, ok := node.(*dom.Text); ok {
		rt.text = t.Data()
	}
	return rt
}

// NewRenderTextWith constructs a RenderText with an explicit text string (used for
// anonymous / generated text content).
func NewRenderTextWith(node dom.Node, st *style.ComputedStyle, text string) *RenderText {
	rt := &RenderText{text: text}
	rt.initBase(rt, node, st)
	if st != nil {
		rt.transform = st.WhiteSpace
	}
	return rt
}

// Type returns ObjectText.
func (r *RenderText) Type() RenderObjectType { return ObjectText }

// IsText reports that this object is a RenderText, mirroring RenderObject::isText().
func (r *RenderText) IsText() bool { return true }

// IsRenderText reports that this object is a RenderText.
func (r *RenderText) IsRenderText() bool { return true }

// RenderName returns a debug name for the text.
func (r *RenderText) RenderName() string { return "RenderText" }

// Text returns the raw text content, mirroring RenderText::text().
func (r *RenderText) Text() string { return r.text }

// SetText replaces the text content, mirroring RenderText::setText().
func (r *RenderText) SetText(s string) {
	if r.text != s {
		r.text = s
		r.segments = nil
		r.Dirty()
	}
}

// OriginalText returns the text content after applying text-transform, mirroring
// RenderText::originalText(). text-transform (uppercase/lowercase/capitalize) is
// applied lazily here.
func (r *RenderText) OriginalText() string {
	if r.style == nil {
		return r.text
	}
	switch r.style.TextTransform {
	case "uppercase":
		return strings.ToUpper(r.text)
	case "lowercase":
		return strings.ToLower(r.text)
	case "capitalize":
		return capitalizeSentences(r.text)
	}
	return r.text
}

// Length returns the number of runes in the text, mirroring RenderText::textLength().
func (r *RenderText) Length() int {
	return len([]rune(r.text))
}

// Segments returns the inline box segments produced by layout, mirroring the line-box
// list on RenderText.
func (r *RenderText) Segments() []InlineTextBox { return r.segments }

// SetSegments replaces the inline box segments (called by the inline formatting
// context after layout).
func (r *RenderText) SetSegments(s []InlineTextBox) { r.segments = s }

// Layout is a no-op for RenderText; text segments are produced by the containing
// block's inline formatting context. This mirrors RenderText::layout() which is empty.
func (r *RenderText) Layout(state *layout.LayoutState) {
	r.ClearNeedsLayout()
}

// ContainsOnlyWhitespace reports whether the text consists entirely of whitespace
// characters, mirroring RenderText::containsOnlyWhitespace().
func (r *RenderText) ContainsOnlyWhitespace() bool {
	return strings.TrimSpace(r.text) == ""
}

// capitalizeSentences capitalizes the first letter of each word.
func capitalizeSentences(s string) string {
	rs := []rune(s)
	cap := true
	for i, c := range rs {
		if cap && c != ' ' {
			rs[i] = []rune(strings.ToUpper(string(c)))[0]
			cap = false
		}
		if c == ' ' {
			cap = true
		}
	}
	return string(rs)
}
