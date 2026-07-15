// Tests for the app host utility functions.
// These tests do not require GLFW or a display.

package app

import (
	"testing"

	"wb-ui/dom"
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
