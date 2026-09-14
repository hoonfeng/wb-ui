// 交互校验（interactive validation）测试：HTML §4.10.21.2 —— 与
// checkValidity()（静默）相对，交互校验对每个无效控件派发 invalid 事件、
// 由 UA 聚焦第一个无效控件。表单提交（requestSubmit / 提交按钮点击）在
// 未声明 novalidate / formnovalidate 时都走这条路径。

package html5

import (
	"testing"

	"wb-ui/engine/dom"
)

// formWithRequiredInput 搭一个 form + 空 required input，并记录事件计数。
func formWithRequiredInput(t *testing.T) (*dom.Element, *dom.Element, *int, *int) {
	t.Helper()
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	in := doc.CreateElement("input")
	in.SetAttribute("type", "text")
	in.SetAttribute("required", "required")
	if err := form.AppendChild(in); err != nil {
		t.Fatalf("AppendChild(input): %v", err)
	}
	submitCount, invalidCount := 0, 0
	form.AddEventListener("submit", dom.EventListenerFunc(func(dom.Event) { submitCount++ }))
	in.AddEventListener("invalid", dom.EventListenerFunc(func(dom.Event) { invalidCount++ }))
	return form, in, &submitCount, &invalidCount
}

func TestForm_RequestSubmit_InteractiveValidation(t *testing.T) {
	form, in, submitCount, invalidCount := formWithRequiredInput(t)
	f, _ := ToFormElement(form)

	// 空 required → 校验失败：不派发 submit 事件，input 上派发 invalid 事件。
	if f.RequestSubmit(nil) {
		t.Error("有无效控件时 RequestSubmit() = true, want false")
	}
	if *submitCount != 0 {
		t.Errorf("校验失败时不应派发 submit 事件，实际 %d 次", *submitCount)
	}
	if *invalidCount != 1 {
		t.Errorf("校验失败时应在无效控件上派发 1 次 invalid 事件，实际 %d 次", *invalidCount)
	}

	// 填上值 → 通过并派发 submit 事件。
	in.SetAttribute("value", "ok")
	if !f.RequestSubmit(nil) {
		t.Error("填好值后 RequestSubmit() = false, want true")
	}
	if *submitCount != 1 {
		t.Errorf("校验通过应派发 1 次 submit 事件，实际 %d 次", *submitCount)
	}
	if *invalidCount != 1 {
		t.Errorf("校验通过不应再派发 invalid 事件，实际 %d 次", *invalidCount)
	}
}

func TestForm_RequestSubmit_NoValidate(t *testing.T) {
	// 表单声明 novalidate：跳过交互校验，直接走提交。
	form, _, submitCount, invalidCount := formWithRequiredInput(t)
	form.SetAttribute("novalidate", "")
	f, _ := ToFormElement(form)
	if !f.RequestSubmit(nil) {
		t.Error("novalidate 表单 RequestSubmit() = false, want true")
	}
	if *submitCount != 1 || *invalidCount != 0 {
		t.Errorf("novalidate：submit 次数 = %d（want 1）、invalid 次数 = %d（want 0）",
			*submitCount, *invalidCount)
	}

	// 提交者声明 formnovalidate：同样跳过校验。
	form2, _, submitCount2, invalidCount2 := formWithRequiredInput(t)
	submitter := form2.OwnerDocument().CreateElement("button")
	submitter.SetAttribute("type", "submit")
	submitter.SetAttribute("formnovalidate", "")
	_ = form2.AppendChild(submitter)
	f2, _ := ToFormElement(form2)
	if !f2.RequestSubmit(submitter) {
		t.Error("formnovalidate 提交者 RequestSubmit() = false, want true")
	}
	if *submitCount2 != 1 || *invalidCount2 != 0 {
		t.Errorf("formnovalidate：submit 次数 = %d（want 1）、invalid 次数 = %d（want 0）",
			*submitCount2, *invalidCount2)
	}
}

func TestForm_SubmitSkipsValidation(t *testing.T) {
	form, _, submitCount, invalidCount := formWithRequiredInput(t)
	f, _ := ToFormElement(form)
	// form.submit() 不做校验也不派发 submit 事件（这正是 requestSubmit 的
	// 存在理由）。
	f.Submit()
	if *submitCount != 0 {
		t.Errorf("form.submit() 不应派发 submit 事件，实际 %d 次", *submitCount)
	}
	if *invalidCount != 0 {
		t.Errorf("form.submit() 不应派发 invalid 事件，实际 %d 次", *invalidCount)
	}
}

