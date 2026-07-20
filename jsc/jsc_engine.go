// jsc 包 — JavaScript 引擎（简易 AST 解释器）
package jsc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"wb-ui/jsc/parser"
)

// ===== AST Node =====
type Node interface{ NodeType() string }

type Program struct{ Body []Node }
func (p *Program) NodeType() string { return "Program" }
type ExprStmt struct{ Expr Node }
func (e *ExprStmt) NodeType() string { return "ExprStatement" }
type VarDecl struct{ Name string; Init Node }
type VarStmt struct{ Declarations []VarDecl }
func (v *VarStmt) NodeType() string { return "VarStatement" }
type AssignExpr struct{ Name string; Rhs Node }
func (a *AssignExpr) NodeType() string { return "AssignExpr" }
type IfStmt struct{ Test, Consequent, Alternate Node }
func (i *IfStmt) NodeType() string { return "IfStatement" }
type WhileStmt struct{ Test, Body Node }
func (w *WhileStmt) NodeType() string { return "WhileStatement" }
type ForStmt struct{ Init, Test, Upd, Body Node }
func (f *ForStmt) NodeType() string { return "ForStatement" }
type ReturnStmt struct{ Arg Node }
func (r *ReturnStmt) NodeType() string { return "ReturnStatement" }
type BlockStmt struct{ Body []Node }
func (b *BlockStmt) NodeType() string { return "BlockStatement" }
type FuncDecl struct{ Name string; Params []string; Body *BlockStmt }
func (f *FuncDecl) NodeType() string { return "FunctionDeclaration" }
type NumberLit struct{ Value float64 }
func (n *NumberLit) NodeType() string { return "NumberLiteral" }
type StringLit struct{ Value string }
func (s *StringLit) NodeType() string { return "StringLiteral" }
type BoolLit struct{ Value bool }
func (b *BoolLit) NodeType() string { return "BooleanLiteral" }
type NullLit struct{}
func (n *NullLit) NodeType() string { return "NullLiteral" }
type IdentExpr struct{ Name string }
func (i *IdentExpr) NodeType() string { return "Identifier" }
type BinaryExpr struct{ Op string; Left, Right Node }
func (b *BinaryExpr) NodeType() string { return "BinaryExpression" }
type UnaryExpr struct{ Op string; Arg Node; Prefix bool }
func (u *UnaryExpr) NodeType() string { return "UnaryExpression" }
type CallExpr struct{ Callee Node; Arguments []Node }
func (c *CallExpr) NodeType() string { return "CallExpression" }
type CondExpr struct{ Test, Consequent, Alternate Node }
func (c *CondExpr) NodeType() string { return "ConditionalExpression" }
type SequenceExpr struct{ Expressions []Node }
func (s *SequenceExpr) NodeType() string { return "SequenceExpression" }

// ===== Parser =====
type Parser struct {
	lex    *parser.Lexer
	tok    parser.JSToken
	tt     parser.JSTokenType
	strict bool
}

func NewParser(lex *parser.Lexer) *Parser {
	p := &Parser{lex: lex}
	p.next()
	return p
}
func (p *Parser) next() {
	p.tok = parser.JSToken{}
	p.tt = p.lex.Lex(&p.tok, 0, p.strict)
	p.tok.Type = p.tt
}
func (p *Parser) eat(tt parser.JSTokenType) bool {
	if p.tt == tt { p.next(); return true }
	return false
}
func (p *Parser) expect(tt parser.JSTokenType) {
	if !p.eat(tt) { panic(fmt.Sprintf("期望记号 %d，实际 %d", tt, p.tt)) }
}
func (p *Parser) tokText() string {
	return p.lex.GetToken(p.tok)
}

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	for p.tt != parser.EOFTOK && p.tt != parser.ERRORTOK {
		stmt := p.parseStmt()
		if stmt != nil { prog.Body = append(prog.Body, stmt) }
		if p.tt == parser.SEMICOLON { p.next() }
	}
	return prog
}

func (p *Parser) parseStmt() Node {
	switch p.tt {
	case parser.FUNCTION: return p.parseFuncDecl()
	case parser.VAR, parser.LET, parser.CONSTTOKEN: return p.parseVarStmt()
	case parser.RETURN: return p.parseReturnStmt()
	case parser.IF: return p.parseIfStmt()
	case parser.WHILE: return p.parseWhileStmt()
	case parser.FOR: return p.parseForStmt()
	case parser.DO: return p.parseDoWhileStmt()
	case parser.BREAK: p.next(); return nil
	case parser.CONTINUE: p.next(); return nil
	case parser.OPENBRACE: return p.parseBlockStmt()
	case parser.SEMICOLON: p.next(); return nil
	default: return p.parseExprStmt()
	}
}

