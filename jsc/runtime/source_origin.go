/*
 * Copyright (C) 2016 Yusuke Suzuki <utatane.tea@gmail.com>.
 * Copyright (C) 2025 wb-ui Authors.
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
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. AND ITS CONTRIBUTORS ``AS IS''
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
 * THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL APPLE INC. OR ITS CONTRIBUTORS
 * BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
 * THE POSSIBILITY OF SUCH DAMAGE.
 */

// SourceOrigin.h (WebKit) — Source origin tracking

package runtime

// SourceOrigin represents the origin (URL) of JavaScript source code.
// Corresponds to JSC::SourceOrigin
type SourceOrigin struct {
	url     string
	fetcher *ScriptFetcher
}

// ScriptFetcher is a placeholder for the script fetching mechanism.
type ScriptFetcher struct{}

func NewSourceOrigin(url string) SourceOrigin {
	return SourceOrigin{url: url}
}

func NewSourceOriginWithFetcher(url string, fetcher *ScriptFetcher) SourceOrigin {
	return SourceOrigin{url: url, fetcher: fetcher}
}

func (s SourceOrigin) URL() string           { return s.url }
func (s SourceOrigin) String() string         { return s.url }
func (s SourceOrigin) IsNull() bool           { return s.url == "" }
func (s SourceOrigin) Fetcher() *ScriptFetcher { return s.fetcher }
