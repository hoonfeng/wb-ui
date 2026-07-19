// Parser.h — Main JavaScript parser (recursive descent)
// Source: Source/JavaScriptCore/parser/Parser.h
// Simplified Go translation.

package parser

import "wb-ui/jsc/runtime"

// Parser corresponds to JSC::Parser
// Recursive-descent parser for JavaScript.
type Parser struct {
	vm          *runtime.VM
	lexer       *Lexer
	arena       *ParserArena
	sourceCode  SourceCode
	builtinMode JSParserBuiltinMode
	scriptMode  JSParserScriptMode
	error       ParserError

	// Parser state
	token        JSToken
	tokenType    JSTokenType
	semanticFlag int

	// Context
	allowAwait bool
	allowYield bool
	allowSuper bool
	allowImport bool
	inLexicalScope int
}

func NewParser(vm *runtime.VM, lexer *Lexer, arena *ParserArena, sourceCode SourceCode,
	builtinMode JSParserBuiltinMode, scriptMode JSParserScriptMode) *Parser {

	p := &Parser{
		vm:          vm,
		lexer:       lexer,
		arena:       arena,
		sourceCode:  sourceCode,
		builtinMode: builtinMode,
		scriptMode:  scriptMode,
		allowAwait:  scriptMode == JSParserScriptModeModule,
		allowYield:  false,
		allowSuper:  false,
		allowImport: true,
	}
	lexer.SetCode(sourceCode, arena)
	return p
}

func (p *Parser) VM() *runtime.VM            { return p.vm }
func (p *Parser) Lexer() *Lexer              { return p.lexer }
func (p *Parser) ParserArena() *ParserArena  { return p.arena }
func (p *Parser) SourceCode() SourceCode     { return p.sourceCode }
func (p *Parser) HasError() bool             { return p.error.IsValid() }
func (p *Parser) Error() ParserError         { return p.error }

// nextToken — advance to next token
func (p *Parser) nextToken(flags OptionSet[LexerFlags], strictMode bool) JSTokenType {
	p.tokenType = p.lexer.Lex(&p.token, flags, strictMode)
	return p.tokenType
}

// peek — return current token type (already lexed)
func (p *Parser) peek() JSTokenType { return p.tokenType }

// consume — eat expected token, advance
func (p *Parser) consume(expected JSTokenType) bool {
	if p.tokenType == expected {
		p.nextToken(OptionSet[LexerFlags]{}, false)
		return true
	}
	return false
}

// expect — consume or report error
func (p *Parser) expect(expected JSTokenType) bool {
	if p.tokenType == expected {
		p.nextToken(OptionSet[LexerFlags]{}, false)
		return true
	}
	p.error = NewParserErrorWithMessage(ParserErrorSyntaxError, ParserSyntaxErrorIrrecoverable,
		p.token, "Expected token", p.lexer.LineNumber())
	return false
}

// match — check if current token matches
func (p *Parser) match(expected JSTokenType) bool {
	return p.tokenType == expected
}

// ============= Top Level =============

// Parse — parse source code and return AST
func (p *Parser) Parse(builder *ASTBuilder) *SourceElements {
	p.nextToken(OptionSet[LexerFlags]{}, false)

	if p.tokenType == EOFTOK {
		return NewSourceElementsNode(p.token.StartPosition, nil)
	}

	stmts := p.parseSourceElements(builder)
	return stmts
}

// parseSourceElements — parse list of statements
func (p *Parser) parseSourceElements(builder *ASTBuilder) *SourceElements {
	var stmts []StatementNode
	pos := p.token.StartPosition

	for p.tokenType != EOFTOK && p.tokenType != CLOSEBRACE {
		stmt := p.parseStatement(builder)
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
	}

	return NewSourceElementsNode(pos, stmts)
}

// ============= Statements =============

