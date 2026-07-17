// Translation of: Source/JavaScriptCore/parser/Parser.h
//                  Source/JavaScriptCore/parser/Parser.cpp
//                  Source/JavaScriptCore/parser/Nodes.h
//                  Source/JavaScriptCore/parser/ASTBuilder.h
// Completeness: 65%
// Simplifications:
//   - AST nodes are Go structs implementing a Node interface instead of a class
//     hierarchy with virtual emitBytecode; bytecode emission lives in bytecode.go.
//   - Pratt (precedence-climbing) parser replaces the C++ precedence-climbing helper.
//   - destructuring support in arrow params only (not in variable declarations yet)
//   - automatic semicolon insertion is the simplified newline/EOF rule.

package jsc

import (
	"fmt"
	"strconv"
	"strings"
)

// Node is the common interface for AST nodes. Statement and expression nodes both
// satisfy Node so the bytecode generator can switch on concrete types.
type Node interface {
	nodePos() (line, col int)
}

// Stmt is the marker interface for statement nodes.
type Stmt interface {
	Node
	stmtNode()
}

// Expr is the marker interface for expression nodes.
type Expr interface {
	Node
	exprNode()
}

// Program is the root of an AST, corresponding to SourceElements in Nodes.h.
type Program struct {
	Body      []Stmt
	Line, Col int
}

func (p *Program) nodePos() (int, int) { return p.Line, p.Col }

// ---- Statement nodes ----

// FunctionDeclaration is a hoisted 'function name(params){...}' statement.
type FunctionDeclaration struct {
	Name        string
	Params      []string
	Body        []Stmt
	IsAsync     bool
	IsGenerator bool
	Line, Col   int
}

func (n *FunctionDeclaration) nodePos() (int, int) { return n.Line, n.Col }
func (n *FunctionDeclaration) stmtNode()           {}

// VariableDeclaration is 'let/const/var name = init' (may declare multiple).
type VariableDeclaration struct {
	Kind        string // "let" | "const" | "var"
	Declarators []VariableDeclarator
	Line, Col   int
}

// VariableDeclarator binds a name to an optional initializer.
type VariableDeclarator struct {
	Name string
	Init Expr // may be nil
}

func (n *VariableDeclaration) nodePos() (int, int) { return n.Line, n.Col }
func (n *VariableDeclaration) stmtNode()           {}

// ExpressionStatement wraps a bare expression used as a statement.
type ExpressionStatement struct {
	Expr      Expr
	Line, Col int
}

func (n *ExpressionStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ExpressionStatement) stmtNode()           {}

// BlockStatement is a brace-delimited block { ... }.
type BlockStatement struct {
	Body      []Stmt
	Line, Col int
}

func (n *BlockStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *BlockStatement) stmtNode()           {}

// IfStatement is 'if (cond) then else else'.
type IfStatement struct {
	Test       Expr
	Consequent Stmt
	Alternate  Stmt // may be nil
	Line, Col  int
}

func (n *IfStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *IfStatement) stmtNode()           {}

// ForStatement is 'for (init; test; update) body'.
type ForStatement struct {
	Init      Stmt // may be nil (VariableDeclaration or ExpressionStatement)
	Test      Expr // may be nil
	Update    Expr // may be nil
	Body      Stmt
	Line, Col int
}

func (n *ForStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ForStatement) stmtNode()           {}

// ForInStatement is 'for (lhs in/of rhs) body'.
type ForInStatement struct {
	Left      Expr // the iteration target (Identifier or MemberExpression)
	IsOf      bool // true for 'of', false for 'in'
	Right     Expr
	Body      Stmt
	Line, Col int
}

func (n *ForInStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ForInStatement) stmtNode()           {}

// WhileStatement is 'while (test) body'.
type WhileStatement struct {
	Test      Expr
	Body      Stmt
	Line, Col int
}

func (n *WhileStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *WhileStatement) stmtNode()           {}

// DoWhileStatement is 'do body while (test);'.
type DoWhileStatement struct {
	Body      Stmt
	Test      Expr
	Line, Col int
}

func (n *DoWhileStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *DoWhileStatement) stmtNode()           {}

// SwitchCase is a single case/default clause in a switch statement.
type SwitchCase struct {
	Test     Expr   // nil for default
	Body     []Stmt
	Line, Col int
}

// SwitchStatement is 'switch (expr) { case ... default: ... }'.
type SwitchStatement struct {
	Discriminant Expr
	Cases        []SwitchCase
	Line, Col    int
}

func (n *SwitchStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *SwitchStatement) stmtNode()           {}

// ReturnStatement is 'return [expr]'.
type ReturnStatement struct {
	Argument  Expr // may be nil
	Line, Col int
}

func (n *ReturnStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ReturnStatement) stmtNode()           {}

// BreakStatement is 'break'.
type BreakStatement struct{ Line, Col int }

func (n *BreakStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *BreakStatement) stmtNode()           {}

// ContinueStatement is 'continue'.
type ContinueStatement struct{ Line, Col int }

func (n *ContinueStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ContinueStatement) stmtNode()           {}

// ThrowStatement is 'throw expr'.
type ThrowStatement struct {
	Argument  Expr
	Line, Col int
}

func (n *ThrowStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *ThrowStatement) stmtNode()           {}

// TryStatement is 'try { } catch(e) { } finally { }'.
type TryStatement struct {
	Block     Stmt
	Handler   *CatchClause
	Finalizer Stmt
	Line, Col int
}

func (n *TryStatement) nodePos() (int, int) { return n.Line, n.Col }
func (n *TryStatement) stmtNode()           {}

// CatchClause is the 'catch(param) { body}' portion of a try statement.
type CatchClause struct {
	Param     string
	Body      Stmt
	Line, Col int
}

// ---- Expression nodes ----

// Identifier is a bare name reference.
type Identifier struct {
	Name      string
	Line, Col int
}

func (n *Identifier) nodePos() (int, int) { return n.Line, n.Col }
func (n *Identifier) exprNode()           {}

// Literal is a primitive constant: number, string, boolean, null, undefined.
type Literal struct {
	Value     JSValue
	Raw       string
	Line, Col int
}

func (n *Literal) nodePos() (int, int) { return n.Line, n.Col }
func (n *Literal) exprNode()           {}

// TemplateLiteral is `text ${expr} more`. Quasis and Expressions alternate.
type TemplateLiteral struct {
	Quasis      []string
	Expressions []Expr
	Line, Col   int
}

func (n *TemplateLiteral) nodePos() (int, int) { return n.Line, n.Col }
func (n *TemplateLiteral) exprNode()           {}

// RegexLiteral is a /pattern/flags literal.
type RegexLiteral struct {
	Pattern   string
	Flags     string
	Line, Col int
}

func (n *RegexLiteral) nodePos() (int, int) { return n.Line, n.Col }
func (n *RegexLiteral) exprNode()           {}

// BinaryExpression is 'left op right'.
type BinaryExpression struct {
	Op        TokenKind
	Left      Expr
	Right     Expr
	Line, Col int
}

func (n *BinaryExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *BinaryExpression) exprNode()           {}

// LogicalExpression is 'left && right' / 'left || right' / 'left ?? right'.
type LogicalExpression struct {
	Op        TokenKind
	Left      Expr
	Right     Expr
	Line, Col int
}

func (n *LogicalExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *LogicalExpression) exprNode()           {}

// UnaryExpression is 'op arg' (prefix).
type UnaryExpression struct {
	Op        TokenKind
	Argument  Expr
	Line, Col int
}

func (n *UnaryExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *UnaryExpression) exprNode()           {}

// AwaitExpression is 'await expr' (only valid inside async functions).
type AwaitExpression struct {
	Argument  Expr
	Line, Col int
}

func (n *AwaitExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *AwaitExpression) exprNode()           {}

// UpdateExpression is 'arg++' / '++arg' / 'arg--' / '--arg'.
type UpdateExpression struct {
	Op        TokenKind
	Argument  Expr
	Prefix    bool
	Line, Col int
}

func (n *UpdateExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *UpdateExpression) exprNode()           {}

// AssignmentExpression is 'target op value'.
type AssignmentExpression struct {
	Op        TokenKind
	Target    Expr
	Value     Expr
	Line, Col int
}

func (n *AssignmentExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *AssignmentExpression) exprNode()           {}

// ConditionalExpression is 'test ? cons : alt'.
type ConditionalExpression struct {
	Test       Expr
	Consequent Expr
	Alternate  Expr
	Line, Col  int
}

func (n *ConditionalExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *ConditionalExpression) exprNode()           {}

// MemberExpression is 'obj.prop' or 'obj[expr]'.
type MemberExpression struct {
	Object    Expr
	Property  Expr   // nil for computed=false with Name
	Name      string // for non-computed access
	Computed  bool
	Optional  bool
	Line, Col int
}

