package layout

import (
	"testing"

	"wb-ui/style"
)

// TestIncrementalLayout_FFCPrune 验证 flex 容器整容器剪枝：flex 容器 clean 且
// 位置未变时跳过；其兄弟 dirty 不影响 flex 容器的 item 几何。
func TestIncrementalLayout_FFCPrune(t *testing.T) {
	root := mkBlock()
	flex := mkFlex()
	flex.style.Width = style.Length{Value: 300, Unit: "px"}
	flex.style.Height = style.Length{Value: 60, Unit: "px"}
	item1 := mkBlockWH(50, 40)
	item2 := mkBlockWH(50, 40)
	flex.AddChild(item1)
	flex.AddChild(item2)
	block := mkBlockWH(100, 30)
	root.AddChild(flex)
	root.AddChild(block)

	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	_, item1Y1, _, _ := rectOf(item1, state)
	_, item2Y1, _, _ := rectOf(item2, state)

	// 改 block 高度 30→80，flex 容器位置不变（在 block 前面），应剪枝跳过。
	block.style.Height = style.Length{Value: 80, Unit: "px"}
	block.MarkDirty()

	LayoutRoot(root, state)
	roundTree(root, state)

	_, item1Y2, _, _ := rectOf(item1, state)
	_, item2Y2, _, _ := rectOf(item2, state)

	if item1Y2 != item1Y1 || item2Y2 != item2Y1 {
		t.Errorf("flex item y 变化: item1 %v->%v, item2 %v->%v (flex 容器 clean 应跳过)",
			item1Y1, item1Y2, item2Y1, item2Y2)
	}
}

// TestIncrementalLayout_FFCDirtyMatchFull 验证 flex 容器 dirty 时（item 变化）重算，
// 结果与从零全量布局一致。
func TestIncrementalLayout_FFCDirtyMatchFull(t *testing.T) {
	build := func() *ElementBox {
		root := mkBlock()
		flex := mkFlex()
		flex.style.Width = style.Length{Value: 400, Unit: "px"}
		flex.AddChild(mkBlockWH(60, 40))
		flex.AddChild(mkBlockWH(80, 40))
		root.AddChild(flex)
		root.AddChild(mkBlockWH(100, 30))
		return root
	}

	// 增量路径：首次全量 → 改 flex 第一个 item 高度 → 复用 state 增量布局。
	inc := build()
	state := NewLayoutState(800, 600)
	stretchRootToViewport(inc, state)
	LayoutRoot(inc, state)
	roundTree(inc, state)

	flexInc := inc.Children()[0].(*ElementBox)
	itemInc := flexInc.Children()[0].(*ElementBox)
	itemInc.style.Height = style.Length{Value: 90, Unit: "px"}
	itemInc.MarkDirty()

	LayoutRoot(inc, state)
	roundTree(inc, state)

	// 全量路径：等价树（含相同改动）从零全量布局。
	full := build()
	flexFull := full.Children()[0].(*ElementBox)
	itemFull := flexFull.Children()[0].(*ElementBox)
	itemFull.style.Height = style.Length{Value: 90, Unit: "px"}
	fullState := NewLayoutState(800, 600)
	stretchRootToViewport(full, fullState)
	LayoutRoot(full, fullState)
	roundTree(full, fullState)

	var walk func(ia, fb *ElementBox, path string)
	walk = func(ia, fb *ElementBox, path string) {
		gx, gy, gw, gh := rectOf(ia, state)
		fx, fy, fw, fh := rectOf(fb, fullState)
		if gx != fx || gy != fy || gw != fw || gh != fh {
			t.Errorf("%s: 增量=(%.1f,%.1f %.1fx%.1f) 全量=(%.1f,%.1f %.1fx%.1f)",
				path, gx, gy, gw, gh, fx, fy, fw, fh)
		}
		for i := 0; i < len(ia.Children()) && i < len(fb.Children()); i++ {
			ca, ok1 := ia.Children()[i].(*ElementBox)
			cb, ok2 := fb.Children()[i].(*ElementBox)
			if ok1 && ok2 {
				walk(ca, cb, path+"/child"+string(rune('0'+i)))
			}
		}
	}
	walk(inc, full, "root")
}
