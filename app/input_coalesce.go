package app

import "wb-ui/engine/platform/window"

// coalesceCursorMoves 把事件批里**连续**的鼠标移动合并为段内最后一条（T4
// 事件派发聚合）。
//
// 为什么要合并：系统会重发 WM_MOUSEMOVE，拖拽时一轮 PollEvents 也常收到一串
// 移动。逐条走完整管线意味着每条都做命中测试 + hover 样式重算 + mousemove
// 冒泡派发 + 渲染脏标记，而中间坐标页面根本观察不到——浏览器同样只把合并后的
// 位置派发给页面（UI Events 的 coalesced events 语义）。
//
// 为什么只合并「连续」段：只丢「下一条也是移动」的移动，段内最后一条必然
// 保留，且不会跨过按键/滚轮等事件，位置语义与其余事件的相对顺序都不变。
// 输入切片不被修改（PollEvents 的返回值可能被窗口层复用），无合并时原样返回。
func coalesceCursorMoves(events []window.Event) []window.Event {
	if len(events) < 2 {
		return events
	}
	merged := func(i int) bool {
		return events[i].Type == window.EventCursorMove && events[i+1].Type == window.EventCursorMove
	}
	dropped := 0
	for i := 0; i+1 < len(events); i++ {
		if merged(i) {
			dropped++
		}
	}
	if dropped == 0 {
		return events
	}
	out := make([]window.Event, 0, len(events)-dropped)
	for i, ev := range events {
		if i+1 < len(events) && merged(i) {
			continue
		}
		out = append(out, ev)
	}
	return out
}
