// CSS transform 值的 CSSOM 序列化。
//
// 浏览器的 getComputedStyle(el).transform **不返回**级联里的原始字符串，而是把
// transform 归一成矩阵：translate(20px,10px) → "matrix(1, 0, 0, 1, 20, 10)"。
// wbui 此前直接回写原始值，与浏览器不一致（H7；
// dev/fixtures/webshot/g2_transform.html 实测：Edge 得 matrix(...)、wbui 得
// translate(...)）。
//
// 序列化规则与 Edge 逐项实测对齐（基线 = dev/fixtures/webshot/h7_transform_norm.html
// 的 Edge 产物）：
//   - 2D：matrix(a, b, c, d, e, f)，**逗号后带一个空格**；
//   - 数字：**6 位有效数字**（0.7071067811865476 → 0.707107、tan(20°) → 0.36397、
//     tan(-5°) → 0.0874887），去尾零；
//   - **极小残差归零**：cos(90°) = 6.12e-17 → "0"，故 rotate(90deg) →
//     matrix(0, 1, -1, 0, 0, 0)；
//   - none / 空 → "none"。
//
// 长度单位的百分比参照元素 border-box 尺寸，em/ex/ch 参照元素 font-size；
// 3D 函数（translateZ/rotateX/rotateY/translate3d/scale3d…）只有在退化为 2D
// 等价（z=0、xz 角为 0、sz=1）时才归一，否则返回 ok=false 让调用方保留原值——
// 以免把 3D 变换**错报**成 2D matrix。
package css

import (
	"math"
	"strconv"
	"strings"
)

// tfMat 是 2D 仿射矩阵 [a c e; b d f; 0 0 1]：x' = a*x + c*y + e；y' = b*x + d*y + f。
// 与 engine/rendering 的同名内部类型同构；此处独立定义，因为 CSSOM 序列化属于
// 样式层，不应反向依赖渲染层。
type tfMat struct{ a, b, c, d, e, f float64 }

// tfIdent 是单位矩阵。
var tfIdent = tfMat{a: 1, d: 1}

// tfMul 右乘：M = M·N（与 transform 列表「从左到右依次作用」的语义一致）。
func (m tfMat) tfMul(n tfMat) tfMat {
	return tfMat{
		a: m.a*n.a + m.c*n.b,
		b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d,
		d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e,
		f: m.b*n.e + m.d*n.f + m.f,
	}
}

// TransformToMatrix 把 CSS transform 值按 CSSOM 归一为 "matrix(a, b, c, d, e, f)"。
// refW/refH 是元素 border-box 尺寸（百分比参照），fontSize 是元素 font-size（px，
// 供 em/ex/ch 使用）。返回 ("none", true) 表示值为 none/空；ok=false 表示该值
// 无法用 2D 矩阵表示（含真 3D 变换）或语法未识别——调用方应保留原值。
func TransformToMatrix(value string, refW, refH, fontSize float64) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return "none", true
	}
	toks := tfTokens(value)
	if len(toks) == 0 {
		return "", false
	}
	m := tfIdent
	for _, tok := range toks {
		fn, args := tfSplitFunc(tok)
		if fn == "" {
			return "", false
		}
		next, ok := tfOpMatrix(fn, args, refW, refH, fontSize)
		if !ok {
			return "", false
		}
		m = m.tfMul(next)
	}
	return tfFormatMatrix(m), true
}

