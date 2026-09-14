// engine/js/bindings/canvasfont.go — canvas 2D ctx.font 简写解析（__wbMeasureText 用）。
// 支持浏览器 canvas 常见格式："13px Consolas, monospace"、
// "bold 13px 'Cascadia Mono'"、"italic 700 13px serif"。
package bindings

import (
	"strconv"
	"strings"
)

// parseCanvasFontSpec 解析 canvas ctx.font 简写为 (family, size, weight,
// style)。无法解析 px 字号时回退 10px sans-serif（canvas 默认）。
func parseCanvasFontSpec(spec string) (family string, size float64, weight int, style string) {
	family, size, weight, style = "sans-serif", 10, 400, "normal"
	s := strings.TrimSpace(spec)
	if s == "" {
		return
	}
	// 剥离前导 weight/style 关键字（canvas 语法：style weight size family）。
	tokens := strings.Fields(s)
	i := 0
	for i < len(tokens) {
		t := strings.ToLower(tokens[i])
		switch t {
		case "italic", "oblique":
			style = t
			i++
		case "bold", "bolder":
			weight = 700
			i++
		case "normal":
			i++
		default:
			if n, err := strconv.Atoi(t); err == nil && n >= 1 && n <= 1000 {
				weight = n
				i++
			} else {
				goto familyPart
			}
		}
	}
familyPart:
	rest := strings.TrimSpace(strings.Join(tokens[i:], " "))
	// 字号 + 家族：<size>px <family> 或 <family>
	if m := len(rest); m > 0 {
		if sp := strings.IndexByte(rest, ' '); sp > 0 {
			sizeTok := rest[:sp]
			if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(sizeTok, "px"), "PX"), 64); err == nil && v > 0 {
				size = v
				family = strings.TrimSpace(rest[sp+1:])
			} else {
				family = rest
			}
		} else if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(rest, "px"), "PX"), 64); err == nil && v > 0 {
			// 只有字号无家族：保持默认家族
			size = v
		} else {
			family = rest
		}
	}
	if family == "" {
		family = "sans-serif"
	}
	return
}
