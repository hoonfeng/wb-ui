// ExpressionInfo - stores source location info for bytecode
package bytecode

type ExpressionInfo struct {
	bytecodeOffset uint32
	line           uint32
	column         uint32
	mode           uint8 // 0=error, 1=debug
}

func NewExpressionInfo(offset, line, column uint32) ExpressionInfo {
	return ExpressionInfo{
		bytecodeOffset: offset,
		line:           line,
		column:         column,
	}
}

func (e ExpressionInfo) BytecodeOffset() uint32 { return e.bytecodeOffset }
func (e ExpressionInfo) Line() uint32 { return e.line }
func (e ExpressionInfo) Column() uint32 { return e.column }