func (p *Parser) parseBlockStmt() *BlockStmt {
	p.expect(parser.OPENBRACE)
	block := &BlockStmt{}
	for p.tt != parser.CLOSEBRACE && p.tt != parser.EOFTOK {
		stmt := p.parseStmt()
		if stmt != nil { block.Body = append(block.Body, stmt) }
		if p.tt == parser.SEMICOLON { p.next() }
	}
	p.expect(parser.CLOSEBRACE)
	return block
}

func (p *Parser) parseFuncDecl() *FuncDecl {
	p.next()
	name := ""
	if p.tt == parser.IDENT { name = p.tokText(); p.next() }
	p.expect(parser.OPENPAREN)
	var params []string
	for p.tt != parser.CLOSEPAREN && p.tt != parser.EOFTOK {
		if p.tt == parser.IDENT { params = append(params, p.tokText()); p.next() }
		p.eat(parser.COMMA)
	}
	p.expect(parser.CLOSEPAREN)
	body := p.parseBlockStmt()
	return &FuncDecl{Name: name, Params: params, Body: body}
}

func (p *Parser) parseVarStmt() *VarStmt {
	p.next()
	stmt := &VarStmt{}
	for {
		name := p.tokText(); p.next()
		decl := VarDecl{Name: name}
		if p.tt == parser.EQUAL { p.next(); decl.Init = p.parseExpr(0) }
		stmt.Declarations = append(stmt.Declarations, decl)
		if p.tt != parser.COMMA { break }
		p.next()
	}
	return stmt
}

func (p *Parser) parseReturnStmt() *ReturnStmt {
	p.next()
	stmt := &ReturnStmt{}
	if p.tt != parser.SEMICOLON && p.tt != parser.CLOSEBRACE && p.tt != parser.EOFTOK {
		stmt.Arg = p.parseExpr(0)
	}
	return stmt
}

func (p *Parser) parseIfStmt() *IfStmt {
	p.next()
	p.expect(parser.OPENPAREN)
	test := p.parseExpr(0)
	p.expect(parser.CLOSEPAREN)
	cons := p.parseStmt()
	alt := Node(nil)
	if p.tt == parser.ELSE { p.next(); alt = p.parseStmt() }
	return &IfStmt{Test: test, Consequent: cons, Alternate: alt}
}

func (p *Parser) parseWhileStmt() *WhileStmt {
	p.next()
	p.expect(parser.OPENPAREN)
	test := p.parseExpr(0)
	p.expect(parser.CLOSEPAREN)
	body := p.parseStmt()
	return &WhileStmt{Test: test, Body: body}
}

func (p *Parser) parseDoWhileStmt() *WhileStmt {
	p.next()
	body := p.parseStmt()
	p.expect(parser.WHILE)
	p.expect(parser.OPENPAREN)
	test := p.parseExpr(0)
	p.expect(parser.CLOSEPAREN)
	return &WhileStmt{Test: test, Body: body}
}

func (p *Parser) parseForStmt() *ForStmt {
	p.next()
	p.expect(parser.OPENPAREN)
	var init, test, upd Node
	if p.tt == parser.VAR || p.tt == parser.LET || p.tt == parser.CONSTTOKEN {
		init = p.parseVarStmt()
	} else if p.tt != parser.SEMICOLON {
		init = p.parseExprStmt()
	}
	p.expect(parser.SEMICOLON)
	if p.tt != parser.SEMICOLON { test = p.parseExpr(0) }
	p.expect(parser.SEMICOLON)
	if p.tt != parser.CLOSEPAREN { upd = p.parseExpr(0) }
	p.expect(parser.CLOSEPAREN)
	body := p.parseStmt()
	return &ForStmt{Init: init, Test: test, Upd: upd, Body: body}
}

func (p *Parser) parseExprStmt() *ExprStmt {
	expr := p.parseExpr(0)
	p.eat(parser.SEMICOLON)
	return &ExprStmt{Expr: expr}
}

