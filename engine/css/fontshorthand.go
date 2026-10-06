package css

import (
	"regexp"
	"strconv"
	"strings"
)

// fontSizeTokenRe 匹配 font 简写里的字号 token：13px、13px/1.4、12pt/1.5em、
// 12px/normal 以及以斜杠收尾的 12px/ 等。
//
// ★ `/normal`（以及 thin/thick 之外的关键字形式）必须被捕获：此前 line-height
// 组只允许「数字 + 可选单位」，`font: 12px/normal Arial` 整个 token 匹配失败
// → 简写不展开、size 丢失、family 被解析成 `normal "Arial"`（把 line-height
// 关键字一起吞进字体名）→ 字体族无从匹配、退回默认 typeface，行高与字宽全错
// （font-metric-line-height 夹具：期望 Arial 网格对齐行高 14px，实测按默认
// 字体度量 16px，整列 marker 下移 20px）。
// 允许斜杠后为空（`12px/` 与后随独立 token 的情形在 ParseFontShorthand 里接续）。
var fontSizeTokenRe = regexp.MustCompile(`^([0-9]*\.?[0-9]+(?:px|em|rem|pt|%|vh|vw|vmin|vmax))(?:/(normal|[0-9]*\.?[0-9]*(?:px|em|rem|pt|%)?))?$`)

// ParseFontShorthand 解析 CSS font 简写（浏览器标准）：
//
//	font: [ <font-style> || <font-variant> || <font-weight> || <font-stretch> ]?
//	      <font-size> [ / <line-height> ]? <font-family>
//
// 返回展开的 (style, variant, weight, size, lineHeight, family)。family 可含
// 空格（如 "Times New Roman"），取 size 后的剩余部分；关键字按标准归类。
//
// ★ 之所以放在 css 包而不是 style 包内部，是因为有**两个**消费方：
//   - engine/style 的 applyDeclaration：展开到 ComputedStyle 的字体子属性
//     （布局与绘制读的是它）；
//   - engine/js/bindings 的 computedStyleFor：展开 getComputedStyle 的级联 map。
//     该函数此前只手工展开 padding/margin/overflow/border，**没有 font**，于是
//     `getComputedStyle(el).fontSize` 读不到简写里的字号、回退初始值 16px，而布局
//     实际用的是 24px —— 同一元素「读到的字号」与「画出来的字号」脱节。
//
// 两个消费方共用同一份实现，避免简写语义分叉（此前 style 包一份、bindings 零份）。
func ParseFontShorthand(s string) (style, variant, weight, size, lineHeight, family string, ok bool) {
	tokens := strings.Fields(s)
	if len(tokens) == 0 {
		return
	}
	// 1) 找 size token（可能含 /line-height 或后随独立 /lh token）
	sizeIdx := -1
	for i, tok := range tokens {
		if m := fontSizeTokenRe.FindStringSubmatch(tok); m != nil {
			sizeIdx = i
			size = m[1]
			if m[2] != "" {
				lineHeight = m[2]
			}
			break
		}
	}
	if sizeIdx < 0 {
		return
	}
	// 2) size 前：style / variant / weight / stretch 关键字
	for _, tok := range tokens[:sizeIdx] {
		switch strings.ToLower(tok) {
		case "italic", "oblique":
			style = strings.ToLower(tok)
		case "small-caps":
			variant = "small-caps"
		case "bold", "bolder", "lighter":
			weight = strings.ToLower(tok)
		case "normal":
			// normal 既可能是 font-style 也可能是 font-weight，取缺省
			if style == "" {
				style = "normal"
			}
		default:
			if n, err := strconv.Atoi(tok); err == nil && n >= 100 && n <= 900 && n%100 == 0 {
				weight = tok
			}
		}
	}
	// 3) size 后：独立 /lh token 或 family
	rest := tokens[sizeIdx+1:]
	if lineHeight == "" && len(rest) > 0 {
		switch {
		case rest[0] == "/":
			// `font: 12px / normal Arial`：斜杠独立成 token，行高是下一个 token。
			if len(rest) > 1 {
				lineHeight = rest[1]
				rest = rest[2:]
			} else {
				rest = rest[1:]
			}
		case strings.HasPrefix(rest[0], "/"):
			// `font: 12px/1.4 Arial` 或 `font: 12px/ normal Arial`（斜杠粘在
			// 字号 token 尾部且其后另有 token）。
			lineHeight = strings.TrimPrefix(rest[0], "/")
			rest = rest[1:]
			if lineHeight == "" && len(rest) > 0 {
				lineHeight = rest[0]
				rest = rest[1:]
			}
		}
	}
	family = strings.Join(rest, " ")
	ok = true
	return
}
