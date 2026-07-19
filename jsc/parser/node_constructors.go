// NodeConstructors.h — AST node constructor functions
// Source: Source/JavaScriptCore/parser/NodeConstructors.h

package parser

import "wb-ui/jsc/runtime"

// === Expression Node Constructors ===

func NewBooleanNodeData(pos JSTextPosition, val bool) *BooleanNode {
	return &BooleanNode{NodeBase: NewNodeBase(pos), value: val}
}

func NewNumberNodeData(pos JSTextPosition, val float64) *NumberNode {
	return &NumberNode{NodeBase: NewNodeBase(pos), value: val}
}

func NewStringNodeData(pos JSTextPosition, val string) *StringNode {
	return &StringNode{NodeBase: NewNodeBase(pos), value: val}
}

func NewBigIntNodeData(pos JSTextPosition, val string, radix uint8) *BigIntNode {
	return &BigIntNode{NodeBase: NewNodeBase(pos), value: val, radix: radix}
}

func NewNullNode(pos JSTextPosition) *NullNode {
	return &NullNode{NodeBase: NewNodeBase(pos)}
}

func NewThisNode(pos JSTextPosition) *ThisNode {
	return &ThisNode{NodeBase: NewNodeBase(pos)}
}

func NewSuperNode(pos JSTextPosition) *SuperNode {
	return &SuperNode{NodeBase: NewNodeBase(pos)}
}

func NewResolveNode(pos JSTextPosition, ident string) *ResolveNode {
	return &ResolveNode{NodeBase: NewNodeBase(pos), identifier: ident}
}

func NewBracketAccessorNode(pos JSTextPosition, base, subscript ExpressionNode) *BracketAccessorNode {
	return &BracketAccessorNode{NodeBase: NewNodeBase(pos), base: base, subscript: subscript}
}

func NewDotAccessorNode(pos JSTextPosition, base ExpressionNode, ident *runtime.Identifier) *DotAccessorNode {
	return &DotAccessorNode{NodeBase: NewNodeBase(pos), base: base, identifier: ident.String()}
}

func NewNewExprNode(pos JSTextPosition, callee ExpressionNode, args []ExpressionNode) *NewExprNode {
	return &NewExprNode{NodeBase: NewNodeBase(pos), callee: callee, args: args}
}

func NewFuncCallExprNode(pos JSTextPosition, callee ExpressionNode, args []ExpressionNode) *FuncCallExprNode {
	return &FuncCallExprNode{NodeBase: NewNodeBase(pos), callee: callee, args: args}
}

func NewArrayNode(pos JSTextPosition, elements []ExpressionNode) *ArrayNode {
	return &ArrayNode{NodeBase: NewNodeBase(pos), elements: elements}
}

func NewObjectNode(pos JSTextPosition, props []*PropertyNode) *ObjectNode {
	return &ObjectNode{NodeBase: NewNodeBase(pos), properties: props}
}

func NewPropertyNode(pos JSTextPosition, name string, value ExpressionNode, kind PropertyKind) *PropertyNode {
	return &PropertyNode{NodeBase: NewNodeBase(pos), name: name, value: value, kind: kind}
}

func NewUnaryOpNode(pos JSTextPosition, op JSTokenType, expr ExpressionNode) *UnaryOpNode {
	return &UnaryOpNode{NodeBase: NewNodeBase(pos), operator: op, expr: expr}
}

func NewBinaryOpNode(pos JSTextPosition, op JSTokenType, left, right ExpressionNode) *BinaryOpNode {
	return &BinaryOpNode{NodeBase: NewNodeBase(pos), operator: op, left: left, right: right}
}

func NewConditionalNode(pos JSTextPosition, cond, trueExpr, falseExpr ExpressionNode) *ConditionalNode {
	return &ConditionalNode{NodeBase: NewNodeBase(pos), condition: cond, trueExpr: trueExpr, falseExpr: falseExpr}
}

func NewTemplateLiteralNode(pos JSTextPosition, strs []string, exprs []ExpressionNode) *TemplateLiteralNode {
	return &TemplateLiteralNode{NodeBase: NewNodeBase(pos), strings: strs, exprs: exprs}
}