// ===== Pratt 表达式解析 =====
var precMap = map[parser.JSTokenType]int{
	parser.COALESCE: 1, parser.OR: 2, parser.AND: 3,
	parser.BITOR: 4, parser.BITXOR: 5, parser.BITAND: 6,
	parser.EQEQ: 7, parser.NE: 7, parser.STREQ: 7, parser.STRNEQ: 7,
	parser.LT: 8, parser.GT: 8, parser.LE: 8, parser.GE: 8,
	parser.INSTANCEOF: 8, parser.INTOKEN: 8,
	parser.LSHIFT: 9, parser.RSHIFT: 9, parser.URSHIFT: 9,
	parser.PLUS: 10, parser.MINUS: 10,
	parser.TIMES: 11, parser.DIVIDE: 11, parser.MOD: 11,
	parser.POW: 12,
}

func (p *Parser) parseExpr(prec int) Node {
	left := p.parsePrefix()
	if left == nil { return nil }
	for {
		if p.tt == parser.QUESTION { return p.parseCond(left) }
		if p.tt == parser.EQUAL {
			p.next()
			rhs := p.parseExpr(1)
			if id, ok := left.(*IdentExpr); ok {
				left = &AssignExpr{Name: id.Name, Rhs: rhs}
			}
			continue
		}
		tokPrec := precMap[p.tt]
		if tokPrec == 0 || tokPrec <= prec { break }
		op := tokOp(p.tt)
		p.next()
		right := p.parseExpr(tokPrec)
		left = &BinaryExpr{Op: op, Left: left, Right: right}
	}
	if p.tt == parser.COMMA && prec <= 0 {
		seq := &SequenceExpr{Expressions: []Node{left}}
		for p.tt == parser.COMMA { p.next(); seq.Expressions = append(seq.Expressions, p.parseExpr(0)) }
		left = seq
	}
	return left
}

func tokOp(tt parser.JSTokenType) string {
	switch tt {
	case parser.PLUS: return "+"; case parser.MINUS: return "-"
	case parser.TIMES: return "*"; case parser.DIVIDE: return "/"; case parser.MOD: return "%"
	case parser.LT: return "<"; case parser.GT: return ">"; case parser.LE: return "<="; case parser.GE: return ">="
	case parser.EQEQ: return "=="; case parser.NE: return "!="; case parser.STREQ: return "==="; case parser.STRNEQ: return "!=="
	case parser.AND: return "&&"; case parser.OR: return "||"
	case parser.BITAND: return "&"; case parser.BITOR: return "|"; case parser.BITXOR: return "^"
	case parser.LSHIFT: return "<<"; case parser.RSHIFT: return ">>"; case parser.URSHIFT: return ">>>"
	case parser.POW: return "**"
	default: return ""
	}
}

func (p *Parser) parsePrefix() Node {
	switch p.tt {
	case parser.INTEGER, parser.DOUBLE:
		val := p.tok.Data.DoubleValue
		p.next()
		return &NumberLit{Value: val}
	case parser.STRING:
		text := p.tokText()
		p.next()
		return &StringLit{Value: text}
	case parser.TRUETOKEN: p.next(); return &BoolLit{Value: true}
	case parser.FALSETOKEN: p.next(); return &BoolLit{Value: false}
	case parser.NULLTOKEN: p.next(); return &NullLit{}
	case parser.THISTOKEN: p.next(); return &IdentExpr{Name: "this"}
	case parser.IDENT:
		name := p.tokText()
		p.next()
		// 检查是否为内置函数调用
		if name == "console" && p.tt == parser.DOT {
			p.next()
			method := p.tokText()
			p.next()
			fullName := "console." + method
			if p.tt == parser.OPENPAREN {
				return p.parseCallWithName(fullName)
			}
		}
		// 检查是否为普通函数调用
		if p.tt == parser.OPENPAREN {
			return p.parseCallExpr(&IdentExpr{Name: name})
		}
		return &IdentExpr{Name: name}
	case parser.OPENPAREN:
		p.next(); expr := p.parseExpr(0); p.expect(parser.CLOSEPAREN); return expr
	case parser.PLUS: p.next(); return &UnaryExpr{Op: "+", Arg: p.parseExpr(10), Prefix: true}
	case parser.MINUS: p.next(); return &UnaryExpr{Op: "-", Arg: p.parseExpr(10), Prefix: true}
	case parser.EXCLAMATION: p.next(); return &UnaryExpr{Op: "!", Arg: p.parseExpr(10), Prefix: true}
	case parser.TYPEOF: p.next(); return &UnaryExpr{Op: "typeof", Arg: p.parseExpr(10), Prefix: true}
	case parser.PLUSPLUS: p.next(); return &UnaryExpr{Op: "++", Arg: p.parseExpr(10), Prefix: true}
	case parser.MINUSMINUS: p.next(); return &UnaryExpr{Op: "--", Arg: p.parseExpr(10), Prefix: true}
	case parser.NEW: p.next(); callee := p.parsePrefix(); var a []Node
		if p.tt == parser.OPENPAREN { p.next()
			for p.tt != parser.CLOSEPAREN && p.tt != parser.EOFTOK { a = append(a, p.parseExpr(0)); p.eat(parser.COMMA) }
			p.expect(parser.CLOSEPAREN) }
		return &CallExpr{Callee: callee, Arguments: a}
	case parser.FUNCTION: return p.parseFuncDecl()
	default: return nil
	}
}

