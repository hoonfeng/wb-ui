// step mismatch 测试（HTML §4.10.5.3.8 + 各类型的 default step / scale / base）。
//
// 覆盖三类关键行为：
//   - step 缺失时仍按 default step 校验（number 默认 1、time / datetime-local
//     默认 60 秒），只有 step="any" 才跳过；
//   - step base 的优先次序 min → value 内容属性 → default step base（week 是
//     −259,200,000 ms，其余为 0）；
//   - 各类型的单位换算：date=天、month=月、week=周、time / datetime-local=秒，
//     且 number 的小数 step 不因浮点误差误报。

package html5

import "testing"

func TestStepMismatchNumberAndRange(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  bool
	}{
		// step base 的优先次序：min → value 内容属性 → default step base(0)
		// （HTML §4.10.5.3.8；Blink 的 InputType::FindStepBase 同此）。因此
		// 「期望 mismatch」的用例都必须给出 min——否则 base 就是当前值本身。
		{"整数合法（默认 step 1，base 取 value 属性）", map[string]string{"type": "number", "value": "4"}, false},
		{"小数不合法（min 给 base 0，默认 step 1）", map[string]string{"type": "number", "min": "0", "value": "4.2"}, true},
		{"min 为 base：12 是 2 的整数倍相位", map[string]string{"type": "number", "min": "10", "step": "2", "value": "12"}, false},
		{"min 为 base：13 不在相位上", map[string]string{"type": "number", "min": "10", "step": "2", "value": "13"}, true},
		{"无 min 时 base 是 value 属性本身（与自身同相位）", map[string]string{"type": "number", "step": "2", "value": "13"}, false},
		{"min 带小数（MDN 例子）：1.2 + 2 → 3.2 合法", map[string]string{"type": "number", "min": "1.2", "step": "2", "value": "3.2"}, false},
		{"min 带小数：3.0 不在相位上", map[string]string{"type": "number", "min": "1.2", "step": "2", "value": "3.0"}, true},
		{"小数 step 的浮点误差不误报（0.3 是 0.1 的 3 倍）", map[string]string{"type": "number", "min": "0", "step": "0.1", "value": "0.3"}, false},
		{"小数 step：0.35 不是 0.1 的整数倍", map[string]string{"type": "number", "min": "0", "step": "0.1", "value": "0.35"}, true},
		{"step=any 跳过校验", map[string]string{"type": "number", "min": "0", "step": "any", "value": "13.7"}, false},
		{"step=0 退回 default step（4.2 不合法）", map[string]string{"type": "number", "min": "0", "step": "0", "value": "4.2"}, true},
		{"step 负数退回 default step", map[string]string{"type": "number", "min": "0", "step": "-2", "value": "4.2"}, true},
		{"step 不可解析退回 default step", map[string]string{"type": "number", "min": "0", "step": "abc", "value": "4.2"}, true},
		{"range：min 1 + step 2 → 5 合法", map[string]string{"type": "range", "min": "1", "step": "2", "value": "5"}, false},
		{"range：min 1 + step 2 → 4 不在相位上", map[string]string{"type": "range", "min": "1", "step": "2", "value": "4"}, true},
		{"空值不参与 step 校验", map[string]string{"type": "number", "min": "0", "step": "2", "value": ""}, false},
		{"不可解析的值交给 badInput", map[string]string{"type": "number", "min": "0", "step": "2", "value": "abc"}, false},
		{"不支持 step 的类型不校验", map[string]string{"type": "text", "value": "abc"}, false},
	}
	for _, c := range cases {
		in := newInput(t, c.attrs)
		if got := in.Validity().StepMismatch; got != c.want {
			t.Errorf("%s: StepMismatch = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestStepMismatchDateTypes(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  bool
	}{
		// date：step 以天计，默认 1 天；相位由 min 决定。
		{"date：min + step 2 → 差 2 天合法", map[string]string{"type": "date", "min": "2026-01-01", "step": "2", "value": "2026-01-03"}, false},
		{"date：min + step 2 → 差 3 天不合法", map[string]string{"type": "date", "min": "2026-01-01", "step": "2", "value": "2026-01-04"}, true},
		{"date：min + step 2 → 差 1 天不合法", map[string]string{"type": "date", "min": "2026-01-01", "step": "2", "value": "2026-01-02"}, true},
		{"date：无 min 时 base 是 value 属性（与自身同相位）", map[string]string{"type": "date", "step": "2", "value": "2026-01-04"}, false},
		{"date：默认 step 1 天 → 任意日期合法", map[string]string{"type": "date", "min": "2026-01-01", "value": "2026-01-04"}, false},

		// month：step 以月计，默认 1 月。月长不等，必须用月数比较。
		{"month：min 2026-01 + step 3 → 2026-04 合法（3 个月）", map[string]string{"type": "month", "min": "2026-01", "step": "3", "value": "2026-04"}, false},
		{"month：min 2026-01 + step 3 → 2026-03 不合法（2 个月）", map[string]string{"type": "month", "min": "2026-01", "step": "3", "value": "2026-03"}, true},
		{"month：跨年（2025-11 + 4 个月 = 2026-03）", map[string]string{"type": "month", "min": "2025-11", "step": "4", "value": "2026-03"}, false},
		{"month：跨年差 3 个月不合法", map[string]string{"type": "month", "min": "2025-11", "step": "4", "value": "2026-02"}, true},
		{"month：默认 step 1 月 → 任意月合法", map[string]string{"type": "month", "min": "2026-01", "value": "2026-07"}, false},

		// week：default step base 是 1970-W01（−259,200,000 ms），step 以周计。
		{"week：默认 step 1 周 → 任意周合法", map[string]string{"type": "week", "min": "1970-W01", "value": "1970-W05"}, false},
		{"week：min 1970-W01 + step 2 → 1970-W03 合法（差 2 周）", map[string]string{"type": "week", "min": "1970-W01", "step": "2", "value": "1970-W03"}, false},
		{"week：min 1970-W01 + step 2 → 1970-W04 不合法（差 3 周）", map[string]string{"type": "week", "min": "1970-W01", "step": "2", "value": "1970-W04"}, true},
		{"week：min 1970-W01 + step 2 → 1970-W02 不合法（差 1 周）", map[string]string{"type": "week", "min": "1970-W01", "step": "2", "value": "1970-W02"}, true},
		{"week：跨年按真实周差（2025-W52 → 2026-W02 差 2 周）", map[string]string{"type": "week", "min": "2025-W52", "value": "2026-W02"}, false},

		// time：默认 step 60 秒 → 只允许整分钟（base 由 min 给出）。
		{"time：默认只允许整分钟", map[string]string{"type": "time", "min": "00:00", "value": "12:00"}, false},
		{"time：默认拒绝 30 秒", map[string]string{"type": "time", "min": "00:00", "value": "12:00:30"}, true},
		{"time：step=1 允许任意秒", map[string]string{"type": "time", "min": "00:00", "step": "1", "value": "12:00:30"}, false},
		{"time：min 09:00 + step 900（15 分钟）→ 09:15 合法", map[string]string{"type": "time", "min": "09:00", "step": "900", "value": "09:15"}, false},
		{"time：min 09:00 + step 900 → 09:10 不合法", map[string]string{"type": "time", "min": "09:00", "step": "900", "value": "09:10"}, true},
		{"time：step=any 跳过", map[string]string{"type": "time", "min": "00:00", "step": "any", "value": "12:00:30"}, false},

		// datetime-local：默认 step 60 秒，毫秒轴原点 1970-01-01T00:00。
		{"datetime-local：整分合法", map[string]string{"type": "datetime-local", "min": "1970-01-01T00:00", "value": "2026-07-09T13:45"}, false},
		{"datetime-local：30 秒不合法", map[string]string{"type": "datetime-local", "min": "1970-01-01T00:00", "value": "2026-07-09T13:45:30"}, true},
		{"datetime-local：step=1 允许任意秒", map[string]string{"type": "datetime-local", "min": "1970-01-01T00:00", "step": "1", "value": "2026-07-09T13:45:30"}, false},

		// 空值 / 不可解析值都不算 step mismatch。
		{"date：空值不校验", map[string]string{"type": "date", "min": "2026-01-01", "value": ""}, false},
		{"time：不可解析值交给 badInput", map[string]string{"type": "time", "min": "00:00", "value": "abc"}, false},
	}
	for _, c := range cases {
		in := newInput(t, c.attrs)
		if got := in.Validity().StepMismatch; got != c.want {
			t.Errorf("%s: StepMismatch = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestStepMismatchMakesInvalid step mismatch 必须让元素整体无效（:invalid /
// checkValidity() 为 false），否则约束校验的消费方（表单提交、CSS）看不到它。
func TestStepMismatchMakesInvalid(t *testing.T) {
	in := newInput(t, map[string]string{"type": "time", "min": "00:00", "value": "12:00:30"})
	v := in.Validity()
	if !v.StepMismatch {
		t.Fatal("time 12:00:30（默认 step 60 秒）应 step mismatch")
	}
	if v.Valid() {
		t.Fatal("step mismatch 时 Valid() 应为 false")
	}
	if in.CheckValidity() {
		t.Fatal("step mismatch 时 CheckValidity() 应为 false")
	}
	if v.ValidationMessage(in.El) == "" {
		t.Error("step mismatch 应有校验消息")
	}
	in.SetValue("12:00")
	if !in.CheckValidity() {
		t.Error("改为整分钟后应通过校验")
	}
}
