// Popover 子系统的行为测试（HTML §6.12 的算法）：状态机、top layer 顺序、
// 相互关闭规则、light dismiss、close request、属性变更步骤与 invoker 激活行为。
//
// 测试直接驱动 Go 层算法（不经过 JS），因此把 QueueTask 留空——未注入队列
// 钩子时 popover 同步派发 toggle 事件，断言的事件顺序是确定的（宿主注入的
// 异步队列在 bindings 层测试里覆盖）。
package popover

import (
	"testing"

	"wb-ui/engine/dom"
)

// ── 测试辅助 ────────────────────────────────────────────────────────────────

func newDoc(t *testing.T) (*dom.Document, *dom.Element) {
	t.Helper()
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	if err := doc.AppendChild(html); err != nil {
		t.Fatalf("append html: %v", err)
	}
	body := dom.NewElement(doc, "body")
	if err := html.AppendChild(body); err != nil {
		t.Fatalf("append body: %v", err)
	}
	return doc, body
}

func appendEl(t *testing.T, parent *dom.Element, tag string, attrs ...string) *dom.Element {
	t.Helper()
	doc := parent.OwnerDocument()
	el := dom.NewElement(doc, tag)
	for i := 0; i+1 < len(attrs); i += 2 {
		el.SetAttribute(attrs[i], attrs[i+1])
	}
	if err := parent.AppendChild(el); err != nil {
		t.Fatalf("append %s: %v", tag, err)
	}
	return el
}

// toggleRec 记录一次 toggle 事件的关键字段。
type toggleRec struct {
	typ    string
	old    string
	new    string
	source *dom.Element
}

func (r toggleRec) String() string {
	src := "<nil>"
	if r.source != nil {
		src = r.source.LocalName() + "#" + r.source.GetAttribute("id")
	}
	return r.typ + "(" + r.old + "→" + r.new + ",source=" + src + ")"
}

func recordToggles(el *dom.Element) *[]toggleRec {
	rec := &[]toggleRec{}
	h := dom.EventListenerFunc(func(e dom.Event) {
		ev, ok := e.(*dom.ToggleEvent)
		if !ok {
			return
		}
		*rec = append(*rec, toggleRec{ev.Type(), ev.OldState(), ev.NewState(), ev.Source()})
	})
	el.AddEventListener("beforetoggle", h, false)
	el.AddEventListener("toggle", h, false)
	return rec
}

func recStrings(rec *[]toggleRec) []string {
	out := make([]string, 0, len(*rec))
	for _, r := range *rec {
		out = append(out, r.String())
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── 枚举属性解析 ─────────────────────────────────────────────────────────────

func TestModeOfEnumeration(t *testing.T) {
	_, body := newDoc(t)
	cases := []struct {
		attr   *string
		want   Mode
		reason string
	}{
		{nil, ModeNone, "缺省值默认：No Popover"},
		{ptr(""), ModeAuto, "空值默认：Auto"},
		{ptr("auto"), ModeAuto, "关键字 auto"},
		{ptr("AUTO"), ModeAuto, "关键字 ASCII 大小写不敏感"},
		{ptr(" hint "), ModeHint, "关键字 hint（两侧空白忽略）"},
		{ptr("Hint"), ModeHint, "关键字 Hint"},
		{ptr("manual"), ModeManual, "关键字 manual"},
		{ptr("bogus"), ModeManual, "无效值默认：Manual"},
	}
	for _, c := range cases {
		el := appendEl(t, body, "div")
		if c.attr != nil {
			el.SetAttribute("popover", *c.attr)
		}
		if got := ModeOf(el); got != c.want {
			t.Errorf("ModeOf(popover=%v) = %q，want %q（%s）", c.attr, got, c.want, c.reason)
		}
	}
}

func ptr(s string) *string { return &s }

// ── show / hide 基本行为 ────────────────────────────────────────────────────

func TestShowHideBasics(t *testing.T) {
	doc, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "", "id", "p")
	rec := recordToggles(p)

	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !p.IsPopoverOpen() {
		t.Fatal("showPopover 后 IsPopoverOpen 应为 true")
	}
	if st := p.Popover(); st.Mode != string(ModeAuto) {
		t.Fatalf("opened in popover mode = %q，want auto", st.Mode)
	}
	if got := len(doc.Popover().Stack); got != 1 {
		t.Fatalf("top layer 元素数 = %d，want 1", got)
	}
	// Show 重复调用：规范第一步判定「已显示」→ 静默返回，不再派发事件。
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("重复 Show 应静默：%v", err)
	}
	if got := recStrings(rec); !eqStrings(got, []string{
		"beforetoggle(closed→open,source=<nil>)",
		"toggle(closed→open,source=<nil>)",
	}) {
		t.Fatalf("Show 事件序列 = %v", got)
	}
	*rec = (*rec)[:0]

	if err := Hide(p, true, true, true, nil); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if p.IsPopoverOpen() {
		t.Fatal("hidePopover 后 IsPopoverOpen 应为 false")
	}
	if st := p.Popover(); st.Mode != "" {
		t.Fatalf("关闭后 opened in popover mode 应为空，实际 %q", st.Mode)
	}
	if got := len(doc.Popover().Stack); got != 0 {
		t.Fatalf("关闭后 top layer 元素数 = %d，want 0", got)
	}
	if got := recStrings(rec); !eqStrings(got, []string{
		"beforetoggle(open→closed,source=<nil>)",
		"toggle(open→closed,source=<nil>)",
	}) {
		t.Fatalf("Hide 事件序列 = %v", got)
	}
}

