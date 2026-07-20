package jsenv

import (
	"fmt"
	"testing"
)

func TestMissingES6(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "ClassFields", code: `class A { x = 42 }; new A().x`},
		{name: "StaticClassFields", code: `class A { static x = 42 }; A.x`},
		{name: "PrivateFields", code: `class A { #x = 42; getX() { return this.#x } }; new A().getX()`},
		{name: "NullishAssign", code: `let x; x ??= 42; x`},
		{name: "LogicalAndAssign", code: `let x = 1; x &&= 2; x`},
		{name: "LogicalOrAssign", code: `let x = 0; x ||= 42; x`},
		{name: "Array.at", code: `[1,2,3].at(-1)`},
		{name: "String.at", code: `'hello'.at(-1)`},
		{name: "Object.hasOwn", code: `Object.hasOwn({a:1}, 'a')`},
		{name: "Array.flatMap", code: `[1,2].flatMap(x => [x, x*2]).length`},
		{name: "String.trimEnd", code: `'hello  '.trimEnd()`},
		{name: "String.trimStart", code: `'  hello'.trimStart()`},
		{name: "Promise.allSettled", code: `Promise.allSettled([1,2,3])`},
		{name: "Promise.any", code: `Promise.any([1,2,3])`},
		{name: "globalThis", code: `globalThis === (function() { return this; })()`},
		{name: "nullishCoalescing", code: `null ?? 42`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine()
			got, err := e.Run(tt.code)
			if err != nil {
				fmt.Printf("  ❌ %s: %v (需要改造)\n", tt.name, err)
			} else {
				fmt.Printf("  ✅ %s = %s (已支持)\n", tt.name, got)
			}
		})
	}
}
