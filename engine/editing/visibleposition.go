// Translation of: Source/WebCore/editing/Position.h
//                  Source/WebCore/editing/Position.cpp
//                  Source/WebCore/editing/VisiblePosition.h
//                  Source/WebCore/editing/VisiblePosition.cpp
//                  Source/WebCore/editing/EditingBoundary.h
// Completeness: 60%
// Simplifications:
//   - Position stores a (Container, Offset, AnchorType) tuple directly rather
//     than using a tagged union of (m_anchorNode, m_offset, m_anchorType)
//   - the legacy "legacy editing positions" (Position before/after a node
//     without an explicit offset) are retained as AnchorType values but the
//     editing algorithms that depend on the difference between
//     OffsetInAnchor and BeforeChildren/AfterChildren are simplified: most
//     callers only use OffsetInAnchor in practice
//   - the canonicalization pipeline (canonicalPosition / makeEditableRoot)
//     is reduced to a small set of helpers; full canonicalization (which
//     handles shadow roots, table cells, replaced elements, etc.) is deferred
//     until contenteditable rich-text editing is needed
//   - VisiblePosition is a value type holding a (Position, Affinity) pair

package editing

import (
	"wb-ui/engine/dom"
)

// PositionAnchor mirrors WebCore::Position::AnchorType. It selects how the
// (Container, Offset) pair of a Position is interpreted.
type PositionAnchor uint8

const (
	// AnchorOffset means the position is at the given offset within the
	// container's child list (for Element/Document) or character data (for
	// Text/Comment). Matches Position::OffsetInAnchor.
	AnchorOffset PositionAnchor = iota
	// AnchorBeforeChildren means the position is before all children of the
	// container; Offset is ignored. Matches Position::BeforeChildren.
	AnchorBeforeChildren
	// AnchorAfterChildren means the position is after all children of the
	// container; Offset is ignored. Matches Position::AfterChildren.
	AnchorAfterChildren
	// AnchorBeforeNode means the position is immediately before the container
	// node itself (i.e. in the parent's child list). Matches Position::BeforeAnchor.
	AnchorBeforeNode
	// AnchorAfterNode means the position is immediately after the container
	// node itself. Matches Position::AfterAnchor.
	AnchorAfterNode
)

// Affinity mirrors WebCore::Affinity (defined in VisiblePosition.h).
// It disambiguates cursor positions at line-wrap boundaries: when a long
// line wraps onto two visual lines, the cursor at the end of the first
// visual line and at the start of the second both correspond to the same
// DOM Position. Affinity records which side the user intended.
type Affinity uint8

const (
	// AffinityDownstream is the default affinity; the position is treated as
	// belonging to the following character. Matches DOWNSTREAM.
	AffinityDownstream Affinity = iota
	// AffinityUpstream treats the position as belonging to the preceding
	// character. Matches UPSTREAM.
	AffinityUpstream
)

// DefaultAffinity returns the affinity used when none is specified, mirroring
// VisiblePosition::defaultAffinity.
func DefaultAffinity() Affinity { return AffinityDownstream }

// Position is the Go translation of WebCore::Position. It identifies a single
// location in the DOM tree by a (container, offset, anchor) triple.
//
// For most editing operations the container is a Text node and the offset is
// a character index. For element-level operations the container is an Element
// and the offset is a child index.
type Position struct {
	// Container is the node containing the position. May be a Text/Comment
	// (offset counts characters) or an Element/Document (offset counts
	// children). Nil means the position is null.
	Container dom.Node
	// Offset is the character offset or child index within Container.
	Offset int
	// AnchorType selects the interpretation of (Container, Offset).
	AnchorType PositionAnchor
}

// IsNull reports whether the position is null (no container), mirroring
// Position::isNull().
func (p Position) IsNull() bool { return p.Container == nil }

// IsNotNull is the inverse of IsNull, mirroring Position::isNotNull().
func (p Position) IsNotNull() bool { return p.Container != nil }

// IsOrphan reports whether the container is no longer attached to a document,
// mirroring Position::isOrphan(). A position can be non-null but orphaned when
// the container was removed from the DOM after the position was recorded.
func (p Position) IsOrphan() bool {
	if p.Container == nil {
		return true
	}
	return !p.Container.IsConnected()
}

