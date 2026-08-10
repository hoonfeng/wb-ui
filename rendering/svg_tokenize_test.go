package rendering

import (
	"strings"
	"testing"
)

// TestTokenizeDotLeadNumber: SVG path 数字隐式分隔 —— `-1.82.33` 应拆为
// `-1.82` 和 `.33`（前导点小数），`1.82.33` 同理。标准 SVG BNF 允许
// 数字以 "." 开头紧跟前一个数字结尾（无分隔符）。
func TestTokenizeDotLeadNumber(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a1.65 1.65 0 0 0-1.82.33", []string{"a", "1.65", "1.65", "0", "0", "0", "-1.82", ".33"}},
		{"a1.65 1.65 0 0 0 1.82.33", []string{"a", "1.65", "1.65", "0", "0", "0", "1.82", ".33"}},
		{"a1.65 1.65 0 0 0-1.82-.33", []string{"a", "1.65", "1.65", "0", "0", "0", "-1.82", "-.33"}},
		{"l-8-3-8 3", []string{"l", "-8", "-3", "-8", "3"}},
		{"a2 2 0 0 1-2.83 0", []string{"a", "2", "2", "0", "0", "1", "-2.83", "0"}},
		{"0-1-1.51", []string{"0", "-1", "-1.51"}},
		{"1e-5", []string{"1e-5"}},
		{"M19.4 15", []string{"M", "19.4", "15"}},
	}
	for _, c := range cases {
		got := tokenizeSVGPath(c.in)
		if len(got) != len(c.want) {
			t.Errorf("tokenizeSVGPath(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tokenizeSVGPath(%q)[%d] = %q, want %q (full %v)", c.in, i, got[i], c.want[i], got)
				break
			}
		}
	}
}

// TestGearGearArcParsed: 解析完整齿轮 path，确认左下齿/上齿的两段 1.65 弧
// 都被解析（此前 -1.82.33 / 1.82.33 被吞导致整组 arc 丢弃）。
func TestGearGearArcParsed(t *testing.T) {
	d := "M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
	cmds := parseSVGPathData(d)
	var arcs int
	for _, c := range cmds {
		if c.kind == 'a' || c.kind == 'A' {
			arcs++
			if len(c.args) != 7 {
				t.Errorf("arc args len=%d (%v) — 被隐式分隔吞掉", len(c.args), c.args)
			}
		}
	}
	// 齿轮 path 应有 32 个 arc 参数组（含隐式重复命令拆分）：每齿 2 外凸弧
	// (1.65) + 2 内凹弧 (2)，共 8 齿 = 32。修复前 -1.82.33 / 1.82.33 被吞
	// 导致 2 组 arc 丢弃 → 只剩 30。
	t.Logf("arcs=%d 全部 7 参数", arcs)
	if arcs < 32 {
		t.Errorf("期望 32 个 arc 参数组，实际 %d — 仍有组被吞", arcs)
	}
	// 左下齿区域：找到包含 -1.82 的弧（x 相对 -1.82, y .33）
	found := false
	for _, c := range cmds {
		if c.kind == 'a' && len(c.args) == 7 && strings.Contains(d, "-1.82.33") {
			if c.args[5] == -1.82 && c.args[6] == 0.33 {
				found = true
			}
		}
	}
	if !found {
		// 直接检查解析结果中是否有 args[5]==-1.82 && args[6]==0.33 的弧
		for _, c := range cmds {
			if c.kind == 'a' && len(c.args) == 7 && c.args[5] == -1.82 && c.args[6] == 0.33 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("未找到左下齿弧 a(... -1.82 .33) — 参数解析错误")
	}
}
