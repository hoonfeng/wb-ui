// Translation of: Source/WebCore/editing/CompositionUnderline.h
//                  Source/WebCore/editing/CompositionHighlight.h
// Completeness: 90%
//
// CompositionUnderline describes a styled underline applied to a range of
// text within an IME composition string. IME engines (e.g. Microsoft Pinyin,
// Google Japanese Input) use underlines to indicate the active composition
// segment and selection boundaries.
//
// In WebKit these are stored on the Editor as m_customCompositionUnderlines
// and consumed by the inline-text-box painter to render the underlines.
//
// Note: Color is represented as [4]uint8{R,G,B,A} rather than importing
// wb-ui/engine/platform/graphics to avoid a CGo dependency on the editing package.

package editing

// CompositionUnderlineColor mirrors WebCore::CompositionUnderlineColor.
// It selects whether the underline uses a given explicit color or the text
// color of the rendered text.
type CompositionUnderlineColor bool

const (
	// UnderlineColorGivenColor means the underline uses the Color field.
	UnderlineColorGivenColor CompositionUnderlineColor = true
	// UnderlineColorTextColor means the underline uses the text color.
	UnderlineColorTextColor CompositionUnderlineColor = false
)

// Color is a simple RGBA color used by composition underlines and highlights,
// avoiding a dependency on wb-ui/engine/platform/graphics.
type Color struct {
	R, G, B, A uint8
}

// CompositionUnderline is the Go translation of WebCore::CompositionUnderline.
// It describes a single underline segment within a composition string.
type CompositionUnderline struct {
	// StartOffset is the character offset within the composition string
	// where this underline begins (inclusive).
	StartOffset uint
	// EndOffset is the character offset where this underline ends (exclusive).
	EndOffset uint
	// CompositionUnderlineColor selects the color source (given vs text).
	CompositionUnderlineColor CompositionUnderlineColor
	// Color is the underline color when CompositionUnderlineColor is
	// UnderlineColorGivenColor.
	Color Color
	// Thick reports whether this underline is rendered thick (used for
	// the active clause / selected segment).
	Thick bool
}

// CompositionHighlight is the Go translation of WebCore::CompositionHighlight.
// It describes a background highlight applied to a range within a composition
// string, used by some IMEs to indicate clause boundaries.
type CompositionHighlight struct {
	// StartOffset is the character offset where the highlight begins.
	StartOffset uint
	// EndOffset is the character offset where the highlight ends.
	EndOffset uint
	// Color is the background highlight color.
	Color Color
}