// Equal reports whether two positions refer to the same logical location,
// mirroring Position::operator==.
func (p Position) Equal(other Position) bool {
	if p.Container != other.Container {
		return false
	}
	if p.AnchorType != other.AnchorType {
		return false
	}
	if p.AnchorType == AnchorOffset && p.Offset != other.Offset {
		return false
	}
	return true
}

// MakePosition is a convenience constructor for the common case of an
// offset-in-anchor position. It mirrors Position() / Position::create.
func MakePosition(container dom.Node, offset int) Position {
	return Position{Container: container, Offset: offset, AnchorType: AnchorOffset}
}

// PositionBeforeNode creates a position immediately before the given node,
// mirroring positionBeforeNode(node). The container is the node's parent and
// the anchor type is AnchorBeforeNode.
func PositionBeforeNode(node dom.Node) Position {
	if node == nil {
		return Position{}
	}
	parent := node.ParentNode()
	if parent == nil {
		return Position{Container: node, AnchorType: AnchorBeforeNode}
	}
	idx := indexOfChild(parent, node)
	return Position{Container: parent, Offset: idx, AnchorType: AnchorOffset}
}

// PositionAfterNode creates a position immediately after the given node,
// mirroring positionAfterNode(node).
func PositionAfterNode(node dom.Node) Position {
	if node == nil {
		return Position{}
	}
	parent := node.ParentNode()
	if parent == nil {
		return Position{Container: node, AnchorType: AnchorAfterNode}
	}
	idx := indexOfChild(parent, node)
	return Position{Container: parent, Offset: idx + 1, AnchorType: AnchorOffset}
}

// PositionBeforeChildren creates a position before all children of the
// container, mirroring positionBeforeChildren(node).
func PositionBeforeChildren(container dom.Node) Position {
	return Position{Container: container, AnchorType: AnchorBeforeChildren}
}

// PositionAfterChildren creates a position after all children of the
// container, mirroring positionAfterChildren(node).
func PositionAfterChildren(container dom.Node) Position {
	return Position{Container: container, AnchorType: AnchorAfterChildren}
}

// indexOfChild returns the index of child in parent's child list, or -1.
func indexOfChild(parent, child dom.Node) int {
	if parent == nil || child == nil {
		return -1
	}
	idx := 0
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if c == child {
			return idx
		}
		idx++
	}
	return -1
}

// VisiblePosition is the Go translation of WebCore::VisiblePosition. A
// VisiblePosition wraps a Position together with an Affinity; it represents
// a cursor location as the user sees it on screen.
//
// VisiblePositions are produced by canonicalizing a Position through the
// rendering tree: positions that fall inside non-rendered nodes, replaced
// elements, or across line-wrap boundaries are adjusted to the nearest
// visually-meaningful location. In this port canonicalization is minimal
// (we mostly pass through the underlying Position), but the type is retained
// so that editing commands can carry affinity information through the
// undo/redo stack and IME pipeline.
type VisiblePosition struct {
	Pos Position
	// Affinity is the cursor affinity at line-wrap boundaries.
	Affinity Affinity
}

// IsNull reports whether the visible position is null.
func (v VisiblePosition) IsNull() bool { return v.Pos.IsNull() }

// IsNotNull is the inverse of IsNull.
func (v VisiblePosition) IsNotNull() bool { return v.Pos.IsNotNull() }

// DeepEquivalent returns the underlying Position, mirroring
// VisiblePosition::deepEquivalent().
func (v VisiblePosition) DeepEquivalent() Position { return v.Pos }

// Equal reports whether two visible positions are equal (same position and
// affinity), mirroring VisiblePosition::operator==.
func (v VisiblePosition) Equal(other VisiblePosition) bool {
	return v.Pos.Equal(other.Pos) && v.Affinity == other.Affinity
}

// MakeVisiblePosition wraps a Position with the default affinity, mirroring
// VisiblePosition(node, offset, affinity) convenience.
func MakeVisiblePosition(p Position) VisiblePosition {
	return VisiblePosition{Pos: p, Affinity: DefaultAffinity()}
}

// MakeVisiblePositionWithAffinity wraps a Position with an explicit affinity.
func MakeVisiblePositionWithAffinity(p Position, a Affinity) VisiblePosition {
	return VisiblePosition{Pos: p, Affinity: a}
}
