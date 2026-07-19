// 版权所有 (C) 2009-2019 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ParserArena.h 翻译为 Go

package parser

import (
	"wb-ui/jsc/runtime"
)

// MaximumCachableCharacter 表示可缓存的字符最大值
const MaximumCachableCharacter = 128

// IdentifierArena 标识符分配竞技场
type IdentifierArena struct {
	identifiers      []runtime.Identifier
	shortIdentifiers [MaximumCachableCharacter]*runtime.Identifier
	recentIdentifiers [MaximumCachableCharacter]*runtime.Identifier
}

// NewIdentifierArena 创建 IdentifierArena
func NewIdentifierArena() *IdentifierArena {
	a := &IdentifierArena{}
	a.Clear()
	return a
}

// Clear 清空所有缓存的标识符
func (a *IdentifierArena) Clear() {
	a.identifiers = nil
	for i := 0; i < MaximumCachableCharacter; i++ {
		a.shortIdentifiers[i] = nil
		a.recentIdentifiers[i] = nil
	}
}

// MakeIdentifier 创建或获取缓存的标识符
func (a *IdentifierArena) MakeIdentifier(vm *runtime.VM, chars []byte) *runtime.Identifier {
	if len(chars) == 0 {
		// 返回空标识符
		return &runtime.Identifier{}
	}
	first := chars[0]
	if int(first) >= MaximumCachableCharacter {
		a.identifiers = append(a.identifiers, runtime.Identifier{})
		return &a.identifiers[len(a.identifiers)-1]
	}
	if len(chars) == 1 {
		if ident := a.shortIdentifiers[first]; ident != nil {
			return ident
		}
		a.identifiers = append(a.identifiers, runtime.Identifier{})
		a.shortIdentifiers[first] = &a.identifiers[len(a.identifiers)-1]
		return &a.identifiers[len(a.identifiers)-1]
	}
	ident := a.recentIdentifiers[first]
	if ident != nil {
		return ident
	}
	a.identifiers = append(a.identifiers, runtime.Identifier{})
	a.recentIdentifiers[first] = &a.identifiers[len(a.identifiers)-1]
	return &a.identifiers[len(a.identifiers)-1]
}

// MakeEmptyIdentifier 创建空标识符
func (a *IdentifierArena) MakeEmptyIdentifier(vm *runtime.VM) *runtime.Identifier {
	return &runtime.Identifier{}
}

// MakeNumericIdentifier 创建数值标识符
func (a *IdentifierArena) MakeNumericIdentifier(vm *runtime.VM, number float64) *runtime.Identifier {
	a.identifiers = append(a.identifiers, runtime.Identifier{})
	return &a.identifiers[len(a.identifiers)-1]
}

// MakePrivateIdentifier 创建私有标识符
func (a *IdentifierArena) MakePrivateIdentifier(vm *runtime.VM, literal string, index uint32) *runtime.Identifier {
	a.identifiers = append(a.identifiers, runtime.Identifier{})
	return &a.identifiers[len(a.identifiers)-1]
}

// ParserArenaFreeablePoolSize 空闲池大小
const ParserArenaFreeablePoolSize = 8000

// ParserArena 解析器分配竞技场
type ParserArena struct {
	freeableMemory  []byte
	freeablePoolEnd int
	identifierArena *IdentifierArena
	freeablePools   [][]byte
	deletableObjects []interface{}
}

// NewParserArena 创建 ParserArena
func NewParserArena() *ParserArena {
	return &ParserArena{}
}

// Swap 交换两个 ParserArena 的内容
func (a *ParserArena) Swap(other *ParserArena) {
	a.freeableMemory, other.freeableMemory = other.freeableMemory, a.freeableMemory
	a.freeablePoolEnd, other.freeablePoolEnd = other.freeablePoolEnd, a.freeablePoolEnd
	a.identifierArena, other.identifierArena = other.identifierArena, a.identifierArena
	a.freeablePools, other.freeablePools = other.freeablePools, a.freeablePools
	a.deletableObjects, other.deletableObjects = other.deletableObjects, a.deletableObjects
}

// AllocateFreeable 分配可释放内存
func (a *ParserArena) AllocateFreeable(size int) []byte {
	if size <= 0 {
		return nil
	}
	return make([]byte, size)
}

// AllocateDeletable 分配可删除对象
func (a *ParserArena) AllocateDeletable(size int) interface{} {
	// 简化实现
	return nil
}

// IdentifierArenaRef 返回标识符竞技场
func (a *ParserArena) IdentifierArenaRef() *IdentifierArena {
	if a.identifierArena == nil {
		a.identifierArena = NewIdentifierArena()
	}
	return a.identifierArena
}

func alignSizeForArena(size int) int {
	alignment := 8 // WTF::AllocAlignmentInteger
	return (size + alignment - 1) & ^(alignment - 1)
}

func (a *ParserArena) allocateFreeablePool() {
	pool := make([]byte, ParserArenaFreeablePoolSize)
	a.freeablePools = append(a.freeablePools, pool)
	a.freeableMemory = pool
	a.freeablePoolEnd = ParserArenaFreeablePoolSize
}

func (a *ParserArena) deallocateObjects() {
	a.deletableObjects = nil
}
