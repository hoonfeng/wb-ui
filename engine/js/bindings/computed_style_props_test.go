package bindings

import (
	"strings"
	"testing"
)

// legacyWhitelistProps 是 getComputedStyle 白名单的「展开形式」——用于等价性
// 断言与基准对照。它与包级预计算表共享同一数据源，因此断言的是「表」这一
// 中间层没有引入偏差（成员/顺序/kebab 键推导）。
var legacyWhitelistProps = strings.Split(computedStylePropWhitelistCSV, ",")

// TestComputedStylePropEntriesConsistent 断言包级预计算表与优化前的
//「函数内构造 []string + 逐项 camelToKebab」逐项等价。
//
// 背景：原先 getComputedStyle 每次调用都在函数内构造 117 元素切片并对每项跑
// camelToKebab；优化把两者提到包级一次算完。这个测试保证提速没有改变语义
// ——成员、顺序（含 backgroundRepeat/backgroundPosition/backgroundSize 三个
// 重复项）与键推导必须逐项一致，否则会出现「属性静默不再回写」这类只有在
// 真实页面上才暴露的退化。
func TestComputedStylePropEntriesConsistent(t *testing.T) {
	if len(computedStylePropEntries) != len(legacyWhitelistProps) {
		t.Fatalf("预计算表项数 = %d，期望 %d", len(computedStylePropEntries), len(legacyWhitelistProps))
	}
	for i, e := range computedStylePropEntries {
		if e.prop != legacyWhitelistProps[i] {
			t.Fatalf("第 %d 项 = %q，期望 %q（顺序/成员漂移）", i, e.prop, legacyWhitelistProps[i])
		}
		wantKey := e.prop
		if k := camelToKebab(e.prop); k != e.prop {
			wantKey = k
		}
		if e.key != wantKey {
			t.Fatalf("%s 的键 = %q，期望 %q", e.prop, e.key, wantKey)
		}
	}
	// 抽查键推导（多词属性必须转 kebab、单词属性保持原样）：
	for _, tc := range []struct{ prop, key string }{
		{"color", "color"},
		{"backgroundColor", "background-color"},
		{"paddingTop", "padding-top"},
		{"borderTopWidth", "border-top-width"},
		{"zIndex", "z-index"},
		{"gridTemplateColumns", "grid-template-columns"},
	} {
		found := false
		for _, e := range computedStylePropEntries {
			if e.prop == tc.prop {
				found = true
				if e.key != tc.key {
					t.Fatalf("%s 的键 = %q，期望 %q", tc.prop, e.key, tc.key)
				}
			}
		}
		if !found {
			t.Fatalf("白名单缺少 %s", tc.prop)
		}
	}
}

// BenchmarkComputedStyleWhitelistLegacy 复现优化前的**每次调用固定开销**：
// 构造切片（用 strings.Split 模拟，偏保守地与被移除的字面量同量级）+ 逐项
// camelToKebab。
func BenchmarkComputedStyleWhitelistLegacy(b *testing.B) {
	sink := 0
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		props := strings.Split(computedStylePropWhitelistCSV, ",")
		for _, prop := range props {
			key := prop
			if k := camelToKebab(prop); k != prop {
				key = k
			}
			sink += len(key)
		}
	}
	_ = sink
}

// BenchmarkComputedStyleWhitelistPrecomputed 是优化后的每次调用开销：
// 只遍历预计算表（真实路径还含 map 查找与 cs.Set，不在本基准内）。
func BenchmarkComputedStyleWhitelistPrecomputed(b *testing.B) {
	sink := 0
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, e := range computedStylePropEntries {
			sink += len(e.key)
		}
	}
	_ = sink
}
