// 版权所有 (C) 2008-2025 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/SourceProvider.h 翻译为 Go

package parser

// SourceProviderSourceType 表示源代码提供者类型
type SourceProviderSourceType uint8

const (
	SourceProviderProgram     SourceProviderSourceType = 0
	SourceProviderModule      SourceProviderSourceType = 1
	SourceProviderWebAssembly SourceProviderSourceType = 2
	SourceProviderJSON        SourceProviderSourceType = 3
	SourceProviderImportMap   SourceProviderSourceType = 4
)

// SourceProvider 源代码提供者
type SourceProvider struct {
	Source               string
	SourceURL            string
	PreRedirectURL       string
	SourceURLDirective   string
	SourceMappingURLDirective string
	Type                 SourceProviderSourceType
	ID                   uintptr
	Taintedness          uint8
}

const SourceProviderNullID uintptr = 1

// NewSourceProvider 创建 SourceProvider
func NewSourceProvider(source, sourceURL string, sourceType SourceProviderSourceType) *SourceProvider {
	return &SourceProvider{
		Source:    source,
		SourceURL: sourceURL,
		Type:      sourceType,
		ID:        0,
	}
}

// Hash 返回源代码的哈希值
func (p *SourceProvider) Hash() uint32 {
	return 0
}

// SourceVal 返回源代码视图
func (p *SourceProvider) SourceVal() string {
	return p.Source
}

// GetRange 返回指定范围的源代码
func (p *SourceProvider) GetRange(start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(p.Source) {
		end = len(p.Source)
	}
	if start >= end {
		return ""
	}
	return p.Source[start:end]
}

// AsID 返回提供者的 ID
func (p *SourceProvider) AsID() uintptr {
	if p.ID == 0 {
		p.ID = SourceProviderNullID
	}
	return p.ID
}

// SourceType 返回源代码类型
func (p *SourceProvider) SourceType() SourceProviderSourceType {
	return p.Type
}

// IsModuleType 判断是否是模块类型
func (p *SourceProvider) IsModuleType() bool {
	return p.Type == SourceProviderModule || p.Type == SourceProviderJSON
}
