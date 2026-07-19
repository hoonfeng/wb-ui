// 版权所有 (C) 2008, 2013 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/SourceCode.h 翻译为 Go

package parser

// SourceCode 源代码
type SourceCode struct {
	Provider         *SourceProvider
	StartOffset      int
	EndOffset        int
	FirstLine        int
	StartColumn      int
}

// NewSourceCode 创建 SourceCode
func NewSourceCode() SourceCode {
	return SourceCode{FirstLine: 0, StartColumn: 0}
}

// NewSourceCodeWithProvider 创建带有 SourceProvider 的 SourceCode
func NewSourceCodeWithProvider(provider *SourceProvider) SourceCode {
	return SourceCode{Provider: provider, FirstLine: 1, StartColumn: 1}
}

// NewSourceCodeDetailed 创建详细的 SourceCode
func NewSourceCodeDetailed(provider *SourceProvider, startOffset, endOffset, firstLine, startColumn int) SourceCode {
	return SourceCode{
		Provider:    provider,
		StartOffset: startOffset,
		EndOffset:   endOffset,
		FirstLine:   firstLine,
		StartColumn: startColumn,
	}
}

// FirstLineVal 返回第一行行号
func (s *SourceCode) FirstLineVal() int {
	if s.FirstLine <= 0 {
		return 1
	}
	return s.FirstLine
}

// StartColumnVal 返回起始列号
func (s *SourceCode) StartColumnVal() int {
	if s.StartColumn <= 0 {
		return 1
	}
	return s.StartColumn
}

// ProviderID 返回提供者的 ID
func (s *SourceCode) ProviderID() uintptr {
	if s.Provider == nil {
		return SourceProviderNullID
	}
	return s.Provider.AsID()
}

// ProviderRef 返回源代码提供者
func (s *SourceCode) ProviderRef() *SourceProvider {
	return s.Provider
}

// SubExpression 返回子表达式对应的源代码
func (s *SourceCode) SubExpression(openBrace, closeBrace uint32, firstLine, startColumn int) SourceCode {
	return NewSourceCodeDetailed(s.Provider, int(openBrace), int(closeBrace)+1, firstLine, startColumn+1)
}

// MakeSource 创建源代码
func MakeSource(source, sourceOrigin string, filename string, startLine, startColumn int) SourceCode {
	provider := NewSourceProvider(source, filename, SourceProviderProgram)
	return NewSourceCodeDetailed(provider, 0, len(source), startLine, startColumn)
}