func TestShowRequiresPopoverAttribute(t *testing.T) {
	_, body := newDoc(t)
	plain := appendEl(t, body, "div")
	err := Show(plain, true, nil)
	perr, ok := err.(*Error)
	if !ok || perr.Name != "NotSupportedError" {
		t.Fatalf("无 popover 属性时 Show 应抛 NotSupportedError，实际 %v", err)
	}
	// throwExceptions=false（invoker 路径）时不报错也不显示。
	if err := Show(plain, false, nil); err != nil {
		t.Fatalf("throwExceptions=false 时不应返回错误，实际 %v", err)
	}
	if plain.IsPopoverOpen() {
		t.Fatal("无 popover 属性的元素不应被显示")
	}
}

func TestShowDisconnectedElement(t *testing.T) {
	doc, _ := newDoc(t)
	orphan := dom.NewElement(doc, "div")
	orphan.SetAttribute("popover", "")
	err := Show(orphan, true, nil)
	perr, ok := err.(*Error)
	if !ok || perr.Name != "InvalidStateError" {
		t.Fatalf("未连接元素 Show 应抛 InvalidStateError，实际 %v", err)
	}
}

func TestShowModalDialogIsInvalidPopoverHost(t *testing.T) {
	_, body := newDoc(t)
	dlg := appendEl(t, body, "dialog", "popover", "")
	dlg.SetModalState(true)
	err := Show(dlg, true, nil)
	perr, ok := err.(*Error)
	if !ok || perr.Name != "InvalidStateError" {
		t.Fatalf("模态 dialog 上的 popover 应抛 InvalidStateError，实际 %v", err)
	}
}

func TestBeforeToggleCancelable(t *testing.T) {
	doc, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "")
	var cancel dom.EventListener = dom.EventListenerFunc(func(e dom.Event) {
		if e.(*dom.ToggleEvent).NewState() == StateOpen {
			e.PreventDefault()
		}
	})
	p.AddEventListener("beforetoggle", cancel, false)

	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show 返回错误：%v", err)
	}
	if p.IsPopoverOpen() {
		t.Fatal("beforetoggle 被取消后不应显示")
	}
	if doc.Popover().Showing {
		t.Fatal("取消后 document 的 showing popover 标记应复位")
	}
	// 取消后仍能再次正常显示（重入标记没有泄漏）。
	p.RemoveEventListener("beforetoggle", cancel, false)
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("二次 Show：%v", err)
	}
	if !p.IsPopoverOpen() {
		t.Fatal("取消一次后应仍可显示")
	}
}

// ── popover 之间的相互关闭 ──────────────────────────────────────────────────