func (p *Parser) parseCallExpr(callee Node) Node {
	p.next()
	var args []Node
	for p.tt != parser.CLOSEPAREN && p.tt != parser.EOFTOK {
		args = append(args, p.parseExpr(0))
		p.eat(parser.COMMA)
	}
	p.expect(parser.CLOSEPAREN)
	return &CallExpr{Callee: callee, Arguments: args}
}

func (p *Parser) parseCallWithName(name string) Node {
	p.next() // (
	var args []Node
	for p.tt != parser.CLOSEPAREN && p.tt != parser.EOFTOK {
		args = append(args, p.parseExpr(0))
		p.eat(parser.COMMA)
	}
	p.expect(parser.CLOSEPAREN)
	return &CallExpr{Callee: &IdentExpr{Name: name}, Arguments: args}
}

func (p *Parser) parseCond(test Node) Node {
	p.next()
	cons := p.parseExpr(0)
	p.expect(parser.COLON)
	alt := p.parseExpr(0)
	return &CondExpr{Test: test, Consequent: cons, Alternate: alt}
}

// ===== Interpreter =====
type Value struct {
	Type string
	Num  float64
	Str  string
	Bool bool
	Fn   *FuncValue
}
type FuncValue struct {
	Name   string
	Params []string
	Body   *BlockStmt
	Env    *Env
}
type Env struct {
	Vars   map[string]Value
	Parent *Env
	Ret    *Value
	Break  bool
	Cont   bool
}

func NewEnv() *Env { return &Env{Vars: make(map[string]Value)} }
func (e *Env) push() *Env { return &Env{Vars: make(map[string]Value), Parent: e} }
func (e *Env) get(name string) (Value, bool) {
	if v, ok := e.Vars[name]; ok { return v, true }
	if e.Parent != nil { return e.Parent.get(name) }
	return Value{Type: "undefined"}, false
}
func (e *Env) set(name string, v Value) {
	if _, ok := e.Vars[name]; ok { e.Vars[name] = v; return }
	if e.Parent != nil { e.Parent.set(name, v); return }
	e.Vars[name] = v
}
func (e *Env) decl(name string, v Value) { e.Vars[name] = v }

func numVal(f float64) Value  { return Value{Type: "number", Num: f} }
func strVal(s string) Value   { return Value{Type: "string", Str: s} }
func boolVal(b bool) Value    { return Value{Type: "boolean", Bool: b} }
func nullVal() Value          { return Value{Type: "null"} }
func undefVal() Value         { return Value{Type: "undefined"} }
func fnVal(f *FuncValue) Value { return Value{Type: "function", Fn: f} }

func toNum(v Value) float64 {
	switch v.Type {
	case "number": return v.Num
	case "string": f, _ := strconv.ParseFloat(v.Str, 64); return f
	case "boolean": if v.Bool { return 1 }; return 0
	case "null": return 0
	default: return math.NaN()
	}
}
func toBoolVal(v Value) bool {
	switch v.Type {
	case "boolean": return v.Bool
	case "number": return v.Num != 0 && !math.IsNaN(v.Num)
	case "string": return v.Str != ""
	case "null", "undefined": return false
	default: return true
	}
}
func toStr(v Value) string {
	switch v.Type {
	case "string": return v.Str
	case "number":
		if v.Num == math.Trunc(v.Num) && !math.IsInf(v.Num, 0) {
			return fmt.Sprintf("%.0f", v.Num)
		}
		return fmt.Sprintf("%g", v.Num)
	case "boolean": if v.Bool { return "true" }; return "false"
	case "null": return "null"
	case "undefined": return "undefined"
	default: return ""
	}
}

