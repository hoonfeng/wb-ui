// JSC 引擎主入口
//
// 提供 JavaScript 解析和执行功能
package jsc

import (
	"fmt"
	"wb-ui/jsc/parser"
)

// Interpreter JS 解释器
type Interpreter struct{}

// NewInterpreter 创建新的 JS 解释器
func NewInterpreter() *Interpreter {
	return &Interpreter{}
}

// Run 执行 JS 代码
func (r *Interpreter) Run(src string) (interface{}, error) {
	return r.Evaluate(src)
}

// Evaluate 评估 JS 代码
func (r *Interpreter) Evaluate(src string) (interface{}, error) {
	// 创建源代码
	source := parser.MakeSource(src, "", "test.js", 1, 1)
	parseMode := parser.ProgramMode
	p := parser.NewParser(&source, parser.ImplementationVisibilityPublic, parseMode,
		parser.FunctionModeNone, parser.SuperBindingNotNeeded, parser.Classic)

	// 解析
	_ = p
	return nil, fmt.Errorf("JSC 引擎尚在开发中")
}