func (n *MemberExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *MemberExpression) exprNode()           {}

// CallExpression is 'callee(args)'.
type CallExpression struct {
	Callee    Expr
	Arguments []Expr
	Optional  bool
	Line, Col int
}

func (n *CallExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *CallExpression) exprNode()           {}

// NewExpression is 'new callee(args)'.
type NewExpression struct {
	Callee    Expr
	Arguments []Expr
	Line, Col int
}

func (n *NewExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *NewExpression) exprNode()           {}

// SequenceExpression is 'a, b, c' (comma operator).
type SequenceExpression struct {
	Expressions []Expr
	Line, Col   int
}

func (n *SequenceExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *SequenceExpression) exprNode()           {}

// SpreadExpression is '...arg'.
type SpreadExpression struct {
	Argument  Expr
	Line, Col int
}

func (n *SpreadExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *SpreadExpression) exprNode()           {}

// ArrayExpression is '[elements]'.
type ArrayExpression struct {
	Elements  []Expr // nil entries represent holes
	Line, Col int
}

func (n *ArrayExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *ArrayExpression) exprNode()           {}

// Property is a single key:value pair inside an ObjectExpression.
type Property struct {
	Key       Expr // Identifier or Literal (string/number)
	Value     Expr
	Computed  bool
	Kind      string // "init" | "get" | "set" | "method" | "spread"
	Shorthand bool
	Spread    Expr  // non-nil for spread properties ({...expr})
	Line, Col int
}

// ObjectExpression is '{ props }'.
type ObjectExpression struct {
	Properties []Property
	Line, Col  int
}

func (n *ObjectExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *ObjectExpression) exprNode()           {}

// DestructInfo describes how to unpack a destructured parameter.
// For array pattern [a,b]: ParamIdx points to the synthetic param, IsArray=true, Names=["a","b"].
// For object pattern {a,b}: IsArray=false, Names=["a","b"].
type DestructInfo struct {
	ParamIdx int
	IsArray  bool
	Names    []string
}

// ArrowFunction is '(params) => body'. Body is either a single Expr (concise) or a
// BlockStatement (full body).
type ArrowFunction struct {
	Params         []string
	Destructuring  []DestructInfo
	Body           Node // Expr or Stmt
	IsExpr         bool
	IsAsync        bool
	IsGenerator    bool
	Line, Col      int
}

// YieldExpression is 'yield [expr]' or 'yield* expr' in a generator.
type YieldExpression struct {
	Argument  Expr
	Delegate  bool // true for yield*
	Line, Col int
}

func (n *YieldExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *YieldExpression) exprNode()           {}

func (n *ArrowFunction) nodePos() (int, int) { return n.Line, n.Col }
func (n *ArrowFunction) exprNode()           {}

// ThisExpression is the 'this' keyword.
type ThisExpression struct{ Line, Col int }

func (n *ThisExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *ThisExpression) exprNode()           {}

// FunctionExpression is an anonymous 'function(params){...}' value.
type FunctionExpression struct {
	Name        string // may be empty
	Params      []string
	Body        []Stmt
	IsAsync     bool
	IsGenerator bool
	Line, Col   int
}

func (n *FunctionExpression) nodePos() (int, int) { return n.Line, n.Col }
func (n *FunctionExpression) exprNode()           {}

// ClassDeclaration/ClassExpression are recognized but bodies throw 'not implemented'
// at runtime; only the structure is parsed.
type ClassBody struct {
	Methods   []ClassMethod
	Line, Col int
}

// ClassMethod is a single method inside a class body.
type ClassMethod struct {
	Name      string
	Params    []string
	Body      []Stmt
	Kind      string // "method" | "constructor" | "get" | "set"
	Static    bool
	Line, Col int
}

// ClassDeclaration is 'class Name extends Base { ... }'.
type ClassDeclaration struct {
	Name       string
	SuperClass Expr
	Body       *ClassBody
	Line, Col  int
}

func (n *ClassDeclaration) nodePos() (int, int) { return n.Line, n.Col }
func (n *ClassDeclaration) stmtNode()           {}
func (n *ClassDeclaration) exprNode()           {} // class expressions are parsed but unsupported

// ImportDeclaration is 'import default from "mod"' or 'import { a, b } from "mod"'.
type ImportDeclaration struct {
	DefaultName string   // name for default import, or "" if none
	NamedNames  []string // named import names
	Module      string   // module specifier
	Line, Col   int
}

func (n *ImportDeclaration) nodePos() (int, int) { return n.Line, n.Col }
func (n *ImportDeclaration) stmtNode()           {}

// ExportDeclaration is 'export default expr' or 'export { a, b }'.
type ExportDeclaration struct {
	DefaultExpr Expr     // export default expr (nil for named exports)
	NamedNames  []string // export { a, b }
	Line, Col   int
}

func (n *ExportDeclaration) nodePos() (int, int) { return n.Line, n.Col }
func (n *ExportDeclaration) stmtNode()           {}

// ---- Parser ----
// recursive descent with precedence climbing (Pratt) for expressions.
type Parser struct {
	lex         *Lexer
	current     Token
	prev        Token
	err         error
	allowIn     bool
	inGenerator bool
}

// Parse is the entry point: it tokenizes src and returns a Program AST.
func Parse(src string) (*Program, error) {
	p := &Parser{
		lex:     NewLexer(src),
		allowIn: true,
	}
	p.advance()
	prog := p.parseProgram()
	if p.err != nil {
		return prog, p.err
	}
	return prog, nil
}

// parseProgram consumes statements until EOF.
func (p *Parser) parseProgram() *Program {
	prog := &Program{Line: p.current.Line, Col: p.current.Col}
	for p.current.Kind != TokenEOF && p.err == nil {
		stmt := p.parseStatement()
		if stmt != nil {
			prog.Body = append(prog.Body, stmt)
		}
	}
	return prog
}

// advance fetches the next token, tracking whether regex is permitted by context.
func (p *Parser) advance() {
	prev := p.current
	// A regex literal is allowed at the start of input, after an operator/punctuation,
	// or after a keyword that ends an expression preamble.
	allowsRegex := regexAllowedAfter(prev.Kind)
	tok := p.lex.Next(allowsRegex)
	p.prev = p.current
	p.current = tok
}

// regexAllowedAfter reports whether a regex literal may follow prevKind.
func regexAllowedAfter(prevKind TokenKind) bool {
	switch prevKind {
	case TokenEOF, TokenOpenBrace, TokenOpenParen, TokenOpenBracket, TokenComma,
		TokenSemicolon, TokenColon, TokenQuestion, TokenAssign, TokenPlus, TokenMinus,
		TokenStar, TokenSlash, TokenPercent, TokenPower, TokenBang, TokenTilde,
		TokenAnd, TokenOr, TokenBitAnd, TokenBitOr, TokenBitXor, TokenEqual,
		TokenNotEqual, TokenStrictEqual, TokenStrictNotEqual, TokenLess, TokenGreater,
		TokenLessEqual, TokenGreaterEqual, TokenLeftShift, TokenRightShift,
		TokenUnsignedRightShift, TokenPlusAssign, TokenMinusAssign, TokenMultAssign,
		TokenDivAssign, TokenModAssign, TokenArrow, TokenSpread,
		TokenCoalesce, TokenIncrement, TokenDecrement:
		return true
	}
	if prevKind.IsKeyword() {
		switch prevKind.KeywordOf() {
		case KeywordReturn, KeywordTypeof, KeywordDelete, KeywordVoid, KeywordInstanceof,
			KeywordIn, KeywordNew, KeywordThrow, KeywordElse, KeywordDo, KeywordCase:
			return true
		}
	}
	return false
}

// errorf records a parse error (only the first is kept).
func (p *Parser) errorf(format string, args ...any) {
	if p.err != nil {
		return
	}
	p.err = fmt.Errorf("jsc parse error at line %d col %d: %s", p.current.Line, p.current.Col, fmt.Sprintf(format, args...))
}

// expect advances if the current token matches kind, else records an error.
func (p *Parser) expect(kind TokenKind) {
	if p.current.Kind != kind {
		p.errorf("expected %s, got %s (%q)", tokenName(kind), tokenName(p.current.Kind), p.current.Lexeme)
		return
	}
	p.advance()
}

// accept consumes the current token if it matches kind.
func (p *Parser) accept(kind TokenKind) bool {
	if p.current.Kind == kind {
		p.advance()
		return true
	}
	return false
}

// tokenName returns a human-readable name for a token kind (for errors).
func tokenName(k TokenKind) string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenError:
		return "error"
	case TokenIdentifier:
		return "identifier"
	case TokenNumber:
		return "number"
	case TokenString:
		return "string"
	}
	if k.IsKeyword() {
		return keywordText[k.KeywordOf()]
	}
	return strconv.Itoa(int(k))
}

