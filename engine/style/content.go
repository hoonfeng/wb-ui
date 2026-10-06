package style

import "strings"

// ParseContentText 把 `content` 声明的 CSSOM 序列化值解析成**实际绘制的文本**。
//
// 背景：ComputedStyle.Content 保存的是 CSSOM 序列化形态（css.serializeToken
// 对 TokenString 重新加引号），因此 `content: '▸'` 在 ComputedStyle 里是
// `"▸"`。直接当成文本会连引号一起画出来（桌面端实测渲染树里出现文本
// `"▸"`）——必须解析。
//
// 语义（CSS Content §4 / CSS 2.1 §12.1）：content 由若干组件值组成，只有
// 「字符串」贡献文本（相邻字符串拼接，不插空格）。其余组件值
// （none / normal / url(…) / counter(…) / attr(…) / open-quote 等）本函数
// 返回空串（不绘制文本），与浏览器「无文本内容」的可见结果一致。
//
// 字符串内的 CSS 转义照规范解析（"consume an escaped code point"）：`\` 后
// 1~6 位十六进制 + 可选空白 → 该码点；其余情况 `\x` → 字面 x。
func ParseContentText(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	switch strings.ToLower(v) {
	case "none", "normal":
		return ""
	}
	var sb strings.Builder
	i := 0
	for i < len(v) {
		switch c := v[i]; {
		case c == '"' || c == '\'':
			s, next, ok := parseContentString(v, i)
			if !ok {
				// 未闭合的字符串：规范把剩余输入当作字符串内容的一部分，
				// 这里以「到此为止」处理（不把残料画到界面上）。
				return sb.String()
			}
			sb.WriteString(s)
			i = next
		case isContentNameByte(c):
			// 标识符 / 函数（url(…) / counter(…) / attr(…)）：整段跳过。
			j := i
			for j < len(v) && isContentNameByte(v[j]) {
				j++
			}
			if j < len(v) && v[j] == '(' {
				j = skipBalancedParens(v, j)
			}
			i = j
		default:
			i++
		}
	}
	return sb.String()
}

// parseContentString 解析从 i（指向引号）开始的 CSS 字符串字面量，返回
// （解码后的文本, 结束位置, 是否闭合）。
func parseContentString(s string, i int) (string, int, bool) {
	quote := s[i]
	var sb strings.Builder
	j := i + 1
	for j < len(s) {
		c := s[j]
		if c == quote {
			return sb.String(), j + 1, true
		}
		if c == '\\' {
			j++
			if j >= len(s) {
				break
			}
			if s[j] == '\n' {
				// 反斜杠 + 换行 = 续行（不产生字符）。
				j++
				continue
			}
			if isHexDigitByte(s[j]) {
				cp := 0
				n := 0
				for n < 6 && j+n < len(s) && isHexDigitByte(s[j+n]) {
					cp = cp*16 + hexValue(s[j+n])
					n++
				}
				j += n
				// 十六进制转义后可跟一个空白终止符（消费掉）。
				if j < len(s) && (s[j] == ' ' || s[j] == '\t') {
					j++
				}
				if cp > 0 && cp <= 0x10FFFF {
					sb.WriteRune(rune(cp))
				}
				continue
			}
			// 其它转义：字面该字符（含 `\"`、`\\`）。
			sb.WriteByte(s[j])
			j++
			continue
		}
		sb.WriteByte(c)
		j++
	}
	return sb.String(), j, false
}

// skipBalancedParens 从 s[i]=='(' 起跳到配对右括号之后（含嵌套；未闭合时返回 len(s)）。
func skipBalancedParens(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return len(s)
}

func isHexDigitByte(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// isContentNameByte 报告字节是否可能属于 CSS 标识符（含转义起始 `\` 与
// ≥0x80 的多字节 UTF-8 字节）。content 里只有「标识符 / 函数」这类非字符串
// 组件需要整体跳过，不需要完整的 name-code-point 判定。
func isContentNameByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '\\':
		return true
	case c >= 0x80:
		return true
	}
	return false
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}
