package html5

import (
	"math"
	"strconv"
	"strings"
)

// ─── step mismatch（HTML §4.10.5.3.8 与各输入类型的 default step / scale / base）─
//
// 规范把 step 校验拆成三个量：
//
//   - allowed value step：step 属性的值 × step scale factor。属性缺失、解析失败、
//     零或负数时退回 default step × scale factor；取值为 "any"（或类型不支持
//     step）时没有 allowed value step，不做 step 校验。
//   - step base：min 属性（可解析）→ 否则 value 内容属性（可解析）→ 否则该类型
//     的 default step base（只有 week 定义了它：−259,200,000 ms，即 1970-W01 的
//     周一 1969-12-29）→ 否则 0。
//   - 数值化（axis value）：把 value / min / base 按类型转到同一条数值轴上——
//     date / week / datetime-local 用自 1970-01-01T00:00Z 的毫秒，time 用自午夜
//     的毫秒，month 用自 1970-01 的月数（月长不等，不能换算成毫秒），number /
//     range 用数值本身。
//
// 判定：value 与 step base 之差不是 allowed value step 的整数倍 → step mismatch。
//
// ⚠️ 两个常见误解（本文件按规范实现，且是此前只对 number / range 校验时漏掉的）：
//
//  1. step 缺失时**仍然校验**（用 default step）：`<input type=number value=1.5>`
//     是 step mismatch（number 的默认 step 是 1），`<input type=time
//     value=12:00:30>` 也是（time / datetime-local 的默认 step 是 60 秒，只允许
//     整分钟）。只有 step="any" 才跳过。
//  2. step base 在 min 缺失时可以就是 value 属性本身，因此
//     `<input type=number value=10 step=3>` 不 mismatch（base 即 10），而
//     `<input type=number value=15 step=10>` mismatch。

// stepSpec 描述一个输入类型的 step 参数（规范：每个 type 状态定义 default step
// 与 step scale factor，个别类型另有 default step base）。
type stepSpec struct {
	applicable  bool    // 该类型是否支持 step 属性
	defaultStep float64 // default step
	scaleFactor float64 // step scale factor（把 step 换算到该类型的数值轴单位）
	defaultBase float64 // default step base（0 = 「回到 0」）
}

// inputStepSpec 返回类型的 step 参数。取值全部来自规范中各 type 状态的定义。
func inputStepSpec(t InputType) stepSpec {
	switch t {
	case InputDate:
		// 「step 以天表示，step scale factor 是 86,400,000，default step 是 1 天」
		return stepSpec{applicable: true, defaultStep: 1, scaleFactor: 86400000}
	case InputMonth:
		// 「step 以月表示，step scale factor 是 1（算法本身用月），default step 是 1 月」
		return stepSpec{applicable: true, defaultStep: 1, scaleFactor: 1}
	case InputWeek:
		// 「step 以周表示，scale factor 604,800,000，default step 1 周，
		//   default step base −259,200,000（1970-W01 的起点）」
		return stepSpec{applicable: true, defaultStep: 1, scaleFactor: 604800000,
			defaultBase: -259200000}
	case InputTime, InputDateTimeLocal:
		// 「step 以秒表示，scale factor 1000，default step 是 60 秒」
		return stepSpec{applicable: true, defaultStep: 60, scaleFactor: 1000}
	case InputNumber, InputRange:
		// 「step scale factor 是 1，default step 是 1」
		return stepSpec{applicable: true, defaultStep: 1, scaleFactor: 1}
	}
	return stepSpec{}
}

// stepAxisValue 把 value / min 属性字符串按类型转到 step 比较用的数值轴上
// （见文件头）。ok=false 表示该字符串按此类型不可解析（调用方不据此判违规：
// 那是 badInput 的职责）。
func stepAxisValue(t InputType, s string) (float64, bool) {
	switch t {
	case InputNumber, InputRange:
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0, false
		}
		return v, true
	case InputMonth:
		y, m, ok := parseMonth(s)
		if !ok {
			return 0, false
		}
		return float64((y-1970)*12 + (m - 1)), true
	case InputDate, InputWeek, InputDateTimeLocal:
		tm, ok := parseDateTimeValue(t, s)
		if !ok {
			return 0, false
		}
		// 三者都以 1970-01-01T00:00Z 为毫秒轴原点（week 取该周周一，
		// 与 default step base −259,200,000 一致）。
		return float64(tm.UnixMilli()), true
	case InputTime:
		// 规范：time 的数值是自午夜的毫秒（没有日期部分，因此不能用
		// UnixMilli——parseTimeStr 的基准日是 Go 的零年）。
		tm, ok := parseTimeStr(s)
		if !ok {
			return 0, false
		}
		ms := int64(tm.Hour())*3600000 + int64(tm.Minute())*60000 +
			int64(tm.Second())*1000
		return float64(ms), true
	}
	return 0, false
}

