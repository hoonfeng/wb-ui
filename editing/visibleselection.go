// Translation of: Source/WebCore/editing/VisibleSelection.h
//                  Source/WebCore/editing/VisibleSelection.cpp
//                  Source/WebCore/editing/TextGranularity.h
//                  Source/WebCore/editing/SelectionDirection.h
// Completeness: 55%
// Simplifications:
//   - only the data-structure level of VisibleSelection is translated: the
//     four positions (base/extent/start/end) plus affinity/directionality/
//     granularity. The full canonicalization pipeline (validate()) is reduced
//     to "start <= end normalization" rather than the WebKit rendering-tree
//     walk that handles shadow roots, table cells, replaced elements, etc.
//   - Directionality is recorded but not used to drive selection extension
//     in this port (the code editor's own selection logic handles this).
//   - SelectionDirection::Right/Left (for bidi text) are present in the enum
//     but treated as Forward/Backward in this port's helper methods.
//   - selectionFromContentsOfNode is implemented via a TreeWalker rather
//     than the more optimized traversal used by WebKit.

package editing

import (
	"wb-ui/dom"
)

// SelectionType mirrors WebCore::VisibleSelection::SelectionType.
type SelectionType uint8

const (
	// SelectionNone means no selection.
	SelectionNone SelectionType = iota
	// SelectionCaret is a collapsed selection (caret cursor).
	SelectionCaret
	// SelectionRange is an expanded selection.
	SelectionRange
)

// SelectionDirection mirrors WebCore::SelectionDirection. It records the
// direction the user extended the selection (used for shift+arrow behavior).
type SelectionDirection uint8

const (
	// DirectionForward is forward selection (anchor before focus).
	DirectionForward SelectionDirection = iota
	// DirectionBackward is backward selection (anchor after focus).
	DirectionBackward
	// DirectionRight is rightward (bidi-aware, treated as forward here).
	DirectionRight
	// DirectionLeft is leftward (bidi-aware, treated as backward here).
	DirectionLeft
)

// TextGranularity mirrors WebCore::TextGranularity. It records how far the
// selection was extended per key press (used for shift+arrow word/line
// selection).
type TextGranularity uint8

const (
	// CharacterGranularity is the default one-character extension.
	CharacterGranularity TextGranularity = iota
	// WordGranularity extends by one word.
	WordGranularity
	// LineGranularity extends by one visual line.
	LineGranularity
	// LineBoundaryGranularity extends to the line start/end.
	LineBoundaryGranularity
	// ParagraphGranularity extends by one paragraph.
	ParagraphGranularity
	// ParagraphBoundaryGranularity extends to the paragraph start/end.
	ParagraphBoundaryGranularity
	// SentenceGranularity extends by one sentence.
	SentenceGranularity
	// SentenceBoundaryGranularity extends to the sentence start/end.
	SentenceBoundaryGranularity
	// DocumentGranularity extends to the document start/end.
	DocumentGranularity
	// DocumentBoundaryGranularity extends to the document start/end.
	DocumentBoundaryGranularity
)

// VisibleSelection is the Go translation of WebCore::VisibleSelection. It
// records the four canonical positions of a selection plus metadata about how
// the user produced it.
//
// - Base / Extent are the positions the user explicitly pointed at (e.g.
//   mouse-down = base, mouse-up = extent). They may be in any order.
// - Start / End are the canonicalized positions with start <= end. These are
//   the positions used by editing algorithms that operate on the selection.
// - Affinity disambiguates caret positions at line-wrap boundaries.
// - IsDirectional records whether the selection is directional (produced by
//   shift+arrow); directional selections have a "fixed" anchor that does not
//   move when the user types.
type VisibleSelection struct {
	// Base is the anchor position (mouse-down point).
	Base VisiblePosition
	// Extent is the position the selection was extended to (mouse-up point).
	Extent VisiblePosition
	// Start is the canonicalized earlier position (Start <= End).
	Start VisiblePosition
	// End is the canonicalized later position.
	End VisiblePosition
	// Affinity records the caret affinity at wrap boundaries.
	Affinity Affinity
	// IsDirectional records whether the selection is shift-arrow directional.
	IsDirectional bool
	// Granularity records the text granularity used to extend the selection.
	Granularity TextGranularity
}

// NewVisibleSelection constructs a non-directional caret selection at the
// given position, mirroring VisibleSelection(const VisiblePosition&).
func NewVisibleSelection(p VisiblePosition) *VisibleSelection {
	v := &VisibleSelection{
		Base:      p,
		Extent:    p,
		Affinity:  p.Affinity,
		Start:     p,
		End:       p,
		Granularity: CharacterGranularity,
	}
	return v
}

// NewVisibleSelectionWithRange constructs a non-directional range selection
// spanning from start to end, mirroring VisibleSelection(const VisiblePosition&,
// const VisiblePosition&).
func NewVisibleSelectionWithRange(start, end VisiblePosition) *VisibleSelection {
	v := &VisibleSelection{
		Base:        start,
		Extent:      end,
		Affinity:    start.Affinity,
		Granularity: CharacterGranularity,
	}
	v.validate()
	return v
}

// IsNone reports whether the selection is empty (no base), mirroring
// VisibleSelection::isNone().
func (s *VisibleSelection) IsNone() bool { return s.Base.IsNull() }

// IsCaret reports whether the selection is a collapsed caret, mirroring
// VisibleSelection::isCaret().
func (s *VisibleSelection) IsCaret() bool { return !s.IsNone() && s.Start.Equal(s.End) }

