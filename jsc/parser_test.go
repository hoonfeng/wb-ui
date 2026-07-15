package jsc

import "testing"

// parseExpr parses src as a single expression statement and returns the expression.
func parseExpr(t *testing.T, src string) Expr {
	t.Helper()
	prog, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	if len(prog.Body) == 0 {
		t.Fatalf("Parse(%q) produced empty program", src)
	}
	es, ok := prog.Body[0].(*ExpressionStatement)
	if !ok {
		t.Fatalf("first statement is %T, want ExpressionStatement", prog.Body[0])
	}
	return es.Expr
}

func TestParserNumberLiteral(t *testing.T) {
	e := parseExpr(t, "42")
	lit, ok := e.(*Literal)
	if !ok {
		t.Fatalf("got %T, want *Literal", e)
	}
	if !lit.Value.IsNumber() || lit.Value.AsNumber() != 42 {
		t.Fatalf("value = %v, want 42", lit.Value)
	}
}

func TestParserStringLiteral(t *testing.T) {
	e := parseExpr(t, `"hi"`)
	lit, ok := e.(*Literal)
	if !ok {
		t.Fatalf("got %T, want *Literal", e)
	}
	if lit.Value.AsString() != "hi" {
		t.Fatalf("value = %v, want hi", lit.Value)
	}
}

func TestParserBinaryExpression(t *testing.T) {
	e := parseExpr(t, "1 + 2 * 3")
	bin, ok := e.(*BinaryExpression)
	if !ok {
		t.Fatalf("got %T, want *BinaryExpression", e)
	}
	if bin.Op != TokenPlus {
		t.Fatalf("op = %v, want +", bin.Op)
	}
	// Right side should be 2 * 3 (higher precedence).
	right, ok := bin.Right.(*BinaryExpression)
	if !ok {
		t.Fatalf("right = %T, want *BinaryExpression", bin.Right)
	}
	if right.Op != TokenStar {
		t.Fatalf("right op = %v, want *", right.Op)
	}
}

func TestParserPrecedence(t *testing.T) {
	// 2 + 3 * 4 => (2 + (3 * 4))
	e := parseExpr(t, "2 + 3 * 4")
	bin := e.(*BinaryExpression)
	if r, ok := bin.Right.(*BinaryExpression); !ok || r.Op != TokenStar {
		t.Fatalf("expected 3*4 on right, got %T", bin.Right)
	}
	// 2 * 3 + 4 => ((2 * 3) + 4)
	e = parseExpr(t, "2 * 3 + 4")
	bin = e.(*BinaryExpression)
	if l, ok := bin.Left.(*BinaryExpression); !ok || l.Op != TokenStar {
		t.Fatalf("expected 2*3 on left, got %T", bin.Left)
	}
}

func TestParserParenGrouping(t *testing.T) {
	e := parseExpr(t, "(1 + 2) * 3")
	bin, ok := e.(*BinaryExpression)
	if !ok {
		t.Fatalf("got %T, want *BinaryExpression", e)
	}
	if _, ok := bin.Left.(*BinaryExpression); !ok {
		t.Fatalf("left = %T, want grouped BinaryExpression", bin.Left)
	}
}

func TestParserPowerRightAssoc(t *testing.T) {
	// 2 ** 3 ** 2 => 2 ** (3 ** 2)
	e := parseExpr(t, "2 ** 3 ** 2")
	bin := e.(*BinaryExpression)
	if _, ok := bin.Right.(*BinaryExpression); !ok {
		t.Fatalf("right should be nested power, got %T", bin.Right)
	}
}

func TestParserAssignment(t *testing.T) {
	e := parseExpr(t, "x = 5")
	a, ok := e.(*AssignmentExpression)
	if !ok {
		t.Fatalf("got %T, want *AssignmentExpression", e)
	}
	if a.Op != TokenAssign {
		t.Fatalf("op = %v, want =", a.Op)
	}
	id, ok := a.Target.(*Identifier)
	if !ok || id.Name != "x" {
		t.Fatalf("target = %v, want identifier x", a.Target)
	}
}

