// *
// Copyright (C) 2019-2023 Apple Inc. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
// notice, this list of conditions and the following disclaimer in the
// documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY APPLE INC. ``AS IS'' AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED.  IN NO EVENT SHALL APPLE INC. OR
// CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
// EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
// PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
// OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//

// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/API/JSAPIGlobalObject.h 翻译为 Go

package api

// 此文件是 JavaScriptCore API/JSAPIGlobalObject.h 的 Go 翻译版本

// ScriptFetcher 结构体定义
type ScriptFetcher struct {
	// TODO: 添加字段
}

// NewScriptFetcher 创建新的 ScriptFetcher 实例
func NewScriptFetcher() *ScriptFetcher {
	return &ScriptFetcher{}
}

// ScriptFetchParameters 结构体定义
type ScriptFetchParameters struct {
	// TODO: 添加字段
}

// NewScriptFetchParameters 创建新的 ScriptFetchParameters 实例
func NewScriptFetchParameters() *ScriptFetchParameters {
	return &ScriptFetchParameters{}
}

// JSAPIGlobalObject 结构体定义
type JSAPIGlobalObject struct {
	// TODO: 添加字段
}

// NewJSAPIGlobalObject 创建新的 JSAPIGlobalObject 实例
func NewJSAPIGlobalObject() *JSAPIGlobalObject {
	return &JSAPIGlobalObject{}
}

// reportUncaughtExceptionAtEventLoop  函数
func reportUncaughtExceptionAtEventLoop(_0 *JSGlobalObject, _1 *Exception)  {
	// TODO: 实现函数体
}

// loadAndEvaluateJSScriptModule JSValue 函数
func loadAndEvaluateJSScriptModule(JSLockHolder& const,  JSScript) JSValue {
	// TODO: 实现函数体
	var zero JSValue
	return zero
}

// moduleLoaderResolve Identifier 函数
func moduleLoaderResolve(_0 *JSGlobalObject, _1 *JSModuleLoader, keyValue JSValue, referrerValue JSValue, _4 RefPtr<ScriptFetcher>, useImportMap bool) Identifier {
	// TODO: 实现函数体
	var zero Identifier
	return zero
}

// moduleLoaderEvaluate JSValue 函数
func moduleLoaderEvaluate(_0 *JSGlobalObject, _1 *JSModuleLoader, _2 JSValue, _3 JSValue, _4 RefPtr<ScriptFetcher>, _5 JSValue, _6 JSValue) JSValue {
	// TODO: 实现函数体
	var zero JSValue
	return zero
}
