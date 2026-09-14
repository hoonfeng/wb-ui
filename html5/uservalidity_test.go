// user validity 的「焦点会话」记忆测试（HTML §4.10.18.1 / MDN :user-valid）：
// 值在获得焦点时无效、而用户在焦点仍在控件内时把它改成有效 → 立即获得 user
// validity（:user-invalid 是镜像方向）。没有焦点会话时（脚本赋值、程序化写值）
// 一律不置位——脚本改变值不构成用户交互。

package html5

import (
	"testing"

	"wb-ui/dom"
)

// TestNoteFocusGainedRecordsValidity 覆盖 dom.SetFocused(true) → 本包注入的
// NoteFocusGained 写入「聚焦时是否有效」记忆的契约，以及哪些元素不记忆。
func TestNoteFocusGainedRecordsValidity(t *testing.T) {
	form, in, _, _ := formWithRequiredInput(t)

	// 空 required：聚焦瞬间无效 → 记忆 (valid=false, known=true)。
	in.SetFocused(true)
	if valid, known := in.FocusValidity(); !known || valid {
		t.Errorf("无效控件聚焦后 FocusValidity = (%v, %v), want (false, true)", valid, known)
	}
	// 失焦清除记忆（记忆只在焦点会话内有效）。
	in.SetFocused(false)
	if _, known := in.FocusValidity(); known {
		t.Error("失焦后应清除焦点会话记忆")
	}
	// 有值时聚焦 → 记忆为有效。
	in.SetAttribute("value", "ok")
	in.SetFocused(true)
	if valid, known := in.FocusValidity(); !known || !valid {
		t.Errorf("有效控件聚焦后 FocusValidity = (%v, %v), want (true, true)", valid, known)
	}
	in.SetFocused(false)

	doc := form.OwnerDocument()
	// barred 元素（readonly）：:user-valid / :user-invalid 都不匹配 → 不记忆。
	ro := doc.CreateElement("input")
	ro.SetAttribute("type", "text")
	ro.SetAttribute("required", "required")
	ro.SetAttribute("readonly", "readonly")
	if err := form.AppendChild(ro); err != nil {
		t.Fatalf("AppendChild(readonly input): %v", err)
	}
	ro.SetFocused(true)
	if _, known := ro.FocusValidity(); known {
		t.Error("barred（readonly）元素不应记录焦点会话记忆")
	}
	// 非表单控件（div）：同样不记忆。
	div := doc.CreateElement("div")
	div.SetFocused(true)
	if _, known := div.FocusValidity(); known {
		t.Error("非表单控件不应记录焦点会话记忆")
	}
}

// TestNoteUserInputFlipsValidityInFocusSession 覆盖 MDN 第 3 条的两个方向：
// 聚焦时无效→改成有效（:user-valid 方向）、聚焦时有效→改成无效（:user-invalid
// 方向）都立即置 user validity；有效性没翻转时不置位。
func TestNoteUserInputFlipsValidityInFocusSession(t *testing.T) {
	changes := 0
	prev := OnUserValidityChanged
	OnUserValidityChanged = func(*dom.Element) { changes++ }
	defer func() { OnUserValidityChanged = prev }()

	// ── 方向一：聚焦时无效 → 用户输入使其有效 ──
	_, in, _, _ := formWithRequiredInput(t)
	in.SetFocused(true)
	if valid, _ := in.FocusValidity(); valid {
		t.Fatal("空 required 聚焦时记忆应为无效")
	}
	in.SetAttribute("value", "ok") // 模拟用户击键写入值
	NoteUserInput(in)
	if !in.UserInteracted() {
		t.Error("焦点内把无效值改成有效后，user validity 应为 true（:user-valid 立即生效）")
	}
	if changes != 1 {
		t.Errorf("user validity 变化应通知宿主 1 次，实际 %d 次", changes)
	}
	NoteUserInput(in) // 幂等：已置位不再通知
	if changes != 1 {
		t.Errorf("已置位时不应重复通知，实际 %d 次", changes)
	}

	// ── 方向二（镜像）：聚焦时有效 → 用户清空使其无效 ──
	_, in2, _, _ := formWithRequiredInput(t)
	in2.SetAttribute("value", "ok")
	in2.SetFocused(true)
	if valid, known := in2.FocusValidity(); !known || !valid {
		t.Fatal("有值 required 聚焦时记忆应为有效")
	}
	in2.RemoveAttribute("value")
	NoteUserInput(in2)
	if !in2.UserInteracted() {
		t.Error("焦点内把有效值改成无效后，user validity 应为 true（:user-invalid 立即生效）")
	}

	// ── 未翻转：聚焦时无效，用户改了值但仍无效 → 不置位（避免纠正过程中闪烁）──
	_, in3, _, _ := formWithRequiredInput(t)
	in3.SetFocused(true)
	NoteUserInput(in3)
	if in3.UserInteracted() {
		t.Error("有效性未翻转时不应置 user validity")
	}

	// ── barred 控件不参与判定 ──
	_, in4, _, _ := formWithRequiredInput(t)
	in4.SetAttribute("readonly", "readonly")
	in4.SetFocused(true)
	NoteUserInput(in4)
	if in4.UserInteracted() {
		t.Error("barred（readonly）控件不应置 user validity")
	}
}

// TestNoteUserInputWithoutFocusSessionIgnored 覆盖「没有焦点会话 → 不置位」：
// 脚本赋值即使翻转了有效性也不构成用户交互。
func TestNoteUserInputWithoutFocusSessionIgnored(t *testing.T) {
	// 从未聚焦。
	_, in, _, _ := formWithRequiredInput(t)
	in.SetAttribute("value", "ok")
	NoteUserInput(in)
	if in.UserInteracted() {
		t.Error("从未聚焦时脚本赋值不应置 user validity")
	}

	// 焦点会话已结束（失焦后记忆被清除）。
	_, in2, _, _ := formWithRequiredInput(t)
	in2.SetFocused(true)
	in2.SetFocused(false)
	in2.SetAttribute("value", "ok")
	NoteUserInput(in2)
	if in2.UserInteracted() {
		t.Error("失焦后（无焦点会话记忆）写值不应置 user validity")
	}
}
