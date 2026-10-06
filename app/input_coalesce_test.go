package app

import (
	"testing"

	"wb-ui/engine/platform/window"
)

func moveEv(x, y float64) window.Event {
	return window.Event{Type: window.EventCursorMove, X: x, Y: y}
}

// eventEqual 逐字段比较：window.Event 含 DropFiles []string，结构体不可直接
// 用 != 比较。
func eventEqual(a, b window.Event) bool {
	if a.Type != b.Type || a.X != b.X || a.Y != b.Y || a.Button != b.Button ||
		a.Action != b.Action || a.ScrollX != b.ScrollX || a.ScrollY != b.ScrollY {
		return false
	}
	return len(a.DropFiles) == len(b.DropFiles)
}

func TestCoalesceCursorMoves(t *testing.T) {
	press := window.Event{Type: window.EventMouseButton, X: 1, Y: 2, Button: 0, Action: 0}
	scroll := window.Event{Type: window.EventScroll, ScrollY: 1}

	cases := []struct {
		name string
		in   []window.Event
		want []window.Event
	}{
		{"空批", nil, nil},
		{"单条移动", []window.Event{moveEv(1, 1)}, []window.Event{moveEv(1, 1)}},
		{
			"连续三条移动只留最后一条",
			[]window.Event{moveEv(1, 1), moveEv(2, 2), moveEv(3, 3)},
			[]window.Event{moveEv(3, 3)},
		},
		{
			"按键打断的移动各自保留",
			[]window.Event{moveEv(1, 1), moveEv(2, 2), press, moveEv(3, 3), moveEv(4, 4)},
			[]window.Event{moveEv(2, 2), press, moveEv(4, 4)},
		},
		{
			"非移动事件原样保留且顺序不变",
			[]window.Event{scroll, moveEv(5, 5), scroll},
			[]window.Event{scroll, moveEv(5, 5), scroll},
		},
		{
			"两段连续移动各留一段最后一条",
			[]window.Event{moveEv(1, 1), moveEv(2, 2), scroll, moveEv(3, 3), moveEv(4, 4), moveEv(5, 5)},
			[]window.Event{moveEv(2, 2), scroll, moveEv(5, 5)},
		},
	}
	for _, c := range cases {
		got := coalesceCursorMoves(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("%s: 条数 %d，期望 %d（got=%+v）", c.name, len(got), len(c.want), got)
		}
		for i := range got {
			if !eventEqual(got[i], c.want[i]) {
				t.Fatalf("%s: 第 %d 条 %+v，期望 %+v", c.name, i, got[i], c.want[i])
			}
		}
	}
}

// TestCoalesceCursorMovesDoesNotMutateInput 保证批合并不改写窗口层返回的切片
// （PollEvents 的返回值可能被窗口层复用）。
func TestCoalesceCursorMovesDoesNotMutateInput(t *testing.T) {
	in := []window.Event{moveEv(1, 1), moveEv(2, 2), moveEv(3, 3)}
	_ = coalesceCursorMoves(in)
	for i, want := range []float64{1, 2, 3} {
		if in[i].X != want {
			t.Fatalf("输入切片被改写：in[%d].X=%v，期望 %v", i, in[i].X, want)
		}
	}
}
