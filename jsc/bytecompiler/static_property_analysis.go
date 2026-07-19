// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit StaticPropertyAnalysis.h.

package bytecompiler

// StaticPropertyAnalysis corresponds to WebKit's StaticPropertyAnalysis.
// Tracks properties assigned to an object for optimization guesses.
type StaticPropertyAnalysis struct {
	refCount       int
	instructionRef *InstructionStreamMutableRef
	propertyIndexes map[uint32]struct{}
}

// NewStaticPropertyAnalysis creates a new StaticPropertyAnalysis.
func NewStaticPropertyAnalysis(instructionRef *InstructionStreamMutableRef) *StaticPropertyAnalysis {
	return &StaticPropertyAnalysis{
		instructionRef:   instructionRef,
		propertyIndexes: make(map[uint32]struct{}),
	}
}

// AddPropertyIndex adds a property index to the analysis.
func (a *StaticPropertyAnalysis) AddPropertyIndex(propertyIndex uint32) {
	a.propertyIndexes[propertyIndex] = struct{}{}
}

// Record records the analysis (simplified: no-op in Go version).
func (a *StaticPropertyAnalysis) Record() {
	// In WebKit, this records the property indexes to the instruction metadata.
	// Simplified in Go version.
}

// PropertyIndexCount returns the number of unique property indexes.
func (a *StaticPropertyAnalysis) PropertyIndexCount() int {
	return len(a.propertyIndexes)
}

// Ref increments the reference count.
func (a *StaticPropertyAnalysis) Ref() { a.refCount++ }

// Deref decrements the reference count.
func (a *StaticPropertyAnalysis) Deref() {
	a.refCount--
	if a.refCount < 0 {
		panic("StaticPropertyAnalysis refCount went negative")
	}
}

// RefCount returns the reference count.
func (a *StaticPropertyAnalysis) RefCount() int { return a.refCount }

// HasOneRef returns true if refCount >= 1 (in practice: exactly 1 remaining reference).
func (a *StaticPropertyAnalysis) HasOneRef() bool { return a.refCount <= 1 && a.refCount > 0 }