func TestParserFunctionDeclaration(t *testing.T) {
	prog, err := Parse("function add(a, b) { return a + b; }")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("body len = %d, want 1", len(prog.Body))
	}
	fd, ok := prog.Body[0].(*FunctionDeclaration)
	if !ok {
		t.Fatalf("got %T, want *FunctionDeclaration", prog.Body[0])
	}
	if fd.Name != "add" || len(fd.Params) != 2 {
		t.Fatalf("name=%q params=%v", fd.Name, fd.Params)
	}
}

func TestParserArrowFunction(t *testing.T) {
	e := parseExpr(t, "(a, b) => a + b")
	af, ok := e.(*ArrowFunction)
	if !ok {
		t.Fatalf("got %T, want *ArrowFunction", e)
	}
	if len(af.Params) != 2 || af.Params[0] != "a" {
		t.Fatalf("params = %v", af.Params)
	}
	if !af.IsExpr {
		t.Fatalf("expected concise expression body")
	}
}

func TestParserArrowSingleParam(t *testing.T) {
	e := parseExpr(t, "x => x * 2")
	af, ok := e.(*ArrowFunction)
	if !ok {
		t.Fatalf("got %T, want *ArrowFunction", e)
	}
	if len(af.Params) != 1 || af.Params[0] != "x" {
		t.Fatalf("params = %v", af.Params)
	}
}

func TestParserIfStatement(t *testing.T) {
	prog, err := Parse("if (x > 0) { return 1; } else { return 2; }")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	ifs, ok := prog.Body[0].(*IfStatement)
	if !ok {
		t.Fatalf("got %T, want *IfStatement", prog.Body[0])
	}
	if ifs.Alternate == nil {
		t.Fatalf("expected else branch")
	}
}

func TestParserForStatement(t *testing.T) {
	prog, err := Parse("for (let i = 0; i < 10; i++) { sum += i; }")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	fs, ok := prog.Body[0].(*ForStatement)
	if !ok {
		t.Fatalf("got %T, want *ForStatement", prog.Body[0])
	}
	if fs.Test == nil || fs.Update == nil {
		t.Fatalf("for loop missing test/update")
	}
}

func TestParserObjectLiteral(t *testing.T) {
	// Wrap in parens: a bare '{' at statement start is a block, not an object.
	e := parseExpr(t, "({a: 1, b: \"x\", \"c\": 2})")
	oe, ok := e.(*ObjectExpression)
	if !ok {
		t.Fatalf("got %T, want *ObjectExpression", e)
	}
	if len(oe.Properties) != 3 {
		t.Fatalf("properties = %d, want 3", len(oe.Properties))
	}
}

func TestParserArrayLiteral(t *testing.T) {
	e := parseExpr(t, "[1, 2, 3]")
	ae, ok := e.(*ArrayExpression)
	if !ok {
		t.Fatalf("got %T, want *ArrayExpression", e)
	}
	if len(ae.Elements) != 3 {
		t.Fatalf("elements = %d, want 3", len(ae.Elements))
	}
}

func TestParserArrayHoles(t *testing.T) {
	e := parseExpr(t, "[1, , 3]")
	ae := e.(*ArrayExpression)
	if len(ae.Elements) != 3 {
		t.Fatalf("elements = %d, want 3", len(ae.Elements))
	}
	if ae.Elements[1] != nil {
		t.Fatalf("expected hole at index 1")
	}
}

func TestParserMemberExpression(t *testing.T) {
	e := parseExpr(t, "a.b.c")
	me, ok := e.(*MemberExpression)
	if !ok {
		t.Fatalf("got %T, want *MemberExpression", e)
	}
	if me.Name != "c" {
		t.Fatalf("name = %q, want c", me.Name)
	}
	if _, ok := me.Object.(*MemberExpression); !ok {
		t.Fatalf("object = %T, want nested MemberExpression", me.Object)
	}
}

func TestParserComputedMember(t *testing.T) {
	e := parseExpr(t, "a[0]")
	me, ok := e.(*MemberExpression)
	if !ok {
		t.Fatalf("got %T, want *MemberExpression", e)
	}
	if !me.Computed {
		t.Fatalf("expected computed member")
	}
}

func TestParserCallExpression(t *testing.T) {
	e := parseExpr(t, "f(1, 2)")
	ce, ok := e.(*CallExpression)
	if !ok {
		t.Fatalf("got %T, want *CallExpression", e)
	}
	if len(ce.Arguments) != 2 {
		t.Fatalf("args = %d, want 2", len(ce.Arguments))
	}
}

