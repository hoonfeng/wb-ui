// invoker（popovertarget / popovertargetaction / commandfor / command）的行为
// 测试（HTML §6.12.1 与 form-elements 里 button 的激活行为）。
package popover

import (
	"testing"
)

func TestPopoverTargetActivationShowsAndHides(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	btn := appendEl(t, body, "button", "popovertarget", "p", "popovertargetaction", "show")
	rec := recordToggles(p)

	if !RunActivation(btn, btn) {
		t.Fatal("invoker 点击应被 popover 消费")
	}
	if !p.IsPopoverOpen() {
		t.Fatal("popovertargetaction=show 应显示目标 popover")
	}
	// source 非 null：这是本引擎里 ToggleEvent.source 唯一出现的场景。
	if got := recStrings(rec); len(got) == 0 || (*rec)[0].source != btn {
		t.Fatalf("invoker 触发的事件 source 应为按钮，实际 %v", got)
	}
	// show 且已显示：不重复派发事件。
	*rec = (*rec)[:0]
	if !RunActivation(btn, btn) {
		t.Fatal("重复点击仍应被消费")
	}
	if len(*rec) != 0 {
		t.Fatalf("目标已处于请求状态时不应派发事件，实际 %v", recStrings(rec))
	}

	btn.SetAttribute("popovertargetaction", "hide")
	if !RunActivation(btn, btn) {
		t.Fatal("hide 动作应被消费")
	}
	if p.IsPopoverOpen() {
		t.Fatal("popovertargetaction=hide 应隐藏目标 popover")
	}

	// 缺省动作是 toggle。
	btn.RemoveAttribute("popovertargetaction")
	RunActivation(btn, btn)
	if !p.IsPopoverOpen() {
		t.Fatal("缺省 popovertargetaction 应是 toggle（第一次点击显示）")
	}
	RunActivation(btn, btn)
	if p.IsPopoverOpen() {
		t.Fatal("再次点击应关闭（toggle 语义）")
	}
}

func TestPopoverTargetResolutionRules(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	appendEl(t, body, "div", "id", "plain")

	// 1) popovertarget 指向的不是 popover 元素 → 无目标。
	btn := appendEl(t, body, "button", "popovertarget", "plain")
	if got := TargetElementOf(btn); got != nil {
		t.Fatalf("非 popover 目标应解析为 nil，实际 %v", got)
	}
	// 2) ID 不存在 → nil。
	btn.SetAttribute("popovertarget", "missing")
	if got := TargetElementOf(btn); got != nil {
		t.Fatalf("不存在的 ID 应解析为 nil，实际 %v", got)
	}
	// 3) 正常解析。
	btn.SetAttribute("popovertarget", "p")
	if got := TargetElementOf(btn); got != p {
		t.Fatalf("TargetElementOf = %v，want p", got)
	}
	// 4) disabled 的按钮不是 invoker；Activation 也不消费。
	btn.SetAttribute("disabled", "")
	if got := TargetElementOf(btn); got != nil {
		t.Fatalf("disabled 按钮不应解析出目标，实际 %v", got)
	}
	if RunActivation(btn, btn) {
		t.Fatal("disabled 按钮的激活不应被 popover 消费")
	}
	btn.RemoveAttribute("disabled")

	// 5) input type=button 可以是 invoker；type=text 不行。
	inButton := appendEl(t, body, "input", "type", "button", "popovertarget", "p")
	if got := TargetElementOf(inButton); got != p {
		t.Fatalf("input type=button 应是 invoker，实际 %v", got)
	}
	inText := appendEl(t, body, "input", "type", "text", "popovertarget", "p")
	if got := TargetElementOf(inText); got != nil {
		t.Fatalf("input type=text 不应是 invoker，实际 %v", got)
	}
}

func TestSubmitButtonIsNotInvokerWhenItHasFormOwner(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	form := appendEl(t, body, "form")
	btn := appendEl(t, form, "button", "popovertarget", "p") // type 缺省 = submit

	if got := TargetElementOf(btn); got != nil {
		t.Fatalf("表单内的提交按钮不应解析出 popover 目标，实际 %v", got)
	}
	if RunActivation(btn, btn) {
		t.Fatal("提交按钮的激活应由表单逻辑处理，popover 不应消费")
	}
	// 同一按钮移到表单外：没有 form owner 的提交按钮按规范可以当 invoker。
	body.AppendChild(btn)
	if got := TargetElementOf(btn); got != p {
		t.Fatalf("无 form owner 的提交按钮应可作 invoker，实际 %v", got)
	}
}

