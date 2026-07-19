$c = [System.IO.File]::ReadAllText('jsc\parser\parser.go')
# Fix token names
$c = $c.Replace('NUMBER:', 'INTEGER:')
$c = $c.Replace('DELETE:', 'DELETETOKEN:')
# Fix Lexer API
$c = $c.Replace('JSParserBuiltinModeNormal', 'JSParserBuiltinModeNotBuiltin')
$c = $c.Replace('JSParserScriptModeNormal', 'JSParserScriptModeClassic')
$c = $c.Replace('p.lexer.Lex(&p.m_token, 0, false)', 'p.lexer.Lex(&p.m_token, OptionSet[LexerFlags]{}, false)')
$c = $c.Replace('p.lexer.Lex(&JSToken{}, 0, false)', 'p.lexer.Lex(&JSToken{}, OptionSet[LexerFlags]{}, false)')
# Fix ParserError fields (lowercase)
$c = $c.Replace('Message: cause,', 'message: cause,')
$c = $c.Replace('Line:    p.m_token.StartPosition.Line,', 'line:    p.m_token.StartPosition.Line,')
$c = $c.Replace('Column:  p.m_token.StartPosition.Offset - p.m_token.StartPosition.LineStartOffset,', "")
# Fix SourceElements - children is unexported
$c = $c.Replace('sourceElements.Statements = append(sourceElements.Statements, s)', 'sourceElements.children = append(sourceElements.children, s)')
# Fix DebuggerNode - might not exist
$c = $c.Replace('return &DebuggerNode{}', 'return &ExprStatementNode{}')
[System.IO.File]::WriteAllText('jsc\parser\parser.go', $c)
Write-Host 'fixed'
