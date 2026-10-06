package css

import "testing"

// TestTransformToMatrixCSSOM 用 Edge 实测基线逐项校验 CSSOM 归一。
// 期望值来源：dev/fixtures/webshot/h7_transform_norm.html 的 Edge 产物
// （dev/output/wbui-audit/h7_transform_norm.edge.txt）—— 探针里每个用例都是
// width:200px;height:100px;font-size:16px 的 div，故 refW=200、refH=100、fs=16。
func TestTransformToMatrixCSSOM(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"translate(20px,10px)", "matrix(1, 0, 0, 1, 20, 10)"},
		{"translateX(5px)", "matrix(1, 0, 0, 1, 5, 0)"},
		{"translateY(-3px)", "matrix(1, 0, 0, 1, 0, -3)"},
		{"translate(50%,25%)", "matrix(1, 0, 0, 1, 100, 25)"},
		{"translate(1em,2em)", "matrix(1, 0, 0, 1, 16, 32)"},
		{"translate(0.5px)", "matrix(1, 0, 0, 1, 0.5, 0)"},
		{"translate(0.125px)", "matrix(1, 0, 0, 1, 0.125, 0)"},
		{"scale(0.5)", "matrix(0.5, 0, 0, 0.5, 0, 0)"},
		{"scale(2,3)", "matrix(2, 0, 0, 3, 0, 0)"},
		{"scaleX(1.5)", "matrix(1.5, 0, 0, 1, 0, 0)"},
		{"scaleY(0.25)", "matrix(1, 0, 0, 0.25, 0, 0)"},
		{"scale(0.1)", "matrix(0.1, 0, 0, 0.1, 0, 0)"},
		{"scale(3)", "matrix(3, 0, 0, 3, 0, 0)"},
		// 6 位有效数字 + 极小残差归零：cos(90°) 的 6.12e-17 → "0"。
		{"rotate(45deg)", "matrix(0.707107, 0.707107, -0.707107, 0.707107, 0, 0)"},
		{"rotate(90deg)", "matrix(0, 1, -1, 0, 0, 0)"},
		{"rotate(180deg)", "matrix(-1, 0, 0, -1, 0, 0)"},
		{"rotate(270deg)", "matrix(0, -1, 1, 0, 0, 0)"},
		{"rotate(360deg)", "matrix(1, 0, 0, 1, 0, 0)"},
		{"rotate(0.25turn)", "matrix(0, 1, -1, 0, 0, 0)"},
		{"rotate(1rad)", "matrix(0.540302, 0.841471, -0.841471, 0.540302, 0, 0)"},
		{"skew(10deg,20deg)", "matrix(1, 0.36397, 0.176327, 1, 0, 0)"},
		{"skewX(10deg)", "matrix(1, 0, 0.176327, 1, 0, 0)"},
		{"skewY(-5deg)", "matrix(1, -0.0874887, 0, 1, 0, 0)"},
		{"matrix(1,2,3,4,5,6)", "matrix(1, 2, 3, 4, 5, 6)"},
		{"matrix(0.5,0,0,0.5,10,20)", "matrix(0.5, 0, 0, 0.5, 10, 20)"},
		{"rotate(30deg) scale(2)", "matrix(1.73205, 1, -1, 1.73205, 0, 0)"},
		{"translate(10px) rotate(90deg)", "matrix(0, 1, -1, 0, 10, 0)"},
		{"none", "none"},
		{"", "none"},
		{"   ", "none"},
	}
	for _, c := range cases {
		got, ok := TransformToMatrix(c.in, 200, 100, 16)
		if !ok {
			t.Errorf("TransformToMatrix(%q) ok=false，期望 %q", c.in, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("TransformToMatrix(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestTransformToMatrix3DFallsBack 校验真 3D 变换不会被**错报**成 2D matrix：
// ok 必须为 false，交由调用方保留原值。
func TestTransformToMatrix3DFallsBack(t *testing.T) {
	for _, in := range []string{
		"translateZ(10px)",       // 非零 z 平移
		"translate3d(1px,2px,3px)", // 非零 z
		"rotateX(30deg)",         // 真 3D 旋转
		"rotateY(45deg)",
		"scale3d(1,1,2)", // 非 1 的 sz
		"perspective(100px)",
		"rotate3d(1,1,1,45deg)",
		"none2",     // 未知函数
		"bogus(1px)", // 非法语法
	} {
		if got, ok := TransformToMatrix(in, 200, 100, 16); ok {
			t.Errorf("TransformToMatrix(%q) ok=true（got %q），期望 false（应保留原值）", in, got)
		}
	}
}

// TestTransformToMatrix2DEquivalent3D 3D 函数退化为 2D 等价的常见写法
// （z=0 / 角为 0 / sz=1，如 GPU 加速 hack 里的 translateZ(0)）应当归一。
func TestTransformToMatrix2DEquivalent3D(t *testing.T) {
	cases := []struct{ in, want string }{
		{"translateZ(0)", "matrix(1, 0, 0, 1, 0, 0)"},
		{"translate3d(1px,2px,0)", "matrix(1, 0, 0, 1, 1, 2)"},
		{"rotateX(0deg)", "matrix(1, 0, 0, 1, 0, 0)"},
		{"scale3d(2,3,1)", "matrix(2, 0, 0, 3, 0, 0)"},
		{"rotateZ(90deg)", "matrix(0, 1, -1, 0, 0, 0)"},
	}
	for _, c := range cases {
		got, ok := TransformToMatrix(c.in, 200, 100, 16)
		if !ok || got != c.want {
			t.Errorf("TransformToMatrix(%q) = %q,%v，期望 %q,true", c.in, got, ok, c.want)
		}
	}
}

// TestTfFormatNum 逐个钉住分量序列化规则（6 位有效数字、去尾零、极小归零）。
func TestTfFormatNum(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{20, "20"},
		{0.5, "0.5"},
		{0.125, "0.125"},
		{0.1, "0.1"},
		{0.7071067811865476, "0.707107"},
		{0.36397023426620234, "0.36397"},
		{-0.08748866352592401, "-0.0874887"},
		{1.7320508075688772, "1.73205"},
		{6.123233995736766e-17, "0"}, // cos(90°)：极小残差归零
		{-2.4492935982947064e-16, "0"},
	}
	for _, c := range cases {
		if got := tfFormatNum(c.in); got != c.want {
			t.Errorf("tfFormatNum(%v) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