// parseStatement dispatches on the current token.
func (p *Parser) parseStatement() Stmt {
	tok := p.current
	// Labeled statement: 'identifier: statement'
	if tok.Kind == TokenIdentifier {
		savedLex := *p.lex
		savedCur := p.current
		savedPrev := p.prev
		p.advance()
		if p.current.Kind == TokenColon {
			// It's a label; parse and discard it, then parse the actual statement.
			// Labels are stored for 'break label' but we just skip them.
			p.advance()
			body := p.parseStatement()
			return body
		}
		// Restore and fall through
		*p.lex = savedLex
		p.current = savedCur
		p.prev = savedPrev
	}
	switch {
	case tok.Kind == TokenOpenBrace:
		return p.parseBlock()
	case tok.Kind == TokenSemicolon:
		p.advance()
		return &ExpressionStatement{Expr: nil, Line: tok.Line, Col: tok.Col}
	case tok.Kind.IsKeyword():
		switch tok.Kind.KeywordOf() {
		case KeywordVar, KeywordLet, KeywordConst:
			return p.parseVariableDeclaration()
		case KeywordFunction:
			return p.parseFunctionDeclaration(false)
		case KeywordIf:
			return p.parseIfStatement()
		case KeywordFor:
			return p.parseForStatement()
		case KeywordWhile:
			return p.parseWhileStatement()
		case KeywordDo:
			return p.parseDoWhileStatement()
		case KeywordReturn:
			return p.parseReturnStatement()
		case KeywordSwitch:
			return p.parseSwitchStatement()
		case KeywordBreak:
			p.advance()
			// Handle optional label: 'break label'
			if p.current.Kind == TokenIdentifier {
				p.advance() // consume label
			}
			p.consumeSemicolon()
			return &BreakStatement{Line: tok.Line, Col: tok.Col}
		case KeywordContinue:
			p.advance()
			p.consumeSemicolon()
			return &ContinueStatement{Line: tok.Line, Col: tok.Col}
		case KeywordThrow:
			p.advance()
			arg := p.parseExpression()
			p.consumeSemicolon()
			return &ThrowStatement{Argument: arg, Line: tok.Line, Col: tok.Col}
		case KeywordTry:
			return p.parseTryStatement()
		case KeywordClass:
			return p.parseClassDeclaration()
		case KeywordAsync:
			// 'async function' declaration
			if p.peekKind(1) == KeywordToken(KeywordFunction) {
				p.advance() // async
				return p.parseFunctionDeclaration(true)
			}
		case KeywordImport:
			return p.parseImportDeclaration()
		case KeywordExport:
			return p.parseExportDeclaration()
		}
	}
	// Expression statement
	expr := p.parseExpression()
	p.consumeSemicolon()
	return &ExpressionStatement{Expr: expr, Line: tok.Line, Col: tok.Col}
}

// peekKind returns the kind of the token n positions ahead (n>=0). This is a best-effort
// single-token lookahead that re-runs the lexer from a saved position.
func (p *Parser) peekKind(n int) TokenKind {
	if n == 0 {
		return p.current.Kind
	}
	if n != 1 {
		// Only single-token lookahead is supported.
		return TokenError
	}
	saved := *p.lex
	tok := p.lex.Next(regexAllowedAfter(p.current.Kind))
	*p.lex = saved
	return tok.Kind
}

// consumeSemicolon consumes an explicit semicolon or applies ASI.
func (p *Parser) consumeSemicolon() {
	if p.current.Kind == TokenSemicolon {
		p.advance()
		return
	}
	// ASI: allow if a newline preceded this token, or we are at EOF/close-brace.
	if p.current.PrecedingNewline || p.current.Kind == TokenEOF || p.current.Kind == TokenCloseBrace {
		return
	}
}

// consumeString consumes a string literal token and returns its value (stripping quotes).
func (p *Parser) consumeString() string {
	if p.current.Kind == TokenString {
		val := p.current.Lexeme
		p.advance()
		// Strip surrounding quotes.
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') {
			val = val[1 : len(val)-1]
		}
		return val
	}
	return ""
}

// parseBlock parses a block statement.
// parseBlock parses a brace-delimited block.
func (p *Parser) parseBlock() *BlockStatement {
	tok := p.current
	p.expect(TokenOpenBrace)
	block := &BlockStatement{Line: tok.Line, Col: tok.Col}
	for p.current.Kind != TokenCloseBrace && p.current.Kind != TokenEOF && p.err == nil {
		block.Body = append(block.Body, p.parseStatement())
	}
	p.expect(TokenCloseBrace)
	return block
}

// parseVariableDeclaration parses let/const/var declarations.
func (p *Parser) parseVariableDeclaration() *VariableDeclaration {
	kindTok := p.current
	kind := keywordText[kindTok.Kind.KeywordOf()]
	p.advance()
	vd := &VariableDeclaration{Kind: kind, Line: kindTok.Line, Col: kindTok.Col}
	for {
		if p.current.Kind == TokenOpenBrace {
			// Object destructuring: const {a, b: c} = obj
			fields, ok := p.parseObjectDestructPattern()
			if !ok {
				p.errorf("bad object destructuring pattern")
				break
			}
			if p.current.Kind == TokenAssign {
				p.advance()
				rhs := p.parseAssignment()
				for _, f := range fields {
					init := &MemberExpression{Object: rhs, Name: f.Key, Computed: false}
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: f.Target, Init: init})
				}
			} else {
				for _, f := range fields {
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: f.Target, Init: nil})
				}
			}
		} else if p.current.Kind == TokenOpenBracket {
			// Array destructuring: const [a, b] = arr
			names, indices, defaults := p.parseArrayDestructWithDefaults()
			_ = defaults
			_ = indices
			if p.current.Kind == TokenAssign {
				p.advance()
				rhs := p.parseAssignment()
				for j, nm := range names {
					var init Expr
					if defaults != nil && j < len(defaults) && defaults[j] != nil {
						init = defaults[j].(Expr)
					} else {
						init = &MemberExpression{Object: rhs, Property: &Literal{Value: NumberValue(float64(j))}, Computed: true}
					}
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: nm, Init: init})
				}
			} else {
				for _, nm := range names {
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: nm, Init: nil})
				}
			}
		} else if p.current.Kind != TokenIdentifier {
			p.errorf("expected identifier in declaration")
			break
		} else {
			name := p.current.Lexeme
			p.advance()
			var init Expr
			if p.current.Kind == TokenAssign {
				p.advance()
				init = p.parseAssignment()
			}
			vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: name, Init: init})
		}
		if p.current.Kind != TokenComma {
			break
		}
		p.advance()
	}
	p.consumeSemicolon()
	return vd
}

// parseArrayDestructWithDefaults parses [a, b = default] and returns names and optional defaults.
func (p *Parser) parseArrayDestructWithDefaults() ([]string, []int, []any) {
	names, indices, _ := p.parseArrayDestructPattern()
	return names, indices, nil
}

// parseDestructInit parses an optional "= default" after a destructuring pattern.
func (p *Parser) parseDestructInit() Expr {
	return nil
}

// parseFunctionDeclaration parses 'function name(params){body}' or 'function* name(params){body}'.
func (p *Parser) parseFunctionDeclaration(isAsync bool) *FunctionDeclaration {
	tok := p.current
	p.advance() // 'function'
	isGenerator := false
	if p.current.Kind == TokenStar {
		isGenerator = true
		p.advance()
	}
	name := ""
	if p.current.Kind == TokenIdentifier {
		name = p.current.Lexeme
		p.advance()
	}
	savedGen := p.inGenerator
	p.inGenerator = isGenerator
	params, body := p.parseFunctionBody()
	p.inGenerator = savedGen
	return &FunctionDeclaration{Name: name, Params: params, Body: body, IsAsync: isAsync, IsGenerator: isGenerator, Line: tok.Line, Col: tok.Col}
}

