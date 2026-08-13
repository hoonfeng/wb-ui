package layout

import (
	"testing"

	"wb-ui/style"
)

// TestIncrementalLayout_BFCPrune 验证 BFC 脏子树剪枝：中间子元素高度变化时，
// 前面的兄弟几何不变、后面的兄弟位置跟随下移（位置由父循环更新，内容布局跳过）。
func TestIncrementalLayout_BFCPrune(t *testing.T) {
	root := mkBlock()
	a := mkBlockWH(100, 50)
	b := mkBlockWH(100, 50)
	c := mkBlockWH(100, 50)
	root.AddChild(a)
	root.AddChild(b)
	root.AddChild(c)

	// 首次全量布局（复用 state 模拟 RenderView 跨帧复用）。
	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	_, aY1, _, _ := rectOf(a, state)
	_, bY1, _, _ := rectOf(b, state)
	_, cY1, _, _ := rectOf(c, state)

	// 改 b 高度 50 → 100（MarkDirty 传播到 root）。
	b.style.Height = style.Length{Value: 100, Unit: "px"}
	b.MarkDirty()

	// 增量布局（复用 state）。
	LayoutRoot(root, state)
	roundTree(root, state)

	_, aY2, _, aH2 := rectOf(a, state)
	_, bY2, _, bH2 := rectOf(b, state)
	_, cY2, _, _ := rectOf(c, state)

	if aY2 != aY1 {
		t.Errorf("a.y changed %v -> %v, want unchanged (clean 子树应跳过)", aY1, aY2)
	}
	if aH2 != 50 {
		t.Errorf("a.height = %v, want 50", aH2)
	}
	if bY2 != bY1 {
		t.Errorf("b.y changed %v -> %v, want unchanged", bY1, bY2)
	}
	if bH2 != 100 {
		t.Errorf("b.height = %v, want 100 (dirty 子树应重算)", bH2)
	}
	if cY2 != cY1+50 {
		t.Errorf("c.y = %v, want %v (后续兄弟位置应随 b 变高下移 50)", cY2, cY1+50)
	}
}

// TestIncrementalLayout_MatchFull 验证增量布局结果与从零全量布局完全一致。
func TestIncrementalLayout_MatchFull(t *testing.T) {
	build := func() *ElementBox {
		root := mkBlock()
		root.AddChild(mkBlockWH(200, 30))
		mid := mkBlock()
		mid.AddChild(mkBlockWH(150, 20))
		mid.AddChild(mkBlockWH(150, 40))
		root.AddChild(mid)
		root.AddChild(mkBlockWH(200, 30))
		return root
	}

	// 增量路径：首次全量 → 改 mid 第二个子高度 → 复用 state 增量布局。
	inc := build()
	state := NewLayoutState(800, 600)
	stretchRootToViewport(inc, state)
	LayoutRoot(inc, state)
	roundTree(inc, state)

	// 找到 mid 的第二个子，改高度并 MarkDirty。
	mid := inc.Children()[1].(*ElementBox)
	midChild2 := mid.Children()[1].(*ElementBox)
	midChild2.style.Height = style.Length{Value: 80, Unit: "px"}
	midChild2.MarkDirty()

	LayoutRoot(inc, state)
	roundTree(inc, state)

	// 全量路径：等价树（含相同改动）从零全量布局。
	full := build()
	fullMid := full.Children()[1].(*ElementBox)
	fullMidChild2 := fullMid.Children()[1].(*ElementBox)
	fullMidChild2.style.Height = style.Length{Value: 80, Unit: "px"}
	fullState := NewLayoutState(800, 600)
	stretchRootToViewport(full, fullState)
	LayoutRoot(full, fullState)
	roundTree(full, fullState)

	// 逐节点比较几何。
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

// TestIncrementalLayout_AutoHeightShrinks 验证 auto 高度容器在内容变矮时正确收缩
// （修复增量布局复用 geometry 时 contentHeight 残留导致 auto 高度不收缩的 bug）。
func TestIncrementalLayout_AutoHeightShrinks(t *testing.T) {
	root := mkBlock()
	container := mkBlock() // height:auto
	child := mkBlockWH(100, 90)
	container.AddChild(child)
	root.AddChild(container)

	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	_, _, _, h1 := rectOf(container, state)
	if h1 != 90 {
		t.Fatalf("container 初始高度 = %v, want 90", h1)
	}

	// 子高度 90 → 40，MarkDirty 子（传播到 container 和 root）。
	child.style.Height = style.Length{Value: 40, Unit: "px"}
	child.MarkDirty()

	LayoutRoot(root, state)
	roundTree(root, state)

	_, _, _, h2 := rectOf(container, state)
	if h2 != 40 {
		t.Errorf("container auto 高度未收缩: %v -> %v, want 40", h1, h2)
	}
}
