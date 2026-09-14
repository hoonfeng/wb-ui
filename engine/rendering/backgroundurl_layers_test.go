package rendering

// 多层背景 `background-image: url(a.png), url(b.png)` 的 URL 解析。
//
// 必须取**第一层**的 url()：本引擎只绘制第一层叠图，但解析不能出错——旧实现
// 用 LastIndex(")") 取最后一个右括号，多层写法会得到 `a.png), url(b.png` 这种
// 垃圾 URL，连第一层都加载不出来（多层背景在页面里很常见）。

import "testing"

func TestParseBackgroundURLMultiLayerAndQuotedParens(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{`url(a.png)`, "a.png", true},
		{`url( a.png )`, "a.png", true},
		{`url(a.png), url(b.png)`, "a.png", true}, // 多层：取第一层
		{`url(a.png), linear-gradient(red, blue)`, "a.png", true},
		{`url(a.png) center / cover no-repeat`, "a.png", true}, // 简写尾部
		{`url("a)b.png")`, "a)b.png", true},                    // 引号内的 ")" 不截断
		{`url('x y.png')`, "x y.png", true},
		{`URL(a.png)`, "a.png", true}, // 大小写不敏感（url( 前缀）
		{`linear-gradient(red,blue)`, "", false},
		{`none`, "", false},
		{``, "", false},
	} {
		got, ok := parseBackgroundURL(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseBackgroundURL(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