func TestAutoClosesOtherAutoPopovers(t *testing.T) {
	_, body := newDoc(t)
	a := appendEl(t, body, "div", "popover", "auto", "id", "a")
	b := appendEl(t, body, "div", "popover", "auto", "id", "b")

	if err := Show(a, true, nil); err != nil {
		t.Fatalf("Show(a): %v", err)
	}
	if err := Show(b, true, nil); err != nil {
		t.Fatalf("Show(b): %v", err)
	}
	if a.IsPopoverOpen() {
		t.Fatal("打开第二个 auto popover 后，第一个应被关闭")
	}
	if !b.IsPopoverOpen() {
		t.Fatal("第二个 auto popover 应处于显示状态")
	}
}

func TestManualPopoverIsInert(t *testing.T) {
	doc, body := newDoc(t)
	a := appendEl(t, body, "div", "popover", "auto")
	m := appendEl(t, body, "div", "popover", "manual")

	if err := Show(a, true, nil); err != nil {
		t.Fatalf("Show(a): %v", err)
	}
	if err := Show(m, true, nil); err != nil {
		t.Fatalf("Show(manual): %v", err)
	}
	if !a.IsPopoverOpen() {
		t.Fatal("manual popover 打开时不应关闭 auto popover")
	}
	if st := m.Popover(); st.Mode != "" {
		t.Fatalf("manual popover 的 opened in popover mode 应为空（规范只在 auto/hint 设置），实际 %q", st.Mode)
	}
	// manual 不在 showing auto/hint list 中 → 不参与 close request 与 light dismiss。
	if top := topmostAutoOrHintPopover(doc); top != a {
		t.Fatalf("topmost auto or hint popover = %v，want a", top)
	}
	if !CloseRequest(doc) {
		t.Fatal("close request 应关闭 auto popover a")
	}
	if a.IsPopoverOpen() {
		t.Fatal("close request 后 a 应关闭")
	}
	if !m.IsPopoverOpen() {
		t.Fatal("manual popover 不响应 close request，应保持显示")
	}
	if CloseRequest(doc) {
		t.Fatal("只剩 manual popover 时 close request 不应被消费")
	}
}

func TestHintClosesOtherHintsButNotAuto(t *testing.T) {
	_, body := newDoc(t)
	a := appendEl(t, body, "div", "popover", "auto")
	h1 := appendEl(t, body, "div", "popover", "hint", "id", "h1")
	h2 := appendEl(t, body, "div", "popover", "hint", "id", "h2")

	if err := Show(a, true, nil); err != nil {
		t.Fatalf("Show(a): %v", err)
	}
	if err := Show(h1, true, nil); err != nil {
		t.Fatalf("Show(h1): %v", err)
	}
	if !a.IsPopoverOpen() || !h1.IsPopoverOpen() {
		t.Fatal("hint popover 打开时不应关闭 auto popover")
	}
	if st := h1.Popover(); st.Mode != string(ModeHint) {
		t.Fatalf("hint popover 的 opened in popover mode = %q，want hint", st.Mode)
	}
	if err := Show(h2, true, nil); err != nil {
		t.Fatalf("Show(h2): %v", err)
	}
	if h1.IsPopoverOpen() {
		t.Fatal("打开第二个 hint popover 时第一个应被关闭")
	}
	if !a.IsPopoverOpen() {
		t.Fatal("hint popover 的相互关闭不应影响 auto popover")
	}
	if !h2.IsPopoverOpen() {
		t.Fatal("第二个 hint popover 应处于显示状态")
	}
}

