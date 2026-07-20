// JS bindings for the wb-ui editor (code editor component).
//
// Exposes a wb namespace to JavaScript with an Editor constructor:
//
//	const ed = new wb.Editor({ language: "go", value: "package main" });
//	ed.getValue();          // string
//	ed.setValue("code");    // void
//	ed.onChange(callback);  // void
//	ed.setLanguage("js");   // void
//	ed.focus();             // void
//
// This follows the same pattern as bindings/dom.go: functions are registered
// as native JS functions on the global object.

package editor

import (
	"wb-ui/jsc"
	"wb-ui/platform/graphics"
)

// editorBindingsData holds per-editor-instance state for the JS bridge.
type editorBindingsData struct {
	view       *EditorView
	onChangeJS jsc.JSValue // JS callback (function)
}

// RegisterEditorJSBindings registers editor bindings under the "wb" namespace
// on the JS global object. After calling this, JS code can use:
//
//	ed = new wb.Editor(config)
//
// where config is an object with optional keys: language, theme, value,
// showLineNumbers.
func RegisterEditorJSBindings(rt *jsc.Interpreter) {
	// Create or get the wb namespace object.
	wb := ensureWBNamespace(rt)

	// wb.Editor constructor.
	wb.Set("Editor", jsc.FunctionValue(jsc.NewNativeFunction("Editor", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		// Parse config object.
		lang := "text"
		value := ""
		showLineNumbers := true

		if len(args) > 0 && args[0].IsObject() {
			cfg := args[0].AsObject()
			if v, ok := cfg.GetByKey("language"); ok && v.IsString() {
				lang = v.AsString()
			}
			if v, ok := cfg.GetByKey("value"); ok && v.IsString() {
				value = v.AsString()
			}
			if v, ok := cfg.GetByKey("showLineNumbers"); ok {
				showLineNumbers = v.ToBoolean()
			}
		}

		// Create the editor.
		sel := SelectionCaret(0)
		state := NewState(Config{
			Doc:       TextFromString(value),
			Selection: &sel,
		})

		font := graphics.Font{Family: "Consolas", Size: 14}
		view := NewEditorView(EditorViewConfig{
			State:           state,
			Font:            font,
			ShowLineNumbers: showLineNumbers,
			Width:           800,
			Height:          600,
		})

		// Set language if specified.
		if lang != "" && lang != "text" {
			if langFn := lookupLanguage(lang); langFn != nil {
				view.SetLanguage(langFn())
			}
		}

		// Build the editor wrapper object with methods.
		edObj := jsc.NewObject(in.ObjectPrototype())
		edObj.SetClassName("Editor")
		edObj.SetInternal(&editorBindingsData{view: view})

		// getValue()
		edObj.Set("getValue", jsc.FunctionValue(jsc.NewNativeFunction("getValue", func(in2 *jsc.Interpreter, this2 jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			d := dataFromThis(this2)
			if d == nil {
				return jsc.Undefined()
			}
			return jsc.StringValue(d.view.state.Doc.String())
		}, 0)))

		// setValue(text)
		edObj.Set("setValue", jsc.FunctionValue(jsc.NewNativeFunction("setValue", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			d := dataFromThis(this2)
			if d == nil {
				return jsc.Undefined()
			}
			text := ""
			if len(args2) > 0 {
				text = args2[0].ToString()
			}
			// Replace the entire document.
			docLen := d.view.state.Doc.Length()
			changes := NewChangeSetWithText(
				[]ChangeDesc{{FromA: 0, ToA: docLen, InsertLength: len([]rune(text))}},
				[]Text{TextFromString(text)},
			)
			sel2 := SelectionCaret(0)
			d.view.Dispatch(TransactionSpec{
				Selection:    &sel2,
				HasSelection: true,
				Changes:      changes,
			})
			return jsc.Undefined()
		}, 1)))

		// onChange(callback)
		edObj.Set("onChange", jsc.FunctionValue(jsc.NewNativeFunction("onChange", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			d := dataFromThis(this2)
			if d == nil {
				return jsc.Undefined()
			}
			if len(args2) > 0 && args2[0].IsCallable() {
				d.onChangeJS = args2[0]
			}
			return jsc.Undefined()
		}, 1)))

		// setLanguage(lang)
		edObj.Set("setLanguage", jsc.FunctionValue(jsc.NewNativeFunction("setLanguage", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			d := dataFromThis(this2)
			if d == nil {
				return jsc.Undefined()
			}
			if len(args2) > 0 {
				langName := args2[0].ToString()
				if langFn := lookupLanguage(langName); langFn != nil {
					d.view.SetLanguage(langFn())
				}
			}
			return jsc.Undefined()
		}, 1)))

		// focus()
		edObj.Set("focus", jsc.FunctionValue(jsc.NewNativeFunction("focus", func(in2 *jsc.Interpreter, this2 jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined() // no-op; focus not implemented in this port
		}, 0)))

		return jsc.ObjectValue(edObj)
	}, 1)))
}

// ensureWBNamespace returns the global "wb" object, creating it on first access.
func ensureWBNamespace(rt *jsc.Interpreter) *jsc.JSObject {
	g := rt.GlobalObject()
	if v, ok := g.GetByKey("wb"); ok && v.IsObject() {
		return v.AsObject()
	}
	wb := jsc.NewObject(rt.ObjectPrototype())
	wb.SetClassName("wb")
	g.Set("wb", jsc.ObjectValue(wb))
	return wb
}

// dataFromThis extracts the editorBindingsData from the 'this' value of a method call.
func dataFromThis(this jsc.JSValue) *editorBindingsData {
	if !this.IsObject() {
		return nil
	}
	o := this.AsObject()
	if o.Internal() == nil {
		return nil
	}
	d, ok := o.Internal().(*editorBindingsData)
	if !ok {
		return nil
	}
	return d
}
