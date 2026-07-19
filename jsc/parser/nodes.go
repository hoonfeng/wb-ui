// Nodes.h — JavaScript AST node types (skeleton)
// Full 1:1 translation of WebKit's Nodes.h
// Source: Source/JavaScriptCore/parser/Nodes.h
//
// Uses Go interfaces to represent the C++ class hierarchy.

package parser

import "wb-ui/jsc/runtime"

// ============= Core Interfaces =============

// Node — base interface for all AST nodes
type Node interface {
	Position() JSTextPosition
	SetPosition(JSTextPosition)
}

// ExpressionNode — base interface for expression nodes
type ExpressionNode interface {
	Node
	isExpressionNode()
}

// StatementNode — base interface for statement nodes
type StatementNode interface {
	Node
	isStatementNode()
}

// ============= Node Base Implementation =============

type NodeBase struct {
	pos JSTextPosition
}

func NewNodeBase(pos JSTextPosition) NodeBase { return NodeBase{pos: pos} }
func (n *NodeBase) Position() JSTextPosition    { return n.pos }
func (n *NodeBase) SetPosition(p JSTextPosition) { n.pos = p }

// ============= EXPRESSIONS =============

// BooleanNode
type BooleanNode struct {
	NodeBase
	value bool
}
func (n *BooleanNode) isExpressionNode() {}

// NumberNode
type NumberNode struct {
	NodeBase
	value float64
}
func (n *NumberNode) isExpressionNode() {}

// StringNode
type StringNode struct {
	NodeBase
	value string
}
func (n *StringNode) isExpressionNode() {}

// BigIntNode
type BigIntNode struct {
	NodeBase
	value string
	radix uint8
}
func (n *BigIntNode) isExpressionNode() {}

// NullNode
type NullNode struct{ NodeBase }
func (n *NullNode) isExpressionNode() {}

// ThisNode
type ThisNode struct{ NodeBase }
func (n *ThisNode) isExpressionNode() {}

// SuperNode
type SuperNode struct{ NodeBase }
func (n *SuperNode) isExpressionNode() {}

// ResolveNode
type ResolveNode struct {
	NodeBase
	identifier string
}
func (n *ResolveNode) isExpressionNode() {}

// BracketAccessorNode
type BracketAccessorNode struct {
	NodeBase
	base      ExpressionNode
	subscript ExpressionNode
}
func (n *BracketAccessorNode) isExpressionNode() {}

// DotAccessorNode
type DotAccessorNode struct {
	NodeBase
	base       ExpressionNode
	identifier string
}
func (n *DotAccessorNode) isExpressionNode() {}

// NewExprNode
type NewExprNode struct {
	NodeBase
	callee ExpressionNode
	args   []ExpressionNode
}
func (n *NewExprNode) isExpressionNode() {}

// FuncCallExprNode
type FuncCallExprNode struct {
	NodeBase
	callee ExpressionNode
	args   []ExpressionNode
}
func (n *FuncCallExprNode) isExpressionNode() {}

// ArrayNode
type ArrayNode struct {
	NodeBase
	elements []ExpressionNode
}
func (n *ArrayNode) isExpressionNode() {}

// ObjectNode
type ObjectNode struct {
	NodeBase
	properties []*PropertyNode
}
func (n *ObjectNode) isExpressionNode() {}

// PropertyNode
type PropertyNode struct {
	NodeBase
	name  string
	value ExpressionNode
	kind  PropertyKind
}

// PropertyKind
type PropertyKind int
const (
	PropertyKindInit   PropertyKind = 0
	PropertyKindGet    PropertyKind = 1
	PropertyKindSet    PropertyKind = 2
	PropertyKindSpread PropertyKind = 3
)

// UnaryOpNode
type UnaryOpNode struct {
	NodeBase
	operator JSTokenType
	expr     ExpressionNode
}
func (n *UnaryOpNode) isExpressionNode() {}

