// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit Label.h.

package bytecompiler

import "math"

type boundLabelType uint8

const (
	boundLabelOffset          boundLabelType = 0
	boundLabelGeneratorForward  boundLabelType = 1
	boundLabelGeneratorBackward boundLabelType = 2
)

// BoundLabel represents a resolved or unresolved jump target.
type BoundLabel struct {
	typ       boundLabelType
	generator *BytecodeGenerator
	label     *Label
	target    int
	saved     int
}

func NewBoundLabelFromOffset(offset int) *BoundLabel {
	return &BoundLabel{typ: boundLabelOffset, target: offset}
}

func NewBoundLabelFromGenerator(generator *BytecodeGenerator, label *Label) *BoundLabel {
	return &BoundLabel{typ: boundLabelGeneratorForward, generator: generator, label: label}
}

func NewBoundLabelFromGeneratorOffset(generator *BytecodeGenerator, offset int) *BoundLabel {
	return &BoundLabel{typ: boundLabelGeneratorBackward, generator: generator, target: offset}
}

func (b *BoundLabel) Target() int {
	switch b.typ {
	case boundLabelOffset:
		return b.target
	case boundLabelGeneratorBackward:
		return b.target - b.generator.Writer().Position()
	case boundLabelGeneratorForward:
		return 0
	default:
		panic("BoundLabel: unreachable")
	}
}

func (b *BoundLabel) SaveTarget() int {
	if b.typ == boundLabelGeneratorForward {
		b.saved = b.generator.Writer().Position()
		return 0
	}
	b.saved = b.Target()
	return b.saved
}

func (b *BoundLabel) CommitTarget() int {
	if b.typ == boundLabelGeneratorForward {
		b.label.m_unresolvedJumps = append(b.label.m_unresolvedJumps, b.saved)
		return 0
	}
	return b.saved
}

// Label corresponds to WebKit GenericLabel<JSGeneratorTraits>.
type Label struct {
	refCount          int
	location          int // invalidLocation if forward
	bound             bool
	m_unresolvedJumps []int
}

const invalidLocation = math.MaxUint32

func NewLabel() *Label {
	return &Label{location: invalidLocation}
}

func (l *Label) IsForward() bool { return l.location == invalidLocation }
func (l *Label) IsBound() bool   { return l.bound }

func (l *Label) Location() int {
	if l.IsForward() {
		panic("Label is forward (location not set)")
	}
	l.bound = true
	return l.location
}

func (l *Label) SetLocation(generator *BytecodeGeneratorBase, location int) {
	l.location = location
}

func (l *Label) Bind(generator *BytecodeGenerator) *BoundLabel {
	l.bound = true
	if !l.IsForward() {
		return NewBoundLabelFromGeneratorOffset(generator, l.location)
	}
	return NewBoundLabelFromGenerator(generator, l)
}

func (l *Label) BindWithOffset(offset int) *BoundLabel {
	l.bound = true
	if !l.IsForward() {
		return NewBoundLabelFromOffset(l.location - offset)
	}
	l.m_unresolvedJumps = append(l.m_unresolvedJumps, offset)
	return NewBoundLabelFromOffset(0)
}

func (l *Label) BindToCurrent() *BoundLabel {
	return l.BindWithOffset(0)
}

func (l *Label) Ref()  { l.refCount++ }
func (l *Label) Deref() {
	l.refCount--
	if l.refCount < 0 {
		panic("Label refCount went negative")
	}
}

func (l *Label) RefCount() int                    { return l.refCount }
func (l *Label) HasOneRef() bool                  { return l.refCount == 1 }
func (l *Label) UnresolvedJumps() []int           { return l.m_unresolvedJumps }
