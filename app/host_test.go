// Tests for the app host utility functions.
// These tests do not require GLFW or a display.

package app

import (
	"testing"

	"wb-ui/engine/dom"
)

func TestIsTextFormControl_Nil(t *testing.T) {
	if isTextFormControl(nil) {
		t.Error("isTextFormControl(nil) = true, want false")
	}
}

func TestIsTextFormControl_Textarea(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("textarea")
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(textarea) = false, want true")
	}
}

func TestIsTextFormControl_InputText(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "text")
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=text]) = false, want true")
	}
}

func TestIsTextFormControl_InputDefault(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	// Default type is "text"
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(input without type) = false, want true (default = text)")
	}
}

func TestIsTextFormControl_InputCheckbox(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "checkbox")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=checkbox]) = true, want false")
	}
}

func TestIsTextFormControl_InputRadio(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "radio")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=radio]) = true, want false")
	}
}

func TestIsTextFormControl_InputSubmit(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "submit")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=submit]) = true, want false")
	}
}

func TestIsTextFormControl_InputPassword(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "password")
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=password]) = false, want true")
	}
}

func TestIsTextFormControl_Div(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(div) = true, want false")
	}
}

func TestFocusedElementValue_Nil(t *testing.T) {
	if got := focusedElementValue(nil); got != "" {
		t.Errorf("focusedElementValue(nil) = %q, want empty", got)
	}
}

func TestFocusedElementValue_Input(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "text")
	el.SetAttribute("value", "hello")
	if got := focusedElementValue(el); got != "hello" {
		t.Errorf("focusedElementValue(input) = %q, want hello", got)
	}
}

func TestFocusedElementValue_Div(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	el.SetTextContent("hello")
	if got := focusedElementValue(el); got != "hello" {
		t.Errorf("focusedElementValue(div) = %q, want hello", got)
	}
}

func TestSetFocusedElementValue_Input(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	setFocusedElementValue(el, "world")
	if got := el.GetAttribute("value"); got != "world" {
		t.Errorf("input value after set = %q, want world", got)
	}
}

func TestSetFocusedElementValue_Div(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	setFocusedElementValue(el, "world")
	if got := el.TextContent(); got != "world" {
		t.Errorf("div textContent after set = %q, want world", got)
	}
}

func TestSetFocusedElementValue_Nil(t *testing.T) {
	// Should not panic
	setFocusedElementValue(nil, "test")
}

// TestSetFocusedElementValue_FlipsUserValidity 覆盖 app 层的用户输入写入点
// （IME/组合输入的字符提交都经 setFocusedElementValue）与 user validity 的
// 联动：控件在焦点会话内被写入使有效性翻转的值 → 立即获得 user validity
// （MDN :user-valid 第 3 条，见 engine/html5/uservalidity.go）；没有焦点会话时
// （程序化写值）不置位。
func TestSetFocusedElementValue_FlipsUserValidity(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "text")
	el.SetAttribute("required", "required")

	// 未聚焦（无焦点会话）：脚本式写值不构成用户交互。
	setFocusedElementValue(el, "ok")
	if el.UserInteracted() {
		t.Error("无焦点会话时写值不应置 user validity")
	}

	// 聚焦会话内：空 required（无效）→ 写入有效值 → 立即置位。
	el.RemoveAttribute("value")
	el.SetFocused(true)
	if _, known := el.FocusValidity(); !known {
		t.Fatal("聚焦后应建立焦点会话记忆")
	}
	setFocusedElementValue(el, "ok")
	if !el.UserInteracted() {
		t.Error("焦点会话内写值使无效变有效后应置 user validity（:user-valid 立即生效）")
	}
}

func TestIsTextFormControl_InputHidden(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "hidden")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=hidden]) = true, want false")
	}
}

func TestIsTextFormControl_InputRange(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "range")
	if isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=range]) = true, want false")
	}
}

func TestIsTextFormControl_InputNumber(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "number")
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=number]) = false, want true")
	}
}

func TestIsTextFormControl_InputEmail(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "email")
	if !isTextFormControl(el) {
		t.Error("isTextFormControl(input[type=email]) = false, want true")
	}
}
