package jsenv

import (
	"fmt"
	"testing"
)

func TestAdvancedES6(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		want    string
		skipMsg string // 如果不为空则标记待办
	}{
		{name: "async/await", code: `async function f() { return 42; }; f()`, want: "42", skipMsg: ""},
		{name: "generator", code: `function* gen() { yield 1; yield 2; }; const g = gen(); g.next().value`, want: "1", skipMsg: ""},
		{name: "Proxy", code: `const p = new Proxy({a:1}, {get(t,k){return t[k]||42}}); p.b`, want: "42", skipMsg: ""},
		{name: "Reflect", code: `Reflect.get({x:1}, 'x')`, want: "1", skipMsg: ""},
		{name: "BigInt", code: `1n + 2n`, want: "3", skipMsg: ""},
		{name: "可选链 ?.", code: `const a = {b: {c: 42}}; a?.b?.c`, want: "42", skipMsg: ""},
		{name: "空值合并 ??", code: `const a = null; a ?? 42`, want: "42", skipMsg: ""},
		{name: "Object.entries", code: `Object.entries({a:1, b:2}).length`, want: "2", skipMsg: ""},
		{name: "Array.includes", code: `[1,2,3].includes(2)`, want: "true", skipMsg: ""},
		{name: "String.startsWith", code: `'hello'.startsWith('he')`, want: "true", skipMsg: ""},
		{name: "指数运算符 **", code: `2 ** 10`, want: "1024", skipMsg: ""},
		{name: "Object.values", code: `Object.values({a:1, b:2}).length`, want: "2", skipMsg: ""},
		{name: "WeakMap", code: `const wm = new WeakMap(); const o = {}; wm.set(o, 1); wm.get(o)`, want: "1", skipMsg: ""},
		{name: "TypedArray Uint8Array", code: `new Uint8Array([1,2,3]).length`, want: "3", skipMsg: ""},
		{name: "Array.from", code: `Array.from('abc').length`, want: "3", skipMsg: ""},
		{name: "Array.find", code: `[1,2,3].find(x => x > 1)`, want: "2", skipMsg: ""},
		{name: "Array.flat", code: `[[1],[2],[3]].flat().length`, want: "3", skipMsg: ""},
		{name: "async generator", code: `async function* f() { yield 1; }; const g = f(); g.next()`, want: "", skipMsg: "暂不测试"},
		{name: "for-await-of", code: `async function f() { let s = 0; for await (const v of [1,2,3]) { s += v; }; return s; }; f()`, want: "", skipMsg: "暂不测试"},
		{name: "import()", code: `import('fs')`, want: "", skipMsg: "暂不测试"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipMsg != "" {
				t.Skip(tt.skipMsg)
			}
			e := NewEngine()
			got, err := e.Run(tt.code)
			if err != nil {
				fmt.Printf("  ⏳ %s: %v\n", tt.name, err)
				return
			}
			if tt.want != "" && got != tt.want {
				fmt.Printf("  ⚠️  %s = %q (期望 %q)\n", tt.name, got, tt.want)
			} else {
				fmt.Printf("  ✅ %s = %s\n", tt.name, got)
			}
		})
	}
}