// ===== Eval =====
func Eval(node Node, env *Env) Value {
	if node == nil { return undefVal() }
	switch n := node.(type) {
	case *Program: return evalProgram(n, env)
	case *ExprStmt: return Eval(n.Expr, env)
	case *VarStmt: return evalVarStmt(n, env)
	case *AssignExpr: return evalAssign(n, env)
	case *IfStmt: return evalIfStmt(n, env)
	case *WhileStmt: return evalWhileStmt(n, env)
	case *ForStmt: return evalForStmt(n, env)
	case *BlockStmt: return evalBlockStmt(n, env)
	case *ReturnStmt: return evalReturnStmt(n, env)
	case *FuncDecl: return evalFuncDecl(n, env)
	case *NumberLit: return numVal(n.Value)
	case *StringLit: return strVal(n.Value)
	case *BoolLit: return boolVal(n.Value)
	case *NullLit: return nullVal()
	case *IdentExpr: return evalIdentExpr(n, env)
	case *BinaryExpr: return evalBinaryExpr(n, env)
	case *UnaryExpr: return evalUnaryExpr(n, env)
	case *CallExpr: return evalCallExpr(n, env)
	case *CondExpr: return evalCondExpr(n, env)
	case *SequenceExpr: return evalSeqExpr(n, env)
	default: return undefVal()
	}
}

func evalProgram(p *Program, env *Env) Value {
	var last Value
	for _, s := range p.Body { last = Eval(s, env); if env.Ret != nil { return *env.Ret } }
	return last
}
func evalVarStmt(s *VarStmt, env *Env) Value {
	for _, d := range s.Declarations {
		v := undefVal()
		if d.Init != nil { v = Eval(d.Init, env) }
		env.decl(d.Name, v)
	}
	return undefVal()
}
func evalAssign(a *AssignExpr, env *Env) Value {
	v := Eval(a.Rhs, env)
	env.set(a.Name, v)
	return v
}
func evalIfStmt(s *IfStmt, env *Env) Value {
	if toBoolVal(Eval(s.Test, env)) { r := Eval(s.Consequent, env); if env.Ret != nil { return *env.Ret }; return r }
	if s.Alternate != nil { r := Eval(s.Alternate, env); if env.Ret != nil { return *env.Ret }; return r }
	return undefVal()
}
func evalWhileStmt(s *WhileStmt, env *Env) Value {
	for toBoolVal(Eval(s.Test, env)) {
		Eval(s.Body, env)
		if env.Ret != nil { return *env.Ret }
		if env.Break { env.Break = false; break }
		if env.Cont { env.Cont = false; continue }
	}
	return undefVal()
}
func evalForStmt(s *ForStmt, env *Env) Value {
	le := env.push()
	if s.Init != nil { Eval(s.Init, le) }
	for {
		if s.Test != nil && !toBoolVal(Eval(s.Test, le)) { break }
		Eval(s.Body, le)
		if le.Ret != nil { return *le.Ret }
		if le.Break { le.Break = false; break }
		if le.Cont { le.Cont = false; if s.Upd != nil { Eval(s.Upd, le) }; continue }
		if s.Upd != nil { Eval(s.Upd, le) }
	}
	return undefVal()
}
func evalBlockStmt(s *BlockStmt, env *Env) Value {
	be := env.push()
	var last Value
	for _, stmt := range s.Body {
		last = Eval(stmt, be)
		if be.Ret != nil { env.Ret = be.Ret; return *env.Ret }
		if be.Break { env.Break = true; return last }
		if be.Cont { env.Cont = true; return last }
	}
	return last
}
func evalReturnStmt(s *ReturnStmt, env *Env) Value {
	var v Value
	if s.Arg != nil { v = Eval(s.Arg, env) } else { v = undefVal() }
	env.Ret = &v
	return v
}
func evalFuncDecl(f *FuncDecl, env *Env) Value {
	fn := &FuncValue{Name: f.Name, Params: f.Params, Body: f.Body, Env: env}
	if f.Name != "" { env.decl(f.Name, fnVal(fn)) }
	return fnVal(fn)
}
func evalIdentExpr(e *IdentExpr, env *Env) Value {
	if e.Name == "undefined" { return undefVal() }
	v, ok := env.get(e.Name)
	if !ok { return undefVal() }
	return v
}
func evalBinaryExpr(b *BinaryExpr, env *Env) Value {
	switch b.Op {
	case "&&":
		l := Eval(b.Left, env); if !toBoolVal(l) { return l }; return Eval(b.Right, env)
	case "||":
		l := Eval(b.Left, env); if toBoolVal(l) { return l }; return Eval(b.Right, env)
	default:
		l := toNum(Eval(b.Left, env)); r := toNum(Eval(b.Right, env))
		switch b.Op {
		case "+": return numVal(l + r); case "-": return numVal(l - r)
		case "*": return numVal(l * r); case "/": if r == 0 { return numVal(math.Inf(1)) }; return numVal(l / r)
		case "%": return numVal(math.Mod(l, r)); case "**": return numVal(math.Pow(l, r))
		case "<": return boolVal(l < r); case ">": return boolVal(l > r)
		case "<=": return boolVal(l <= r); case ">=": return boolVal(l >= r)
		case "==", "===": return boolVal(l == r); case "!=", "!==": return boolVal(l != r)
		default: return numVal(l + r)
		}
	}
}
func evalUnaryExpr(u *UnaryExpr, env *Env) Value {
	a := Eval(u.Arg, env)
	switch u.Op {
	case "-": return numVal(-toNum(a)); case "+": return numVal(toNum(a))
	case "!": return boolVal(!toBoolVal(a))
	case "typeof": return strVal(typeStr(a))
	case "void": return undefVal()
	default: return a
	}
}
func typeStr(v Value) string {
	switch v.Type { case "undefined": return "undefined"; case "null": return "object"
	case "boolean": return "boolean"; case "number": return "number"
	case "string": return "string"; case "function": return "function"
	default: return "object" }
}
func evalCallExpr(c *CallExpr, env *Env) Value {
	if id, ok := c.Callee.(*IdentExpr); ok {
		if bf, ok := builtins[id.Name]; ok { return bf(c.Arguments, env) }
		fnVal, found := env.get(id.Name)
		if !found || fnVal.Type != "function" { return undefVal() }
		return callFn(fnVal.Fn, c.Arguments, env)
	}
	return undefVal()
}
func callFn(fn *FuncValue, args []Node, env *Env) Value {
	ce := fn.Env.push()
	for i, p := range fn.Params {
		if i < len(args) { ce.decl(p, Eval(args[i], env)) } else { ce.decl(p, undefVal()) }
	}
	res := evalBlockStmt(fn.Body, ce)
	if ce.Ret != nil { return *ce.Ret }
	return res
}
func evalCondExpr(c *CondExpr, env *Env) Value {
	if toBoolVal(Eval(c.Test, env)) { return Eval(c.Consequent, env) }
	return Eval(c.Alternate, env)
}
func evalSeqExpr(s *SequenceExpr, env *Env) Value {
	var last Value
	for _, e := range s.Expressions { last = Eval(e, env) }
	return last
}