// allowedValueStep 返回 allowed value step；0 表示没有（step="any" 或类型不
// 支持 step），调用方应跳过 step 校验。
func allowedValueStep(in HTMLInputElement, spec stepSpec) float64 {
	attr := strings.TrimSpace(in.Step())
	if attr == "" {
		return spec.defaultStep * spec.scaleFactor
	}
	if strings.EqualFold(attr, "any") {
		return 0
	}
	v, err := strconv.ParseFloat(attr, 64)
	if err != nil || v <= 0 {
		// 解析失败 / 零 / 负数 → 退回 default step。规范原文如此：非法属性值
		// 不等于「不校验」。
		return spec.defaultStep * spec.scaleFactor
	}
	return v * spec.scaleFactor
}

// stepBaseOf 按规范的算法求 step base：min 属性 → value 内容属性 → default
// step base。
func stepBaseOf(in HTMLInputElement, spec stepSpec) float64 {
	t := in.Type()
	if s := strings.TrimSpace(in.Min()); s != "" {
		if v, ok := stepAxisValue(t, s); ok {
			return v
		}
	}
	if s := strings.TrimSpace(in.El.GetAttribute("value")); s != "" {
		if v, ok := stepAxisValue(t, s); ok {
			return v
		}
	}
	// default step base 的量纲与各类型的轴一致：week 是毫秒，其余类型
	// 的 0 分别对应 1970-01-01T00:00、1970-01 / 1970-W01 的原点、数值 0。
	return spec.defaultBase
}

// axisIsInteger 报告该类型的数值轴是否一定是整数（毫秒 / 月）。number / range
// 是唯一可能出现小数的类型——它们的 step 也常常是小数（0.1、0.25），需要走
// 容差判定。
func axisIsInteger(t InputType) bool {
	switch t {
	case InputDate, InputMonth, InputWeek, InputTime, InputDateTimeLocal:
		return true
	}
	return false
}

// isIntegralMultiple 报告 diff 是否为 step 的整数倍。
//
// 整数轴（日期/时间/月）：diff 与 step 通常都是整数（毫秒 / 月，甚至
// step="0.5" 天 = 43,200,000 ms 也是整数），用取模精确判定——毫秒级轴值最大
// 1e13 < 2^53，float64 精确表示整数，取模没有误差；只有极端小数 step 才退回
// 容差判定。
//
// number / range：diff 与 step 都可能来自十进制输入（0.3 / 0.1），float64 的
// 商有误差（0.3 / 0.1 = 2.9999999999999996），因此用容差判定：相对误差 ≤1e-9
// 视为整数倍（按 step 换算的绝对容差 = 1e-9 × step，不会吞掉真实的偏离）。
func isIntegralMultiple(diff, step float64, integerAxis bool) bool {
	if diff < 0 {
		diff = -diff
	}
	if step <= 0 {
		return false
	}
	if integerAxis {
		d, s := math.Round(diff), math.Round(step)
		if d == diff && s == step {
			return math.Mod(d, s) == 0
		}
	}
	q := diff / step
	return math.Abs(q-math.Round(q)) <= 1e-9
}

// StepMismatch 报告 input 的当前值是否 suffering from a step mismatch
// （HTML §4.10.5.3.8）。空值与不可解析值都不算（前者由 valueMissing、后者由
// badInput 负责），类型不支持 step 或 step="any" 时也不算。
func StepMismatch(in HTMLInputElement) bool {
	t := in.Type()
	spec := inputStepSpec(t)
	if !spec.applicable {
		return false
	}
	step := allowedValueStep(in, spec)
	if step <= 0 {
		return false
	}
	valStr := strings.TrimSpace(in.Value())
	if valStr == "" {
		return false
	}
	value, ok := stepAxisValue(t, valStr)
	if !ok {
		return false
	}
	base := stepBaseOf(in, spec)
	return !isIntegralMultiple(base-value, step, axisIsInteger(t))
}
