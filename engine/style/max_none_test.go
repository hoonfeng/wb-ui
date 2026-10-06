package style

// ★ 回归测试：max-width / max-height 的 `none` 关键字。
//
// 背景（AI-PS 响应式缺陷，2026-10 定位）：
//   设计工程产物给 SFC scoped 选择器写了 `.screen-main[data-v-*]{max-width:1440px}`，
//   覆盖层用 `#app .screen-main{max-width:none}` 解除该约束（ID 特异性更高）。
//   浏览器 computed = none；wb-ui 引擎 computed = 1440px。
//
//   原因：parseLength 只识别 auto / fit-content / min-content / max-content，
//   `none` 落到末尾 `return Length{}, false` → `max-width:none` 声明被**静默丢弃** →
//   层叠回落到低优先级的 scoped 规则。而语义侧早已预留
//   （layout.resolveMinMax 把 `maxL.Unit == "none"` 视作无上限 maxAuto=true），
//   缺的只是解析层产出该 Unit。
//
// 后果（本案）：flex 容器的交叉轴 clamp（flexformattingcontext.go 的 cw 计算）
//   按 max-width 收缩子项基准，而 item 自身几何未收缩 → 容器几何 1920、
//   子项 `width:100%` = 1440（右侧留白 480），且 `align-self:stretch` 一并失效。

import (
	"testing"

	"wb-ui/engine/dom"
)

// scopedTree 构造真实场景的 DOM：<div id="app"><main class="screen-main" data-v-x></main></div>
func scopedTree(t *testing.T) (*dom.Document, *dom.Element) {
	t.Helper()
	doc := dom.NewDocument()
	app := dom.NewElement(doc, "div")
	app.SetId("app")
	el := dom.NewElement(doc, "main")
	el.SetAttribute("class", "screen-main")
	el.SetAttribute("data-v-f95a577b", "")
	if err := app.AppendChild(el); err != nil {
		t.Fatal(err)
	}
	return doc, el
}

// TestResolver_MaxWidthNoneOverridesScoped 是本案的直接回归：ID 特异性的
// `max-width:none` 必须胜过类+属性选择器的 `max-width:1440px`。
func TestResolver_MaxWidthNoneOverridesScoped(t *testing.T) {
	_, el := scopedTree(t)

	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `
.screen-main[data-v-f95a577b]{width:100%;max-width:1440px}
#app .screen-main{width:100%;max-width:none}`))
	cs := r.ResolveElement(el)
	if cs.MaxWidth.Unit != "none" {
		t.Fatalf("max-width=%v (Unit=%q) want none：`#app .screen-main{max-width:none}` 未生效（none 被当作非法值丢弃 → 回落 scoped 的 1440px）",
			cs.MaxWidth, cs.MaxWidth.Unit)
	}
}

// TestResolver_MaxWidthIDSpecificityWithLength 对照用例：把覆盖层改成有效长度，
// 确认层叠（ID > 类+属性）本身没问题 —— 隔离出「none 解析缺失」这一根因。
func TestResolver_MaxWidthIDSpecificityWithLength(t *testing.T) {
	_, el := scopedTree(t)

	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `
.screen-main[data-v-f95a577b]{max-width:1440px}
#app .screen-main{max-width:2000px}`))
	cs := r.ResolveElement(el)
	if cs.MaxWidth.Value != 2000 {
		t.Fatalf("max-width=%v want 2000px（#app 规则应凭 ID 特异性胜出）", cs.MaxWidth)
	}
}

// TestResolver_MaxHeightNone：高度方向同款（`max-height:none` 也必须可解析）。
func TestResolver_MaxHeightNone(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")

	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div{max-height:300px} div{max-height:none}"))
	cs := r.ResolveElement(el)
	if cs.MaxHeight.Unit != "none" {
		t.Fatalf("max-height=%v (Unit=%q) want none", cs.MaxHeight, cs.MaxHeight.Unit)
	}
}
