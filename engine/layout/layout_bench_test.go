package layout

import "testing"

// buildBenchTree 构造一个约 3 层的 block 树（约 400 节点），模拟真实页面子树。
func buildBenchTree() *ElementBox {
	root := mkBlock()
	for i := 0; i < 20; i++ {
		section := mkBlockWH(600, 40)
		for j := 0; j < 20; j++ {
			item := mkBlockWH(200, 20)
			item.AddChild(mkBlockWH(180, 15))
			section.AddChild(item)
		}
		root.AddChild(section)
	}
	return root
}

func markTreeDirty(box *ElementBox) {
	box.MarkDirty()
	for _, c := range box.Children() {
		if eb, ok := c.(*ElementBox); ok {
			markTreeDirty(eb)
		}
	}
}

// BenchmarkLayoutFull 全量布局（所有 box 脏）。
func BenchmarkLayoutFull(b *testing.B) {
	root := buildBenchTree()
	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		markTreeDirty(root)
		LayoutRoot(root, state)
		roundTree(root, state)
	}
}

// BenchmarkLayoutIncremental 增量布局（只脏一个叶子子树）。
func BenchmarkLayoutIncremental(b *testing.B) {
	root := buildBenchTree()
	state := NewLayoutState(800, 600)
	stretchRootToViewport(root, state)
	LayoutRoot(root, state)
	roundTree(root, state)

	// 取第 10 个 section 的第 10 个 item 的叶子，作为反复变更目标。
	section := root.Children()[10].(*ElementBox)
	item := section.Children()[10].(*ElementBox)
	leaf := item.Children()[0].(*ElementBox)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		leaf.MarkDirty()
		LayoutRoot(root, state)
		roundTree(root, state)
	}
}