func (p *Parser) parseStatement(builder *ASTBuilder) StatementNode {
	switch p.tokenType {
	case FUNCTION:
		return p.parseFuncDecl(builder)
	case VAR, LET, CONSTTOKEN:
		return p.parseVarStatement(builder)
	case IF:
		return p.parseIfStatement(builder)
	case FOR:
		return p.parseForStatement(builder)
	case WHILE:
		return p.parseWhileStatement(builder)
	case DO:
		return p.parseDoWhileStatement(builder)
	case SWITCH:
		return p.parseSwitchStatement(builder)
	case TRY:
		return p.parseTryStatement(builder)
	case THROW:
		return p.parseThrowStatement(builder)
	case RETURN:
		return p.parseReturnStatement(builder)
	case BREAK:
		return p.parseBreakStatement(builder)
	case CONTINUE:
		return p.parseContinueStatement(builder)
	case WITH:
		return p.parseWithStatement(builder)
	case CLASSTOKEN:
		return p.parseClassDecl(builder)
	case DEBUGGER:
		return p.parseDebuggerStatement(builder)
	case OPENBRACE:
		return p.parseBlock(builder)
	case SEMICOLON:
		pos := p.token.StartPosition
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewEmptyStatementNode(pos)
	case IMPORT:
		return p.parseImportStatement(builder)
	case EXPORT_:
		return p.parseExportStatement(builder)
	default:
		return p.parseExprStatement(builder)
	}
}

func (p *Parser) parseBlock(builder *ASTBuilder) *BlockNode {
	pos := p.token.StartPosition
	if !p.expect(OPENBRACE) {
		return nil
	}

	var stmts []StatementNode
	for p.tokenType != CLOSEBRACE && p.tokenType != EOFTOK {
		stmt := p.parseStatement(builder)
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
	}
	p.expect(CLOSEBRACE)

	return NewBlockNode(pos, stmts)
}

func (p *Parser) parseVarStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	varType := VariableTypeVar
	if p.tokenType == LET {
		varType = VariableTypeLet
	} else if p.tokenType == CONSTTOKEN {
		varType = VariableTypeConst
	}

	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	var decls []*VariableDeclarationNode
	for {
		decl := p.parseVariableDecl(builder, varType)
		if decl != nil {
			decls = append(decls, decl)
		}
		if p.tokenType == COMMA {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		} else {
			break
		}
	}
	p.expect(SEMICOLON)
	return NewVarStatementNode(pos, decls)
}

func (p *Parser) parseVariableDecl(builder *ASTBuilder, varType VariableType) *VariableDeclarationNode {
	pos := p.token.StartPosition
	if p.tokenType != IDENT {
		return nil
	}
	// Get identifier
	ident := p.token.Data.Ident
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	var init ExpressionNode
	if p.tokenType == EQUAL {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		init = p.parseAssignmentExpression(builder)
	}

	name := ""
	if ident != nil {
		name = *ident
	}
	return NewVariableDeclarationNode(pos, name, init, varType)
}

func (p *Parser) parseIfStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(OPENPAREN)
	cond := p.parseExpression(builder)
	p.expect(CLOSEPAREN)
	ifBlk := p.parseStatement(builder)

	var elseBlk StatementNode
	if p.tokenType == ELSE {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		elseBlk = p.parseStatement(builder)
	}

	return NewIfElseNode(pos, cond, ifBlk, elseBlk)
}

func (p *Parser) parseWhileStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(OPENPAREN)
	cond := p.parseExpression(builder)
	p.expect(CLOSEPAREN)
	body := p.parseStatement(builder)
	return NewWhileNode(pos, cond, body)
}

func (p *Parser) parseDoWhileStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	body := p.parseStatement(builder)
	p.expect(WHILE)
	p.expect(OPENPAREN)
	cond := p.parseExpression(builder)
	p.expect(CLOSEPAREN)
	p.expect(SEMICOLON)
	return NewDoWhileNode(pos, body, cond)
}