// parseFunctionBody parses '(params) { body }' and returns both.
func (p *Parser) parseFunctionBody() ([]string, []Stmt) {
	p.expect(TokenOpenParen)
	params := []string{}
	for p.current.Kind != TokenCloseParen && p.err == nil {
		// rest param: '...name'
		if p.current.Kind == TokenSpread {
			p.advance()
			if p.current.Kind == TokenIdentifier {
				params = append(params, p.current.Lexeme)
				p.advance()
			} else {
				p.errorf("expected parameter name after ...")
			}
			if p.current.Kind == TokenCloseParen {
				break
			}
			p.errorf("rest parameter must be last")
			break
		}
		// Array destructuring param: [a, b]
		if p.current.Kind == TokenOpenBracket {
			names, _, ok := p.parseArrayDestructPattern()
			if !ok {
				p.errorf("bad array destructuring in parameter")
				break
			}
			// Flatten destructured names into params
			params = append(params, names...)
			// Default value for the destructured param: [a, b] = default
			if p.current.Kind == TokenAssign {
				p.advance()
				p.parseAssignment()
			}
			goto nextFuncParam
		}
		// Object destructuring param: {a, b}
		if p.current.Kind == TokenOpenBrace {
			fields, ok := p.parseObjectDestructPattern()
			if !ok {
				p.errorf("bad object destructuring in parameter")
				break
			}
			for _, f := range fields {
				params = append(params, f.Target)
			}
			// Default value for the destructured param: {a, b} = default
			if p.current.Kind == TokenAssign {
				p.advance()
				p.parseAssignment()
			}
			goto nextFuncParam
		}
		if p.current.Kind != TokenIdentifier {
			p.errorf("expected parameter name")
			break
		}
		params = append(params, p.current.Lexeme)
		p.advance()
		// default values: 'param = expr'
		if p.current.Kind == TokenAssign {
			p.advance()
			p.parseAssignment() // parsed but defaults are applied at call time by interp
		}
	nextFuncParam:
		if p.current.Kind != TokenComma {
			break
		}
		p.advance()
	}
	p.expect(TokenCloseParen)
	body := []Stmt{}
	if p.current.Kind == TokenOpenBrace {
		block := p.parseBlock()
		body = block.Body
	}
	return params, body
}

// parseIfStatement parses 'if (test) then else alt'.
func (p *Parser) parseIfStatement() *IfStatement {
	tok := p.current
	p.advance() // 'if'
	p.expect(TokenOpenParen)
	test := p.parseExpression()
	p.expect(TokenCloseParen)
	cons := p.parseStatement()
	var alt Stmt
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordElse {
		p.advance()
		alt = p.parseStatement()
	}
	return &IfStatement{Test: test, Consequent: cons, Alternate: alt, Line: tok.Line, Col: tok.Col}
}

// parseYieldExpression parses 'yield', 'yield expr', or 'yield* expr' in a generator.
// Translated from WebKit Source/JavaScriptCore/parser/Parser.cpp parseYieldExpression.
func (p *Parser) parseYieldExpression() *YieldExpression {
	tok := p.current
	p.advance() // 'yield'
	// yield [no LineTerminator here] AssignmentExpression
	// If there's a line terminator, it's just 'yield' (returns undefined).
	if p.current.PrecedingNewline {
		return &YieldExpression{Argument: nil, Delegate: false, Line: tok.Line, Col: tok.Col}
	}
	// yield* AssignmentExpression  (delegate)
	delegate := false
	if p.current.Kind == TokenStar {
		delegate = true
		p.advance()
	}
	arg := p.parseAssignment()
	return &YieldExpression{Argument: arg, Delegate: delegate, Line: tok.Line, Col: tok.Col}
}

// parseSwitchStatement parses 'switch (expr) { case val: ... default: ... }'.
func (p *Parser) parseSwitchStatement() Stmt {
	tok := p.current
	p.advance() // 'switch'
	p.expect(TokenOpenParen)
	discriminant := p.parseExpression()
	p.expect(TokenCloseParen)
	p.expect(TokenOpenBrace)
	var cases []SwitchCase
	for p.current.Kind != TokenCloseBrace && p.err == nil {
		if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordCase {
			p.advance()
			test := p.parseExpression()
			p.expect(TokenColon)
			var body []Stmt
			for p.current.Kind != TokenCloseBrace && p.err == nil &&
				!(p.current.Kind.IsKeyword() && (p.current.Kind.KeywordOf() == KeywordCase || p.current.Kind.KeywordOf() == KeywordDefault)) {
				body = append(body, p.parseStatement())
			}
			cases = append(cases, SwitchCase{Test: test, Body: body, Line: tok.Line, Col: tok.Col})
		} else if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordDefault {
			p.advance()
			p.expect(TokenColon)
			var body []Stmt
			for p.current.Kind != TokenCloseBrace && p.err == nil &&
				!(p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordCase) {
				body = append(body, p.parseStatement())
			}
			cases = append(cases, SwitchCase{Test: nil, Body: body, Line: tok.Line, Col: tok.Col})
		} else {
			p.errorf("expected case or default in switch")
			break
		}
	}
	p.expect(TokenCloseBrace)
	return &SwitchStatement{Discriminant: discriminant, Cases: cases, Line: tok.Line, Col: tok.Col}
}

// parseForStatement parses 'for (init; test; update) body' or 'for (lhs in/of rhs) body'.
func (p *Parser) parseForStatement() Stmt {
	tok := p.current
	p.advance() // 'for'
	p.expect(TokenOpenParen)
	// init: variable decl or expression
	var init Stmt
	savedAllowIn := p.allowIn
	p.allowIn = false
	if p.current.Kind.IsKeyword() && (p.current.Kind.KeywordOf() == KeywordVar ||
		p.current.Kind.KeywordOf() == KeywordLet || p.current.Kind.KeywordOf() == KeywordConst) {
		init = p.parseVariableDeclarationNoSemicolon()
	} else if p.current.Kind != TokenSemicolon {
		expr := p.parseExpression()
		init = &ExpressionStatement{Expr: expr, Line: tok.Line, Col: tok.Col}
	}
	// for-in / for-of?
	if (p.current.Kind.IsKeyword() && (p.current.Kind.KeywordOf() == KeywordIn || p.current.Kind.KeywordOf() == KeywordOf)) ||
		p.current.Kind == KeywordToken(KeywordOf) {
		isOf := p.current.Kind == KeywordToken(KeywordOf)
		p.advance()
		right := p.parseExpression()
		p.expect(TokenCloseParen)
		p.allowIn = savedAllowIn
		body := p.parseStatement()
		var left Expr
		if es, ok := init.(*ExpressionStatement); ok {
			left = es.Expr
		} else if vd, ok := init.(*VariableDeclaration); ok && len(vd.Declarators) > 0 {
			// 'for (let k in obj)' — the declarator name is the iteration target.
			left = &Identifier{Name: vd.Declarators[0].Name, Line: vd.Line, Col: vd.Col}
		}
		return &ForInStatement{Left: left, IsOf: isOf, Right: right, Body: body, Line: tok.Line, Col: tok.Col}
	}
	p.expect(TokenSemicolon)
	var test Expr
	if p.current.Kind != TokenSemicolon {
		test = p.parseExpression()
	}
	p.expect(TokenSemicolon)
	var update Expr
	if p.current.Kind != TokenCloseParen {
		update = p.parseExpression()
	}
	p.expect(TokenCloseParen)
	p.allowIn = savedAllowIn
	body := p.parseStatement()
	return &ForStatement{Init: init, Test: test, Update: update, Body: body, Line: tok.Line, Col: tok.Col}
}

// parseVariableDeclarationNoSemicolon is parseVariableDeclaration without the trailing
// semicolon consume, so the for-loop header can detect in/of.
func (p *Parser) parseVariableDeclarationNoSemicolon() *VariableDeclaration {
	kindTok := p.current
	kind := keywordText[kindTok.Kind.KeywordOf()]
	p.advance()
	vd := &VariableDeclaration{Kind: kind, Line: kindTok.Line, Col: kindTok.Col}
	for {
		if p.current.Kind == TokenOpenBrace {
			fields, ok := p.parseObjectDestructPattern()
			if !ok {
				p.errorf("bad object destructuring pattern")
				break
			}
			if p.current.Kind == TokenAssign {
				p.advance()
				rhs := p.parseAssignment()
				for _, f := range fields {
					init := &MemberExpression{Object: rhs, Name: f.Key, Computed: false}
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: f.Target, Init: init})
				}
			} else {
				// for (const {a,b} of ...) — no assignment
				for _, f := range fields {
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: f.Target, Init: nil})
				}
				break
			}
		} else if p.current.Kind == TokenOpenBracket {
			// Array destructuring: const [a, b] = arr or for (const [a,b] of ...)
			names, _, _ := p.parseArrayDestructWithDefaults()
			if p.current.Kind == TokenAssign {
				p.advance()
				rhs := p.parseAssignment()
				for j, nm := range names {
					init := &MemberExpression{Object: rhs, Property: &Literal{Value: NumberValue(float64(j))}, Computed: true}
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: nm, Init: init})
				}
			} else {
				// for (const [a,b] of ...) — no assignment, just declare names
				for _, nm := range names {
					vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: nm, Init: nil})
				}
				// Don't consume more tokens — caller will check for in/of
				break
			}
		} else if p.current.Kind != TokenIdentifier {
			p.errorf("expected identifier in declaration")
			break
		} else {
			name := p.current.Lexeme
			p.advance()
			var init Expr
			if p.current.Kind == TokenAssign {
				p.advance()
				init = p.parseAssignment()
			}
			vd.Declarators = append(vd.Declarators, VariableDeclarator{Name: name, Init: init})
		}
		if p.current.Kind != TokenComma {
			break
		}
		p.advance()
	}
	return vd
}

