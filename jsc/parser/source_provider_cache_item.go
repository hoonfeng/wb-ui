// SourceProviderCacheItem.h Go 翻译
package parser

// 临时常量，直到 nodes.go 补充
const TaintedByWithScopeLexicallyScopedFeature LexicallyScopedFeatures = 1 << 4

// SourceProviderCacheItemCreationParameters 缓存项创建参数
type SourceProviderCacheItemCreationParameters struct {
	LastTokenLine             int
	LastTokenStartOffset      int
	LastTokenEndOffset        int
	LastTokenLineStartOffset  int
	EndFunctionOffset         int
	ParameterCount            int
	LexicallyScopedFeatures   LexicallyScopedFeatures
	InnerArrowFunctionFeatures InnerArrowFunctionCodeFeatures
	ImplementationVisibility  ImplementationVisibility
	UsedVariables             []uintptr
	TokenType                 JSTokenType
	ConstructorKind           ConstructorKind
	ExpectedSuperBinding      SuperBinding
	NeedsFullActivation       bool
	UsesEval                  bool
	UsesImportMeta            bool
	NeedsSuperBinding         bool
	IsBodyArrowExpression     bool
}

// SourceProviderCacheItem 缓存项
type SourceProviderCacheItem struct {
	NeedsFullActivation       bool
	EndFunctionOffset         int
	UsesEval                  bool
	LastTokenLine             int
	StrictMode                bool
	LastTokenStartOffset      int
	ExpectedSuperBinding      int
	LastTokenEndOffset        int
	NeedsSuperBinding         bool
	ParameterCount            int
	TaintedByWithScope        bool
	LastTokenLineStartOffset  int
	IsBodyArrowExpression     bool
	UsedVariablesCount        int
	TokenType                 int
	InnerArrowFunctionFeatures int
	ConstructorKind           int
	ImplementationVisibility  int
	UsesImportMeta            bool
	Variables                 []uintptr
}

func NewSourceProviderCacheItem(params *SourceProviderCacheItemCreationParameters) *SourceProviderCacheItem {
	item := &SourceProviderCacheItem{
		NeedsFullActivation:       params.NeedsFullActivation,
		EndFunctionOffset:         params.EndFunctionOffset,
		UsesEval:                  params.UsesEval,
		LastTokenLine:             params.LastTokenLine,
		StrictMode:                (params.LexicallyScopedFeatures & StrictModeLexicallyScopedFeature) != 0,
		LastTokenStartOffset:      params.LastTokenStartOffset,
		ExpectedSuperBinding:      int(params.ExpectedSuperBinding),
		LastTokenEndOffset:        params.LastTokenEndOffset,
		NeedsSuperBinding:         params.NeedsSuperBinding,
		ParameterCount:            params.ParameterCount,
		TaintedByWithScope:        (params.LexicallyScopedFeatures & TaintedByWithScopeLexicallyScopedFeature) != 0,
		LastTokenLineStartOffset:  params.LastTokenLineStartOffset,
		IsBodyArrowExpression:     params.IsBodyArrowExpression,
		UsedVariablesCount:        len(params.UsedVariables),
		TokenType:                 int(params.TokenType),
		InnerArrowFunctionFeatures: int(params.InnerArrowFunctionFeatures),
		ConstructorKind:           int(params.ConstructorKind),
		ImplementationVisibility:  int(params.ImplementationVisibility),
		UsesImportMeta:            params.UsesImportMeta,
		Variables:                 make([]uintptr, len(params.UsedVariables)),
	}
	copy(item.Variables, params.UsedVariables)
	return item
}

func (item *SourceProviderCacheItem) EndFunctionToken() JSToken {
	tok := JSToken{}
	if item.IsBodyArrowExpression {
		tok.Type = JSTokenType(item.TokenType)
	} else {
		tok.Type = CLOSEBRACE
	}
	tok.Data.Offset = uint32(item.LastTokenStartOffset)
	tok.StartPosition.Offset = item.LastTokenStartOffset
	tok.StartPosition.Line = item.LastTokenLine
	tok.StartPosition.LineStartOffset = item.LastTokenLineStartOffset
	tok.EndPosition.Offset = item.LastTokenEndOffset
	return tok
}

func (item *SourceProviderCacheItem) LexicallyScopedFeatures() LexicallyScopedFeatures {
	var features LexicallyScopedFeatures
	if item.StrictMode { features |= StrictModeLexicallyScopedFeature }
	if item.TaintedByWithScope { features |= TaintedByWithScopeLexicallyScopedFeature }
	return features
}

func (item *SourceProviderCacheItem) UsedVariables() []uintptr { return item.Variables }
