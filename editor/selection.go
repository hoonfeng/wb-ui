// Translation of: CodeMirror 6 — packages/state/src/selection.ts
//                  https://github.com/codemirror/state/blob/main/src/selection.ts
//
// Completeness: 80%
// Differences from CM6:
//   - CM6 uses UTF-16 code unit positions; this port uses rune offsets
//     (consistent with the Text model).
//   - The bidirectional text support (bidiLevel) is omitted in the first
//     version; it will be added when RTL text support is needed.
//   - `goalColumn` for vertical cursor movement is stored on the Range
//     rather than a separate map (simplified).
//   - Methods use PascalCase (Go convention).

package editor

// Assoc indicates the association of an empty selection (caret) at a
// line boundary — whether the caret "belongs" to the line before or
// after the break. Mirrors CM6's Assoc.
type Assoc int

const (
	// AssocBefore means the caret is associated with the line before it.
	AssocBefore Assoc = -1
	// AssocDefault means the caret has default association.
	AssocDefault Assoc = 0
	// AssocAfter means the caret is associated with the line after it.
	AssocAfter Assoc = 1
)

// Range represents a single selection range within a document. A range has
// an anchor (the fixed end) and a head (the movable end / caret position).
// If anchor == head the range is empty (a caret).
//
// In CM6, Range is immutable; methods like Extend return a new Range.
// This port follows the same convention.
type Range struct {
	// anchor is the fixed end of the selection.
	anchor int
	// head is the movable end (caret position).
	head int
	// assoc indicates the association of an empty range at a line break.
	assoc Assoc
	// goalColumn is the desired column for vertical cursor movement
	// (when moving up/down, the caret tries to stay at this column).
	// -1 means undefined.
	goalColumn int
}

// NewRange creates a Range with the given anchor and head. The anchor and
// head may be in any order (anchor > head means the selection is backward).
func NewRange(anchor, head int) Range {
	return Range{anchor: anchor, head: head, goalColumn: -1}
}

// NewRangeCaret creates an empty Range (caret) at the given position.
func NewRangeCaret(pos int) Range {
	return NewRange(pos, pos)
}

// Anchor returns the anchor position (the fixed end of the selection).
func (r Range) Anchor() int { return r.anchor }

// Head returns the head position (the caret / movable end).
func (r Range) Head() int { return r.head }

// From returns the smaller of anchor and head (the start of the range).
func (r Range) From() int {
	if r.anchor < r.head {
		return r.anchor
	}
	return r.head
}

// To returns the larger of anchor and head (the end of the range).
func (r Range) To() int {
	if r.anchor > r.head {
		return r.anchor
	}
	return r.head
}

// Empty reports whether the range is empty (anchor == head, i.e. a caret).
func (r Range) Empty() bool { return r.anchor == r.head }

// Assoc returns the association of the range.
func (r Range) Assoc() Assoc { return r.assoc }

// GoalColumn returns the desired column for vertical movement (-1 if unset).
func (r Range) GoalColumn() int { return r.goalColumn }

// WithAssoc returns a new Range with the given association.
func (r Range) WithAssoc(a Assoc) Range {
	r.assoc = a
	return r
}

// WithGoalColumn returns a new Range with the given goal column.
func (r Range) WithGoalColumn(col int) Range {
	r.goalColumn = col
	return r
}

// Extend returns a new Range with the head moved to `head` and the anchor
// unchanged. This is used when extending a selection (e.g. Shift+Click).
func (r Range) Extend(head int) Range {
	return Range{anchor: r.anchor, head: head, assoc: r.assoc, goalColumn: -1}
}

// ExtendRange extends a range to cover `from`–`to`, keeping the anchor
// on the side closer to `from`.
func (r Range) ExtendRange(from, to int) Range {
	if from <= r.anchor && to >= r.anchor {
		// The range fully contains the current anchor; pick the end
		// farther from the anchor as the new head.
		if r.anchor-from > to-r.anchor {
			return r.Extend(from)
		}
		return r.Extend(to)
	}
	// Pick the end closer to the head.
	if absInt(r.head-from) < absInt(r.head-to) {
		return r.Extend(from)
	}
	return r.Extend(to)
}

// Eq reports whether two ranges are equal (same anchor, head, assoc).
func (r Range) Eq(other Range) bool {
	return r.anchor == other.anchor && r.head == other.head && r.assoc == other.assoc
}

// Bound clamps the range to [0, length] and returns a new range.
func (r Range) Bound(length int) Range {
	a := clampInt(r.anchor, 0, length)
	h := clampInt(r.head, 0, length)
	return Range{anchor: a, head: h, assoc: r.assoc, goalColumn: r.goalColumn}
}

// EditorSelection holds one or more selection ranges, one of which is the
// "main" (primary) range. Multi-cursor editing is supported by having
// multiple ranges.
//
// EditorSelection is immutable; methods that modify the selection return
// a new EditorSelection.
type EditorSelection struct {
	// ranges are the selection ranges, in document order.
	ranges []Range
	// main is the index of the primary range in `ranges`.
	main int
}

// NewEditorSelection creates an EditorSelection with the given ranges and
// main index. If `main` is out of range, the last range is used.
func NewEditorSelection(ranges []Range, main int) EditorSelection {
	if len(ranges) == 0 {
		return EditorSelection{ranges: []Range{NewRangeCaret(0)}, main: 0}
	}
	if main < 0 || main >= len(ranges) {
		main = len(ranges) - 1
	}
	// Copy the ranges slice to ensure immutability.
	rs := make([]Range, len(ranges))
	copy(rs, ranges)
	return EditorSelection{ranges: rs, main: main}
}

