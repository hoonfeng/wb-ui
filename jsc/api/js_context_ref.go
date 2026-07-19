package api

// JSContextRef API provides functions for working with JavaScript contexts.

// JSContextGroupCreate creates a new context group.
func JSContextGroupCreate() JSContextGroupRef {
	return 0
}

// JSContextGroupRetain retains a context group.
func JSContextGroupRetain(group JSContextGroupRef) JSContextGroupRef {
	return group
}

// JSContextGroupRelease releases a context group.
func JSContextGroupRelease(group JSContextGroupRef) {
	_ = group
}

// JSGlobalContextCreate creates a new global context.
func JSGlobalContextCreate(globalClass JSClassRef) JSGlobalContextRef {
	_ = globalClass
	return 0
}

// JSGlobalContextCreateInGroup creates a new global context in a group.
func JSGlobalContextCreateInGroup(group JSContextGroupRef, globalClass JSClassRef) JSGlobalContextRef {
	_ = group
	_ = globalClass
	return 0
}

// JSGlobalContextRetain retains a global context.
func JSGlobalContextRetain(ctx JSGlobalContextRef) JSGlobalContextRef {
	return ctx
}

// JSGlobalContextRelease releases a global context.
func JSGlobalContextRelease(ctx JSGlobalContextRef) {
	_ = ctx
}

// JSContextGetGlobalObject returns the global object of a context.
func JSContextGetGlobalObject(ctx JSContextRef) JSObjectRef {
	_ = ctx
	return 0
}

// JSContextGetGroup returns the group of a context.
func JSContextGetGroup(ctx JSContextRef) JSContextGroupRef {
	_ = ctx
	return 0
}

// JSContextGetGlobalContext returns the global context.
func JSContextGetGlobalContext(ctx JSContextRef) JSGlobalContextRef {
	_ = ctx
	return 0
}

// JSEvaluateScript evaluates a script in the context.
func JSEvaluateScript(ctx JSContextRef, script JSStringRef, thisObject JSObjectRef, sourceURL JSStringRef, startingLineNumber int, exception *JSValueRef) JSValueRef {
	_ = ctx
	_ = script
	_ = thisObject
	_ = sourceURL
	_ = startingLineNumber
	_ = exception
	return 0
}

// JSCheckScriptSyntax checks the syntax of a script.
func JSCheckScriptSyntax(ctx JSContextRef, script JSStringRef, sourceURL JSStringRef, startingLineNumber int, exception *JSValueRef) bool {
	_ = ctx
	_ = script
	_ = sourceURL
	_ = startingLineNumber
	_ = exception
	return true
}

// JSGarbageCollect performs garbage collection.
func JSGarbageCollect(ctx JSContextRef) {
	_ = ctx
}
