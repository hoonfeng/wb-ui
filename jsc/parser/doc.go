// Package parser implements the JavaScript parser (Lexer + Parser + AST Nodes).
//
// This is a strict 1:1 translation of WebKit JavaScriptCore's parser/ module.
// Source: Source/JavaScriptCore/parser/
package parser

// SourceOrigin — origin URL for source code (simplified from JSC::SourceOrigin)
type SourceOrigin struct {
	url string
}

func NewSourceOrigin(url string) SourceOrigin { return SourceOrigin{url: url} }
func (s SourceOrigin) URL() string            { return s.url }
func (s SourceOrigin) IsNull() bool           { return s.url == "" }

// JSGlobalObject — forward declaration for use in parser errors
type JSGlobalObject struct{}