// SelectionRange creates an EditorSelection with a single range.
func SelectionRange(anchor, head int) EditorSelection {
	return EditorSelection{ranges: []Range{NewRange(anchor, head)}, main: 0}
}

// SelectionCaret creates an EditorSelection with a single caret.
func SelectionCaret(pos int) EditorSelection {
	return EditorSelection{ranges: []Range{NewRangeCaret(pos)}, main: 0}
}

// Ranges returns the selection ranges.
func (s EditorSelection) Ranges() []Range { return s.ranges }

// Main returns the primary (main) range.
func (s EditorSelection) Main() Range {
	if s.main >= 0 && s.main < len(s.ranges) {
		return s.ranges[s.main]
	}
	if len(s.ranges) > 0 {
		return s.ranges[0]
	}
	return NewRangeCaret(0)
}

// MainIndex returns the index of the main range.
func (s EditorSelection) MainIndex() int { return s.main }

// PrimaryHead returns the head of the main range.
func (s EditorSelection) PrimaryHead() int { return s.Main().Head() }

// Empty reports whether all ranges are empty (carets).
func (s EditorSelection) Empty() bool {
	for _, r := range s.ranges {
		if !r.Empty() {
			return false
		}
	}
	return true
}

// AsSingle returns the single range if there is only one range, otherwise
// returns the main range. Mirrors CM6's EditorSelection.asSingle().
func (s EditorSelection) AsSingle() Range {
	if len(s.ranges) == 1 {
		return s.ranges[0]
	}
	return s.Main()
}

// AddRange returns a new EditorSelection with `r` added. If `open` is true,
// the new range becomes the main; otherwise the main is unchanged.
// Mirrors CM6's EditorSelection.addRange().
func (s EditorSelection) AddRange(r Range, open bool) EditorSelection {
	rs := make([]Range, 0, len(s.ranges)+1)
	rs = append(rs, s.ranges...)
	rs = append(rs, r)
	main := s.main
	if open {
		main = len(rs) - 1
	}
	return EditorSelection{ranges: rs, main: main}
}

// ReplaceRange returns a new EditorSelection with the range at index `which`
// replaced by `r`. If `which` is -1, replaces the main range.
func (s EditorSelection) ReplaceRange(r Range, which int) EditorSelection {
	if which == -1 {
		which = s.main
	}
	rs := make([]Range, len(s.ranges))
	copy(rs, s.ranges)
	if which >= 0 && which < len(rs) {
		rs[which] = r
	}
	return EditorSelection{ranges: rs, main: s.main}
}

// Eq reports whether two selections are equal.
func (s EditorSelection) Eq(other EditorSelection) bool {
	if len(s.ranges) != len(other.ranges) || s.main != other.main {
		return false
	}
	for i := range s.ranges {
		if !s.ranges[i].Eq(other.ranges[i]) {
			return false
		}
	}
	return true
}

// Map returns a new EditorSelection with all ranges mapped through a
// position-mapping function. Used to update selections after document
// changes. Ranges that become empty after mapping and are adjacent to
// a change may be filtered out (mirrors CM6's EditorSelection.map()).
func (s EditorSelection) Map(mapping ChangeSet) EditorSelection {
	rs := make([]Range, 0, len(s.ranges))
	for _, r := range s.ranges {
		anchor := mapping.MapPos(r.Anchor(), 1)
		head := mapping.MapPos(r.Head(), 1)
		// If both endpoints collapse to the same position, make a caret.
		if anchor == head && !r.Empty() {
			// Selection was non-empty but mapped to empty (deleted content).
			// Keep it as a caret at the deletion point.
			rs = append(rs, NewRangeCaret(anchor))
		} else {
			nr := NewRange(anchor, head)
			if r.Empty() {
				nr = nr.WithAssoc(r.Assoc())
			}
			rs = append(rs, nr)
		}
	}
	// Normalize: merge overlapping ranges if they are all carets.
	if allCarets(rs) {
		rs = mergeAdjacentCarets(rs)
	}
	return EditorSelection{ranges: rs, main: minInt(s.main, len(rs)-1)}
}

// Bound returns a new EditorSelection with all ranges clamped to [0, length].
func (s EditorSelection) Bound(length int) EditorSelection {
	rs := make([]Range, len(s.ranges))
	for i, r := range s.ranges {
		rs[i] = r.Bound(length)
	}
	return EditorSelection{ranges: rs, main: s.main}
}

// Filter removes ranges that are entirely within deleted regions. Not
// fully implemented (CM6 has complex filtering); for now, Map handles
// the basic case.

// ----- Helper functions -----

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func allCarets(rs []Range) bool {
	for _, r := range rs {
		if !r.Empty() {
			return false
		}
	}
	return true
}

// mergeAdjacentCarets merges caret ranges at the same position, keeping
// only the first one. This prevents duplicate carets after mapping.
func mergeAdjacentCarets(rs []Range) []Range {
	if len(rs) <= 1 {
		return rs
	}
	out := make([]Range, 0, len(rs))
	out = append(out, rs[0])
	for i := 1; i < len(rs); i++ {
		if rs[i].Head() != out[len(out)-1].Head() {
			out = append(out, rs[i])
		}
	}
	return out
}