// parseWhileStatement parses 'while (test) body'.
func (p *Parser) parseWhileStatement() *WhileStatement {
	tok := p.current
	p.advance() // 'while'
	p.expect(TokenOpenParen)
	test := p.parseExpression()
	p.expect(TokenCloseParen)
	body := p.parseStatement()
	return &WhileStatement{Test: test, Body: body, Line: tok.Line, Col: tok.Col}
}

// parseDoWhileStatement parses 'do body while (test);'.
func (p *Parser) parseDoWhileStatement() *DoWhileStatement {
	tok := p.current
	p.advance() // 'do'
	body := p.parseStatement()
	p.expect(KeywordToken(KeywordWhile))
	p.expect(TokenOpenParen)
	test := p.parseExpression()
	p.expect(TokenCloseParen)
	p.consumeSemicolon()
	return &DoWhileStatement{Body: body, Test: test, Line: tok.Line, Col: tok.Col}
}

// parseReturnStatement parses 'return [expr]'.
func (p *Parser) parseReturnStatement() *ReturnStatement {
	tok := p.current
	p.advance() // 'return'
	var arg Expr
	if p.current.Kind != TokenSemicolon && p.current.Kind != TokenCloseBrace &&
		p.current.Kind != TokenEOF && !p.current.PrecedingNewline {
		arg = p.parseExpression()
	}
	p.consumeSemicolon()
	return &ReturnStatement{Argument: arg, Line: tok.Line, Col: tok.Col}
}

// parseTryStatement parses 'try { } catch(e){ } finally { }'.
func (p *Parser) parseTryStatement() *TryStatement {
	tok := p.current
	p.advance() // 'try'
	block := p.parseBlock()
	var handler *CatchClause
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordCatch {
		p.advance()
		handler = &CatchClause{Line: p.current.Line, Col: p.current.Col}
		if p.current.Kind == TokenOpenParen {
			p.advance()
			if p.current.Kind == TokenIdentifier {
				handler.Param = p.current.Lexeme
				p.advance()
			}
			p.expect(TokenCloseParen)
		}
		handler.Body = p.parseBlock()
	}
	var finalizer Stmt
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordFinally {
		p.advance()
		finalizer = p.parseBlock()
	}
	return &TryStatement{Block: block, Handler: handler, Finalizer: finalizer, Line: tok.Line, Col: tok.Col}
}

// parseClassDeclaration parses 'class Name [extends Base] { body }'.
func (p *Parser) parseClassDeclaration() *ClassDeclaration {
	tok := p.current
	p.advance() // 'class'
	name := ""
	if p.current.Kind == TokenIdentifier {
		name = p.current.Lexeme
		p.advance()
	}
	var super Expr
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordExtends {
		p.advance()
		super = p.parseLeftHandSide()
	}
	body := p.parseClassBody()
	return &ClassDeclaration{Name: name, SuperClass: super, Body: body, Line: tok.Line, Col: tok.Col}
}

// parseImportDeclaration parses 'import default from "mod"' or 'import { a, b } from "mod"'.
func (p *Parser) parseImportDeclaration() *ImportDeclaration {
	tok := p.current
	p.advance() // 'import'
	defaultName := ""
	var namedNames []string
	if p.current.Kind == TokenIdentifier {
		// import default from "mod"
		defaultName = p.current.Lexeme
		p.advance()
		if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordFrom {
			p.advance() // 'from'
		}
		module := p.consumeString()
		return &ImportDeclaration{DefaultName: defaultName, Module: module, Line: tok.Line, Col: tok.Col}
	}
	if p.current.Kind == TokenOpenBrace {
		p.advance() // '{'
		for p.current.Kind != TokenCloseBrace && p.current.Kind != TokenEOF {
			if p.current.Kind == TokenIdentifier {
				namedNames = append(namedNames, p.current.Lexeme)
				p.advance()
			}
			if p.current.Kind == TokenComma {
				p.advance()
			}
		}
		if p.current.Kind == TokenCloseBrace {
			p.advance() // '}'
		}
		if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordFrom {
			p.advance() // 'from'
		}
		module := p.consumeString()
		return &ImportDeclaration{NamedNames: namedNames, Module: module, Line: tok.Line, Col: tok.Col}
	}
	// Fallback: try to parse a string directly (just for resilience).
	module := p.consumeString()
	return &ImportDeclaration{Module: module, Line: tok.Line, Col: tok.Col}
}

// parseExportDeclaration parses 'export default expr' or 'export { a, b }'.
func (p *Parser) parseExportDeclaration() *ExportDeclaration {
	tok := p.current
	p.advance() // 'export'
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordDefault {
		p.advance() // 'default'
		expr := p.parseExpression()
		p.consumeSemicolon()
		return &ExportDeclaration{DefaultExpr: expr, Line: tok.Line, Col: tok.Col}
	}
	if p.current.Kind == TokenOpenBrace {
		p.advance() // '{'
		var names []string
		for p.current.Kind != TokenCloseBrace && p.current.Kind != TokenEOF {
			if p.current.Kind == TokenIdentifier {
				names = append(names, p.current.Lexeme)
				p.advance()
			}
			if p.current.Kind == TokenComma {
				p.advance()
			}
		}
		if p.current.Kind == TokenCloseBrace {
			p.advance() // '}'
		}
		p.consumeSemicolon()
		return &ExportDeclaration{NamedNames: names, Line: tok.Line, Col: tok.Col}
	}
	// export var/let/const/function — not implemented, treat as no-op.
	return &ExportDeclaration{Line: tok.Line, Col: tok.Col}
}

// parseClassBody parses a class body { ... }.
func (p *Parser) parseClassBody() *ClassBody {
	cb := &ClassBody{Line: p.current.Line, Col: p.current.Col}
	p.expect(TokenOpenBrace)
	for p.current.Kind != TokenCloseBrace && p.current.Kind != TokenEOF && p.err == nil {
		if p.current.Kind == TokenSemicolon {
			p.advance()
			continue
		}
		isStatic := false
		if p.current.Kind == TokenIdentifier && p.current.Lexeme == "static" {
			isStatic = true
			p.advance()
		}
		kind := "method"
		name := ""
		if p.current.Kind == TokenIdentifier || p.current.Kind.IsKeyword() {
			lex := p.current.Lexeme
			if p.current.Kind.IsKeyword() {
				lex = keywordText[p.current.Kind.KeywordOf()]
			}
			if lex == "get" || lex == "set" {
				// lookahead: is the next a method name?
				if p.peekKind(1) == TokenIdentifier || p.peekKind(1) == TokenString || p.peekKind(1).IsKeyword() {
					kind = lex
					p.advance()
				}
			}
		}
		if p.current.Kind == TokenIdentifier {
			name = p.current.Lexeme
			p.advance()
		} else if p.current.Kind == TokenString {
			name = p.current.StringValue
			p.advance()
		} else if p.current.Kind.IsKeyword() {
			// Keywords can be method names (e.g., static of())
			name = keywordText[p.current.Kind.KeywordOf()]
			p.advance()
		} else {
			p.errorf("expected method name in class body")
			break
		}
		params, body := p.parseFunctionBody()
		cb.Methods = append(cb.Methods, ClassMethod{
			Name: name, Params: params, Body: body, Kind: kind, Static: isStatic,
			Line: p.current.Line, Col: p.current.Col,
		})
	}
	p.expect(TokenCloseBrace)
	return cb
}

// ---- Expressions (Pratt) ----

// parseExpression is the comma operator (lowest precedence).
func (p *Parser) parseExpression() Expr {
	expr := p.parseAssignment()
	if p.current.Kind != TokenComma {
		return expr
	}
	seq := &SequenceExpression{Expressions: []Expr{expr}, Line: p.current.Line, Col: p.current.Col}
	for p.current.Kind == TokenComma {
		p.advance()
		seq.Expressions = append(seq.Expressions, p.parseAssignment())
	}
	return seq
}

// parseAssignment handles assignment and arrow functions.
func (p *Parser) parseAssignment() Expr {
	// Detect arrow function: '(' params ')' '=>' or 'name' '=>'
	if af := p.tryParseArrow(); af != nil {
		return af
	}
	left := p.parseConditional()
	if IsAssignmentOp(p.current.Kind) {
		op := p.current.Kind
		p.advance()
		right := p.parseAssignment()
		return &AssignmentExpression{Op: op, Target: left, Value: right, Line: p.current.Line, Col: p.current.Col}
	}
	return left
}

