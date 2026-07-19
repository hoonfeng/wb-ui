/*
 * Copyright (C) 2008-2025 Apple Inc. All rights reserved.
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

// SourceProvider.h — Source code provider (abstract + StringSourceProvider)

package parser

import (
	"sync/atomic"
)

// SourceProviderSourceType corresponds to JSC::SourceProviderSourceType
type SourceProviderSourceType uint8

const (
	SourceProviderSourceTypeProgram      SourceProviderSourceType = 0
	SourceProviderSourceTypeModule       SourceProviderSourceType = 1
	SourceProviderSourceTypeWebAssembly  SourceProviderSourceType = 2
	SourceProviderSourceTypeJSON         SourceProviderSourceType = 3
	SourceProviderSourceTypeImportMap    SourceProviderSourceType = 4
)

// SourceProvider corresponds to JSC::SourceProvider
type SourceProvider struct {
	refCount                  int32
	sourceType                SourceProviderSourceType
	sourceOrigin              SourceOrigin
	sourceURL                 string
	preRedirectURL            string
	sourceURLDirective        string
	sourceMappingURLDirective string
	startPosition             TextPosition
	id                        int64
	taintedness               SourceTaintedOrigin
}

const SourceProviderNullID int64 = 1

var nextSourceProviderID int64 = 2

func NewSourceProvider(sourceOrigin SourceOrigin, sourceURL string, preRedirectURL string,
	taintedness SourceTaintedOrigin, startPosition TextPosition, sourceType SourceProviderSourceType) *SourceProvider {
	return &SourceProvider{
		sourceType:    sourceType,
		sourceOrigin:  sourceOrigin,
		sourceURL:     sourceURL,
		preRedirectURL: preRedirectURL,
		startPosition: startPosition,
		taintedness:   taintedness,
	}
}

func (p *SourceProvider) Retain()              { atomic.AddInt32(&p.refCount, 1) }
func (p *SourceProvider) Release()             { atomic.AddInt32(&p.refCount, -1) }
func (p *SourceProvider) Hash() uint32         { return 0 }
func (p *SourceProvider) Source() StringView   { return StringView{} }

func (p *SourceProvider) GetRange(start, end int) string {
	s := p.Source().String()
	if start < 0 { start = 0 }
	if end > len(s) { end = len(s) }
	if start >= end { return "" }
	return s[start:end]
}

func (p *SourceProvider) SourceOrigin() SourceOrigin               { return p.sourceOrigin }
func (p *SourceProvider) SourceURL() string                        { return p.sourceURL }
func (p *SourceProvider) PreRedirectURL() string                   { return p.preRedirectURL }
func (p *SourceProvider) SourceURLDirective() string               { return p.sourceURLDirective }
func (p *SourceProvider) SourceMappingURLDirective() string        { return p.sourceMappingURLDirective }
func (p *SourceProvider) StartPosition() TextPosition              { return p.startPosition }
func (p *SourceProvider) SourceType() SourceProviderSourceType     { return p.sourceType }

func (p *SourceProvider) IsModuleType() bool {
	return p.sourceType == SourceProviderSourceTypeModule || p.sourceType == SourceProviderSourceTypeJSON
}

func (p *SourceProvider) AsID() int64 {
	if p.id == 0 {
		p.id = atomic.AddInt64(&nextSourceProviderID, 1)
	}
	return p.id
}

func (p *SourceProvider) SetSourceURLDirective(d string)       { p.sourceURLDirective = d }
func (p *SourceProvider) SetSourceMappingURLDirective(d string) { p.sourceMappingURLDirective = d }
func (p *SourceProvider) SetSourceTaintedOrigin(t SourceTaintedOrigin) { p.taintedness = t }
func (p *SourceProvider) SourceTaintedOrigin() SourceTaintedOrigin     { return p.taintedness }
func (p *SourceProvider) CouldBeTainted() bool                         { return p.taintedness != SourceTaintedUntainted }
func (p *SourceProvider) IsScriptBufferSourceProvider() bool           { return false }
func (p *SourceProvider) LockUnderlyingBuffer()                       {}
func (p *SourceProvider) UnlockUnderlyingBuffer()                     {}

// StringSourceProvider corresponds to JSC::StringSourceProvider
type StringSourceProvider struct {
	SourceProvider
	source string
}

func NewStringSourceProvider(source string, sourceOrigin SourceOrigin, sourceURL string,
	taintedness SourceTaintedOrigin, startPosition TextPosition, sourceType SourceProviderSourceType) *StringSourceProvider {
	return &StringSourceProvider{
		SourceProvider: *NewSourceProvider(sourceOrigin, sourceURL, "", taintedness, startPosition, sourceType),
		source:         source,
	}
}

func (p *StringSourceProvider) Hash() uint32 { return hashString(p.source) }
func (p *StringSourceProvider) Source() StringView { return NewStringView(p.source) }

func (p *StringSourceProvider) GetRange(start, end int) string {
	if start < 0 { start = 0 }
	if end > len(p.source) { end = len(p.source) }
	if start >= end { return "" }
	return p.source[start:end]
}

// CachedBytecode — placeholder
type CachedBytecode struct{}

// StringView — simple string wrapper
type StringView struct {
	s string
}

func NewStringView(s string) StringView { return StringView{s: s} }
func (v StringView) String() string     { return v.s }
func (v StringView) Length() int        { return len(v.s) }

// TextPosition — corresponds to WTF::TextPosition
type TextPosition struct {
	Line   OrdinalNumber
	Column OrdinalNumber
}

func NewTextPosition(line, column OrdinalNumber) TextPosition {
	return TextPosition{Line: line, Column: column}
}

// OrdinalNumber — 1-based position
type OrdinalNumber struct{ value int }

func NewOrdinalNumber(v int) OrdinalNumber     { return OrdinalNumber{value: v} }
func OrdinalNumberBeforeFirst() OrdinalNumber   { return OrdinalNumber{value: 0} }
func (o OrdinalNumber) OneBasedInt() int        { return o.value }
func (o OrdinalNumber) IsBeforeFirst() bool     { return o.value == 0 }

// helpers
func hashString(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// SourceProviderBufferGuard — RAII guard
type SourceProviderBufferGuard struct {
	provider *SourceProvider
}

func NewSourceProviderBufferGuard(provider *SourceProvider) *SourceProviderBufferGuard {
	g := &SourceProviderBufferGuard{provider: provider}
	if g.provider != nil {
		g.provider.LockUnderlyingBuffer()
	}
	return g
}

func (g *SourceProviderBufferGuard) Close() {
	if g.provider != nil {
		g.provider.UnlockUnderlyingBuffer()
	}
}
func (g *SourceProviderBufferGuard) Provider() *SourceProvider { return g.provider }