// tfTokens 按顶层空白切分 transform 列表（括号内的空格不切）。
func tfTokens(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			depth++
			cur.WriteRune(r)
		case r == ')':
			depth--
			cur.WriteRune(r)
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && depth == 0:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// tfSplitFunc 把 "translate(20px, 10px)" 拆成 ("translate", ["20px","10px"])。
// 名称小写化（CSS 函数名大小写不敏感）。
func tfSplitFunc(tok string) (string, []string) {
	i := strings.IndexByte(tok, '(')
	if i < 0 {
		return "", nil
	}
	name := strings.ToLower(strings.TrimSpace(tok[:i]))
	body := tok[i+1:]
	if j := strings.LastIndexByte(body, ')'); j >= 0 {
		body = body[:j]
	}
	args := tfSplitArgs(body)
	return name, args
}

// tfSplitArgs 按逗号或空白切分参数列表。
func tfSplitArgs(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// tfOpMatrix 返回单个 transform 函数对应的矩阵；ok=false 表示无法用 2D 表示
// 或参数非法。
func tfOpMatrix(fn string, args []string, refW, refH, fontSize float64) (tfMat, bool) {
	num := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(args[i]), 64)
		return v, err == nil
	}
	length := func(i int, ref float64) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		return tfLength(strings.TrimSpace(args[i]), ref, fontSize)
	}
	angle := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		return tfAngleDeg(strings.TrimSpace(args[i]))
	}
	// rad 把角度（度）转弧度。
	rad := func(deg float64) float64 { return deg * math.Pi / 180 }

	switch fn {
	case "translate":
		tx, ok := length(0, refW)
		if !ok {
			return tfIdent, false
		}
		ty := 0.0
		if len(args) > 1 {
			if ty, ok = length(1, refH); !ok {
				return tfIdent, false
			}
		}
		return tfMat{a: 1, d: 1, e: tx, f: ty}, true
	case "translatex":
		tx, ok := length(0, refW)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: 1, d: 1, e: tx}, true
	case "translatey":
		ty, ok := length(0, refH)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: 1, d: 1, f: ty}, true
	case "translatez":
		// 纯 Z 平移对 2D 是恒等；z≠0 时本投影不再准确 → 交回原值。
		z, ok := length(0, 0)
		if !ok || z != 0 {
			return tfIdent, false
		}
		return tfIdent, true
	case "translate3d":
		tx, ok1 := length(0, refW)
		ty, ok2 := length(1, refH)
		z, ok3 := length(2, 0)
		if !ok1 || !ok2 || !ok3 || z != 0 {
			return tfIdent, false
		}
		return tfMat{a: 1, d: 1, e: tx, f: ty}, true
	case "scale":
		sx, ok := num(0)
		if !ok {
			return tfIdent, false
		}
		sy := sx
		if len(args) > 1 {
			if sy, ok = num(1); !ok {
				return tfIdent, false
			}
		}
		return tfMat{a: sx, d: sy}, true
	case "scalex":
		sx, ok := num(0)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: sx, d: 1}, true
	case "scaley":
		sy, ok := num(0)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: 1, d: sy}, true
	case "scale3d":
		sx, ok1 := num(0)
		sy, ok2 := num(1)
		sz, ok3 := num(2)
		if !ok1 || !ok2 || !ok3 || sz != 1 {
			return tfIdent, false
		}
		return tfMat{a: sx, d: sy}, true
	case "rotate", "rotatez":
		deg, ok := angle(0)
		if !ok {
			return tfIdent, false
		}
		c, s := math.Cos(rad(deg)), math.Sin(rad(deg))
		return tfMat{a: c, b: s, c: -s, d: c}, true
	case "rotatex", "rotatey":
		// 仅当角度退化为 0（identity）时才是 2D 等价。
		deg, ok := angle(0)
		if !ok || math.Abs(math.Mod(deg, 360)) > 1e-9 {
			return tfIdent, false
		}
		return tfIdent, true
	case "skew":
		ax, ok := angle(0)
		if !ok {
			return tfIdent, false
		}
		ay := 0.0
		if len(args) > 1 {
			if ay, ok = angle(1); !ok {
				return tfIdent, false
			}
		}
		return tfMat{a: 1, b: math.Tan(rad(ay)), c: math.Tan(rad(ax)), d: 1}, true
	case "skewx":
		ax, ok := angle(0)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: 1, c: math.Tan(rad(ax)), d: 1}, true
	case "skewy":
		ay, ok := angle(0)
		if !ok {
			return tfIdent, false
		}
		return tfMat{a: 1, b: math.Tan(rad(ay)), d: 1}, true
	case "matrix":
		if len(args) < 6 {
			return tfIdent, false
		}
		var v [6]float64
		for i := 0; i < 6; i++ {
			x, ok := num(i)
			if !ok {
				return tfIdent, false
			}
			v[i] = x
		}
		return tfMat{a: v[0], b: v[1], c: v[2], d: v[3], e: v[4], f: v[5]}, true
	}
	// matrix3d / perspective / rotate3d 等：本投影无法忠实表示 → 交回原值。
	return tfIdent, false
}

// tfLength 解析 transform 里的长度：px / % / em / rem / ex / ch / pt / pc / in /
// cm / mm / q，以及无单位 0。ref 是百分比参照长度，fontSize 供 em/ex/ch 使用。
func tfLength(s string, ref, fontSize float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	i := 0
	if s[i] == '+' || s[i] == '-' {
		i++
	}
	dot := false
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || (s[i] == '.' && !dot)) {
		if s[i] == '.' {
			dot = true
		}
		i++
	}
	if i == 0 || (i == 1 && (s[0] == '+' || s[0] == '-')) {
		return 0, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(s[i:])) {
	case "":
		return num, num == 0 // 无单位仅 0 合法
	case "px":
		return num, true
	case "%":
		return num * ref / 100, true
	case "em":
		return num * fontSize, true
	case "rem":
		return num * 16, true
	case "ex", "ch":
		return num * fontSize * 0.5, true
	case "pt":
		return num * 96 / 72, true
	case "pc":
		return num * 16, true
	case "in":
		return num * 96, true
	case "cm":
		return num * 96 / 2.54, true
	case "mm":
		return num * 96 / 25.4, true
	case "q":
		return num * 96 / 101.6, true
	}
	return 0, false
}

// tfAngleDeg 解析 CSS 角度并转为度。
func tfAngleDeg(s string) (float64, bool) {
	t := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasSuffix(t, "deg"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(t, "deg")), 64)
		return v, err == nil
	case strings.HasSuffix(t, "grad"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(t, "grad")), 64)
		return v * 0.9, err == nil
	case strings.HasSuffix(t, "turn"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(t, "turn")), 64)
		return v * 360, err == nil
	case strings.HasSuffix(t, "rad"):
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(t, "rad")), 64)
		return v * 180 / math.Pi, err == nil
	}
	v, err := strconv.ParseFloat(t, 64)
	return v, err == nil
}

// tfFormatMatrix 序列化为 "matrix(a, b, c, d, e, f)"。
func tfFormatMatrix(m tfMat) string {
	var b strings.Builder
	b.WriteString("matrix(")
	for i, v := range [6]float64{m.a, m.b, m.c, m.d, m.e, m.f} {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(tfFormatNum(v))
	}
	b.WriteString(")")
	return b.String()
}

// tfFormatNum 按 CSSOM 规则序列化矩阵分量：6 位有效数字、去尾零，
// 且 |v| < 1e-6 的极小残差归零。
//
// 实测基线：0.7071067811865476 → "0.707107"；tan(20°) → "0.36397"；
// tan(-5°) → "-0.0874887"；cos(90°) = 6.12e-17 → "0"。
func tfFormatNum(v float64) string {
	if math.Abs(v) < 1e-6 {
		return "0"
	}
	const sig = 6
	exp := math.Floor(math.Log10(math.Abs(v)))
	pow := math.Pow(10, float64(sig-1)-exp)
	r := math.Round(v*pow) / pow
	if r == 0 || math.Abs(r) < 1e-6 {
		return "0"
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}
