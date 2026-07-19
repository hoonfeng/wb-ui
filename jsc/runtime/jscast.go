// Translation of: Source/JavaScriptCore/runtime/JSCast.h
//
// Type-safe downcasting helpers mirroring jsCast<T>, jsDynamicCast<T>.
// Go port uses JSType-range checks + unsafe pointer conversion.

package runtime

import "unsafe"

// JSTypeRange represents a contiguous range of JSType values (inclusive).
type JSTypeRange struct {
	First JSType
	Last  JSType
}

// Contains returns true if type falls within [First, Last].
func (r JSTypeRange) Contains(t JSType) bool {
	return r.First <= t && t <= r.Last
}

// castJSCell performs an unsafe but deterministic cast from *JSCell to *T.
// T must embed JSCell as its first field (guaranteed by all JSC types).
func castJSCell[T any](cell *JSCell) *T {
	if cell == nil {
		return nil
	}
	return (*T)(unsafe.Pointer(cell))
}

// --- jsCast<T> equivalents (unchecked, asserts type) ---

func AsJSCell(obj *JSObject) *JSCell       { return &obj.JSCell }
func AsObject(cell *JSCell) *JSObject      { return castJSCell[JSObject](cell) }
func AsArray(cell *JSCell) *JSArray        { return castJSCell[JSArray](cell) }
func AsFunction(cell *JSCell) *JSFunction  { return castJSCell[JSFunction](cell) }
func AsString(cell *JSCell) *JSString      { return castJSCell[JSString](cell) }
func AsSymbol(cell *JSCell) *Symbol        { return castJSCell[Symbol](cell) }
func AsBigInt(cell *JSCell) *JSBigInt      { return castJSCell[JSBigInt](cell) }
func AsPromise(cell *JSCell) *JSPromise     { return castJSCell[JSPromise](cell) }
func AsProxy(cell *JSCell) *ProxyObject    { return castJSCell[ProxyObject](cell) }
func AsMap(cell *JSCell) *JSMap            { return castJSCell[JSMap](cell) }
func AsSet(cell *JSCell) *JSSet            { return castJSCell[JSSet](cell) }
func AsGlobalObject(cell *JSCell) *JSGlobalObject  { return castJSCell[JSGlobalObject](cell) }
func AsGetterSetter(cell *JSCell) *GetterSetter    { return castJSCell[GetterSetter](cell) }
func AsBooleanObject(cell *JSCell) *BooleanObject  { return castJSCell[BooleanObject](cell) }
func AsNumberObject(cell *JSCell) *NumberObject    { return castJSCell[NumberObject](cell) }
func AsStringObject(cell *JSCell) *StringObject    { return castJSCell[StringObject](cell) }
func AsErrorInstance(cell *JSCell) *ErrorInstance   { return castJSCell[ErrorInstance](cell) }
func AsRegExpObject(cell *JSCell) *RegExpObject    { return castJSCell[RegExpObject](cell) }
func AsDateInstance(cell *JSCell) *DateInstance     { return castJSCell[DateInstance](cell) }

// --- jsDynamicCast<T> equivalents (checked, nil on failure) ---

// DynamicCastObject performs a checked downcast to JSObject.
func DynamicCastObject(cell *JSCell) *JSObject {
	if cell == nil || !cell.IsObject() {
		return nil
	}
	return AsObject(cell)
}

// DynamicCastArray performs a checked downcast to JSArray.
func DynamicCastArray(cell *JSCell) *JSArray {
	if cell == nil {
		return nil
	}
	t := cell.Type()
	if t != ArrayType && t != DerivedArrayType {
		return nil
	}
	return AsArray(cell)
}

// DynamicCastFunction performs a checked downcast to JSFunction.
func DynamicCastFunction(cell *JSCell) *JSFunction {
	if cell == nil || cell.Type() != JSFunctionType {
		return nil
	}
	return AsFunction(cell)
}

// DynamicCastString performs a checked downcast to JSString.
func DynamicCastString(cell *JSCell) *JSString {
	if cell == nil || cell.Type() != StringType {
		return nil
	}
	return AsString(cell)
}

// DynamicCastSymbol performs a checked downcast to Symbol.
func DynamicCastSymbol(cell *JSCell) *Symbol {
	if cell == nil || cell.Type() != SymbolType {
		return nil
	}
	return AsSymbol(cell)
}

// DynamicCastBigInt performs a checked downcast to JSBigInt.
func DynamicCastBigInt(cell *JSCell) *JSBigInt {
	if cell == nil || cell.Type() != HeapBigIntType {
		return nil
	}
	return AsBigInt(cell)
}

// DynamicCastPromise performs a checked downcast to JSPromise.
func DynamicCastPromise(cell *JSCell) *JSPromise {
	if cell == nil || cell.Type() != JSPromiseType {
		return nil
	}
	return AsPromise(cell)
}

// DynamicCastProxy performs a checked downcast to ProxyObject.
func DynamicCastProxy(cell *JSCell) *ProxyObject {
	if cell == nil || cell.Type() != ProxyObjectType {
		return nil
	}
	return AsProxy(cell)
}

// DynamicCastMap performs a checked downcast to JSMap.
func DynamicCastMap(cell *JSCell) *JSMap {
	if cell == nil || cell.Type() != JSMapType {
		return nil
	}
	return AsMap(cell)
}

// DynamicCastSet performs a checked downcast to JSSet.
func DynamicCastSet(cell *JSCell) *JSSet {
	if cell == nil || cell.Type() != JSSetType {
		return nil
	}
	return AsSet(cell)
}

// DynamicCastGlobalObject performs a checked downcast to JSGlobalObject.
func DynamicCastGlobalObject(cell *JSCell) *JSGlobalObject {
	if cell == nil || cell.Type() != GlobalObjectType {
		return nil
	}
	return AsGlobalObject(cell)
}

// DynamicCastGetterSetter performs a checked downcast to GetterSetter.
func DynamicCastGetterSetter(cell *JSCell) *GetterSetter {
	if cell == nil || cell.Type() != GetterSetterType {
		return nil
	}
	return AsGetterSetter(cell)
}
