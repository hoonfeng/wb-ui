// jsc/lazy.go — 惰性属性对象：基于 vendored goja 的 DynamicObject 机制，
// 属性在首次访问时经 LazyPropSet 物化。用于 DOM 元素包装器
// （bindings.wrapElement）：每个元素不再一次性安装 ~200 个自有属性
// （每元素 ~60µs 的 SetAccessor/newNativeFunc 开销 + GC 压力），改为
// createElement 只付对象+Internal 成本，属性按需创建。
package jsc

import "wb-ui.com/goja"

// LazyPropSet 是惰性对象的属性供应器。
type LazyPropSet interface {
	// Get 返回属性值（含 accessor 的即时求值）。缺失返回 Undefined()。
	Get(key string) JSValue
	// Set 处理赋值。返回 true 表示已处理（含 getter-only 属性的静默忽略）。
	Set(key string, v JSValue) bool
	Has(key string) bool
	Delete(key string) bool
	// Keys 返回全部已知属性名（for-in / Object.keys 用）。
	Keys() []string
}

// LazyLiveProps 可选接口：Live(key)==true 的属性是「活值」（accessor），
// 每次读取必须重新求值，适配器不得缓存（DOM live 语义：firstChild/
// parentNode 随树变化——缓存的旧值会让 Vue 的 insertStaticContent 循环
// 永远拿到同一个 firstChild 死循环）。不实现该接口则全部缓存。
type LazyLiveProps interface {
	Live(key string) bool
}

// lazyGojaAdapter 把 LazyPropSet 适配为 goja.DynamicObject，并缓存已物化
// 的 goja.Value（保证函数同一性：el.appendChild === el.appendChild）。
type lazyGojaAdapter struct {
	interp *Interpreter
	h      LazyPropSet
	cache  map[string]goja.Value
}

func (a *lazyGojaAdapter) get(key string) goja.Value {
	if v, ok := a.cache[key]; ok {
		return v
	}
	jv := a.h.Get(key)
	if jv.IsUndefined() {
		return nil
	}
	gv := jv.val(a.interp.VM())
	// ★ live 属性（accessor）不缓存：每次读取重新求值。
	if lp, ok := a.h.(LazyLiveProps); ok && lp.Live(key) {
		return gv
	}
	a.cache[key] = gv
	return gv
}

func (a *lazyGojaAdapter) Get(key string) goja.Value { return a.get(key) }

func (a *lazyGojaAdapter) Set(key string, val goja.Value) bool {
	ok := a.h.Set(key, JSValue{v: val, interp: a.interp})
	if ok {
		delete(a.cache, key)
	}
	return ok
}

func (a *lazyGojaAdapter) Has(key string) bool  { return a.h.Has(key) }
func (a *lazyGojaAdapter) Delete(key string) bool {
	ok := a.h.Delete(key)
	if ok {
		delete(a.cache, key)
	}
	return ok
}
func (a *lazyGojaAdapter) Keys() []string { return a.h.Keys() }

// NewLazyObject 创建由 h 供属性的惰性 JS 对象。proto 非 nil 时设置为对象
// 原型（Element.prototype 等）。SetInternal 照常可用（goja Object.Internal
// 独立于 dynamic 属性机制）。
func NewLazyObject(interp *Interpreter, proto *JSObject, h LazyPropSet) *JSObject {
	if interp == nil {
		return NewObject(proto)
	}
	ad := &lazyGojaAdapter{interp: interp, h: h, cache: map[string]goja.Value{}}
	dyn := interp.VM().NewDynamicObject(ad)
	obj := &JSObject{obj: dyn, interp: interp}
	if proto != nil && proto.obj != nil {
		// 用 goja 官方 SetPrototype API（dynamic object 的 __proto__ 赋值
		// 走 foreign set 语义，在 handler.Has 返回 false 时能生效；但显式
		// API 更可靠，不依赖 Object.prototype.__proto__ setter 链）。
		_ = obj.obj.SetPrototype(proto.obj)
	}
	return obj
}
