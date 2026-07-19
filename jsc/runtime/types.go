// 包 runtime 是 JavaScriptCore runtime 模块的 Go 翻译
//
// 从 WebKit Source/JavaScriptCore/runtime/ 翻译为 Go
package runtime

// JSValue 表示 JavaScript 值
type JSValue struct {
	Value uint64
}

// JsNull 返回 null 值
func JsNull() JSValue {
	return JSValue{Value: 0}
}

// JsBoolean 返回布尔值
func JsBoolean(b bool) JSValue {
	if b {
		return JSValue{Value: 1}
	}
	return JSValue{Value: 0}
}

// JsNumber 返回数值
func JsNumber(generator interface{}, d float64) JSValue {
	return JSValue{}
}

// VM 表示 JavaScript 虚拟机
type VM struct{}

// Identifier 表示标识符
type Identifier struct {
	ImplPtr UniquedStringImplPtr
}

func (id *Identifier) IsEmpty() bool {
	return id.ImplPtr == 0
}

func (id *Identifier) IsNull() bool {
	return id.ImplPtr == 0
}

func (id *Identifier) Impl() UniquedStringImplPtr {
	return id.ImplPtr
}

// UniquedStringImplPtr 指向唯一字符串实现的指针
type UniquedStringImplPtr = uintptr

// RefPtr 是引用计数指针
type RefPtr struct {
	Ptr uintptr
}
