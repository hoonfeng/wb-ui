// 版权所有 (C) 2015 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/runtime/ConstructAbility.h 翻译为 Go

package runtime

// ConstructAbility 表示函数是否可以作为构造函数使用
type ConstructAbility uint8

const (
	CanConstruct    ConstructAbility = 0
	CannotConstruct ConstructAbility = 1
)
