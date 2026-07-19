// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit LabelScope.h.

package bytecompiler

// LabelScope corresponds to WebKit's LabelScope class.
// It tracks break and continue targets for loops, switches, and named labels.
type LabelScope struct {
	refCount     int
	typ          LabelScopeType
	name         *string // nil for unnamed (non-named labels)
	scopeDepth   int
	breakTarget  *Label
	continueTarget *Label
}

// LabelScopeType corresponds to the Type enum in WebKit's LabelScope.
type LabelScopeType int

const (
	LabelScopeTypeLoop       LabelScopeType = iota
	LabelScopeTypeSwitch
	LabelScopeTypeNamedLabel
)

// NewLabelScope creates a new LabelScope.
func NewLabelScope(typ LabelScopeType, name *string, scopeDepth int, breakTarget, continueTarget *Label) *LabelScope {
	return &LabelScope{
		typ:            typ,
		name:           name,
		scopeDepth:     scopeDepth,
		breakTarget:    breakTarget,
		continueTarget: continueTarget,
	}
}

// Type returns the scope type.
func (s *LabelScope) Type() LabelScopeType { return s.typ }

// Name returns the label name (nil for unnamed).
func (s *LabelScope) Name() *string { return s.name }

// ScopeDepth returns the scope depth.
func (s *LabelScope) ScopeDepth() int { return s.scopeDepth }

// BreakTarget returns the break target label.
func (s *LabelScope) BreakTarget() *Label { return s.breakTarget }

// ContinueTarget returns the continue target label (may be nil).
func (s *LabelScope) ContinueTarget() *Label { return s.continueTarget }

// Ref increments ref count.
func (s *LabelScope) Ref() { s.refCount++ }

// Deref decrements ref count.
func (s *LabelScope) Deref() {
	s.refCount--
	if s.refCount < 0 {
		panic("LabelScope refCount went negative")
	}
}

// RefCount returns ref count.
func (s *LabelScope) RefCount() int { return s.refCount }

// HasOneRef returns true if refCount == 1.
func (s *LabelScope) HasOneRef() bool { return s.refCount == 1 }

// BreakTargetMayBeBound returns true if the break target might already be bound.
func (s *LabelScope) BreakTargetMayBeBound() bool {
	if !s.HasOneRef() {
		return true
	}
	if !s.breakTarget.HasOneRef() {
		return true
	}
	return s.breakTarget.IsBound()
}