// ===== 内置函数 =====
var builtins = map[string]func([]Node, *Env) Value{}

func init() {
	builtins["console.log"] = func(args []Node, env *Env) Value {
		var parts []string
		for _, a := range args { parts = append(parts, toStr(Eval(a, env))) }
		fmt.Println(strings.Join(parts, " "))
		return undefVal()
	}
	builtins["parseInt"] = func(args []Node, env *Env) Value {
		if len(args) == 0 { return numVal(0) }
		f, _ := strconv.ParseFloat(toStr(Eval(args[0], env)), 64)
		return numVal(math.Trunc(f))
	}
	builtins["parseFloat"] = func(args []Node, env *Env) Value {
		if len(args) == 0 { return numVal(0) }
		f, _ := strconv.ParseFloat(toStr(Eval(args[0], env)), 64)
		return numVal(f)
	}
	builtins["isNaN"] = func(args []Node, env *Env) Value {
		if len(args) == 0 { return boolVal(true) }
		return boolVal(math.IsNaN(toNum(Eval(args[0], env))))
	}
}

// ===== 入口 =====

// Evaluate 评估 JS 代码
func Evaluate(code string) (string, error) {
	defer func() {
		if r := recover(); r != nil { fmt.Println("JS 解析异常:", r) }
	}()
	lex := parser.NewLexer(nil, parser.NotBuiltin, parser.Classic)
	src := parser.MakeSource(code, "", "input.js", 1, 1)
	arena := parser.NewParserArena()
	lex.SetCode(&src, arena)
	p := NewParser(lex)
	prog := p.parseProgram()
	if p.tt == parser.ERRORTOK { return "", fmt.Errorf("JS 解析错误: %s", lex.GetErrorMessage()) }
	env := NewEnv()
	result := Eval(prog, env)
	return toStr(result), nil
}

// Run 运行 JS 代码
func Run(code string) (string, error) { return Evaluate(code) }