func (p *Parser) parseForStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(OPENPAREN)

	var init, cond, incr ExpressionNode
	var lhs ExpressionNode

	if p.tokenType != SEMICOLON {
		if p.tokenType == VAR || p.tokenType == LET || p.tokenType == CONSTTOKEN {
			_ = p.parseVarStatement(builder)
		} else {
			init = p.parseExpression(builder)
		}
	}

	if p.tokenType == INTOKEN {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		lhs = init
	} else {
		p.expect(SEMICOLON)
		if p.tokenType != SEMICOLON {
			cond = p.parseExpression(builder)
		}
		p.expect(SEMICOLON)
		if p.tokenType != CLOSEPAREN {
			incr = p.parseExpression(builder)
		}
	}

	p.expect(CLOSEPAREN)
	body := p.parseStatement(builder)

	if p.tokenType == INTOKEN {
		return NewForInNode(pos, lhs, init, body)
	}
	return NewForNode(pos, init, cond, incr, body)
}

func (p *Parser) parseSwitchStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(OPENPAREN)
	expr := p.parseExpression(builder)
	p.expect(CLOSEPAREN)
	p.expect(OPENBRACE)

	var cases []*CaseClauseNode
	for p.tokenType == CASE || p.tokenType == DEFAULT {
		casePos := p.token.StartPosition
		var caseExpr ExpressionNode
		if p.tokenType == CASE {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			caseExpr = p.parseExpression(builder)
		} else {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}
		p.expect(COLON)

		var stmts []StatementNode
		for p.tokenType != CASE && p.tokenType != DEFAULT && p.tokenType != CLOSEBRACE && p.tokenType != EOFTOK {
			stmt := p.parseStatement(builder)
			if stmt != nil {
				stmts = append(stmts, stmt)
			}
		}
		cases = append(cases, NewCaseClauseNode(casePos, caseExpr, stmts))
	}
	p.expect(CLOSEBRACE)
	return NewSwitchNode(pos, expr, cases)
}

func (p *Parser) parseTryStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	tryBlk := p.parseBlock(builder)

	var catchBlk *CatchClauseNode
	if p.tokenType == CATCH {
		catchPos := p.token.StartPosition
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		var param ExpressionNode
		if p.tokenType == OPENPAREN {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			param = p.parseExpression(builder)
			p.expect(CLOSEPAREN)
		}
		body := p.parseBlock(builder)
		catchBlk = NewCatchClauseNode(catchPos, param, body)
	}

	var finallyBlk *BlockNode
	if p.tokenType == FINALLY {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		finallyBlk = p.parseBlock(builder)
	}

	return NewTryNode(pos, tryBlk, catchBlk, finallyBlk)
}

func (p *Parser) parseThrowStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	val := p.parseExpression(builder)
	p.expect(SEMICOLON)
	return NewThrowNode(pos, val)
}

func (p *Parser) parseReturnStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	var val ExpressionNode
	if p.tokenType != SEMICOLON {
		val = p.parseExpression(builder)
	}
	p.expect(SEMICOLON)
	return NewReturnNode(pos, val)
}

func (p *Parser) parseBreakStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	if p.tokenType == SEMICOLON {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}
	return NewBreakNode(pos)
}

func (p *Parser) parseContinueStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	if p.tokenType == SEMICOLON {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}
	return NewContinueNode(pos)
}

func (p *Parser) parseWithStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(OPENPAREN)
	expr := p.parseExpression(builder)
	p.expect(CLOSEPAREN)
	body := p.parseStatement(builder)
	return NewWithNode(pos, expr, body)
}

func (p *Parser) parseDebuggerStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	p.expect(SEMICOLON)
	return NewDebuggerStatementNode(pos)
}

func (p *Parser) parseImportStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	_ = pos // simplified import parsing
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	_ = p.parseExpression(builder)
	return NewExprStatementNode(pos, nil)
}

func (p *Parser) parseExportStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	_ = pos
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	stmt := p.parseStatement(builder)
	return stmt
}

// ============= Function & Class =============

