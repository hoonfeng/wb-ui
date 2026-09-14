// ToggleEvent 的 Go 侧构造（dom 包）：oldState / newState / source 三个属性的
// 初始化路径（NewToggleEvent 显式传 source、NewToggleEventFromInit 走字典）。

package dom

import "testing"

func TestToggleEventSource(t *testing.T) {
	doc := NewDocument()
	btn := doc.CreateElement("button")

	// 显式构造：source 原样保留（nil 表示「没有触发者」——本端口 dialog /
	// details 的所有路径都传 nil，见 Source 的注释）。
	ev := NewToggleEvent("toggle", false, false, ToggleStateOpen, ToggleStateClosed, nil)
	if ev.Source() != nil {
		t.Errorf("Source() = %v, want nil", ev.Source())
	}
	if ev.OldState() != ToggleStateOpen || ev.NewState() != ToggleStateClosed {
		t.Errorf("OldState/NewState = %q/%q, want open/closed", ev.OldState(), ev.NewState())
	}

	ev2 := NewToggleEvent("beforetoggle", false, true, ToggleStateClosed, ToggleStateOpen, btn)
	if ev2.Source() != btn {
		t.Errorf("Source() = %v, want %v", ev2.Source(), btn)
	}
	if !ev2.Cancelable() {
		t.Error("beforetoggle 应可取消")
	}

	// 字典构造：source 经 ToggleEventInit 传递；缺省时保持 nil。
	ev3 := NewToggleEventFromInit("toggle", ToggleEventInit{
		EventInit: EventInit{Bubbles: false, Cancelable: false},
		OldState:  ToggleStateClosed,
		NewState:  ToggleStateOpen,
		Source:    btn,
	})
	if ev3.Source() != btn {
		t.Errorf("FromInit Source() = %v, want %v", ev3.Source(), btn)
	}
	ev4 := NewToggleEventFromInit("toggle", ToggleEventInit{})
	if ev4.Source() != nil {
		t.Errorf("FromInit 未给 source 时 Source() = %v, want nil", ev4.Source())
	}
	// 缺省状态按 IDL 默认值为 "closed"。
	if ev4.OldState() != ToggleStateClosed || ev4.NewState() != ToggleStateClosed {
		t.Errorf("缺省状态 = %q/%q, want closed/closed", ev4.OldState(), ev4.NewState())
	}
}
