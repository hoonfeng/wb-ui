// Bounds-transform helpers: compute the axis-aligned bounding box of a CSS
// transform applied to a box rectangle (mirrors applyTransformOpsSized's
// paint-time transforms; used by getBoundingClientRect/getClientRects so
// those DOM APIs report the transformed (visual) rect per CSSOM-View §4.2 —
// a dialog centered with translate(-50%,-50%) must report its visual
// position, not the layout position).
package rendering

import "math"

// transform2D 是 2D 仿射变换（3x3 矩阵的 2D 部分：a c e / b d f / 0 0 1）。
type transform2D struct {
	a, b, c, d, e, f float64
}

func identity2D() transform2D       { return transform2D{1, 0, 0, 1, 0, 0} }
func (t transform2D) mul(o transform2D) transform2D {
	return transform2D{
		a: t.a*o.a + t.c*o.b, b: t.b*o.a + t.d*o.b,
		c: t.a*o.c + t.c*o.d, d: t.b*o.c + t.d*o.d,
		e: t.a*o.e + t.c*o.f + t.e, f: t.b*o.e + t.d*o.f + t.f,
	}
}
func (t transform2D) apply(x, y float64) (float64, float64) {
	return t.a*x + t.c*y + t.e, t.b*x + t.d*y + t.f
}

func translate2D(tx, ty float64) transform2D { return transform2D{1, 0, 0, 1, tx, ty} }
func scale2D(sx, sy float64) transform2D     { return transform2D{sx, 0, 0, sy, 0, 0} }
func rotate2D(deg float64) transform2D {
	rad := deg * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	return transform2D{cos, sin, -sin, cos, 0, 0}
}
func skew2D(ax, ay float64) transform2D {
	tx := math.Tan(ax * math.Pi / 180)
	ty := math.Tan(ay * math.Pi / 180)
	return transform2D{1, ty, tx, 1, 0, 0}
}
func matrix2D(a, b, c, d, e, f float64) transform2D {
	return transform2D{a, b, c, d, e, f}
}

// TransformRect 把 transform 应用到矩形 (x,y,w,h)，返回变换后的轴对齐
// 包围盒。refW/refH 供 translate 百分比解析（元素自身 border-box 尺寸）。
// applied=false 表示无变换或变换为恒等（矩形不变）。
// 语义与 applyTransformOpsSized（painter 正向变换）完全一致：
// 无 transform-origin 处理（绕元素原点变换）。
func TransformRect(transform string, refW, refH, x, y, w, h float64) (float64, float64, float64, float64, bool) {
	transform = trimSpace(transform)
	if transform == "" || transform == "none" {
		return x, y, w, h, false
	}
	m := identity2D()
	applied := false
	for _, tok := range tokenizeTransform(transform) {
		tok = trimSpace(tok)
		if tok == "" {
			continue
		}
		paren := indexByte(tok, '(')
		if paren < 0 || !hasSuffix(tok, ")") {
			continue
		}
		fn := toLower(tok[:paren])
		args := tok[paren+1 : len(tok)-1]
		switch fn {
		case "translatex":
			if v := parseTransformLen(args, refW); v != 0 {
				m = m.mul(translate2D(v, 0))
				applied = true
			}
		case "translatey":
			if v := parseTransformLen(args, refH); v != 0 {
				m = m.mul(translate2D(0, v))
				applied = true
			}
		case "translate":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				tx := parseTransformLen(vals[0], refW)
				ty := tx
				if len(vals) >= 2 {
					ty = parseTransformLen(vals[1], refH)
				}
				if tx != 0 || ty != 0 {
					m = m.mul(translate2D(tx, ty))
					applied = true
				}
			}
		case "translate3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				tx := parseTransformLen(vals[0], refW)
				ty := parseTransformLen(vals[1], refH)
				if tx != 0 || ty != 0 {
					m = m.mul(translate2D(tx, ty))
					applied = true
				}
			}
		case "translatez":
			// 2D 下无效果（无 perspective）
		case "scale":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseScaleValue(vals[0])
				sy := sx
				if len(vals) >= 2 {
					sy = parseScaleValue(vals[1])
				}
				if sx != 1 || sy != 1 {
					m = m.mul(scale2D(sx, sy))
					applied = true
				}
			}
		case "scalex":
			if v := parseScaleValue(args); v != 1 {
				m = m.mul(scale2D(v, 1))
				applied = true
			}
		case "scaley":
			if v := parseScaleValue(args); v != 1 {
				m = m.mul(scale2D(1, v))
				applied = true
			}
		case "scale3d":
			vals := splitSpaceComma(args)
			if len(vals) >= 2 {
				sx := parseScaleValue(vals[0])
				sy := parseScaleValue(vals[1])
				if sx != 1 || sy != 1 {
					m = m.mul(scale2D(sx, sy))
					applied = true
				}
			}
		case "scalez":
			// 2D 下无效果
		case "rotate":
			if v := parseAngle(args); v != 0 {
				m = m.mul(rotate2D(v))
				applied = true
			}
		case "skew":
			vals := splitSpaceComma(args)
			if len(vals) >= 1 {
				sx := parseAngle(vals[0])
				sy := 0.0
				if len(vals) >= 2 {
					sy = parseAngle(vals[1])
				}
				if sx != 0 || sy != 0 {
					m = m.mul(skew2D(sx, sy))
					applied = true
				}
			}
		case "skewx":
			if v := parseAngle(args); v != 0 {
				m = m.mul(skew2D(v, 0))
				applied = true
			}
		case "skewy":
			if v := parseAngle(args); v != 0 {
				m = m.mul(skew2D(0, v))
				applied = true
			}
		case "matrix":
			vals := splitSpaceComma(args)
			if len(vals) >= 6 {
				a := parseScaleValue(vals[0])
				b := parseScaleValue(vals[1])
				c := parseScaleValue(vals[2])
				d := parseScaleValue(vals[3])
				e := parseScaleValue(vals[4])
				f := parseScaleValue(vals[5])
				m = m.mul(matrix2D(a, b, c, d, e, f))
				applied = true
			}
		}
	}
	if !applied {
		return x, y, w, h, false
	}
	// 四角变换后取轴对齐包围盒（CSSOM-View：getBoundingClientRect 返回
	// transform 后的包围矩形）。
	x0, y0 := m.apply(x, y)
	x1, y1 := m.apply(x+w, y)
	x2, y2 := m.apply(x, y+h)
	x3, y3 := m.apply(x+w, y+h)
	minX := math.Min(math.Min(x0, x1), math.Min(x2, x3))
	minY := math.Min(math.Min(y0, y1), math.Min(y2, y3))
	maxX := math.Max(math.Max(x0, x1), math.Max(x2, x3))
	maxY := math.Max(math.Max(y0, y1), math.Max(y2, y3))
	return minX, minY, maxX - minX, maxY - minY, true
}

// ── 简化字符串辅助（避免引入 strings 依赖冲突的命名空间）──
func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
