// ASTBuilder.h — AST builder (creates AST nodes from parser callbacks)
// Source: Source/JavaScriptCore/parser/ASTBuilder.h

package parser

import "wb-ui/jsc/runtime"

// ASTBuilder corresponds to JSC::ASTBuilder
type ASTBuilder struct {
	vm         *runtime.VM
	arena      *ParserArena
	sourceCode SourceCode
}

func NewASTBuilder(vm *runtime.VM, arena *ParserArena, sourceCode SourceCode) *ASTBuilder {
	return &ASTBuilder{vm: vm, arena: arena, sourceCode: sourceCode}
}

func (b *ASTBuilder) VM() *runtime.VM          { return b.vm }
func (b *ASTBuilder) ParserArena() *ParserArena { return b.arena }
func (b *ASTBuilder) SourceCode() SourceCode     { return b.sourceCode }

// --- Expression creation ---

func (b *ASTBuilder) CreateBoolean(pos JSTextPosition, val bool) ExpressionNode {
	return NewBooleanNodeData(pos, val)
}
func (b *ASTBuilder) CreateNumber(pos JSTextPosition, val float64) ExpressionNode {
	return NewNumberNodeData(pos, val)
}
func (b *ASTBuilder) CreateString(pos JSTextPosition, val string) ExpressionNode {
	return NewStringNodeData(pos, val)
}
func (b *ASTBuilder) CreateBigInt(pos JSTextPosition, val string, radix uint8) ExpressionNode {
	return NewBigIntNodeData(pos, val, radix)
}
func (b *ASTBuilder) CreateNull(pos JSTextPosition) ExpressionNode { return NewNullNode(pos) }
func (b *ASTBuilder) CreateThis(pos JSTextPosition) ExpressionNode { return NewThisNode(pos) }
func (b *ASTBuilder) CreateSuper(pos JSTextPosition) ExpressionNode { return NewSuperNode(pos) }
func (b *ASTBuilder) CreateResolve(pos JSTextPosition, ident *runtime.Identifier) ExpressionNode {
	return NewResolveNode(pos, ident.String())
}
func (b *ASTBuilder) CreateBracketAccessor(pos JSTextPosition, base, subscript ExpressionNode) ExpressionNode {
	return NewBracketAccessorNode(pos, base, subscript)
}
func (b *ASTBuilder) CreateDotAccessor(pos JSTextPosition, base ExpressionNode, ident *runtime.Identifier) ExpressionNode {
	return NewDotAccessorNode(pos, base, ident)
}
func (b *ASTBuilder) CreateNewExpr(pos JSTextPosition, callee ExpressionNode, args []ExpressionNode) ExpressionNode {
	return NewNewExprNode(pos, callee, args)
}
func (b *ASTBuilder) CreateFuncCall(pos JSTextPosition, callee ExpressionNode, args []ExpressionNode) ExpressionNode {
	return NewFuncCallExprNode(pos, callee, args)
}
func (b *ASTBuilder) CreateArray(pos JSTextPosition, elements []ExpressionNode) ExpressionNode {
	return NewArrayNode(pos, elements)
}
func (b *ASTBuilder) CreateObject(pos JSTextPosition, props []*PropertyNode) ExpressionNode {
	return NewObjectNode(pos, props)
}
func (b *ASTBuilder) CreateProperty(pos JSTextPosition, name string, val ExpressionNode, kind PropertyKind) *PropertyNode {
	return NewPropertyNode(pos, name, val, kind)
}
func (b *ASTBuilder) CreateUnaryOp(pos JSTextPosition, op JSTokenType, expr ExpressionNode) ExpressionNode {
	return NewUnaryOpNode(pos, op, expr)
}
func (b *ASTBuilder) CreateBinaryOp(pos JSTextPosition, op JSTokenType, left, right ExpressionNode) ExpressionNode {
	return NewBinaryOpNode(pos, op, left, right)
}
func (b *ASTBuilder) CreateConditional(pos JSTextPosition, cond, trueExpr, falseExpr ExpressionNode) ExpressionNode {
	return NewConditionalNode(pos, cond, trueExpr, falseExpr)
}
func (b *ASTBuilder) CreateTemplateLiteral(pos JSTextPosition, strs []string, exprs []ExpressionNode) ExpressionNode {
	return NewTemplateLiteralNode(pos, strs, exprs)
}
func (b *ASTBuilder) CreateComma(pos JSTextPosition, exprs []ExpressionNode) ExpressionNode {
	return NewCommaNode(pos, exprs)
}
func (b *ASTBuilder) CreateSpread(pos JSTextPosition, expr ExpressionNode) ExpressionNode {
	return NewSpreadExpressionNode(pos, expr)
}
func (b *ASTBuilder) CreateYield(pos JSTextPosition, arg ExpressionNode, delegate bool) ExpressionNode {
	return NewYieldExprNode(pos, arg, delegate)
}
func (b *ASTBuilder) CreateAwait(pos JSTextPosition, arg ExpressionNode) ExpressionNode {
	return NewAwaitExprNode(pos, arg)
}
func (b *ASTBuilder) CreateArrowFunc(pos JSTextPosition, params []string, body *BlockNode) ExpressionNode {
	return NewArrowFuncExprNode(pos, params, body)
}
func (b *ASTBuilder) CreateDestructuringAssignment(pos JSTextPosition, pattern *DestructuringPattern, init ExpressionNode) ExpressionNode {
	return NewDestructuringAssignmentNode(pos, pattern, init)
}
func (b *ASTBuilder) CreateMetaProperty(pos JSTextPosition, keyword, property string) ExpressionNode {
	return NewMetaPropertyNode(pos, keyword, property)
}
func (b *ASTBuilder) CreateLogicalOp(pos JSTextPosition, op JSTokenType, left, right ExpressionNode) ExpressionNode {
	return NewLogicalOpNode(pos, op, left, right)
}
func (b *ASTBuilder) CreateImport(pos JSTextPosition) ExpressionNode {
	return NewImportNode(pos)
}