func (p *Parser) parseFuncDecl(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	name := ""
	if p.tokenType == IDENT {
		if p.token.Data.Ident != nil {
			name = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}

	params, body := p.parseFuncBody(builder)
	return NewFuncDeclNode(pos, name, params, body, p.sourceCode)
}

func (p *Parser) parseFuncBody(builder *ASTBuilder) ([]string, *BlockNode) {
	var params []string
	p.expect(OPENPAREN)
	for p.tokenType != CLOSEPAREN && p.tokenType != EOFTOK {
		if p.tokenType == IDENT {
			if p.token.Data.Ident != nil {
				params = append(params, *p.token.Data.Ident)
			}
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}
		if p.tokenType == COMMA {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}
	}
	p.expect(CLOSEPAREN)

	body := p.parseBlock(builder)
	return params, body
}

func (p *Parser) parseClassDecl(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	name := ""
	if p.tokenType == IDENT {
		if p.token.Data.Ident != nil {
			name = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}

	var extends ExpressionNode
	if p.tokenType == EXTENDS {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		extends = p.parseExpression(builder)
	}

	p.expect(OPENBRACE)
	var methods []*ClassMethodNode
	for p.tokenType != CLOSEBRACE && p.tokenType != EOFTOK {
		isStatic := false
		isGetter := false
		isSetter := false

		if p.tokenType == IDENT {
			if p.token.Data.Ident != nil {
				switch *p.token.Data.Ident {
				case "static":
					isStatic = true
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				case "get":
					isGetter = true
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				case "set":
					isSetter = true
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				}
			}
		}

		methodName := ""
		if p.tokenType == IDENT || p.tokenType == PRIVATENAME {
			if p.token.Data.Ident != nil {
				methodName = *p.token.Data.Ident
			}
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}

		methodParams, methodBody := p.parseFuncBody(builder)
		fn := NewFuncExprNode(p.token.StartPosition, methodName, methodParams, methodBody, p.sourceCode)

		methods = append(methods, NewClassMethodNode(
			p.token.StartPosition, methodName, fn, isStatic, isGetter, isSetter,
			methodName == "constructor",
		))
	}
	p.expect(CLOSEBRACE)

	return NewClassDeclNode(pos, name, extends, methods)
}

// ============= Expression =============

func (p *Parser) parseExprStatement(builder *ASTBuilder) StatementNode {
	pos := p.token.StartPosition
	expr := p.parseExpression(builder)
	if p.tokenType == SEMICOLON {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}
	return NewExprStatementNode(pos, expr)
}

func (p *Parser) parseExpression(builder *ASTBuilder) ExpressionNode {
	return p.parseCommaExpression(builder)
}

func (p *Parser) parseCommaExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	expr := p.parseAssignmentExpression(builder)
	for p.tokenType == COMMA {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseAssignmentExpression(builder)
		expr = NewCommaNode(pos, []ExpressionNode{expr, right})
	}
	return expr
}

func (p *Parser) parseAssignmentExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	expr := p.parseTernaryExpression(builder)

	if p.tokenType == EQUAL {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseAssignmentExpression(builder)
		return NewBinaryOpNode(pos, EQUAL, expr, right)
	}

	// Compound assignment operators
	switch p.tokenType {
	case PLUSEQUAL, MINUSEQUAL, MULTEQUAL, DIVEQUAL,
		LSHIFTEQUAL, RSHIFTEQUAL, URSHIFTEQUAL, MODEQUAL, POWEQUAL,
		BITANDEQUAL, BITXOREQUAL, BITOREQUAL,
		COALESCEEQUAL, OREQUAL, ANDEQUAL:
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseAssignmentExpression(builder)
		return NewBinaryOpNode(pos, op, expr, right)
	}

	return expr
}

func (p *Parser) parseTernaryExpression(builder *ASTBuilder) ExpressionNode {
	expr := p.parseLogicalExpression(builder)

	if p.tokenType == QUESTION {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		trueExpr := p.parseAssignmentExpression(builder)
		p.expect(COLON)
		falseExpr := p.parseAssignmentExpression(builder)
		return NewConditionalNode(expr.Position(), expr, trueExpr, falseExpr)
	}
	return expr
}

