// Copyright (C) 2012 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/CodeBlockJettisoningWatchpoint.h

package bytecode

// CodeBlockJettisoningWatchpoint is a watchpoint that triggers jettisoning
// of a CodeBlock when fired. In the interpreter, this is a no-op.
type CodeBlockJettisoningWatchpoint struct {
	// In JIT mode, this fires to discard optimized code.
	// In interpreter mode, this is unused.
}
