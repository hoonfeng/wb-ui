/*
 * Copyright (C) 2012 Apple Inc. All rights reserved.
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

// ParserArena.h — Parser memory arena for AST node allocation

package parser

import (
	"sync"

	"wb-ui/jsc/runtime"
)

// ParserArena corresponds to JSC::ParserArena
// Provides memory management for AST nodes during parsing.
type ParserArena struct {
	mu       sync.Mutex
	types    []*runtime.Identifier
	deleted  bool
}

func NewParserArena() *ParserArena {
	return &ParserArena{}
}

func (arena *ParserArena) Deleted() bool { return arena.deleted }
func (arena *ParserArena) SetDeleted()   { arena.deleted = true }

// AddTypes adds type identifiers to the arena
func (arena *ParserArena) AddTypes(idents []*runtime.Identifier) {
	arena.mu.Lock()
	defer arena.mu.Unlock()
	arena.types = append(arena.types, idents...)
}