func TestCommandForPopover(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	btn := appendEl(t, body, "button", "commandfor", "p", "command", "show-popover")

	if !RunActivation(btn, btn) {
		t.Fatal("commandfor 激活应被消费")
	}
	if !p.IsPopoverOpen() {
		t.Fatal("command=show-popover 应显示目标")
	}
	btn.SetAttribute("command", "hide-popover")
	RunActivation(btn, btn)
	if p.IsPopoverOpen() {
		t.Fatal("command=hide-popover 应隐藏目标")
	}
	btn.SetAttribute("command", "toggle-popover")
	RunActivation(btn, btn)
	if !p.IsPopoverOpen() {
		t.Fatal("command=toggle-popover 在隐藏时应显示目标")
	}
	RunActivation(btn, btn)
	if p.IsPopoverOpen() {
		t.Fatal("command=toggle-popover 在显示时应隐藏目标")
	}

	// 未知/非 popover 命令 → 不是 popover invoker（回退到 popovertarget 路径，
	// 这里没有该属性 → 不消费）。
	btn.SetAttribute("command", "show-modal")
	if got := commandTargetOf(btn); got != nil {
		t.Fatalf("非 popover 命令不应解析出 popover 目标，实际 %v", got)
	}
	if RunActivation(btn, btn) {
		t.Fatal("非 popover 命令不应被 popover 消费")
	}
}

func TestCommandForFallsBackToPopoverTarget(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	// commandfor 指向不存在的元素 → 回退到 popovertarget。
	btn := appendEl(t, body, "button",
		"commandfor", "missing", "command", "show-popover",
		"popovertarget", "p")

	if !RunActivation(btn, btn) {
		t.Fatal("应回退到 popovertarget 路径并消费点击")
	}
	if !p.IsPopoverOpen() {
		t.Fatal("回退路径应显示 popovertarget 指向的 popover")
	}
}

func TestSelfInvokingButtonTogglesItself(t *testing.T) {
	// WPT popover-self-invoke.html：按钮自身即 popover、且 popovertarget 指向自己，
	// 点击应当打开它（不能因为「自引用」被忽略而永远打不开）。
	_, body := newDoc(t)
	btn := appendEl(t, body, "button", "popover", "", "id", "lock", "popovertarget", "lock")

	if !RunActivation(btn, btn) {
		t.Fatal("自引用按钮的点击应被消费")
	}
	if !btn.IsPopoverOpen() {
		t.Fatal("自引用按钮点击后应打开它自己（popover 是其自身的 popover）")
	}
	// 再点一次：toggle 语义关闭。
	RunActivation(btn, btn)
	if btn.IsPopoverOpen() {
		t.Fatal("再次点击应关闭（toggle 语义）")
	}
}

func TestInvokerInsideButtonOwnedPopoverIsIgnored(t *testing.T) {
	// popover 嵌在按钮内部：点击 popover 自己内部的元素不应反复切换它
	// （规范里「eventTarget 是 popover 的（含自身）后代、popover 是 node 的严格
	// 后代 → return」这条保护）。
	_, body := newDoc(t)
	btn := appendEl(t, body, "button", "popovertarget", "nested")
	nested := appendEl(t, btn, "div", "popover", "", "id", "nested")

	if RunActivation(btn, nested) {
		t.Fatal("点击按钮内部 popover 自身时不应被消费")
	}
	if nested.IsPopoverOpen() {
		t.Fatal("该保护下 popover 不应被打开")
	}
}

func TestTargetActionEnumeration(t *testing.T) {
	_, body := newDoc(t)
	btn := appendEl(t, body, "button")
	cases := []struct {
		attr string
		want TargetAction
	}{
		{"", ActionToggle},
		{"toggle", ActionToggle},
		{"show", ActionShow},
		{"SHOW", ActionShow},
		{"hide", ActionHide},
		{"bogus", ActionToggle},
	}
	for _, c := range cases {
		if c.attr == "" {
			btn.RemoveAttribute("popovertargetaction")
		} else {
			btn.SetAttribute("popovertargetaction", c.attr)
		}
		if got := TargetActionOf(btn); got != c.want {
			t.Errorf("TargetActionOf(%q) = %q，want %q", c.attr, got, c.want)
		}
	}
}
