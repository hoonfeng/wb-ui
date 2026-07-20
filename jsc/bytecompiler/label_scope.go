// LabelScope.h Go 翻译
package bytecompiler

type LabelScopeType int
const (
	LabelScopeLoop      LabelScopeType = 0
	LabelScopeSwitch    LabelScopeType = 1
	LabelScopeNamedLabel LabelScopeType = 2
)

type LabelScope struct {
	refCount       int
	Type           LabelScopeType
	Name           string
	scopeDepth     int
	BreakTarget    *Label
	ContinueTarget *Label
}

func NewLabelScope(typ LabelScopeType, name string, scopeDepth int, breakTarget, continueTarget *Label) *LabelScope {
	return &LabelScope{Type: typ, Name: name, scopeDepth: scopeDepth, BreakTarget: breakTarget, ContinueTarget: continueTarget}
}

func (ls *LabelScope) BreakTargetLabel() *Label   { return ls.BreakTarget }
func (ls *LabelScope) ContinueTargetLabel() *Label { return ls.ContinueTarget }
func (ls *LabelScope) TypeVal() LabelScopeType     { return ls.Type }
func (ls *LabelScope) NameVal() string             { return ls.Name }
func (ls *LabelScope) ScopeDepth() int             { return ls.scopeDepth }
func (ls *LabelScope) Ref()                        { ls.refCount++ }
func (ls *LabelScope) Deref()                      { ls.refCount-- }
func (ls *LabelScope) RefCount() int               { return ls.refCount }
func (ls *LabelScope) HasOneRef() bool             { return ls.refCount == 1 }
func (ls *LabelScope) BreakTargetMayBeBound() bool {
	if !ls.HasOneRef() { return true }
	if !ls.BreakTarget.HasOneRef() { return true }
	return ls.BreakTarget.IsBound()
}
