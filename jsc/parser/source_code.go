/*
 * Copyright (C) 2008, 2013 Apple Inc. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 *
 * 1.  Redistributions of source code must retain the above copyright
 *     notice, this list of conditions and the following disclaimer.
 * 2.  Redistributions in binary form must reproduce the above copyright
 *     notice, this list of conditions and the following disclaimer in the
 *     documentation and/or other materials provided with the distribution.
 * 3.  Neither the name of Apple Inc. ("Apple") nor the names of
 *     its contributors may be used to endorse or promote products derived
 *     from this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY APPLE AND ITS CONTRIBUTORS "AS IS" AND ANY
 * EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL APPLE OR ITS CONTRIBUTORS BE LIABLE FOR ANY
 * DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
 * LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
 * ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 * THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

// SourceCode.h — Full source code with line/column metadata

package parser

// SourceProviderInterface — interface for source providers
type SourceProviderInterface interface {
	Source() StringView
	AsID() int64
}

// SourceCode corresponds to JSC::SourceCode
type SourceCode struct {
	provider    SourceProviderInterface
	startOffset int
	endOffset   int
	firstLine   OrdinalNumber
	startColumn OrdinalNumber
}

func NewSourceCode() SourceCode {
	return SourceCode{
		provider:    nil,
		startOffset: 0,
		endOffset:   0,
		firstLine:   OrdinalNumberBeforeFirst(),
		startColumn: OrdinalNumberBeforeFirst(),
	}
}

func NewSourceCodeFromProvider(provider SourceProviderInterface) SourceCode {
	end := 0
	if provider != nil {
		end = provider.Source().Length()
	}
	return SourceCode{
		provider:    provider,
		startOffset: 0,
		endOffset:   end,
		firstLine:   NewOrdinalNumber(1),
		startColumn: NewOrdinalNumber(1),
	}
}

func NewSourceCodeWithPosition(provider SourceProviderInterface, firstLine, startColumn int) SourceCode {
	if firstLine < 1 {
		firstLine = 1
	}
	if startColumn < 1 {
		startColumn = 1
	}
	end := 0
	if provider != nil {
		end = provider.Source().Length()
	}
	return SourceCode{
		provider:    provider,
		startOffset: 0,
		endOffset:   end,
		firstLine:   NewOrdinalNumber(firstLine),
		startColumn: NewOrdinalNumber(startColumn),
	}
}

func NewSourceCodeRange(provider SourceProviderInterface, startOffset, endOffset, firstLine, startColumn int) SourceCode {
	if firstLine < 1 { firstLine = 1 }
	if startColumn < 1 { startColumn = 1 }
	return SourceCode{
		provider:    provider,
		startOffset: startOffset,
		endOffset:   endOffset,
		firstLine:   NewOrdinalNumber(firstLine),
		startColumn: NewOrdinalNumber(startColumn),
	}
}

func (s SourceCode) FirstLine() OrdinalNumber    { return s.firstLine }
func (s SourceCode) StartColumn() OrdinalNumber  { return s.startColumn }
func (s SourceCode) StartOffset() int            { return s.startOffset }
func (s SourceCode) EndOffset() int              { return s.endOffset }
func (s SourceCode) Length() int                 { return s.endOffset - s.startOffset }
func (s SourceCode) IsNull() bool                { return s.provider == nil }

func (s SourceCode) ProviderID() int64 {
	if s.provider == nil { return SourceProviderNullID }
	return s.provider.AsID()
}

func (s SourceCode) Provider() SourceProviderInterface { return s.provider }

func (s SourceCode) View() string {
	if s.provider == nil { return "" }
	src := s.provider.Source().String()
	if s.startOffset >= len(src) { return "" }
	if s.endOffset > len(src) { return src[s.startOffset:] }
	return src[s.startOffset:s.endOffset]
}

func (s SourceCode) SubExpression(openBrace, closeBrace uint32, firstLine, startColumn int) SourceCode {
	startColumn++
	return NewSourceCodeRange(s.provider, int(openBrace), int(closeBrace+1), firstLine, startColumn)
}

// MakeSource — creates a SourceCode from raw source string
func MakeSource(source string, sourceOrigin SourceOrigin, taintedness SourceTaintedOrigin,
	filename string, startPosition TextPosition, sourceType SourceProviderSourceType) SourceCode {

	provider := NewStringSourceProvider(source, sourceOrigin, filename, taintedness,
		startPosition, sourceType)
	return NewSourceCodeWithPosition(provider, startPosition.Line.OneBasedInt(), startPosition.Column.OneBasedInt())
}