func TestNestedPopoverKeepsAncestorOpen(t *testing.T) {
	doc, body := newDoc(t)
	outer := appendEl(t, body, "div", "popover", "auto", "id", "outer")
	inner := appendEl(t, outer, "div", "popover", "auto", "id", "inner")

	if err := Show(outer, true, nil); err != nil {
		t.Fatalf("Show(outer): %v", err)
	}
	// topmost popover ancestor 只在「新 popover 尚未显示」时有意义（规范断言），
	// 因此在 Show(inner) 之前询问。
	if ancestor := topmostPopoverAncestor(inner, nil, true); ancestor != outer {
		t.Fatalf("topmost popover ancestor = %v，want outer", ancestor)
	}
	if err := Show(inner, true, nil); err != nil {
		t.Fatalf("Show(inner): %v", err)
	}
	if !outer.IsPopoverOpen() {
		t.Fatal("嵌套在 popover 内部的 popover 打开时，祖先 popover 应保持显示")
	}
	if !inner.IsPopoverOpen() {
		t.Fatal("内部 popover 应处于显示状态")
	}
	// 栈顺序：先加入的祖先在前（top layer 顺序）。
	stack := doc.Popover().Stack
	if len(stack) != 2 || stack[0] != outer || stack[1] != inner {
		t.Fatalf("top layer 顺序 = %v，want [outer inner]", stack)
	}
}

// ── light dismiss / close request ──────────────────────────────────────────

func TestLightDismissPointerPair(t *testing.T) {
	doc, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "auto", "id", "p")
	inner := appendEl(t, p, "span")
	outside := appendEl(t, body, "button")

	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}

	// 1) popover 内部按下 + 内部抬起 → 不关闭（拖动选择文本不误关）。
	LightDismissPointerDown(doc, inner)
	if doc.Popover().PointerdownTarget != p {
		t.Fatalf("pointerdown 记录 = %v，want p", doc.Popover().PointerdownTarget)
	}
	LightDismissPointerUp(doc, inner)
	if !p.IsPopoverOpen() {
		t.Fatal("popover 内部点击不应关闭它")
	}

	// 2) 内部按下 + 外部抬起（跨边界拖动）→ 不关闭。
	LightDismissPointerDown(doc, inner)
	LightDismissPointerUp(doc, outside)
	if !p.IsPopoverOpen() {
		t.Fatal("pointerdown/pointerup 目标不一致时不应关闭")
	}
	if doc.Popover().PointerdownTarget != nil {
		t.Fatal("pointerup 应清空 pointerdown 记忆")
	}

	// 3) 外部按下 + 外部抬起 → 关闭。
	LightDismissPointerDown(doc, outside)
	if !LightDismissPointerUp(doc, outside) {
		t.Fatal("外部点击关闭 popover 时 LightDismissPointerUp 应返回 true")
	}
	if p.IsPopoverOpen() {
		t.Fatal("popover 外部点击应关闭它")
	}

	// 4) 点在空白处（target=nil）→ 同样关闭。
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}
	LightDismissPointerDown(doc, nil)
	if !LightDismissPointerUp(doc, nil) {
		t.Fatal("空白处点击应关闭 popover")
	}
	if p.IsPopoverOpen() {
		t.Fatal("空白处点击后 popover 应关闭")
	}
}

func TestCloseRequestIgnoresManual(t *testing.T) {
	doc, body := newDoc(t)
	m := appendEl(t, body, "div", "popover", "manual")
	if err := Show(m, true, nil); err != nil {
		t.Fatalf("Show(manual): %v", err)
	}
	if CloseRequest(doc) {
		t.Fatal("manual popover 不响应 close request")
	}
	if !m.IsPopoverOpen() {
		t.Fatal("manual popover 应保持显示")
	}
}

func TestCloseRequestClosesTopmostHintFirst(t *testing.T) {
	doc, body := newDoc(t)
	a := appendEl(t, body, "div", "popover", "auto")
	h := appendEl(t, body, "div", "popover", "hint")
	if err := Show(a, true, nil); err != nil {
		t.Fatalf("Show(a): %v", err)
	}
	if err := Show(h, true, nil); err != nil {
		t.Fatalf("Show(hint): %v", err)
	}
	if !CloseRequest(doc) {
		t.Fatal("close request 应被消费")
	}
	if h.IsPopoverOpen() {
		t.Fatal("close request 应先关闭最上层的 hint popover")
	}
	if !a.IsPopoverOpen() {
		t.Fatal("hint 关闭时其父 auto popover 应保持显示")
	}
}

// ── 属性变更步骤 ─────────────────────────────────────────────────────────────

