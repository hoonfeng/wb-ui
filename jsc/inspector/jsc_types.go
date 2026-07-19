package inspector

// JSGlobalObjectConsoleClient provides console message support for JSC.
type JSGlobalObjectConsoleClient struct {
	vm interface{} // *runtime.VM
}

// JSGlobalObjectDebugger provides debugger integration for JSC.
type JSGlobalObjectDebugger struct{}

// JSGlobalObjectInspectorController is the main inspector controller for JSC.
type JSGlobalObjectInspectorController struct {
	vm interface{}
}

// JSInjectedScriptHost provides the injected script host for JSC.
type JSInjectedScriptHost struct{}

// JSInjectedScriptHostPrototype is the prototype for JSInjectedScriptHost.
type JSInjectedScriptHostPrototype struct{}

// JSJavaScriptCallFrame represents a JavaScript call frame exposed to inspector scripts.
type JSJavaScriptCallFrame struct {
	CallFrameIndex int
	FunctionName   string
	SourceID       string
	Line           int
	Column         int
}

// JSJavaScriptCallFramePrototype is the prototype for JSJavaScriptCallFrame.
type JSJavaScriptCallFramePrototype struct{}

// JavaScriptCallFrame is the native representation of a JavaScript call frame.
type JavaScriptCallFrame struct {
	CallFrameIndex int
	FunctionName   string
	SourceURL      string
	LineNumber     int
	ColumnNumber   int
	IsTailDeleted  bool
}

// PerGlobalObjectWrapperWorld manages wrapper worlds per global object.
type PerGlobalObjectWrapperWorld struct{}
