// interpreter 包 - JavaScriptCore interpreter 模块的 Go 翻译
package interpreter

import "wb-ui/jsc/runtime"

// Register 值寄存器 (stub)
type Register struct{ Value runtime.JSValue }

// CallFrame 调用帧 (stub)
type CallFrame struct{}

// Interpreter 解释器 (stub)
type Interpreter struct{}

// CLoopStack C 循环栈 (stub)
type CLoopStack struct{}

// StackVisitor 栈访问器 (stub)
type StackVisitor struct{}
