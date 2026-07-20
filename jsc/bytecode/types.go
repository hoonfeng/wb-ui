// bytecode 包 - JavaScriptCore bytecode 模块的 Go 翻译
package bytecode

// VirtualRegister 虚拟寄存器
type VirtualRegister struct{ offset int }

func NewVirtualRegister(o int) VirtualRegister { return VirtualRegister{offset: o} }
func (v VirtualRegister) Offset() int          { return v.offset }
func (v VirtualRegister) IsValid() bool         { return v.offset != -1 }

// CodeBlockHash 代码块哈希
type CodeBlockHash struct{}

// Instruction 字节码指令
type Instruction struct{ Data uint64 }

// InstructionStream 指令流
type InstructionStream struct{ Instructions []Instruction }

// CodeBlock 代码块 (stub)
type CodeBlock struct{}

// UnlinkedCodeBlock 未链接代码块 (stub)
type UnlinkedCodeBlock struct{}
