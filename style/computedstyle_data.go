// Translation of: Source/WebCore/style/computed/StyleInheritedData.h
//                  Source/WebCore/style/computed/StyleNonInheritedData.h
//
// InheritedData and NonInheritedData separate CSS properties by inheritance
// behaviour, each wrapped in a DataRef for copy-on-write sharing.

package style

// InheritedData holds CSS properties that inherit by default.
// Each element's ComputedStyle shares a DataRef[InheritedData] with its parent
// until a property is changed via Access().
type InheritedData struct {
	// ── Font ──
	FontFamily  string
	FontSize    Length
	FontStyle   string
	FontWeight  string
	FontVariant string

	// ── Text ──
	Color           Color
	LineHeight      Length
	LetterSpacing   Length
	WordSpacing     Length
	TextIndent      Length
	TextAlign       TextAlignType
	TextDecoration  string
	TextTransform   string
	WhiteSpace      WhiteSpaceType
	TextOverflow    TextOverflowType
	WordBreak       string // "normal" / "break-all" / "break-word" (inherited)
	OverflowWrap    string // "normal" / "break-word" (inherited; word-wrap alias)
	Direction       string
	UnicodeBidi     string

	// ── List ──
	ListStyleType     string
	ListStylePosition string
	ListStyleImage    string

	// ── Visibility / interaction ──
	Visibility    string
	Cursor        string
	UserSelect    string
	PointerEvents string

	// ── Writing mode ──
	WritingMode string // "horizontal-tb", "vertical-rl", "vertical-lr"

	// ── Animation-driven overrides (inherited for animation correctness) ──
	AnimatedColor           Color
	AnimatedBackgroundColor Color
	// AnimatedBackgroundActive：background-color @keyframes 动画是否正在驱动
	// （含 delay/fill 保持）。动画值为透明 (0,0,0,0) 时 A==0 且等于零值，
	// 仅靠 AnimatedBackgroundColor 无法区分「无动画」与「动画值透明」——
	// painter 会误回退静态背景色 → 光标闪烁的隐藏相位仍画静态色（不闪）。
	// 用显式标志：动画驱动期间置 true，结束后清。
	AnimatedBackgroundActive bool
	AnimatedColorActive      bool
}

// DefaultInheritedData returns InheritedData with spec-mandated initial values.
func DefaultInheritedData() *InheritedData {
	return &InheritedData{
		Color:               Color{R: 0, G: 0, B: 0, A: 0xFF},
		FontSize:            Length{Value: 16, Unit: "px"},
		FontFamily:          "serif",
		FontWeight:          "400",
		FontStyle:           "normal",
		FontVariant:         "normal",
		LineHeight:          Length{Value: 1.2, Unit: ""},
		TextAlign:           TextAlignStart,
		WhiteSpace:          WhiteSpaceNormal,
	TextOverflow:        TextOverflowClip,
		Direction:           "ltr",
		Visibility:          "visible",
		Cursor:              "auto",
		UserSelect:          "auto",
		PointerEvents:       "auto",
		WritingMode:         "horizontal-tb",
	}
}