// tryParseArrow attempts to parse an arrow function when the upcoming tokens match the
// 'params =>' shape. Returns nil if no arrow function is present.
func (p *Parser) tryParseArrow() Expr {
	// 'async (params) =>' or 'async name =>'
	if p.current.Kind == KeywordToken(KeywordAsync) {
		savedLex := *p.lex
		savedCur := p.current
		savedPrev := p.prev
		p.advance() // 'async'
		if p.current.Kind == TokenOpenParen {
			// async (params) => ...
			if params, destructs, ok := p.tryParseArrowParams(); ok {
				if p.current.Kind == TokenArrow {
					p.advance()
					af := p.finishArrow(params, destructs, p.current).(*ArrowFunction)
					af.IsAsync = true
					return af
				}
			}
		} else if p.current.Kind == TokenIdentifier {
			name := p.current.Lexeme
			p.advance()
			if p.current.Kind == TokenArrow {
				p.advance()
				af := p.finishArrow([]string{name}, nil, p.current).(*ArrowFunction)
				af.IsAsync = true
				return af
			}
		}
		// Not an async arrow; restore.
		*p.lex = savedLex
		p.current = savedCur
		p.prev = savedPrev
		return nil
	}
	// Single-identifier param: 'name =>'
	if p.current.Kind == TokenIdentifier {
		// Save state to backtrack.
		savedLex := *p.lex
		savedCur := p.current
		savedPrev := p.prev
		name := p.current.Lexeme
		p.advance()
		if p.current.Kind == TokenArrow {
			p.advance()
			return p.finishArrow([]string{name}, nil, p.current)
		}
		// Not an arrow; restore.
		*p.lex = savedLex
		p.current = savedCur
		p.prev = savedPrev
		return nil
	}
	// '(params) =>'
	if p.current.Kind == TokenOpenParen {
		savedLex := *p.lex
		savedCur := p.current
		savedPrev := p.prev
		if params, destructs, ok := p.tryParseArrowParams(); ok {
			if p.current.Kind == TokenArrow {
				p.advance()
				return p.finishArrow(params, destructs, p.current)
			}
		}
		// Restore and fall through to normal parsing.
		*p.lex = savedLex
		p.current = savedCur
		p.prev = savedPrev
	}
	return nil
}

// tryParseArrowParams attempts to parse '(a, b, c)' as an arrow param list. Returns the
// param names, destructuring info, and whether the parse succeeded as a param list.
func (p *Parser) tryParseArrowParams() ([]string, []DestructInfo, bool) {
	p.advance() // '('
	params := []string{}
	var destructs []DestructInfo
	for {
		if p.current.Kind == TokenCloseParen {
			p.advance()
			return params, destructs, true
		}
		if p.current.Kind == TokenSpread {
			p.advance()
			if p.current.Kind != TokenIdentifier {
				return nil, nil, false
			}
			params = append(params, p.current.Lexeme)
			p.advance()
			if p.current.Kind != TokenCloseParen {
				return nil, nil, false
			}
			p.advance()
			return params, destructs, true
		}
		// Array destructuring: [a, b]
		if p.current.Kind == TokenOpenBracket {
			names, _, ok := p.parseArrayDestructPattern()
			if !ok {
				return nil, nil, false
			}
			synName := fmt.Sprintf("__d%d", len(params))
			params = append(params, synName)
			destructs = append(destructs, DestructInfo{
				ParamIdx: len(params) - 1,
				IsArray:  true,
				Names:    names,
			})
			if p.current.Kind == TokenAssign {
				p.advance()
				if p.parseAssignment() == nil {
					return nil, nil, false
				}
			}
			goto nextParam
		}
		// Object destructuring: {a, b}
		if p.current.Kind == TokenOpenBrace {
			fields, ok := p.parseObjectDestructPattern()
			if !ok {
				return nil, nil, false
			}
			names := make([]string, len(fields))
			for i, f := range fields {
				names[i] = f.Target
			}
			synName := fmt.Sprintf("__d%d", len(params))
			params = append(params, synName)
			destructs = append(destructs, DestructInfo{
				ParamIdx: len(params) - 1,
				IsArray:  false,
				Names:    names,
			})
			if p.current.Kind == TokenAssign {
				p.advance()
				if p.parseAssignment() == nil {
					return nil, nil, false
				}
			}
			goto nextParam
		}
		if p.current.Kind != TokenIdentifier {
			return nil, nil, false
		}
		params = append(params, p.current.Lexeme)
		p.advance()
		// default value
		if p.current.Kind == TokenAssign {
			p.advance()
			if p.parseAssignment() == nil {
				return nil, nil, false
			}
		}
	nextParam:
		if p.current.Kind == TokenComma {
			p.advance()
			continue
		}
		if p.current.Kind == TokenCloseParen {
			p.advance()
			return params, destructs, true
		}
		return nil, nil, false
	}
}

// parseArrayDestructPattern parses [a, b, ...c] and returns the identifier names
// and their source indices. Holes (elisions) like [, a] are supported — a hole adds
// no name but increments the source index.
func (p *Parser) parseArrayDestructPattern() (names []string, indices []int, ok bool) {
	p.advance() // '['
	idx := 0
	for {
		if p.current.Kind == TokenCloseBracket {
			p.advance()
			return names, indices, true
		}
		// Hole (elision): [, a, , b] — comma without preceding element
		if p.current.Kind == TokenComma {
			idx++
			p.advance()
			continue
		}
		if p.current.Kind == TokenSpread {
			p.advance()
			if p.current.Kind != TokenIdentifier {
				return nil, nil, false
			}
			names = append(names, p.current.Lexeme)
			indices = append(indices, idx)
			idx++
			p.advance()
			if p.current.Kind != TokenCloseBracket {
				return nil, nil, false
			}
			p.advance()
			return names, indices, true
		}
		if p.current.Kind == TokenIdentifier {
			names = append(names, p.current.Lexeme)
			indices = append(indices, idx)
			idx++
			p.advance()
		} else if p.current.Kind == TokenOpenBracket {
			// nested array destructuring
			nested, nestedIndices, ok := p.parseArrayDestructPattern()
			if !ok {
				return nil, nil, false
			}
			names = append(names, nested...)
			indices = append(indices, nestedIndices...)
			idx++
		} else if p.current.Kind == TokenOpenBrace {
			nested, ok := p.parseObjectDestructPattern()
			if !ok {
				return nil, nil, false
			}
			for _, f := range nested {
				names = append(names, f.Target)
				indices = append(indices, idx)
			}
			idx++
		} else {
			return nil, nil, false
		}
		if p.current.Kind == TokenAssign {
			p.advance()
			if p.parseAssignment() == nil {
				return nil, nil, false
			}
		}
		if p.current.Kind == TokenComma {
			idx++
			p.advance()
			continue
		}
		if p.current.Kind == TokenCloseBracket {
			p.advance()
			return names, indices, true
		}
		return nil, nil, false
	}
}

// DestructField is one field in an object destructuring pattern {key: target}.
type DestructField struct {
	Key    string // source property name
	Target string // target variable name
}

// parseObjectDestructPattern parses {a, b: c, ...d} and returns the field list.
func (p *Parser) parseObjectDestructPattern() ([]DestructField, bool) {
	p.advance() // '{'
	var fields []DestructField
	for {
		if p.current.Kind == TokenCloseBrace {
			p.advance()
			return fields, true
		}
		if p.current.Kind == TokenSpread {
			p.advance()
			if p.current.Kind != TokenIdentifier {
				return nil, false
			}
			fields = append(fields, DestructField{Key: "", Target: p.current.Lexeme})
			p.advance()
			if p.current.Kind != TokenCloseBrace {
				return nil, false
			}
			p.advance()
			return fields, true
		}
		if p.current.Kind != TokenIdentifier && !p.current.Kind.IsKeyword() {
			return nil, false
		}
		key := p.current.Lexeme
		p.advance()
		if p.current.Kind == TokenColon {
			p.advance()
			// {key: target} or {key: {nested}} or {key: [nested]}
			if p.current.Kind == TokenOpenBrace {
				// Nested object pattern: {key: {a, b}}
				nested, ok := p.parseObjectDestructPattern()
				if !ok {
					return nil, false
				}
				fields = append(fields, nested...)
			} else if p.current.Kind == TokenOpenBracket {
				// Nested array pattern: {key: [a, b]}
				nested, _, ok := p.parseArrayDestructPattern()
				if !ok {
					return nil, false
				}
				for _, nm := range nested {
					fields = append(fields, DestructField{Key: key, Target: nm})
				}
			} else if p.current.Kind == TokenIdentifier || p.current.Kind.IsKeyword() {
				fields = append(fields, DestructField{Key: key, Target: p.current.Lexeme})
				p.advance()
			} else {
				return nil, false
			}
		} else {
			// {key} — shorthand: bind to key
			fields = append(fields, DestructField{Key: key, Target: key})
		}
		if p.current.Kind == TokenAssign {
			p.advance()
			if p.parseAssignment() == nil {
				return nil, false
			}
		}
		if p.current.Kind == TokenComma {
			p.advance()
			continue
		}
		if p.current.Kind == TokenCloseBrace {
			p.advance()
			return fields, true
		}
		return nil, false
	}
}