// BinaryOpNode
type BinaryOpNode struct {
	NodeBase
	operator JSTokenType
	left     ExpressionNode
	right    ExpressionNode
}
func (n *BinaryOpNode) isExpressionNode() {}

// ConditionalNode
type ConditionalNode struct {
	NodeBase
	condition ExpressionNode
	trueExpr  ExpressionNode
	falseExpr ExpressionNode
}
func (n *ConditionalNode) isExpressionNode() {}

// TemplateLiteralNode
type TemplateLiteralNode struct {
	NodeBase
	strings []string
	exprs   []ExpressionNode
}
func (n *TemplateLiteralNode) isExpressionNode() {}

// CommaNode
type CommaNode struct {
	NodeBase
	expressions []ExpressionNode
}
func (n *CommaNode) isExpressionNode() {}

// LogicalOpNode
type LogicalOpNode struct {
	NodeBase
	operator JSTokenType
	left     ExpressionNode
	right    ExpressionNode
}
func (n *LogicalOpNode) isExpressionNode() {}

// SpreadExpressionNode
type SpreadExpressionNode struct {
	NodeBase
	expr ExpressionNode
}
func (n *SpreadExpressionNode) isExpressionNode() {}

// YieldExprNode
type YieldExprNode struct {
	NodeBase
	arg      ExpressionNode
	delegate bool
}
func (n *YieldExprNode) isExpressionNode() {}

// AwaitExprNode
type AwaitExprNode struct {
	NodeBase
	arg ExpressionNode
}
func (n *AwaitExprNode) isExpressionNode() {}

// ArrowFuncExprNode
type ArrowFuncExprNode struct {
	NodeBase
	parameters []string
	body       *BlockNode
	exprBody   ExpressionNode
	isExprBody bool
}
func (n *ArrowFuncExprNode) isExpressionNode() {}

// DestructuringAssignmentNode
type DestructuringAssignmentNode struct {
	NodeBase
	pattern     *DestructuringPattern
	initializer ExpressionNode
}
func (n *DestructuringAssignmentNode) isExpressionNode() {}

// MetaPropertyNode
type MetaPropertyNode struct {
	NodeBase
	keyword  string
	property string
}
func (n *MetaPropertyNode) isExpressionNode() {}

// ============= STATEMENTS =============

// ExprStatementNode
type ExprStatementNode struct {
	NodeBase
	expr ExpressionNode
}
func (n *ExprStatementNode) isStatementNode() {}

// VarStatementNode
type VarStatementNode struct {
	NodeBase
	declarations []*VariableDeclarationNode
}
func (n *VarStatementNode) isStatementNode() {}

// VariableDeclarationNode
type VariableDeclarationNode struct {
	NodeBase
	name        string
	initializer ExpressionNode
	varType     VariableType
}

// VariableType
type VariableType int
const (
	VariableTypeVar  VariableType = 0
	VariableTypeLet  VariableType = 1
	VariableTypeConst VariableType = 2
)

// BlockNode
type BlockNode struct {
	NodeBase
	statements []StatementNode
}
func (n *BlockNode) isStatementNode() {}

// IfElseNode
type IfElseNode struct {
	NodeBase
	condition ExpressionNode
	ifBlock   StatementNode
	elseBlock StatementNode
}
func (n *IfElseNode) isStatementNode() {}

// WhileNode
type WhileNode struct {
	NodeBase
	condition ExpressionNode
	body      StatementNode
}
func (n *WhileNode) isStatementNode() {}

// DoWhileNode
type DoWhileNode struct {
	NodeBase
	body      StatementNode
	condition ExpressionNode
}
func (n *DoWhileNode) isStatementNode() {}

// ForNode
type ForNode struct {
	NodeBase
	init      ExpressionNode
	condition ExpressionNode
	incrExpr  ExpressionNode
	body      StatementNode
}
func (n *ForNode) isStatementNode() {}

