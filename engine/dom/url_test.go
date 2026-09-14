package dom

import "testing"

// TestResolveURL 锁定 document.baseURI 语义的相对引用解析：绝对引用与宿主
// 自定义 scheme 原样返回、根相对与文档相对按 base 解析、协议相对补 scheme、
// 无 base / 非绝对 base 时原样返回（调用方据此保持既有行为）。
func TestResolveURL(t *testing.T) {
	const base = "http://example.com/dir/page.html"
	cases := []struct {
		name string
		base string
		ref  string
		want string
	}{
		{"根相对", base, "/style.css", "http://example.com/style.css"},
		{"文档相对", base, "app.js", "http://example.com/dir/app.js"},
		{"上层相对", base, "../up.css", "http://example.com/up.css"},
		{"协议相对", base, "//cdn.example.org/x.css", "http://cdn.example.org/x.css"},
		{"绝对 http 原样", base, "https://s.example.org/x.css", "https://s.example.org/x.css"},
		{"data URL 原样", base, "data:text/css,x", "data:text/css,x"},
		{"宿主逻辑名原样", base, "app://theme.css", "app://theme.css"},
		{"file 绝对原样", base, "file:///C:/x.css", "file:///C:/x.css"},
		{"无 base 原样", "", "app.js", "app.js"},
		{"非绝对 base 原样", "not-a-url", "app.js", "app.js"},
		{"空引用原样", base, "", ""},
		{"带查询与锚点", base, "x.css?v=1#f", "http://example.com/dir/x.css?v=1#f"},
	}
	for _, tc := range cases {
		if got := ResolveURL(tc.base, tc.ref); got != tc.want {
			t.Errorf("%s: ResolveURL(%q, %q) = %q, want %q", tc.name, tc.base, tc.ref, got, tc.want)
		}
	}
}