func TestParserMethodCall(t *testing.T) {
	e := parseExpr(t, "obj.method(42)")
	ce, ok := e.(*CallExpression)
	if !ok {
		t.Fatalf("got %T, want *CallExpression", e)
	}
	me, ok := ce.Callee.(*MemberExpression)
	if !ok {
		t.Fatalf("callee = %T, want *MemberExpression", ce.Callee)
	}
	if me.Name != "method" {
		t.Fatalf("method name = %q", me.Name)
	}
}

func TestParserVariableDeclaration(t *testing.T) {
	prog, err := Parse("let x = 1, y = 2;")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	vd, ok := prog.Body[0].(*VariableDeclaration)
	if !ok {
		t.Fatalf("got %T, want *VariableDeclaration", prog.Body[0])
	}
	if vd.Kind != "let" || len(vd.Declarators) != 2 {
		t.Fatalf("kind=%q declarators=%d", vd.Kind, len(vd.Declarators))
	}
}

func TestParserConditionalExpression(t *testing.T) {
	e := parseExpr(t, "x ? 1 : 2")
	ce, ok := e.(*ConditionalExpression)
	if !ok {
		t.Fatalf("got %T, want *ConditionalExpression", e)
	}
	if ce == nil || ce.Consequent == nil || ce.Alternate == nil {
		t.Fatalf("conditional branches not populated")
	}
}

func TestParserUnaryExpression(t *testing.T) {
	e := parseExpr(t, "-x")
	ue, ok := e.(*UnaryExpression)
	if !ok {
		t.Fatalf("got %T, want *UnaryExpression", e)
	}
	if ue.Op != TokenMinus {
		t.Fatalf("op = %v, want -", ue.Op)
	}
}

func TestParserUpdateExpression(t *testing.T) {
	cases := []struct {
		src    string
		prefix bool
		op     TokenKind
	}{
		{"x++", false, TokenIncrement},
		{"++x", true, TokenIncrement},
		{"x--", false, TokenDecrement},
		{"--x", true, TokenDecrement},
	}
	for _, c := range cases {
		e := parseExpr(t, c.src)
		ue, ok := e.(*UpdateExpression)
		if !ok {
			t.Fatalf("%s: got %T, want *UpdateExpression", c.src, e)
		}
		if ue.Prefix != c.prefix || ue.Op != c.op {
			t.Fatalf("%s: prefix=%v op=%v, want %v %v", c.src, ue.Prefix, ue.Op, c.prefix, c.op)
		}
	}
}

func TestParserTryCatch(t *testing.T) {
	prog, err := Parse("try { f(); } catch (e) { g(); } finally { h(); }")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	ts, ok := prog.Body[0].(*TryStatement)
	if !ok {
		t.Fatalf("got %T, want *TryStatement", prog.Body[0])
	}
	if ts.Handler == nil || ts.Handler.Param != "e" {
		t.Fatalf("handler = %+v", ts.Handler)
	}
	if ts.Finalizer == nil {
		t.Fatalf("expected finally block")
	}
}

func TestParserTemplateLiteral(t *testing.T) {
	e := parseExpr(t, "`a${1}b`")
	tl, ok := e.(*TemplateLiteral)
	if !ok {
		t.Fatalf("got %T, want *TemplateLiteral", e)
	}
	if len(tl.Expressions) != 1 {
		t.Fatalf("expressions = %d, want 1", len(tl.Expressions))
	}
}

func TestParserWhileStatement(t *testing.T) {
	prog, err := Parse("while (x < 10) { x++; }")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if _, ok := prog.Body[0].(*WhileStatement); !ok {
		t.Fatalf("got %T, want *WhileStatement", prog.Body[0])
	}
}

func TestParserNewExpression(t *testing.T) {
	e := parseExpr(t, "new Array(3)")
	ne, ok := e.(*NewExpression)
	if !ok {
		t.Fatalf("got %T, want *NewExpression", e)
	}
	if len(ne.Arguments) != 1 {
		t.Fatalf("args = %d, want 1", len(ne.Arguments))
	}
}

func TestParserParseError(t *testing.T) {
	_, err := Parse("let = 1")
	if err == nil {
		t.Fatalf("expected parse error for 'let = 1'")
	}
}
