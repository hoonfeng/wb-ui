package jsc

import (
	"testing"

	"wb-ui.com/goja"
)

// TestNewArrayNilProtoReusesItemRuntime 回归：NewArray(nil, items) 此前
// 在 items 含已绑定 runtime 的对象时创建全新 goja Runtime，把主 runtime
// 的对象塞进去 → goja 抛 "Illegal runtime transition of an Object"
// （ResizeObserver/MutationObserver/IntersectionObserver 回调、
// NodeList 等元素/记录数组全部走这个函数）。
// 修复后应复用 items 中第一个已绑定 runtime，数组与元素同属一个
// runtime，不再崩溃。
func TestNewArrayNilProtoReusesItemRuntime(t *testing.T) {
	main := NewInterpreter() // 模拟主 runtime

	// 在主 runtime 中创建元素包装对象（等价于 wrapElement(interp, el)）
	obj := NewObject(main.ObjectPrototype())
	obj.Set("tag", StringValue("span"))

	// 此调用在修复前 panic：Illegal runtime transition of an Object
	arr := NewArray(nil, []JSValue{ObjectValue(obj)})
	if arr == nil || arr.obj == nil {
		t.Fatal("NewArray returned nil")
	}
	// 数组必须与元素同属主 runtime（否则后续 Call/Set 仍会崩）
	if arr.interp == nil || arr.interp.vm != main.vm {
		t.Fatalf("array runtime = %p, want main %p", arr.interp, main.vm)
	}
	// 元素属性可读回
	v := arr.obj.Get("0")
	if v == nil {
		t.Fatal("array[0] missing")
	}
	elemObj, isObj := v.(*goja.Object)
	if !isObj || elemObj != obj.obj {
		t.Fatalf("array[0] = %v, want original object %v", v, obj.obj)
	}
	if tag := elemObj.Get("tag"); tag == nil || tag.String() != "span" {
		t.Fatalf("array[0].tag = %v, want span", tag)
	}
}

// TestNewArrayNilProtoMixedValues 混合原始值与绑定对象也不崩。
func TestNewArrayNilProtoMixedValues(t *testing.T) {
	main := NewInterpreter()
	obj := NewObject(main.ObjectPrototype())
	arr := NewArray(nil, []JSValue{StringValue("a"), NumberValue(2), ObjectValue(obj)})
	if arr == nil {
		t.Fatal("NewArray returned nil")
	}
	if arr.interp == nil || arr.interp.vm != main.vm {
		t.Fatalf("array runtime = %p, want main %p", arr.interp, main.vm)
	}
}

// TestNewArrayNilProtoPrimitivesOnly 纯原始值仍创建独立数组（无绑定对象
// 可复用时的历史行为）。
func TestNewArrayNilProtoPrimitivesOnly(t *testing.T) {
	arr := NewArray(nil, []JSValue{StringValue("x"), NumberValue(1)})
	if arr == nil || arr.obj == nil {
		t.Fatal("NewArray returned nil")
	}
	if n := arr.obj.Get("length"); n == nil || n.ToInteger() != 2 {
		t.Fatalf("array length = %v, want 2", n)
	}
}