// NonInheritedData holds CSS properties that do NOT inherit by default.
type NonInheritedData struct {
	// ── Box model ──
	Display           DisplayType
	Position          PositionType
	Float             string
	Clear             string
	OverflowX         OverflowType
	OverflowY         OverflowType
	Width             Length
	Height            Length
	MinWidth          Length
	MinHeight         Length
	MaxWidth          Length
	MaxHeight         Length

	MarginTop         Length
	MarginRight       Length
	MarginBottom      Length
	MarginLeft        Length
	PaddingTop        Length
	PaddingRight      Length
	PaddingBottom     Length
	PaddingLeft       Length

	BorderTopWidth    Length
	BorderRightWidth  Length
	BorderBottomWidth Length
	BorderLeftWidth   Length
	BorderTopColor    Color
	BorderRightColor  Color
	BorderBottomColor Color
	BorderLeftColor   Color
	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string
	// Border*ColorSet records whether the side color was EXPLICITLY set
	// (including `transparent`). CSS semantics: an unset border color falls
	// back to currentColor, but an explicitly transparent one must stay
	// invisible — painters check alpha before drawing.
	BorderTopColorSet    bool
	BorderRightColorSet  bool
	BorderBottomColorSet bool
	BorderLeftColorSet   bool
	BoxSizing            string

	// Border collapse mode (table only): separate (default) or collapse.
	BorderCollapse BorderCollapseType

	// Border radius (single value for all corners).
	BorderRadius Length

	// ── Outline（focus 指示器等）──
	OutlineWidth Length
	OutlineStyle string
	OutlineColor Color
	OutlineSet   bool // 是否显式设置了 outline（含 outline:none）

	// ── Background ──
	BackgroundColor      Color
	BackgroundImage      string
	BackgroundRepeat     string
	BackgroundPosition   string
	BackgroundSize       string
	BackgroundAttachment string
	BackgroundClip       string
	BackgroundOrigin     string

	// ── Flex / Grid container ──
	FlexDirection       string
	FlexWrap            string
	JustifyContent      string
	AlignItems          string
	AlignContent        string
	GridTemplateColumns string
	GridTemplateRows    string
	GridTemplateAreas   string
	GridAutoFlow        string
	GridAutoColumns     string
	GridAutoRows        string
	Gap                 Length
	RowGap              Length
	ColumnGap           Length

	// ── Multi-column ──
	ColumnCount      int    // 0 = auto
	ColumnWidth      Length
	ColumnRuleColor  string
	ColumnRuleStyle  string
	ColumnRuleWidth  Length
	ColumnFill       string

	// ── Flex / Grid item ──
	FlexBasis       Length
	FlexGrow        float64
	FlexShrink      float64
	Order           int
	AlignSelf       string
	JustifySelf     string
	GridRowStart    string
	GridRowEnd      string
	GridColumnStart string
	GridColumnEnd   string

	// ── Sizing / visibility ──
	VerticalAlign string
	Opacity       float64
	// StaticOpacity 是 opacity 的静态计算值（样式解析时保存）。动画
	// （@keyframes opacity）每帧改写 Opacity，StaticOpacity 保持解析值
	// 不变——动画插值端点未声明（如 cm-blink 0%/100% 无 opacity）时
	// 用 StaticOpacity 作 base，恢复静态值（浏览器语义：未声明关键帧
	// = 元素静态计算值）。
	StaticOpacity float64
	ZIndex        int

	// ── Generated content ──
	Content string

	// ── Effects ──
	BoxShadow     string
	TextShadow    string
	Transform     string
	TransformOriginX Length
	TransformOriginY Length
	Filter        string
	BackdropFilter string

	// ── Transitions / Animations ──
	Transition              string
	TransitionProperty      string
	TransitionDuration      float64
	TransitionTimingFunction string
	TransitionDelay         float64
	Animation               string
	AnimationName           string
	AnimationDuration       float64
	AnimationIterationCount int
	AnimationDelay          float64
	AnimationFillMode       string
	AnimationDirection      string
	AnimationTimingFunction string

	// ── Animation transform overrides ──
	TranslateX float64
	TranslateY float64
	ScaleX     float64
	ScaleY     float64

	// ── State ──
	DisplaySet bool
}

// DefaultNonInheritedData returns NonInheritedData with spec-mandated initial values.
func DefaultNonInheritedData() *NonInheritedData {
	return &NonInheritedData{
		Display:            DisplayInline,
		Position:           PositionStatic,
		OverflowX:          OverflowVisible,
		OverflowY:          OverflowVisible,
		BorderTopStyle:     "none",
		BorderRightStyle:   "none",
		BorderBottomStyle:  "none",
		BorderLeftStyle:    "none",
		BackgroundColor:    Color{R: 0, G: 0, B: 0, A: 0},
		FlexGrow:           0,
		FlexShrink:         1,
		Order:              0,
		ColumnCount:        0,
		ColumnRuleStyle:    "none",
		ColumnFill:         "balance",
		Opacity:            1.0,
		StaticOpacity:      1.0,
		ZIndex:             0,
		Content:            "",
		TransitionProperty: "all",
		TransitionTimingFunction: "ease",
		AnimationTimingFunction:  "linear",
		AnimationFillMode:       "none",
		AnimationDirection:      "normal",
	}
}
