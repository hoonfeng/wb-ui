// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit StaticPropertyAnalyzer.h.

package bytecompiler

// StaticPropertyAnalyzer corresponds to WebKit's StaticPropertyAnalyzer.
// Performs flow-insensitive static analysis of the number of properties assigned to an object.
type StaticPropertyAnalyzer struct {
	analyses map[int]*StaticPropertyAnalysis
}

// NewStaticPropertyAnalyzer creates a new analyzer.
func NewStaticPropertyAnalyzer() *StaticPropertyAnalyzer {
	return &StaticPropertyAnalyzer{
		analyses: make(map[int]*StaticPropertyAnalysis),
	}
}

// CreateThis records a 'this' allocation.
func (a *StaticPropertyAnalyzer) CreateThis(dst *RegisterID, instructionRef *InstructionStreamMutableRef) {
	analysis := NewStaticPropertyAnalysis(instructionRef)
	if _, exists := a.analyses[dst.Index()]; exists {
		// Can't have two 'this' in the same constructor.
		panic("Duplicate 'this' in constructor")
	}
	a.analyses[dst.Index()] = analysis
}

// NewObject records a 'new Object()' allocation.
func (a *StaticPropertyAnalyzer) NewObject(dst *RegisterID, instructionRef *InstructionStreamMutableRef) {
	analysis := NewStaticPropertyAnalysis(instructionRef)
	if existing, ok := a.analyses[dst.Index()]; ok {
		a.killAnalysis(existing)
	}
	a.analyses[dst.Index()] = analysis
}

// PutById records a property assignment to an object.
func (a *StaticPropertyAnalyzer) PutById(dst *RegisterID, propertyIndex uint32) {
	analysis := a.analyses[dst.Index()]
	if analysis == nil {
		return
	}
	analysis.AddPropertyIndex(propertyIndex)
}

// Mov records a register-to-register move.
func (a *StaticPropertyAnalyzer) Mov(dst, src *RegisterID) {
	analysis := a.analyses[src.Index()]
	if analysis == nil {
		a.KillRegister(dst)
		return
	}
	if existing, ok := a.analyses[dst.Index()]; ok {
		a.killAnalysis(existing)
	}
	a.analyses[dst.Index()] = analysis
}

// Kill clears analysis for all registers.
func (a *StaticPropertyAnalyzer) Kill() {
	for key, analysis := range a.analyses {
		a.killAnalysis(analysis)
		delete(a.analyses, key)
	}
}

// KillRegister clears analysis for a specific register.
func (a *StaticPropertyAnalyzer) KillRegister(dst *RegisterID) {
	analysis := a.analyses[dst.Index()]
	if analysis == nil {
		return
	}
	if analysis.PropertyIndexCount() == 0 {
		return
	}
	a.killAnalysis(analysis)
	delete(a.analyses, dst.Index())
}

// killAnalysis records and removes a specific analysis.
func (a *StaticPropertyAnalyzer) killAnalysis(analysis *StaticPropertyAnalysis) {
	if analysis == nil {
		return
	}
	if !analysis.HasOneRef() {
		// Aliases for this object still exist, so it might acquire more properties.
		return
	}
	analysis.Record()
}
