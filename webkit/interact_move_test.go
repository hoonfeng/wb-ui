package webkit

// T4 事件派发聚合的引擎侧测试：MouseMoveBatched/FlushMoves 的合并语义，
// 以及移动路径复用命中结果（dispatchMouseAt known）不改变可观察行为。

import (
	"fmt"
	"testing"
)

const moveTwoDivsSrc = `<div id="a" style="width:100px;height:50px"></div>
<div id="b" style="width:100px;height:50px;position:absolute;left:150px;top:0"></div>
<script>
window.__log = [];
window.__moves = 0;
['a','b'].forEach(function(id){
  var el = document.getElementById(id);
  el.addEventListener('mouseenter', function(){ window.__log.push('enter-'+id); });
  el.addEventListener('mouseleave', function(){ window.__log.push('leave-'+id); });
  el.addEventListener('mousemove', function(e){ window.__moves++; window.__last = [e.clientX, e.clientY]; });
});
</script>`

// TestInteractionMouseMoveBatchedCoalesces 批内多条移动只派发一次，坐标取批内
// 最后一次；未 Flush 时不派发，重复 Flush 不会重复派发。
func TestInteractionMouseMoveBatchedCoalesces(t *testing.T) {
	wv := interactTestWebView(t, moveTwoDivsSrc)
	doc := wv.Document()
	a := doc.GetElementById("a")
	if a == nil {
		t.Fatal("#a not found")
	}
	box := findBox(wv, a)
	if box == nil {
		t.Fatal("#a box not found")
	}
	cx, cy := box.Center()

	wv.HandleMouseMoveBatched(cx+1, cy)
	wv.HandleMouseMoveBatched(cx+2, cy+2)
	wv.HandleMouseMoveBatched(cx+3, cy+4)
	if v, err := wv.EvalJS(`window.__moves`); err != nil || v.ToString() != "0" {
		t.Fatalf("未 Flush 就派发了移动：moves=%v err=%v", v, err)
	}
	wv.FlushMouseMoves()
	if v, err := wv.EvalJS(`window.__moves`); err != nil || v.ToString() != "1" {
		t.Fatalf("批内 3 条移动应合并成 1 次派发：moves=%v err=%v", v, err)
	}
	// 派发坐标必须是批内最后一次（cx+3, cy+4）：JS 侧与引擎记录都要对上。
	it := wv.Interaction()
	if it == nil {
		t.Fatal("Interaction() = nil")
	}
	if it.lastX != cx+3 || it.lastY != cy+4 {
		t.Fatalf("合并后坐标应为 (%.1f,%.1f)，实际 (%.1f,%.1f)", cx+3, cy+4, it.lastX, it.lastY)
	}
	script := fmt.Sprintf(`(function(){var p=window.__last; return (p[0]===%v && p[1]===%v) ? 'ok' : ('got:'+p[0]+','+p[1]);})()`, cx+3, cy+4)
	if v, err := wv.EvalJS(script); err != nil || v.ToString() != "ok" {
		t.Fatalf("JS 侧收到的坐标不是批内最后一次：%v err=%v", v, err)
	}
	wv.FlushMouseMoves() // 无待处理
	if v, _ := wv.EvalJS(`window.__moves`); v.ToString() != "1" {
		t.Fatalf("重复 Flush 不应重复派发：moves=%s", v.ToString())
	}
}

// TestInteractionMouseMoveImmediateUnchanged 单条路径（HandleMouseMove）必须
// 逐条立即派发，行为与合并前一致。
func TestInteractionMouseMoveImmediateUnchanged(t *testing.T) {
	wv := interactTestWebView(t, moveTwoDivsSrc)
	doc := wv.Document()
	a := doc.GetElementById("a")
	box := findBox(wv, a)
	if box == nil {
		t.Fatal("#a box not found")
	}
	cx, cy := box.Center()
	wv.HandleMouseMove(cx+1, cy)
	wv.HandleMouseMove(cx+2, cy)
	if v, _ := wv.EvalJS(`window.__moves`); v.ToString() != "2" {
		t.Fatalf("单条路径应逐条派发：moves=%s", v.ToString())
	}
}

// TestInteractionHoverEventsWithHitReuse 命中复用（dispatchMouseAt known）后
// hover 序列与 mousemove 目标仍正确：进入 a → 移到 b，事件顺序为
// enter-a / move(a) → leave-a / enter-b / move(b)。
func TestInteractionHoverEventsWithHitReuse(t *testing.T) {
	wv := interactTestWebView(t, moveTwoDivsSrc)
	doc := wv.Document()
	a, b := doc.GetElementById("a"), doc.GetElementById("b")
	if a == nil || b == nil {
		t.Fatal("元素缺失")
	}
	ab, bb := findBox(wv, a), findBox(wv, b)
	if ab == nil || bb == nil {
		t.Fatal("box 缺失")
	}
	ax, ay := ab.Center()
	bx, by := bb.Center()
	wv.HandleMouseMove(ax, ay)
	wv.HandleMouseMove(bx, by)
	if v, err := wv.EvalJS(`window.__log.join('|')`); err != nil {
		t.Fatalf("读 __log: %v", err)
	} else if got := v.ToString(); got != "enter-a|leave-a|enter-b" {
		t.Fatalf("hover 序列错误：%q", got)
	}
	if v, _ := wv.EvalJS(`window.__moves`); v.ToString() != "2" {
		t.Fatalf("两次移动都应派发 mousemove：moves=%s", v.ToString())
	}
}

// TestInteractionMouseMoveOnBlankStillReachesDocument 未命中元素时移动仍派发到
// document（命中复用的 known=true + target=nil 分支）。
func TestInteractionMouseMoveOnBlankStillReachesDocument(t *testing.T) {
	wv := interactTestWebView(t, `<div id="a" style="width:50px;height:50px"></div>
<script>
window.__docMoves = 0;
document.addEventListener('mousemove', function(){ window.__docMoves++; });
</script>`)
	wv.HandleMouseMoveBatched(390, 290) // 空白处（视口 400x300）
	wv.FlushMouseMoves()
	if v, err := wv.EvalJS(`window.__docMoves`); err != nil || v.ToString() != "1" {
		t.Fatalf("空白处移动应派发到 document：docMoves=%v err=%v", v, err)
	}
}