func TestForm_ReportValidity(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	inputs := make([]*dom.Element, 0, 2)
	invalidCounts := make([]int, 0, 2)
	for i := 0; i < 2; i++ {
		in := doc.CreateElement("input")
		in.SetAttribute("type", "text")
		in.SetAttribute("required", "required")
		_ = form.AppendChild(in)
		inputs = append(inputs, in)
		invalidCounts = append(invalidCounts, 0)
		cnt := &invalidCounts[len(invalidCounts)-1]
		in.AddEventListener("invalid", dom.EventListenerFunc(func(dom.Event) { *cnt++ }))
	}
	f, _ := ToFormElement(form)

	// checkValidity：静默（不派发 invalid 事件）。
	if f.CheckValidity() {
		t.Error("有无效控件时 CheckValidity() = true, want false")
	}
	for i, c := range invalidCounts {
		if c != 0 {
			t.Errorf("CheckValidity 不应派发 invalid 事件（控件 %d 收到 %d 次）", i, c)
		}
	}

	// reportValidity：每个无效控件各派发一次 invalid 事件。
	if f.ReportValidity() {
		t.Error("有无效控件时 ReportValidity() = true, want false")
	}
	for i, c := range invalidCounts {
		if c != 1 {
			t.Errorf("ReportValidity 应让每个无效控件各收到 1 次 invalid（控件 %d 收到 %d 次）", i, c)
		}
	}

	// 全部填好 → 两者都为 true，不再派发事件。
	for _, in := range inputs {
		in.SetAttribute("value", "ok")
	}
	if !f.CheckValidity() || !f.ReportValidity() {
		t.Error("全部有效时 CheckValidity/ReportValidity 应为 true")
	}
	for i, c := range invalidCounts {
		if c != 1 {
			t.Errorf("有效时不应再派发 invalid（控件 %d 收到 %d 次）", i, c)
		}
	}
}

func TestValidateInteractively_ReturnsInvalidControls(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.CreateElement("form")
	bad := doc.CreateElement("input")
	bad.SetAttribute("type", "text")
	bad.SetAttribute("required", "required")
	good := doc.CreateElement("input")
	good.SetAttribute("type", "text")
	good.SetAttribute("value", "ok")
	_ = form.AppendChild(bad)
	_ = form.AppendChild(good)
	f, _ := ToFormElement(form)

	invalid := f.ValidateInteractively()
	if len(invalid) != 1 || invalid[0] != bad {
		t.Fatalf("ValidateInteractively() = %v，want 只含 required 空控件", invalid)
	}

	// InteractiveValidity：不参与校验的元素返回 ok=false。
	if valid, ok := InteractiveValidity(good); !valid || !ok {
		t.Errorf("有效控件 InteractiveValidity = (%v, %v), want (true, true)", valid, ok)
	}
	div := doc.CreateElement("div")
	if valid, ok := InteractiveValidity(div); !valid || ok {
		t.Errorf("非表单控件 InteractiveValidity = (%v, %v), want (true, false)", valid, ok)
	}
	// barred 元素：参与校验但恒有效。
	ro := doc.CreateElement("input")
	ro.SetAttribute("type", "text")
	ro.SetAttribute("required", "required")
	ro.SetAttribute("readonly", "readonly")
	_ = form.AppendChild(ro)
	if valid, ok := InteractiveValidity(ro); !valid || !ok {
		t.Errorf("readonly 控件 InteractiveValidity = (%v, %v), want (true, true)", valid, ok)
	}
}

// TestInteractiveValidationSetsUserValidity 覆盖 user validity（HTML
// §4.10.18.1）：交互校验（提交尝试）把控件的 user validity 置为 true，并通知
// 宿主让样式重算；重复校验不再重复通知。
func TestInteractiveValidationSetsUserValidity(t *testing.T) {
	form, in, _, _ := formWithRequiredInput(t)
	f, _ := ToFormElement(form)

	if st, _ := ConstraintValidity(in); st.UserInteracted {
		t.Fatal("初始 user validity 应为 false")
	}

	changes := 0
	prev := OnUserValidityChanged
	OnUserValidityChanged = func(el *dom.Element) {
		if el == in {
			changes++
		}
	}
	defer func() { OnUserValidityChanged = prev }()

	if f.RequestSubmit(nil) {
		t.Fatal("空 required 时 RequestSubmit() 应为 false")
	}
	st, _ := ConstraintValidity(in)
	if !st.UserInteracted {
		t.Error("提交尝试后 user validity 应为 true")
	}
	if changes != 1 {
		t.Errorf("user validity 变化应通知宿主 1 次，实际 %d 次", changes)
	}

	// 再次校验：状态无变化 → 不重复通知。
	if f.RequestSubmit(nil) {
		t.Fatal("仍然无效时 RequestSubmit() 应为 false")
	}
	if changes != 1 {
		t.Errorf("重复校验不应重复通知，实际 %d 次", changes)
	}
}