// --- Statement creation ---

func (b *ASTBuilder) CreateExprStatement(pos JSTextPosition, expr ExpressionNode) StatementNode {
	return NewExprStatementNode(pos, expr)
}
func (b *ASTBuilder) CreateVarStatement(pos JSTextPosition, decls []*VariableDeclarationNode) StatementNode {
	return NewVarStatementNode(pos, decls)
}
func (b *ASTBuilder) CreateVariableDecl(pos JSTextPosition, name string, init ExpressionNode, varType VariableType) *VariableDeclarationNode {
	return NewVariableDeclarationNode(pos, name, init, varType)
}
func (b *ASTBuilder) CreateBlock(pos JSTextPosition, stmts []StatementNode) *BlockNode {
	return NewBlockNode(pos, stmts)
}
func (b *ASTBuilder) CreateIfElse(pos JSTextPosition, cond ExpressionNode, ifBlk, elseBlk StatementNode) StatementNode {
	return NewIfElseNode(pos, cond, ifBlk, elseBlk)
}
func (b *ASTBuilder) CreateWhile(pos JSTextPosition, cond ExpressionNode, body StatementNode) StatementNode {
	return NewWhileNode(pos, cond, body)
}
func (b *ASTBuilder) CreateDoWhile(pos JSTextPosition, body StatementNode, cond ExpressionNode) StatementNode {
	return NewDoWhileNode(pos, body, cond)
}
func (b *ASTBuilder) CreateFor(pos JSTextPosition, init, cond, incr ExpressionNode, body StatementNode) StatementNode {
	return NewForNode(pos, init, cond, incr, body)
}
func (b *ASTBuilder) CreateForIn(pos JSTextPosition, lhs, expr ExpressionNode, body StatementNode) StatementNode {
	return NewForInNode(pos, lhs, expr, body)
}
func (b *ASTBuilder) CreateForOf(pos JSTextPosition, lhs, expr ExpressionNode, body StatementNode, isAwait bool) StatementNode {
	return NewForOfNode(pos, lhs, expr, body, isAwait)
}
func (b *ASTBuilder) CreateBreak(pos JSTextPosition) StatementNode     { return NewBreakNode(pos) }
func (b *ASTBuilder) CreateContinue(pos JSTextPosition) StatementNode  { return NewContinueNode(pos) }
func (b *ASTBuilder) CreateReturn(pos JSTextPosition, val ExpressionNode) StatementNode {
	return NewReturnNode(pos, val)
}
func (b *ASTBuilder) CreateThrow(pos JSTextPosition, val ExpressionNode) StatementNode {
	return NewThrowNode(pos, val)
}
func (b *ASTBuilder) CreateTry(pos JSTextPosition, tryBlk *BlockNode, catchBlk *CatchClauseNode, finBlk *BlockNode) StatementNode {
	return NewTryNode(pos, tryBlk, catchBlk, finBlk)
}
func (b *ASTBuilder) CreateCatch(pos JSTextPosition, param ExpressionNode, body *BlockNode) *CatchClauseNode {
	return NewCatchClauseNode(pos, param, body)
}
func (b *ASTBuilder) CreateSwitch(pos JSTextPosition, expr ExpressionNode, cases []*CaseClauseNode) StatementNode {
	return NewSwitchNode(pos, expr, cases)
}
func (b *ASTBuilder) CreateCase(pos JSTextPosition, expr ExpressionNode, stmts []StatementNode) *CaseClauseNode {
	return NewCaseClauseNode(pos, expr, stmts)
}
func (b *ASTBuilder) CreateEmpty(pos JSTextPosition) StatementNode       { return NewEmptyStatementNode(pos) }
func (b *ASTBuilder) CreateDebugger(pos JSTextPosition) StatementNode   { return NewDebuggerStatementNode(pos) }
func (b *ASTBuilder) CreateWith(pos JSTextPosition, expr ExpressionNode, body StatementNode) StatementNode {
	return NewWithNode(pos, expr, body)
}
func (b *ASTBuilder) CreateLabel(pos JSTextPosition, name string, body StatementNode) StatementNode {
	return NewLabelNode(pos, name, body)
}
func (b *ASTBuilder) CreateFuncExpr(pos JSTextPosition, name string, params []string, body *BlockNode, source SourceCode) *FuncExprNode {
	return NewFuncExprNode(pos, name, params, body, source)
}
func (b *ASTBuilder) CreateFuncDecl(pos JSTextPosition, name string, params []string, body *BlockNode, source SourceCode) StatementNode {
	return NewFuncDeclNode(pos, name, params, body, source)
}
func (b *ASTBuilder) CreateClassDecl(pos JSTextPosition, name string, extends ExpressionNode, methods []*ClassMethodNode) StatementNode {
	return NewClassDeclNode(pos, name, extends, methods)
}
func (b *ASTBuilder) CreateClassExpr(pos JSTextPosition, name string, extends ExpressionNode, methods []*ClassMethodNode) ExpressionNode {
	return NewClassExprNode(pos, name, extends, methods)
}
func (b *ASTBuilder) CreateClassMethod(pos JSTextPosition, name string, fn *FuncExprNode, isStatic, isGetter, isSetter, isConstructor bool) *ClassMethodNode {
	return NewClassMethodNode(pos, name, fn, isStatic, isGetter, isSetter, isConstructor)
}
func (b *ASTBuilder) CreateSourceElements(pos JSTextPosition, children []StatementNode) *SourceElements {
	return NewSourceElementsNode(pos, children)
}
func (b *ASTBuilder) CreateAssignment(pos JSTextPosition, op JSTokenType, left, right ExpressionNode) ExpressionNode {
	return NewBinaryOpNode(pos, op, left, right)
}