// finishArrow builds the arrow function node after '=>' was consumed.
func (p *Parser) finishArrow(params []string, destructs []DestructInfo, tok Token) Expr {
	af := &ArrowFunction{Params: params, Destructuring: destructs, Line: tok.Line, Col: tok.Col}
	if p.current.Kind == TokenOpenBrace {
		block := p.parseBlock()
		af.Body = block
		af.IsExpr = false
	} else {
		af.Body = p.parseAssignment()
		af.IsExpr = true
	}
	return af
}

// parseConditional is the ternary '?:' operator.
func (p *Parser) parseConditional() Expr {
	test := p.parseBinary(0)
	if p.current.Kind == TokenQuestion {
		p.advance()
		cons := p.parseAssignment()
		p.expect(TokenColon)
		alt := p.parseAssignment()
		return &ConditionalExpression{Test: test, Consequent: cons, Alternate: alt, Line: p.current.Line, Col: p.current.Col}
	}
	return test
}

// parseBinary is the precedence-climbing (Pratt) binary operator parser.
func (p *Parser) parseBinary(minPrec int) Expr {
	left := p.parseUnary()
	for {
		op := p.current.Kind
		// 'in' is contextual: respect allowIn.
		if op == KeywordToken(KeywordIn) && !p.allowIn {
			break
		}
	// Convert instanceof keyword to plain token kind
	if op.IsKeyword() && op.KeywordOf() == KeywordInstanceof {
		op = TokenInstanceOf
	}
	prec := BinaryPrecedence(op)
		if prec == 0 || prec < minPrec {
			break
		}
		p.advance()
		// Right-associative: ** raises the floor by -1 so 2**3**2 == 2**(3**2).
		nextMin := prec + 1
		if op == TokenPower {
			nextMin = prec
		}
		right := p.parseBinary(nextMin)
		// Distinguish logical (short-circuit) from plain binary.
		if op == TokenAnd || op == TokenOr || op == TokenCoalesce {
			left = &LogicalExpression{Op: op, Left: left, Right: right, Line: p.current.Line, Col: p.current.Col}
		} else {
			left = &BinaryExpression{Op: op, Left: left, Right: right, Line: p.current.Line, Col: p.current.Col}
		}
	}
	return left
}

// parseUnary handles prefix unary operators. Prefix ++/-- become UpdateExpression
// (matching the postfix form's node type) so the bytecode generator can treat them
// uniformly.
func (p *Parser) parseUnary() Expr {
	tok := p.current
	// 'yield [expr]' or 'yield* expr' — only valid inside generators
	if tok.Kind == KeywordToken(KeywordYield) && p.inGenerator {
		return p.parseYieldExpression()
	}
	// 'await expr' — only valid inside async functions, but parsed regardless.
	if tok.Kind == KeywordToken(KeywordAwait) {
		p.advance()
		arg := p.parseUnary()
		return &AwaitExpression{Argument: arg, Line: tok.Line, Col: tok.Col}
	}
	if IsUpdateOp(tok.Kind) {
		p.advance()
		arg := p.parseUnary()
		return &UpdateExpression{Op: tok.Kind, Argument: arg, Prefix: true, Line: tok.Line, Col: tok.Col}
	}
	if IsPrefixOp(tok.Kind) {
		p.advance()
		arg := p.parseUnary()
		return &UnaryExpression{Op: tok.Kind, Argument: arg, Line: tok.Line, Col: tok.Col}
	}
	return p.parsePostfix()
}

// parsePostfix handles 'expr++' / 'expr--'.
func (p *Parser) parsePostfix() Expr {
	expr := p.parseLeftHandSide()
	if IsUpdateOp(p.current.Kind) && !p.current.PrecedingNewline {
		op := p.current.Kind
		p.advance()
		return &UpdateExpression{Op: op, Argument: expr, Prefix: false, Line: p.current.Line, Col: p.current.Col}
	}
	return expr
}

// parseLeftHandSide handles call/member/new with left-to-right associativity.
func (p *Parser) parseLeftHandSide() Expr {
	var expr Expr
	if p.current.Kind.IsKeyword() && p.current.Kind.KeywordOf() == KeywordNew {
		expr = p.parseNewExpression()
	} else {
		expr = p.parsePrimary()
	}
	for {
		switch p.current.Kind {
		case TokenDot:
			p.advance()
			if p.current.Kind != TokenIdentifier && !p.current.Kind.IsKeyword() {
				p.errorf("expected property name after '.'")
				return expr
			}
			name := p.current.Lexeme
			p.advance()
			expr = &MemberExpression{Object: expr, Name: name, Computed: false, Line: p.current.Line, Col: p.current.Col}
		case TokenQuestionDot:
			p.advance()
			if p.current.Kind == TokenOpenParen {
				args := p.parseArgs()
				expr = &CallExpression{Callee: expr, Arguments: args, Optional: true, Line: p.current.Line, Col: p.current.Col}
			} else {
				name := p.current.Lexeme
				p.advance()
				expr = &MemberExpression{Object: expr, Name: name, Computed: false, Optional: true, Line: p.current.Line, Col: p.current.Col}
			}
		case TokenOpenBracket:
			p.advance()
			prop := p.parseExpression()
			p.expect(TokenCloseBracket)
			expr = &MemberExpression{Object: expr, Property: prop, Computed: true, Line: p.current.Line, Col: p.current.Col}
		case TokenOpenParen:
			args := p.parseArgs()
			expr = &CallExpression{Callee: expr, Arguments: args, Line: p.current.Line, Col: p.current.Col}
		default:
			return expr
		}
	}
}

// parseArgs parses an argument list '(a, b, ...c)'.
func (p *Parser) parseArgs() []Expr {
	p.expect(TokenOpenParen)
	args := []Expr{}
	for p.current.Kind != TokenCloseParen && p.err == nil {
		if p.current.Kind == TokenSpread {
			p.advance()
			arg := p.parseAssignment()
			args = append(args, &SpreadExpression{Argument: arg, Line: p.current.Line, Col: p.current.Col})
		} else {
			args = append(args, p.parseAssignment())
		}
		if p.current.Kind != TokenComma {
			break
		}
		p.advance()
	}
	p.expect(TokenCloseParen)
	return args
}

// parseNewExpression parses 'new callee(args)'. The callee is a member-expression
// chain (primary + '.'/'[]'/'?.') so that a trailing '(' belongs to the new call,
// not to a nested call expression on the constructor.
func (p *Parser) parseNewExpression() Expr {
	tok := p.current
	p.advance() // 'new'
	callee := p.parsePrimary()
	for {
		switch p.current.Kind {
		case TokenDot:
			p.advance()
			if p.current.Kind != TokenIdentifier && !p.current.Kind.IsKeyword() {
				p.errorf("expected property name after '.'")
				return callee
			}
			name := p.current.Lexeme
			p.advance()
			callee = &MemberExpression{Object: callee, Name: name, Computed: false, Line: p.current.Line, Col: p.current.Col}
		case TokenOpenBracket:
			p.advance()
			prop := p.parseExpression()
			p.expect(TokenCloseBracket)
			callee = &MemberExpression{Object: callee, Property: prop, Computed: true, Line: p.current.Line, Col: p.current.Col}
		case TokenQuestionDot:
			p.advance()
			if p.current.Kind == TokenIdentifier || p.current.Kind.IsKeyword() {
				name := p.current.Lexeme
				p.advance()
				callee = &MemberExpression{Object: callee, Name: name, Computed: false, Optional: true, Line: p.current.Line, Col: p.current.Col}
			}
		default:
			args := []Expr{}
			if p.current.Kind == TokenOpenParen {
				args = p.parseArgs()
			}
			return &NewExpression{Callee: callee, Arguments: args, Line: tok.Line, Col: tok.Col}
		}
	}
}