func NewCommaNode(pos JSTextPosition, exprs []ExpressionNode) *CommaNode {
	return &CommaNode{NodeBase: NewNodeBase(pos), expressions: exprs}
}

func NewSpreadExpressionNode(pos JSTextPosition, expr ExpressionNode) *SpreadExpressionNode {
	return &SpreadExpressionNode{NodeBase: NewNodeBase(pos), expr: expr}
}

func NewYieldExprNode(pos JSTextPosition, arg ExpressionNode, delegate bool) *YieldExprNode {
	return &YieldExprNode{NodeBase: NewNodeBase(pos), arg: arg, delegate: delegate}
}

func NewAwaitExprNode(pos JSTextPosition, arg ExpressionNode) *AwaitExprNode {
	return &AwaitExprNode{NodeBase: NewNodeBase(pos), arg: arg}
}

func NewArrowFuncExprNode(pos JSTextPosition, params []string, body *BlockNode) *ArrowFuncExprNode {
	return &ArrowFuncExprNode{NodeBase: NewNodeBase(pos), parameters: params, body: body}
}

func NewDestructuringAssignmentNode(pos JSTextPosition, pattern *DestructuringPattern, init ExpressionNode) *DestructuringAssignmentNode {
	return &DestructuringAssignmentNode{NodeBase: NewNodeBase(pos), pattern: pattern, initializer: init}
}

func NewMetaPropertyNode(pos JSTextPosition, keyword, property string) *MetaPropertyNode {
	return &MetaPropertyNode{NodeBase: NewNodeBase(pos), keyword: keyword, property: property}
}

func NewImportNode(pos JSTextPosition) *ImportNode {
	return &ImportNode{NodeBase: NewNodeBase(pos)}
}

func NewLogicalOpNode(pos JSTextPosition, op JSTokenType, left, right ExpressionNode) *LogicalOpNode {
	return &LogicalOpNode{NodeBase: NewNodeBase(pos), operator: op, left: left, right: right}
}

// === Statement Node Constructors ===

func NewExprStatementNode(pos JSTextPosition, expr ExpressionNode) *ExprStatementNode {
	return &ExprStatementNode{NodeBase: NewNodeBase(pos), expr: expr}
}

func NewVarStatementNode(pos JSTextPosition, decls []*VariableDeclarationNode) *VarStatementNode {
	return &VarStatementNode{NodeBase: NewNodeBase(pos), declarations: decls}
}

func NewVariableDeclarationNode(pos JSTextPosition, name string, init ExpressionNode, varType VariableType) *VariableDeclarationNode {
	return &VariableDeclarationNode{NodeBase: NewNodeBase(pos), name: name, initializer: init, varType: varType}
}

func NewBlockNode(pos JSTextPosition, stmts []StatementNode) *BlockNode {
	return &BlockNode{NodeBase: NewNodeBase(pos), statements: stmts}
}

func NewIfElseNode(pos JSTextPosition, cond ExpressionNode, ifBlk, elseBlk StatementNode) *IfElseNode {
	return &IfElseNode{NodeBase: NewNodeBase(pos), condition: cond, ifBlock: ifBlk, elseBlock: elseBlk}
}

func NewWhileNode(pos JSTextPosition, cond ExpressionNode, body StatementNode) *WhileNode {
	return &WhileNode{NodeBase: NewNodeBase(pos), condition: cond, body: body}
}

func NewDoWhileNode(pos JSTextPosition, body StatementNode, cond ExpressionNode) *DoWhileNode {
	return &DoWhileNode{NodeBase: NewNodeBase(pos), body: body, condition: cond}
}

func NewForNode(pos JSTextPosition, init, cond, incr ExpressionNode, body StatementNode) *ForNode {
	return &ForNode{NodeBase: NewNodeBase(pos), init: init, condition: cond, incrExpr: incr, body: body}
}

func NewForInNode(pos JSTextPosition, lhs, expr ExpressionNode, body StatementNode) *ForInNode {
	return &ForInNode{NodeBase: NewNodeBase(pos), lhs: lhs, expr: expr, body: body}
}

func NewForOfNode(pos JSTextPosition, lhs, expr ExpressionNode, body StatementNode, isAwait bool) *ForOfNode {
	return &ForOfNode{NodeBase: NewNodeBase(pos), lhs: lhs, expr: expr, body: body, isAwait: isAwait}
}

