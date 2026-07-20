// UnlinkedSourceCode.h Go 翻译
package parser

type UnlinkedSourceCode struct {
	Provider       *SourceProvider
	startOff int
	endOff   int
}

func NewUnlinkedSourceCode() *UnlinkedSourceCode {
	return &UnlinkedSourceCode{startOff: 0, endOff: 0}
}

func NewUnlinkedSourceCodeWithProvider(provider *SourceProvider) *UnlinkedSourceCode {
	return &UnlinkedSourceCode{
		Provider: provider,
		startOff: 0,
		endOff:   len(provider.Source),
	}
}

func NewUnlinkedSourceCodeWithRange(provider *SourceProvider, startOffset, endOffset int) *UnlinkedSourceCode {
	return &UnlinkedSourceCode{
		Provider: provider,
		startOff: startOffset,
		endOff:   endOffset,
	}
}

func (u *UnlinkedSourceCode) IsNull() bool    { return u.Provider == nil }
func (u *UnlinkedSourceCode) StartOffset() int { return u.startOff }
func (u *UnlinkedSourceCode) EndOffset() int   { return u.endOff }
func (u *UnlinkedSourceCode) Length() int      { return u.endOff - u.startOff }
func (u *UnlinkedSourceCode) View() string {
	if u.Provider == nil { return "" }
	return u.Provider.GetRange(u.startOff, u.endOff)
}
