package rendering

// 滚动容器命中补偿回归（第三十一轮）：
// HitTest 主路径 pass2（hitTestLayer 层叠感知命中 walkLayerContent）此前
// 缺 per-box scroll offset 补偿——滚动容器滚过 (sx,sy)≠0 后，点击「视觉
// 位置」会命中「未滚动布局位置」的元素（点颜色行命中上方字体行 → 配置器
// 「鼠标点击位置与生效位置不匹配，不是绝对出现但会触发」根因）。
// 滚动后视觉位置 = 布局位置 - (sx,sy)，命中补偿必须 +(sx,sy)。
import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/html"
	"wb-ui/engine/style"
)

func scrollHitHTML() string {
	return `<html><head><style>
body{margin:0}
.scroll{position:absolute;left:0;top:0;width:300px;height:120px;
        overflow-y:auto;overflow-x:hidden;background:#202;border:1px solid #552}
.item{height:40px;line-height:40px;text-align:center;background:#555;color:#fff}
.a{background:#264}
</style></head><body>
<div class="scroll" id="box">
  <div class="item a" id="i0">ITEM-0</div>
  <div class="item" id="i1">ITEM-1</div>
  <div class="item" id="i2">ITEM-2</div>
  <div class="item" id="i3">ITEM-3</div>
</div>
</body></html>`
}

func TestHitTestScrollOffsetLayerPath(t *testing.T) {
	doc, err := html.ParseDocument(scrollHitHTML())
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(scrollHitHTML()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	boxEl := doc.GetElementById("box")
	i0 := doc.GetElementById("i0")
	i1 := doc.GetElementById("i1")
	if boxEl == nil || i0 == nil || i1 == nil {
		t.Fatal("elements missing")
	}
	box := rv.FindRenderBoxForNode(boxEl)
	if box == nil {
		t.Fatal("no box for .scroll")
	}

	// 容器高 120，4 个 item 高 40 = 160 → 溢出 40，滚动 40 后 i1（布局 y=40）
	// 视觉 y=0，i2（布局 y=80）视觉 y=40。
	rv.SetBoxScrollOffset(box, 0, 40)

	// 视觉点 (150,20)：补偿后布局 y=60 → 应在 i1（ITEM-1，y∈[40,80)）
	if el := HitTest(rv, 150, 20, ""); el != nil {
		if el != i1 {
			t.Fatalf("滚动后视觉 (150,20) 命中 %q(id=%s), want i1(ITEM-1)", el.TextContent(), el.GetAttribute("id"))
		}
	} else {
		t.Fatal("滚动后视觉 (150,20) 命中 nil, want i1")
	}
	// 视觉点 (150,60)：补偿后布局 y=100 → i2
	i2 := doc.GetElementById("i2")
	if el := HitTest(rv, 150, 60, ""); el != nil {
		if el != i2 {
			t.Fatalf("滚动后视觉 (150,60) 命中 %q(id=%s), want i2", el.TextContent(), el.GetAttribute("id"))
		}
	} else {
		t.Fatal("滚动后视觉 (150,60) 命中 nil, want i2")
	}
	// 未滚动（offset 0）的命中不受影响：视觉 y=20 → i0
	rv.SetBoxScrollOffset(box, 0, 0)
	if el := HitTest(rv, 150, 20, ""); el != nil {
		if el != i0 {
			t.Fatalf("未滚动视觉 (150,20) 命中 %q(id=%s), want i0", el.TextContent(), el.GetAttribute("id"))
		}
	} else {
		t.Fatal("未滚动视觉 (150,20) 命中 nil, want i0")
	}
}

// 快照时序回归（第三十三轮）：渲染节流下「滚轮后视觉帧未更新即点击」
// 的语义——命中按「已呈现帧」的滚动偏移解析（所见即所点）。Paint 时
// SnapshotScrollOffsets 记录帧偏移；命中用 PresentedBoxScrollOffset；
// 未渲染的新滚动不改变命中解析，直到下一帧快照。
func TestPresentedScrollOffsetSnapshot(t *testing.T) {
	doc, err := html.ParseDocument(scrollHitHTML())
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(scrollHitHTML()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	boxEl := doc.GetElementById("box")
	i0 := doc.GetElementById("i0")
	i1 := doc.GetElementById("i1")
	i2 := doc.GetElementById("i2")
	box := rv.FindRenderBoxForNode(boxEl)

	// 帧1：未滚动渲染（快照=0）
	rv.SnapshotScrollOffsets()
	rv.SetBoxScrollOffset(box, 0, 40) // 滚轮滚动 40（未渲染）
	// 命中按快照 0：视觉 (150,20) → 布局 20 → i0（用户所见 i0 在 20 处）
	if el := HitTest(rv, 150, 20, ""); el != i0 {
		t.Fatalf("未渲染帧内命中 %v, want i0（快照 0）", el)
	}
	// 帧2：渲染（快照=40）
	rv.SnapshotScrollOffsets()
	if el := HitTest(rv, 150, 20, ""); el != i1 {
		t.Fatalf("帧2 命中 %v, want i1", el)
	}
	// 帧3：再次滚动 80（未渲染）→ 命中仍按快照 40
	rv.SetBoxScrollOffset(box, 0, 80)
	if el := HitTest(rv, 150, 20, ""); el != i1 {
		t.Fatalf("第二次未渲染帧内命中 %v, want i1（快照 40）", el)
	}
	// 帧4：渲染 → 命中按 80：视觉 (150,20) → 布局 100 → i2
	rv.SnapshotScrollOffsets()
	if el := HitTest(rv, 150, 20, ""); el != i2 {
		t.Fatalf("帧4 命中 %v, want i2", el)
	}
}