func TestAttributeChangeWhileShowing(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "auto")
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}
	// "auto" → "AUTO"：枚举值大小写不同但**状态相同**（都是 Auto）→ 不关闭。
	p.SetAttribute("popover", "AUTO")
	if !p.IsPopoverOpen() {
		t.Fatal("属性值变化但状态不变时不应关闭 popover")
	}

	// Auto → Manual：状态变了 → 关闭。
	p.SetAttribute("popover", "manual")
	if p.IsPopoverOpen() {
		t.Fatal("popover 属性状态变化时显示的 popover 应被关闭（attribute change steps）")
	}

	// 移除 popover 属性（Auto → No Popover）：状态变了 → 关闭。
	p.SetAttribute("popover", "auto") // manual → auto（此刻未显示，无副作用）
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("二次 Show: %v", err)
	}
	p.RemoveAttribute("popover")
	if p.IsPopoverOpen() {
		t.Fatal("移除 popover 属性应关闭显示中的 popover")
	}
}

// ── togglePopover 的 force 语义 ──────────────────────────────────────────────

func TestToggleForceSemantics(t *testing.T) {
	_, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "")

	showing, err := Toggle(p, nil, nil)
	if err != nil || !showing {
		t.Fatalf("togglePopover() = %v, %v；want true, nil", showing, err)
	}
	// force=false 且正在显示 → 隐藏。
	forceFalse := false
	showing, err = Toggle(p, &forceFalse, nil)
	if err != nil || showing {
		t.Fatalf("togglePopover(false) = %v, %v；want false, nil", showing, err)
	}
	// force=false 且已隐藏 → 无操作，返回 false。
	showing, err = Toggle(p, &forceFalse, nil)
	if err != nil || showing {
		t.Fatalf("togglePopover(false) 于隐藏状态 = %v, %v；want false, nil", showing, err)
	}
	// force=true → 显示。
	forceTrue := true
	showing, err = Toggle(p, &forceTrue, nil)
	if err != nil || !showing {
		t.Fatalf("togglePopover(true) = %v, %v；want true, nil", showing, err)
	}
	if !p.IsPopoverOpen() {
		t.Fatal("togglePopover(true) 后应显示")
	}
}

func TestToggleOnNonPopoverThrows(t *testing.T) {
	_, body := newDoc(t)
	plain := appendEl(t, body, "div")
	if _, err := Toggle(plain, nil, nil); err == nil {
		t.Fatal("无 popover 属性的元素 togglePopover() 应抛 NotSupportedError")
	}
}

// ── 焦点恢复 ────────────────────────────────────────────────────────────────

func TestFocusRestoreOnHide(t *testing.T) {
	doc, body := newDoc(t)
	btn := appendEl(t, body, "button", "id", "btn")
	p := appendEl(t, body, "div", "popover", "")
	inner := appendEl(t, p, "input", "id", "inner")

	btn.SetFocused(true)
	if doc.FocusedElement() != btn {
		t.Fatal("测试前置：按钮应持有焦点")
	}
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if st := p.Popover(); st.PreviouslyFocused != btn {
		t.Fatalf("首层 popover 应记住原本聚焦的元素，实际 %v", st.PreviouslyFocused)
	}
	// 焦点进入 popover 内部。
	btn.SetFocused(false)
	inner.SetFocused(true)

	if err := Hide(p, true, true, true, nil); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	if doc.FocusedElement() != btn {
		t.Fatalf("关闭后焦点应回到打开前的元素，实际 %v", doc.FocusedElement())
	}
	if p.Popover().PreviouslyFocused != nil {
		t.Fatal("焦点恢复后应清空 previously focused element")
	}
}

func TestAutofocusInsidePopover(t *testing.T) {
	doc, body := newDoc(t)
	p := appendEl(t, body, "div", "popover", "")
	auto := appendEl(t, p, "input", "autofocus", "")
	if err := Show(p, true, nil); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if doc.FocusedElement() != auto {
		t.Fatalf("带 autofocus 的元素应获得焦点（popover focusing steps），实际 %v", doc.FocusedElement())
	}
}
