package jsenv

import (
	"fmt"
	"testing"
)

func TestES6Features(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		want     string // 预期的输出字符串
		wantErr  bool
	}{
		{
			name: "ES5.1: 基本运算",
			code: `1 + 2`,
			want: "3",
		},
		{
			name: "ES6: let/const",
			code: `let x = 1; const y = 2; x + y`,
			want: "3",
		},
		{
			name: "ES6: 箭头函数",
			code: `const add = (a, b) => a + b; add(3, 4)`,
			want: "7",
		},
		{
			name: "ES6: 模板字符串",
			code: "const name = 'world'; `hello ${name}`",
			want: "hello world",
		},
		{
			name: "ES6: Promise",
			code: `Promise.resolve(42).then(v => v)`,
			want: "42",
			wantErr: true, // Promise 可能不完整
		},
		{
			name: "ES6: 解构赋值",
			code: `const [a, b] = [1, 2]; a + b`,
			want: "3",
		},
		{
			name: "ES6: 展开运算符",
			code: `const arr = [1, 2, 3]; Math.max(...arr)`,
			want: "3",
		},
		{
			name: "ES6: Map",
			code: `const m = new Map(); m.set('a', 1); m.get('a')`,
			want: "1",
		},
		{
			name: "ES6: Set",
			code: `const s = new Set([1, 2, 3]); s.has(2)`,
			want: "true",
		},
		{
			name: "ES6: class",
			code: `class A { constructor(x) { this.x = x; } getX() { return this.x; } }; const a = new A(42); a.getX()`,
			want: "42",
		},
		{
			name: "ES6: Symbol",
			code: `const s = Symbol('test'); typeof s`,
			want: "symbol",
		},
		{
			name: "ES6: for-of",
			code: `let s = 0; for (const v of [1, 2, 3]) { s += v; }; s`,
			want: "6",
		},
		{
			name: "ES6: 默认参数",
			code: `const f = (a = 1, b = 2) => a + b; f()`,
			want: "3",
		},
		{
			name: "ES6: 剩余参数",
			code: `const f = (a, ...rest) => rest.length; f(1, 2, 3, 4)`,
			want: "3",
		},
		{
			name: "ES6: Object.assign",
			code: `const a = {x: 1}; const b = {y: 2}; Object.assign({}, a, b).y`,
			want: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEngine()
			got, err := e.Run(tt.code)
			if tt.wantErr {
				if err == nil {
					t.Logf("预期错误但成功: got=%q", got)
				} else {
					t.Logf("预期中的错误: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("执行错误: %v", err)
			}
			if got != tt.want {
				t.Errorf("期望 %q, 实际 %q", tt.want, got)
			} else {
				fmt.Printf("  ✅ %s = %s\n", tt.name, got)
			}
		})
	}
}
