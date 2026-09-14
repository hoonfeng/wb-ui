package app

import (
	"testing"

	"wb-ui/engine/dom"
)

// TestDispatchHoverEvents 验证 hover 事件派发序列（UI Events）：
// mouseout/mouseleave 派发到离开元素，mouseover/mouseenter 派发到进入元素；
// mouseover/mouseout 冒泡且带 relatedTarget，mouseenter/mouseleave 不冒泡且
// relatedTarget 为 null。
func TestDispatchHoverEvents(t *testing.T) {
	h := &Host{}
	d := dom.NewDocument()
	a := d.CreateElement("div")
	_ = d.AppendChild(a)
	b := d.CreateElement("div")
	_ = d.AppendChild(b)

	type rec struct {
		el      string // "a" or "b"
		typ     string
		rt      dom.EventTarget
		bubbles bool
	}
	var got []rec

	wire := func(el *dom.Element, name string) {
		for _, typ := range []string{dom.EventMouseOver, dom.EventMouseOut, dom.EventMouseEnter, dom.EventMouseLeave} {
			el.AddEventListener(typ, dom.EventListenerFunc(func(e dom.Event) {
				me := e.(*dom.MouseEvent)
				got = append(got, rec{el: name, typ: e.Type(), rt: me.RelatedTarget(), bubbles: e.Bubbles()})
			}), false)
		}
	}
	wire(a, "a")
	wire(b, "b")

	h.dispatchHoverEvents(a, b, 10, 20)

	// 期望顺序：a.mouseout → a.mouseleave → b.mouseover → b.mouseenter
	want := []rec{
		{el: "a", typ: "mouseout", rt: b, bubbles: true},
		{el: "a", typ: "mouseleave", rt: nil, bubbles: false},
		{el: "b", typ: "mouseover", rt: a, bubbles: true},
		{el: "b", typ: "mouseenter", rt: nil, bubbles: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].el != want[i].el || got[i].typ != want[i].typ ||
			got[i].rt != want[i].rt || got[i].bubbles != want[i].bubbles {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
