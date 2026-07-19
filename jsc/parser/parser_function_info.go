// 版权所有 (C) 2015-2016 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ParserFunctionInfo.h 翻译为 Go

package parser

import "wb-ui/jsc/runtime"

// ParserFunctionInfo 解析函数信息（Body 类型由 TreeBuilder 决定）
type ParserFunctionInfo struct {
	Name                *runtime.Identifier
	Body                interface{} // *FunctionMetadataNode for ASTBuilder, int for SyntaxChecker
	ParameterCount      uint32
	FunctionLength      uint32
	StartOffset         uint32
	EndOffset           uint32
	StartLine           int
	EndLine             int
	ParametersStartColumn uint32
}

// ParserClassInfo 解析类信息
type ParserClassInfo struct {
	ClassName   *runtime.Identifier
	StartOffset uint32
	EndOffset   uint32
	StartLine   int
	StartColumn uint32
}
