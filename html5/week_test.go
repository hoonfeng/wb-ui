// <input type=week> 的 ISO 周解析测试（parseWeek / parseDateTimeValue）。
//
// 回归背景：Go 的时间布局没有「ISO 周年 + 周号」的解析支持——布局 "2006-W02"
// 里的 "02" 是「月中的第几天」，W 只是字面量。早期实现用
// time.Parse("2006-W02", s) 解析周值，于是 "1970-W03" 被读成 1970-01-03，
// 1970-W01…W04 全部算成同一周（1969-12-29），跨年的周号也会整体错位——
// week 的 min/max 范围校验与 step 相位因此失真。本测试锁住正确的 ISO 换算，
// 含「第 53 周并非每年都有」这条约束。

package html5

import "testing"

func TestParseWeekISO(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantDay string // 该周周一的日期（wantOK=true 时比较）
	}{
		// 1970-W01 的周一是 1969-12-29，即 ISO 毫秒轴的原点（−259,200,000 ms）。
		{"1970-W01", true, "1969-12-29"},
		{"1970-W02", true, "1970-01-05"},
		{"1970-W03", true, "1970-01-12"},
		{"1970-W04", true, "1970-01-19"},
		{"2026-W01", true, "2025-12-29"},
		{"2026-W28", true, "2026-07-06"},
		{"2025-W52", true, "2025-12-22"},
		// 第 53 周只在「1 月 1 日是周四」或「闰年的 1 月 1 日是周三」的年份存在。
		{"2020-W53", true, "2020-12-28"}, // 闰年 + 周三 → 53 周
		{"2026-W53", true, "2026-12-28"}, // 周四 → 53 周
		{"2025-W53", false, ""},          // 周三但平年 → 52 周
		// 非法形式。
		{"2026-W00", false, ""},
		{"2026-W99", false, ""},
		{"2026-W1", false, ""},  // 周号必须两位
		{"26-W01", false, ""},   // 年份至少四位
		{"2026W01", false, ""},  // 缺连字符
		{"2026-01", false, ""},  // 月字符串不是周字符串
		{"abc", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		y, w, ok := parseWeek(c.in)
		if ok != c.wantOK {
			t.Errorf("parseWeek(%q) = (%d, %d, %v), want ok=%v", c.in, y, w, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		tm, okVal := parseDateTimeValue(InputWeek, c.in)
		if !okVal {
			t.Errorf("parseDateTimeValue(week, %q) 失败", c.in)
			continue
		}
		if got := tm.UTC().Format("2006-01-02"); got != c.wantDay {
			t.Errorf("%q (year=%d week=%d) 的周一 = %s, want %s", c.in, y, w, got, c.wantDay)
		}
	}
}

// TestWeekRangeValidation 验证修复后的周解析真的传到了 min/max 比较上：
// 早于 min 的周是 underflow、同周不算、晚于 max 的周是 overflow。
func TestWeekRangeValidation(t *testing.T) {
	in := newInput(t, map[string]string{
		"type": "week", "min": "2026-W10", "max": "2026-W20", "value": "2026-W05",
	})
	if _, under, over := inputRangeState(in); !under || over {
		t.Errorf("2026-W05 < min 2026-W10：underflow=%v overflow=%v, want true/false", under, over)
	}
	in.SetValue("2026-W10") // 与 min 同一周
	if _, under, over := inputRangeState(in); under || over {
		t.Errorf("与 min 同周：underflow=%v overflow=%v, want false/false", under, over)
	}
	in.SetValue("2026-W25")
	if _, under, over := inputRangeState(in); under || !over {
		t.Errorf("2026-W25 > max 2026-W20：underflow=%v overflow=%v, want false/true", under, over)
	}
	// 跨年：2025-W52 早于 min 2026-W10 → underflow。旧实现把周号当「月内
	// 日」，2026-W05 与 2025-W52 会被算成同一周（1 月 5 日 vs 1 月 2 日都在
	// W01），跨年比较同样失真。
	in.SetValue("2025-W52")
	if _, under, over := inputRangeState(in); !under || over {
		t.Errorf("2025-W52 < min 2026-W10：underflow=%v overflow=%v, want true/false", under, over)
	}
}