func (p *Parser) parseLogicalExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseBitwiseExpression(builder)

	for p.tokenType == AND || p.tokenType == OR || p.tokenType == COALESCE {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseBitwiseExpression(builder)
		left = NewLogicalOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseBitwiseExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseEqualityExpression(builder)

	for p.tokenType == BITOR || p.tokenType == BITXOR || p.tokenType == BITAND {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseEqualityExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseEqualityExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseRelationalExpression(builder)

	for p.tokenType == EQEQ || p.tokenType == NE || p.tokenType == STREQ || p.tokenType == STRNEQ {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseRelationalExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseRelationalExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseShiftExpression(builder)

	for p.tokenType == LT || p.tokenType == GT || p.tokenType == LE || p.tokenType == GE ||
		p.tokenType == INSTANCEOF || p.tokenType == INTOKEN {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseShiftExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseShiftExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseAdditiveExpression(builder)

	for p.tokenType == LSHIFT || p.tokenType == RSHIFT || p.tokenType == URSHIFT {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseAdditiveExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseAdditiveExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseMultiplicativeExpression(builder)

	for p.tokenType == PLUS || p.tokenType == MINUS {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseMultiplicativeExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseMultiplicativeExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseUnaryExpression(builder)

	for p.tokenType == TIMES || p.tokenType == DIVIDE || p.tokenType == MOD {
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseUnaryExpression(builder)
		left = NewBinaryOpNode(pos, op, left, right)
		pos = left.Position()
	}
	return left
}

func (p *Parser) parseUnaryExpression(builder *ASTBuilder) ExpressionNode {
	// Prefix increment/decrement
	if p.tokenType == PLUSPLUS || p.tokenType == MINUSMINUS {
		pos := p.token.StartPosition
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		expr := p.parseUnaryExpression(builder)
		_ = op // prefix ++x or --x
		return NewUnaryOpNode(pos, op, expr)
	}

	// Unary operators
	if IsUnaryOp(p.tokenType) || p.tokenType == PLUS || p.tokenType == MINUS {
		pos := p.token.StartPosition
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		// Special handling for typeof/void/delete
		if op == TYPEOF || op == VOIDTOKEN || op == DELETETOKEN || op == EXCLAMATION ||
			op == TILDE || op == PLUS || op == MINUS {
			expr := p.parseUnaryExpression(builder)
			return NewUnaryOpNode(pos, op, expr)
		}
	}

	return p.parseExponentiationExpression(builder)
}

func (p *Parser) parseExponentiationExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	left := p.parseMemberExpression(builder)

	if p.tokenType == POW {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		right := p.parseExponentiationExpression(builder)
		left = NewBinaryOpNode(pos, POW, left, right)
	}
	return left
}

func (p *Parser) parseMemberExpression(builder *ASTBuilder) ExpressionNode {
	expr := p.parsePrimaryExpression(builder)

	for {
		switch p.tokenType {
		case DOT:
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			if p.tokenType == IDENT || p.tokenType == PRIVATENAME {
				name := ""
				if p.token.Data.Ident != nil {
					name = *p.token.Data.Ident
				}
				// Create an Identifier from the name
				_ = name
				ident := runtime.NewIdentifier(name)
			expr = NewDotAccessorNode(expr.Position(), expr, &ident)
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			}
		case OPENBRACKET:
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			subscript := p.parseExpression(builder)
			p.expect(CLOSEBRACKET)
			expr = NewBracketAccessorNode(expr.Position(), expr, subscript)
		case OPENPAREN:
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			var args []ExpressionNode
			for p.tokenType != CLOSEPAREN && p.tokenType != EOFTOK {
				args = append(args, p.parseAssignmentExpression(builder))
				if p.tokenType == COMMA {
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				}
			}
			p.expect(CLOSEPAREN)
			expr = NewFuncCallExprNode(expr.Position(), expr, args)
		case QUESTIONDOT:
			// Optional chaining (simplified)
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			_ = p.parseMemberExpression(builder)
		default:
			return expr
		}
	}
}

func (p *Parser) parsePrimaryExpression(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition

	switch p.tokenType {
	case THISTOKEN:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewThisNode(pos)

	case SUPER:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewSuperNode(pos)

	case NULLTOKEN:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewNullNode(pos)

	case TRUETOKEN:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewBooleanNodeData(pos, true)

	case FALSETOKEN:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewBooleanNodeData(pos, false)

	case INTEGER:
		val := 0.0
		// FIXME: parse integer value
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewNumberNodeData(pos, val)

	case DOUBLE:
		val := p.token.Data.DoubleValue
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewNumberNodeData(pos, val)

	case STRING:
		val := ""
		if p.token.Data.Ident != nil {
			val = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewStringNodeData(pos, val)

	case BIGINT:
		val := ""
		if p.token.Data.BigIntString != nil {
			val = *p.token.Data.BigIntString
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewBigIntNodeData(pos, val, p.token.Data.Radix)

	case IDENT:
		ident := ""
		if p.token.Data.Ident != nil {
			ident = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		// Check for arrow function
		if p.tokenType == ARROWFUNCTION {
			body := NewBlockNode(pos, nil)
			return NewArrowFuncExprNode(pos, []string{ident}, body)
		}
		return NewResolveNode(pos, ident)

	case FUNCTION:
		return p.parseFuncExpr(builder)

	case CLASSTOKEN:
		return p.parseClassExpr(builder)

	case OPENBRACKET:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		var elements []ExpressionNode
		for p.tokenType != CLOSEBRACKET && p.tokenType != EOFTOK {
			if p.tokenType == COMMA {
				elements = append(elements, nil)
				p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			} else {
				elements = append(elements, p.parseAssignmentExpression(builder))
				if p.tokenType == COMMA {
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				}
			}
		}
		p.expect(CLOSEBRACKET)
		return NewArrayNode(pos, elements)

	case OPENBRACE:
		return p.parseObjectLiteral(builder)

	case OPENPAREN:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		expr := p.parseExpression(builder)
		p.expect(CLOSEPAREN)
		return expr

	case NEW:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		callee := p.parseMemberExpression(builder)
		var args []ExpressionNode
		if p.tokenType == OPENPAREN {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			for p.tokenType != CLOSEPAREN && p.tokenType != EOFTOK {
				args = append(args, p.parseAssignmentExpression(builder))
				if p.tokenType == COMMA {
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				}
			}
			p.expect(CLOSEPAREN)
		}
		return NewNewExprNode(pos, callee, args)

	case BACKQUOTE:
		return p.parseTemplateLiteral(builder)

	case DOTDOTDOT:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		expr := p.parseAssignmentExpression(builder)
		return NewSpreadExpressionNode(pos, expr)

	case YIELD:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		var arg ExpressionNode
		if p.tokenType != SEMICOLON {
			arg = p.parseAssignmentExpression(builder)
		}
		return NewYieldExprNode(pos, arg, false)

	case AWAIT:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		arg := p.parseUnaryExpression(builder)
		return NewAwaitExprNode(pos, arg)

	case DELETETOKEN, VOIDTOKEN, TYPEOF, EXCLAMATION, TILDE, PLUS, MINUS:
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		expr := p.parseUnaryExpression(builder)
		return NewUnaryOpNode(pos, op, expr)

	case PLUSPLUS, MINUSMINUS:
		op := p.tokenType
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		expr := p.parseUnaryExpression(builder)
		return NewUnaryOpNode(pos, op, expr)

	case REGEXP:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		// Simplified — return a string node for regexp
		return NewStringNodeData(pos, "/regex/")

	default:
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		return NewResolveNode(pos, "undefined")
	}
}

func (p *Parser) parseFuncExpr(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	name := ""
	if p.tokenType == IDENT {
		if p.token.Data.Ident != nil {
			name = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}

	params, body := p.parseFuncBody(builder)
	return NewFuncExprNode(pos, name, params, body, p.sourceCode)
}

func (p *Parser) parseClassExpr(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())

	name := ""
	if p.tokenType == IDENT {
		if p.token.Data.Ident != nil {
			name = *p.token.Data.Ident
		}
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	}

	var extends ExpressionNode
	if p.tokenType == EXTENDS {
		p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		extends = p.parseExpression(builder)
	}

	p.expect(OPENBRACE)
	var methods []*ClassMethodNode
	for p.tokenType != CLOSEBRACE && p.tokenType != EOFTOK {
		isStatic, isGetter, isSetter := false, false, false

		if p.tokenType == IDENT && p.token.Data.Ident != nil {
			switch *p.token.Data.Ident {
			case "static": isStatic = true; p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			case "get": isGetter = true; p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			case "set": isSetter = true; p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			}
		}

		methodName := ""
		if p.tokenType == IDENT || p.tokenType == PRIVATENAME {
			if p.token.Data.Ident != nil {
				methodName = *p.token.Data.Ident
			}
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}

		methodParams, methodBody := p.parseFuncBody(builder)
		fn := NewFuncExprNode(p.token.StartPosition, methodName, methodParams, methodBody, p.sourceCode)
		methods = append(methods, NewClassMethodNode(p.token.StartPosition, methodName, fn,
			isStatic, isGetter, isSetter, methodName == "constructor"))
	}
	p.expect(CLOSEBRACE)

	return NewClassExprNode(pos, name, extends, methods)
}

func (p *Parser) parseObjectLiteral(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	p.expect(OPENBRACE)

	var props []*PropertyNode
	for p.tokenType != CLOSEBRACE && p.tokenType != EOFTOK {
		if p.tokenType == DOTDOTDOT {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			val := p.parseAssignmentExpression(builder)
			props = append(props, NewPropertyNode(p.token.StartPosition, "", val, PropertyKindSpread))
		} else {
			isGetter, isSetter := false, false
			kind := PropertyKindInit

			if p.tokenType == IDENT && p.token.Data.Ident != nil {
				ident := *p.token.Data.Ident
				if ident == "get" && p.peek() != COLON {
					isGetter = true
					kind = PropertyKindGet
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				} else if ident == "set" && p.peek() != COLON {
					isSetter = true
					kind = PropertyKindSet
					p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				}
			}
			_ = isGetter
			_ = isSetter

			name := ""
			if p.tokenType == IDENT || p.tokenType == STRING {
				if p.token.Data.Ident != nil {
					name = *p.token.Data.Ident
				}
				p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			} else if p.tokenType == INTEGER || p.tokenType == DOUBLE {
				name = "0"
				p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
			} else if p.tokenType == OPENBRACKET {
				// Computed property name
				p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				compName := p.parseExpression(builder)
				_ = compName
				p.expect(CLOSEBRACKET)
				name = "[computed]"
			}

			if p.tokenType == COLON {
				p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
				val := p.parseAssignmentExpression(builder)
				props = append(props, NewPropertyNode(p.token.StartPosition, name, val, kind))
			} else if p.tokenType == OPENPAREN {
				// Method definition
				_, body := p.parseFuncBody(builder)
				fn := NewFuncExprNode(p.token.StartPosition, name, nil, body, p.sourceCode)
				_ = fn
				props = append(props, NewPropertyNode(p.token.StartPosition, name, nil, PropertyKindInit))
			} else {
				// Shorthand
				props = append(props, NewPropertyNode(p.token.StartPosition, name, nil, PropertyKindInit))
			}
		}

		if p.tokenType == COMMA {
			p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
		}
	}
	p.expect(CLOSEBRACE)

	return NewObjectNode(pos, props)
}

func (p *Parser) parseTemplateLiteral(builder *ASTBuilder) ExpressionNode {
	pos := p.token.StartPosition
	// Simplified template literal
	p.nextToken(OptionSet[LexerFlags]{}, p.inStrictMode())
	return NewStringNodeData(pos, "")
}

// ============= Helpers =============

func (p *Parser) inStrictMode() bool {
	return false
}