func NewBreakNode(pos JSTextPosition) *BreakNode {
	return &BreakNode{NodeBase: NewNodeBase(pos)}
}

func NewContinueNode(pos JSTextPosition) *ContinueNode {
	return &ContinueNode{NodeBase: NewNodeBase(pos)}
}

func NewReturnNode(pos JSTextPosition, val ExpressionNode) *ReturnNode {
	return &ReturnNode{NodeBase: NewNodeBase(pos), value: val}
}

func NewThrowNode(pos JSTextPosition, val ExpressionNode) *ThrowNode {
	return &ThrowNode{NodeBase: NewNodeBase(pos), value: val}
}

func NewTryNode(pos JSTextPosition, tryBlk *BlockNode, catchBlk *CatchClauseNode, finallyBlk *BlockNode) *TryNode {
	return &TryNode{NodeBase: NewNodeBase(pos), tryBlock: tryBlk, catchBlock: catchBlk, finallyBlock: finallyBlk}
}

func NewCatchClauseNode(pos JSTextPosition, param ExpressionNode, body *BlockNode) *CatchClauseNode {
	return &CatchClauseNode{NodeBase: NewNodeBase(pos), parameter: param, body: body}
}

func NewSwitchNode(pos JSTextPosition, expr ExpressionNode, cases []*CaseClauseNode) *SwitchNode {
	return &SwitchNode{NodeBase: NewNodeBase(pos), expr: expr, cases: cases}
}

func NewCaseClauseNode(pos JSTextPosition, expr ExpressionNode, stmts []StatementNode) *CaseClauseNode {
	return &CaseClauseNode{NodeBase: NewNodeBase(pos), expr: expr, statements: stmts}
}

func NewEmptyStatementNode(pos JSTextPosition) *EmptyStatementNode {
	return &EmptyStatementNode{NodeBase: NewNodeBase(pos)}
}

func NewDebuggerStatementNode(pos JSTextPosition) *DebuggerStatementNode {
	return &DebuggerStatementNode{NodeBase: NewNodeBase(pos)}
}

func NewWithNode(pos JSTextPosition, expr ExpressionNode, body StatementNode) *WithNode {
	return &WithNode{NodeBase: NewNodeBase(pos), expr: expr, body: body}
}

func NewLabelNode(pos JSTextPosition, name string, body StatementNode) *LabelNode {
	return &LabelNode{NodeBase: NewNodeBase(pos), name: name, body: body}
}

func NewFuncExprNode(pos JSTextPosition, name string, params []string, body *BlockNode, source SourceCode) *FuncExprNode {
	return &FuncExprNode{NodeBase: NewNodeBase(pos), name: name, parameters: params, body: body, sourceCode: source}
}

func NewFuncDeclNode(pos JSTextPosition, name string, params []string, body *BlockNode, source SourceCode) *FuncDeclNode {
	return &FuncDeclNode{NodeBase: NewNodeBase(pos), name: name, parameters: params, body: body, sourceCode: source}
}

func NewClassDeclNode(pos JSTextPosition, name string, extends ExpressionNode, methods []*ClassMethodNode) *ClassDeclNode {
	return &ClassDeclNode{NodeBase: NewNodeBase(pos), name: name, extends: extends, methods: methods}
}

func NewClassExprNode(pos JSTextPosition, name string, extends ExpressionNode, methods []*ClassMethodNode) *ClassExprNode {
	return &ClassExprNode{NodeBase: NewNodeBase(pos), name: name, extends: extends, methods: methods}
}

func NewClassMethodNode(pos JSTextPosition, name string, fn *FuncExprNode, isStatic, isGetter, isSetter, isConstructor bool) *ClassMethodNode {
	return &ClassMethodNode{NodeBase: NewNodeBase(pos), name: name, function: fn, isStatic: isStatic, isGetter: isGetter, isSetter: isSetter, isConstructor: isConstructor}
}

func NewSourceElementsNode(pos JSTextPosition, children []StatementNode) *SourceElements {
	return &SourceElements{NodeBase: NewNodeBase(pos), children: children}
}