// ForInNode
type ForInNode struct {
	NodeBase
	lhs  ExpressionNode
	expr ExpressionNode
	body StatementNode
}
func (n *ForInNode) isStatementNode() {}

// ForOfNode
type ForOfNode struct {
	NodeBase
	lhs     ExpressionNode
	expr    ExpressionNode
	body    StatementNode
	isAwait bool
}
func (n *ForOfNode) isStatementNode() {}

// BreakNode
type BreakNode struct{ NodeBase }
func (n *BreakNode) isStatementNode() {}

// ContinueNode
type ContinueNode struct{ NodeBase }
func (n *ContinueNode) isStatementNode() {}

// ReturnNode
type ReturnNode struct {
	NodeBase
	value ExpressionNode
}
func (n *ReturnNode) isStatementNode() {}

// ThrowNode
type ThrowNode struct {
	NodeBase
	value ExpressionNode
}
func (n *ThrowNode) isStatementNode() {}

// TryNode
type TryNode struct {
	NodeBase
	tryBlock    *BlockNode
	catchBlock  *CatchClauseNode
	finallyBlock *BlockNode
}
func (n *TryNode) isStatementNode() {}

// CatchClauseNode
type CatchClauseNode struct {
	NodeBase
	parameter ExpressionNode
	body      *BlockNode
}

// SwitchNode
type SwitchNode struct {
	NodeBase
	expr  ExpressionNode
	cases []*CaseClauseNode
}
func (n *SwitchNode) isStatementNode() {}

// CaseClauseNode
type CaseClauseNode struct {
	NodeBase
	expr       ExpressionNode
	statements []StatementNode
}

// EmptyStatementNode
type EmptyStatementNode struct{ NodeBase }
func (n *EmptyStatementNode) isStatementNode() {}

// DebuggerStatementNode
type DebuggerStatementNode struct{ NodeBase }
func (n *DebuggerStatementNode) isStatementNode() {}

// WithNode
type WithNode struct {
	NodeBase
	expr ExpressionNode
	body StatementNode
}
func (n *WithNode) isStatementNode() {}

// LabelNode
type LabelNode struct {
	NodeBase
	name string
	body StatementNode
}
func (n *LabelNode) isStatementNode() {}

// ============= FUNCTION / CLASS =============

// FuncExprNode (used as expression)
type FuncExprNode struct {
	NodeBase
	name       string
	parameters []string
	body       *BlockNode
	sourceCode SourceCode
}
func (n *FuncExprNode) isStatementNode() {}
func (n *FuncExprNode) isExpressionNode() {}

// FuncDeclNode
type FuncDeclNode struct {
	NodeBase
	name       string
	parameters []string
	body       *BlockNode
	sourceCode SourceCode
}
func (n *FuncDeclNode) isStatementNode() {}

// ClassDeclNode
type ClassDeclNode struct {
	NodeBase
	name    string
	extends ExpressionNode
	methods []*ClassMethodNode
}
func (n *ClassDeclNode) isStatementNode() {}

// ClassExprNode
type ClassExprNode struct {
	NodeBase
	name    string
	extends ExpressionNode
	methods []*ClassMethodNode
}
func (n *ClassExprNode) isExpressionNode() {}

// ClassMethodNode
type ClassMethodNode struct {
	NodeBase
	name        string
	function    *FuncExprNode
	isStatic    bool
	isGetter    bool
	isSetter    bool
	isConstructor bool
}

// ============= PATTERNS =============

// DestructuringPattern — base type
type DestructuringPattern struct{}

// SourceElements
type SourceElements struct {
	NodeBase
	children []StatementNode
}
func (n *SourceElements) isStatementNode() {}

// ImportNode
type ImportNode struct {
	NodeBase
}
func (n *ImportNode) isExpressionNode() {}

// ============= Identifier helpers =============

func MakeIdentifier(name string) runtime.Identifier {
	panic("unimplemented")
}
