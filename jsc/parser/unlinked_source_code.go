// UnlinkedSourceCode.h — Source code without provider linkage metadata

package parser

// UnlinkedSourceCode corresponds to JSC::UnlinkedSourceCode
type UnlinkedSourceCode struct {
	provider    SourceProviderInterface
	startOffset int
	endOffset   int
}

func NewUnlinkedSourceCode() UnlinkedSourceCode {
	return UnlinkedSourceCode{provider: nil, startOffset: 0, endOffset: 0}
}

func NewUnlinkedSourceCodeFromProvider(provider SourceProviderInterface) UnlinkedSourceCode {
	end := 0
	if provider != nil { end = provider.Source().Length() }
	return UnlinkedSourceCode{
		provider:    provider,
		startOffset: 0,
		endOffset:   end,
	}
}

func NewUnlinkedSourceCodeRange(provider SourceProviderInterface, startOffset, endOffset int) UnlinkedSourceCode {
	return UnlinkedSourceCode{
		provider:    provider,
		startOffset: startOffset,
		endOffset:   endOffset,
	}
}

func (u UnlinkedSourceCode) Provider() SourceProviderInterface { return u.provider }
func (u UnlinkedSourceCode) StartOffset() int   { return u.startOffset }
func (u UnlinkedSourceCode) EndOffset() int     { return u.endOffset }
func (u UnlinkedSourceCode) Length() int        { return u.endOffset - u.startOffset }
func (u UnlinkedSourceCode) IsNull() bool       { return u.provider == nil }

func (u UnlinkedSourceCode) View() string {
	if u.provider == nil { return "" }
	src := u.provider.Source().String()
	if u.startOffset >= len(src) { return "" }
	if u.endOffset > len(src) { return src[u.startOffset:] }
	return src[u.startOffset:u.endOffset]
}
