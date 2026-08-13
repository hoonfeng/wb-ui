package layout

import (
	"testing"

	"wb-ui/style"
)

// TestIncrementalLayout_SkippedSubtreeShifts 验证关键边界：被跳过的 clean box 自身
// 位置被父循环下移后，其 children 的绝对坐标必须跟随平移（否则残影）。
func TestIncrementalLayout_SkippedSubtreeShifts(t *testing.T) {
	root := mkBlock()
	a := mkBlockWH(100, 50)
	b := mkBlockWH(100, 50)
	c := mkBlock()
	c1 := mkBlockWH(80, 20)
	c2 := mkBlockWH(80, 20)
	c.AddChild(c1)
	c.AddChild(c2)
	root.AddChild(a)
	root.AddChild(b)
	root.AddChild(c)

	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	_, c1Y1, _, _ := rectOf(c1, state)
	_, c2Y1, _, _ := rectOf(c2, state)

	// 改 b 高度 50→100，c 位置应下移 50，c 的 children 也应跟随下移 50。
	b.style.Height = style.Length{Value: 100, Unit: "px"}
	b.MarkDirty()

	LayoutRoot(root, state)
	roundTree(root, state)

	_, cY2, _, _ := rectOf(c, state)
	_, c1Y2, _, _ := rectOf(c1, state)
	_, c2Y2, _, _ := rectOf(c2, state)

	if cY2 != 150 {
		t.Errorf("c.y = %v, want 150 (a50+b100)", cY2)
	}
	if c1Y2 != c1Y1+50 {
		t.Errorf("c1.y = %v, want %v (应跟随 c 下移 50)", c1Y2, c1Y1+50)
	}
	if c2Y2 != c2Y1+50 {
		t.Errorf("c2.y = %v, want %v (应跟随 c 下移 50)", c2Y2, c2Y1+50)
	}
}
