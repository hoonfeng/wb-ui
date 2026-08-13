package jsc

import (
	"strings"
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

// TestRunJSCompileCache 验证大脚本走编译缓存：同一段 ≥64KB 脚本执行两次，
// 语义一致且缓存中只有一个条目（第二次命中缓存，不再重新 Compile）。
func TestRunJSCompileCache(t *testing.T) {
	rt := NewInterpreter()
	const n = 40000
	big := "var __big=[" + strings.Repeat("1,", n-1) + "1];"
	if len(big) < progCacheMinLen {
		t.Fatalf("test script too small: %d < %d", len(big), progCacheMinLen)
	}
	for i := 0; i < 2; i++ {
		if _, err := rt.RunJS(big); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	rt.progMu.Lock()
	got := len(rt.progCache)
	rt.progMu.Unlock()
	if got != 1 {
		t.Fatalf("cache entries = %d, want 1", got)
	}
	// 缓存命中路径与首次编译路径语义等价：数组长度正确。
	v, err := rt.RunJS("__big.length")
	if err != nil {
		t.Fatalf("read length: %v", err)
	}
	if v.ToNumber() != float64(n) {
		t.Fatalf("__big.length = %v, want %d", v.ToNumber(), n)
	}
}

// TestRunJSNoCacheForSmall 小脚本（<64KB）不进入编译缓存，避免 hash/缓存
// 管理开销盖过 parse 收益。
func TestRunJSNoCacheForSmall(t *testing.T) {
	rt := NewInterpreter()
	if _, err := rt.RunJS("1+1"); err != nil {
		t.Fatalf("run: %v", err)
	}
	rt.progMu.Lock()
	got := len(rt.progCache)
	rt.progMu.Unlock()
	if got != 0 {
		t.Fatalf("small script cached: %d entries", got)
	}
}
