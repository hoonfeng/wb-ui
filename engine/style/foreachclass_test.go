// forEachClass 的语义等价性测试。
//
// 背景：candidates 热路径原来用 strings.Fields(className) 切分类名，每次调用
// 都分配一个 []string。改成零分配的 forEachClass 之后，必须保证切分结果与
// strings.Fields **逐字节一致**——否则会静默改变 CSS class 匹配（漏匹配或
// 多匹配），那是很难察觉的样式差异。

package style

import (
	"strings"
	"testing"
)

func collectClasses(className string) []string {
	var got []string
	forEachClass(className, func(c string) { got = append(got, c) })
	return got
}

// TestForEachClassMatchesStringsFields 对一批边界输入逐一比对
// forEachClass 与 strings.Fields 的结果。
func TestForEachClassMatchesStringsFields(t *testing.T) {
	cases := []string{
		"",
		"a",
		"a b c",
		"  leading and trailing  ",
		"\t\n\v\f\r mixed \t whitespace \n",
		"a  b",             // 连续空白
		"one-two_three",    // 非空白分隔符不得被切分
		"a.b",              // 点号不是分隔符（class 名里不允许，但语义必须一致）
		"多字节 class 中",     // 非 ASCII 内容
		"a\u00a0b",         // U+00A0 不换行空格：Fields 视为空白
		"caf\u00e9 other",  // 带重音的类名
		"\u2028line\u2029sep", // unicode 行/段分隔符
	}
	for _, in := range cases {
		want := strings.Fields(in)
		got := collectClasses(in)
		if len(want) != len(got) {
			t.Errorf("forEachClass(%q) = %q，strings.Fields = %q（段数不一致）", in, got, want)
			continue
		}
		for i := range want {
			if want[i] != got[i] {
				t.Errorf("forEachClass(%q) = %q，strings.Fields = %q（第 %d 段不同）", in, got, want, i)
			}
		}
	}
}

// TestForEachClassReturnsSubstrings 验证回调拿到的类名是原串的子串切片
// （零分配路径的关键：查 byClass 表时直接用子串，不做任何拼接）。
func TestForEachClassReturnsSubstrings(t *testing.T) {
	const cn = "alpha beta gamma"
	var got []string
	forEachClass(cn, func(c string) {
		if !strings.Contains(cn, c) {
			t.Errorf("类名 %q 不是原 className 的子串", c)
		}
		got = append(got, c)
	})
	if len(got) != 3 || got[0] != "alpha" || got[1] != "beta" || got[2] != "gamma" {
		t.Errorf("切分结果 = %q，期望 [alpha beta gamma]", got)
	}
}
