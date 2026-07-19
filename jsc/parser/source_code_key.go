/*
 * Copyright (C) 2018 Apple Inc. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. ``AS IS'' AND ANY
 * EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED.  IN NO EVENT SHALL APPLE INC. OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
 * EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
 * PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
 * PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
 * OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

// SourceCodeKey.h — Key for caching parsed source code

package parser

// SourceCodeKey corresponds to JSC::SourceCodeKey
// Used as a key in parser caches to uniquely identify source code.
type SourceCodeKey struct {
	sourceCode   SourceCode
	flags        uint8
}

func NewSourceCodeKey(sourceCode SourceCode, flags uint8) SourceCodeKey {
	return SourceCodeKey{sourceCode: sourceCode, flags: flags}
}

func (k SourceCodeKey) SourceCode() SourceCode { return k.sourceCode }
func (k SourceCodeKey) Flags() uint8           { return k.flags }
func (k SourceCodeKey) Hash() uint32 {
	h := k.sourceCode.ProviderID()
	h = h*31 + int64(k.sourceCode.StartOffset())
	h = h*31 + int64(k.sourceCode.EndOffset())
	return uint32(h)
}
