// 包 bytecompiler 是 JavaScriptCore bytecompiler 模块的 Go 翻译
//
// 从 WebKit Source/JavaScriptCore/bytecompiler/ 翻译为 Go
package bytecompiler

// BytecodeGenerator 字节码生成器
type BytecodeGenerator struct{}

// RegisterID 寄存器标识符
type RegisterID struct{}

// Label 表示字节码跳转标签
type Label struct{}

// NewLabel 创建新标签
func NewLabel(ref *Label) *Label {
	return &Label{}
}