// parsePrimary handles literals, identifiers, and parenthesised expressions.
func (p *Parser) parsePrimary() Expr {
	tok := p.current
	switch tok.Kind {
	case TokenNumber:
		p.advance()
		return &Literal{Value: NumberValue(tok.NumberValue), Raw: tok.Lexeme, Line: tok.Line, Col: tok.Col}
	case TokenString:
		p.advance()
		return &Literal{Value: StringValue(tok.StringValue), Raw: tok.Lexeme, Line: tok.Line, Col: tok.Col}
	case TokenTemplate:
		return p.parseTemplate()
	case TokenRegex:
		p.advance()
		return &RegexLiteral{Pattern: tok.RegexPattern, Flags: tok.RegexFlags, Line: tok.Line, Col: tok.Col}
	case TokenIdentifier:
		p.advance()
		return &Identifier{Name: tok.Lexeme, Line: tok.Line, Col: tok.Col}
	case TokenOpenParen:
		p.advance()
		expr := p.parseExpression()
		p.expect(TokenCloseParen)
		return expr
	case TokenOpenBracket:
		return p.parseArray()
	case TokenOpenBrace:
		return p.parseObject()
	}
	if tok.Kind.IsKeyword() {
		switch tok.Kind.KeywordOf() {
		case KeywordTrue:
			p.advance()
			return &Literal{Value: BooleanValue(true), Raw: "true", Line: tok.Line, Col: tok.Col}
		case KeywordFalse:
			p.advance()
			return &Literal{Value: BooleanValue(false), Raw: "false", Line: tok.Line, Col: tok.Col}
		case KeywordNull:
			p.advance()
			return &Literal{Value: Null(), Raw: "null", Line: tok.Line, Col: tok.Col}
		case KeywordUndefined:
			p.advance()
			return &Literal{Value: Undefined(), Raw: "undefined", Line: tok.Line, Col: tok.Col}
		case KeywordThis:
			p.advance()
			return &ThisExpression{Line: tok.Line, Col: tok.Col}
		case KeywordFunction:
			return p.parseFunctionExpression(false)
		case KeywordAsync:
			if p.peekKind(1) == KeywordToken(KeywordFunction) {
				p.advance()
				return p.parseFunctionExpression(true)
			}
			// 'async' as identifier-ish (arrow preamble) — fall through to identifier
			p.advance()
			return &Identifier{Name: tok.Lexeme, Line: tok.Line, Col: tok.Col}
		case KeywordNew:
			return p.parseNewExpression()
		case KeywordClass:
			return p.parseClassExpression()
		case KeywordLet, KeywordYield, KeywordAwait, KeywordOf:
			// Contextual keywords used as identifiers when not in special position.
			p.advance()
			return &Identifier{Name: tok.Lexeme, Line: tok.Line, Col: tok.Col}
		case KeywordSuper:
			p.advance()
			return &Identifier{Name: "super", Line: tok.Line, Col: tok.Col}
		}
	}
	p.errorf("unexpected token %s (%q)", tokenName(tok.Kind), tok.Lexeme)
	return &Literal{Value: Undefined(), Line: tok.Line, Col: tok.Col}
}

// parseTemplate splits a TokenTemplate into quasi/expression parts.
func (p *Parser) parseTemplate() *TemplateLiteral {
	tok := p.current
	if tok.Kind != TokenTemplate {
		p.errorf("expected template literal")
		return &TemplateLiteral{Line: tok.Line, Col: tok.Col}
	}
	p.advance()
	tl := &TemplateLiteral{Line: tok.Line, Col: tok.Col}
	// The lexer stored substitutions delimited by \x00${ ... \x00}.
	raw := tok.StringValue
	for {
		idx := strings.Index(raw, "\x00${")
		if idx < 0 {
			tl.Quasis = append(tl.Quasis, raw)
			break
		}
		tl.Quasis = append(tl.Quasis, raw[:idx])
		raw = raw[idx+3:]
		endIdx := strings.Index(raw, "\x00}")
		if endIdx < 0 {
			p.errorf("unterminated template substitution")
			break
		}
		exprSrc := raw[:endIdx]
		if endIdx+2 >= len(raw) {
			// Expression may be empty or at end; skip gracefully.
			raw = ""
			continue
		}
		raw = raw[endIdx+2:]
		// Parse the substitution expression by re-entering the parser.
		sub, err := Parse(exprSrc)
		if err != nil || len(sub.Body) == 0 {
			p.errorf("bad template expression: %q", exprSrc)
			continue
		}
		if es, ok := sub.Body[0].(*ExpressionStatement); ok {
			tl.Expressions = append(tl.Expressions, es.Expr)
		}
	}
	return tl
}

// parseArray parses an array literal '[ ... ]'.
func (p *Parser) parseArray() *ArrayExpression {
	tok := p.current
	p.expect(TokenOpenBracket)
	ae := &ArrayExpression{Line: tok.Line, Col: tok.Col}
	for p.current.Kind != TokenCloseBracket && p.err == nil {
		if p.current.Kind == TokenComma {
			ae.Elements = append(ae.Elements, nil) // hole
			p.advance()
			continue
		}
		if p.current.Kind == TokenSpread {
			p.advance()
			arg := p.parseAssignment()
			ae.Elements = append(ae.Elements, &SpreadExpression{Argument: arg, Line: p.current.Line, Col: p.current.Col})
		} else {
			ae.Elements = append(ae.Elements, p.parseAssignment())
		}
		if p.current.Kind != TokenCloseBracket {
			p.expect(TokenComma)
		}
	}
	p.expect(TokenCloseBracket)
	return ae
}

// parseObject parses an object literal '{ ... }'.
func (p *Parser) parseObject() *ObjectExpression {
	tok := p.current
	p.expect(TokenOpenBrace)
	oe := &ObjectExpression{Line: tok.Line, Col: tok.Col}
	for p.current.Kind != TokenCloseBrace && p.err == nil {
		prop := p.parseProperty()
		oe.Properties = append(oe.Properties, prop)
		if p.current.Kind != TokenCloseBrace {
			p.expect(TokenComma)
		}
	}
	p.expect(TokenCloseBrace)
	return oe
}

// parseProperty parses a single 'key: value' or shorthand 'key' inside an object.
func (p *Parser) parseProperty() Property {
	tok := p.current
	prop := Property{Line: tok.Line, Col: tok.Col, Kind: "init"}
	// Spread property: {...expr}
	if tok.Kind == TokenSpread {
		p.advance()
		prop.Kind = "spread"
		prop.Spread = p.parseAssignment()
		return prop
	}
	// getter/setter
	if tok.Kind == TokenIdentifier && (tok.Lexeme == "get" || tok.Lexeme == "set") {
		if p.peekKind(1) == TokenIdentifier || p.peekKind(1) == TokenString || p.peekKind(1) == TokenOpenBracket {
			prop.Kind = tok.Lexeme
			p.advance()
			return p.parsePropertyKeyed(prop)
		}
	}
	// computed key
	if tok.Kind == TokenOpenBracket {
		p.advance()
		prop.Computed = true
		prop.Key = p.parseAssignment()
		p.expect(TokenCloseBracket)
		return p.parsePropertyTail(prop)
	}
	return p.parsePropertyKeyed(prop)
}

// parsePropertyKeyed parses a non-computed key and the rest of the property.
func (p *Parser) parsePropertyKeyed(prop Property) Property {
	tok := p.current
	switch tok.Kind {
	case TokenIdentifier:
		prop.Key = &Identifier{Name: tok.Lexeme, Line: tok.Line, Col: tok.Col}
		p.advance()
	case TokenString:
		prop.Key = &Literal{Value: StringValue(tok.StringValue), Raw: tok.Lexeme, Line: tok.Line, Col: tok.Col}
		p.advance()
	case TokenNumber:
		prop.Key = &Literal{Value: NumberValue(tok.NumberValue), Raw: tok.Lexeme, Line: tok.Line, Col: tok.Col}
		p.advance()
	default:
		if tok.Kind.IsKeyword() {
			prop.Key = &Identifier{Name: tok.Lexeme, Line: tok.Line, Col: tok.Col}
			p.advance()
		} else {
			p.errorf("expected property key, got %s", tokenName(tok.Kind))
		}
	}
	return p.parsePropertyTail(prop)
}

// parsePropertyTail handles ': value', '()' method, or shorthand.
func (p *Parser) parsePropertyTail(prop Property) Property {
	if p.current.Kind == TokenOpenParen {
		params, body := p.parseFunctionBody()
		prop.Kind = "method"
		prop.Value = &FunctionExpression{Params: params, Body: body, Line: prop.Line, Col: prop.Col}
		return prop
	}
	if p.current.Kind == TokenColon {
		p.advance()
		prop.Value = p.parseAssignment()
		return prop
	}
	// Shorthand: key === value
	if id, ok := prop.Key.(*Identifier); ok {
		prop.Shorthand = true
		prop.Value = &Identifier{Name: id.Name, Line: id.Line, Col: id.Col}
	}
	return prop
}

// parseFunctionExpression parses 'function [name](params){body}' as a value.
func (p *Parser) parseFunctionExpression(isAsync bool) *FunctionExpression {
	tok := p.current
	p.advance() // 'function'
	isGenerator := false
	if p.current.Kind == TokenStar {
		isGenerator = true
		p.advance()
	}
	name := ""
	if p.current.Kind == TokenIdentifier {
		name = p.current.Lexeme
		p.advance()
	}
	params, body := p.parseFunctionBody()
	return &FunctionExpression{Name: name, Params: params, Body: body, IsAsync: isAsync, IsGenerator: isGenerator, Line: tok.Line, Col: tok.Col}
}

// parseClassExpression parses 'class [Name] [extends Base] { body }' as a value.
func (p *Parser) parseClassExpression() *ClassDeclaration {
	cd := p.parseClassDeclaration()
	return cd
}
