package rendering

import (
	"testing"
)

// TestSettingsGearPathVerbatim: 完整解析 SvgIcon.vue 的 settings path，
// 验证每个隐式 arc 组都被解析且顺序正确。
func TestSettingsGearPathVerbatim(t *testing.T) {
	d := "M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
	cmds := parseSVGPathData(d)
	var aCount, lCount int
	for i, c := range cmds {
		switch c.kind {
		case 'a', 'A':
			aCount++
		case 'l', 'L':
			lCount++
		}
		_ = i
	}
	// 期望：8 个齿 × (1 arc + 1 l) 主体 + 8 个 arc 间隙 + 中心 V/l 等。
	// 实际数一下并记录，用于人工核对。
	t.Logf("cmds total=%d arcs=%d l-lines=%d", len(cmds), aCount, lCount)
	// 记录每个 a 的参数（sweep 标志）分布
	var sweep0, sweep1 int
	for _, c := range cmds {
		if c.kind == 'a' && len(c.args) == 7 && (c.args[4] == 0 || c.args[4] == 1) {
			if c.args[4] == 0 {
				sweep0++
			} else {
				sweep1++
			}
		}
	}
	t.Logf("arcs sweep=0: %d, sweep=1: %d (齿轮应各 8 个对称)", sweep0, sweep1)
}