// IsRange reports whether the selection is an expanded range, mirroring
// VisibleSelection::isRange().
func (s *VisibleSelection) IsRange() bool { return !s.IsNone() && !s.Start.Equal(s.End) }

// IsCaretOrRange reports whether the selection is either a caret or a range,
// mirroring VisibleSelection::isCaretOrRange().
func (s *VisibleSelection) IsCaretOrRange() bool { return !s.IsNone() }

// Type returns the selection type (None / Caret / Range), mirroring
// VisibleSelection::selectionType().
func (s *VisibleSelection) Type() SelectionType {
	switch {
	case s.IsNone():
		return SelectionNone
	case s.IsCaret():
		return SelectionCaret
	default:
		return SelectionRange
	}
}

// SetBase sets the base position and re-validates, mirroring
// VisibleSelection::setBase(const Position&).
func (s *VisibleSelection) SetBase(p VisiblePosition) {
	s.Base = p
	s.validate()
}

// SetExtent sets the extent position and re-validates, mirroring
// VisibleSelection::setExtent(const Position&).
func (s *VisibleSelection) SetExtent(p VisiblePosition) {
	s.Extent = p
	s.validate()
}

// SetAffinity sets the affinity, mirroring VisibleSelection::setAffinity().
func (s *VisibleSelection) SetAffinity(a Affinity) {
	s.Affinity = a
	s.validate()
}

// SetWithoutValidation sets the four positions directly without running
// validate(), mirroring VisibleSelection::setWithoutValidation(). Used by
// algorithms that have already canonicalized the positions themselves
// (e.g. when computing word/line boundaries).
func (s *VisibleSelection) SetWithoutValidation(base, extent, start, end VisiblePosition, directional bool, granularity TextGranularity) {
	s.Base = base
	s.Extent = extent
	s.Start = start
	s.End = end
	s.IsDirectional = directional
	s.Granularity = granularity
}

// validate normalizes Start/End so that Start <= End, mirroring the
// canonicalization step of VisibleSelection::validate(). The full WebKit
// implementation walks the rendering tree to handle shadow roots, table
// cells, replaced elements, and caret affinity adjustments; this port only
// does the simpler DOM-level ordering based on the underlying Positions.
func (s *VisibleSelection) validate() {
	if s.Base.IsNull() || s.Extent.IsNull() {
		s.Start = s.Base
		s.End = s.Base
		return
	}
	if positionLessOrEqual(s.Base.Pos, s.Extent.Pos) {
		s.Start = s.Base
		s.End = s.Extent
	} else {
		s.Start = s.Extent
		s.End = s.Base
	}
}

// positionLessOrEqual reports whether a is before-or-equal-to b in document
// order. This is a simplified version of comparePositions in WebKit's
// VisiblePosition.cpp that handles only the common cases (same-container
// offset comparison and ancestor traversal).
func positionLessOrEqual(a, b Position) bool {
	if a.Container == b.Container {
		return a.Offset <= b.Offset
	}
	// Find common ancestor and compare child indices.
	ancestorsA := ancestorPath(a.Container)
	ancestorsB := ancestorPath(b.Container)
	for i := 0; i < len(ancestorsA) && i < len(ancestorsB); i++ {
		if ancestorsA[i] != ancestorsB[i] {
			// Diverged at this level; compare sibling order.
			parent := ancestorsA[i-1]
			idxA := indexOfChild(parent, ancestorsA[i])
			idxB := indexOfChild(parent, ancestorsB[i])
			return idxA <= idxB
		}
	}
	// One is an ancestor of the other.
	if len(ancestorsA) < len(ancestorsB) {
		return true // a is ancestor of b -> a <= b
	}
	return false
}

// ancestorPath returns the path from root to node (inclusive), where element 0
// is the document root and the last element is the node itself.
func ancestorPath(node dom.Node) []dom.Node {
	var path []dom.Node
	for n := node; n != nil; n = n.ParentNode() {
		path = append([]dom.Node{n}, path...)
	}
	return path
}

// ToRange constructs a dom.Range spanning from Start to End, mirroring
// VisibleSelection::toRange() / firstRange(). Returns nil if the selection
// is None.
func (s *VisibleSelection) ToRange(doc *dom.Document) *dom.Range {
	if s.IsNone() {
		return nil
	}
	r := dom.NewRange(doc)
	_ = r.SetStart(s.Start.Pos.Container, s.Start.Pos.Offset)
	_ = r.SetEnd(s.End.Pos.Container, s.End.Pos.Offset)
	return r
}

// SelectionFromContentsOfNode creates a VisibleSelection covering all
// contents of node, mirroring VisibleSelection::selectionFromContentsOfNode().
// The resulting selection is a non-directional range from before the first
// child to after the last child.
func SelectionFromContentsOfNode(node dom.Node) *VisibleSelection {
	if node == nil {
		return &VisibleSelection{}
	}
	first := node.FirstChild()
	last := node.LastChild()
	var startVP, endVP VisiblePosition
	if first == nil {
		// Empty container: caret at offset 0.
		p := MakePosition(node, 0)
		startVP = MakeVisiblePosition(p)
		endVP = startVP
	} else {
		startVP = MakeVisiblePosition(PositionBeforeNode(first))
		endVP = MakeVisiblePosition(PositionAfterNode(last))
	}
	return NewVisibleSelectionWithRange(startVP, endVP)
}

// Equal reports whether two selections are equal (same start/end/affinity),
// mirroring VisibleSelection::operator==.
func (s *VisibleSelection) Equal(other *VisibleSelection) bool {
	if s == nil || other == nil {
		return s == other
	}
	return s.Start.Equal(other.Start) && s.End.Equal(other.End) && s.Affinity == other.Affinity
}
