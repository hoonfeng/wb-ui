// Package cdp 实现 Chrome DevTools Protocol 的**服务端**（本项目自用的调试通道）。
//
// 范围与定位（详见 docs/implementation-path.md §3）：
//   - S1 自家测试工装：Runtime/Page/Input/DOM(读) —— 用统一协议驱动引擎
//   - S2 Chrome DevTools 前端：追加 CSS/Log
//   - 明确不承诺：Playwright connect_over_cdp（域覆盖面 × 会话语义，成本远超收益）
//
// 分层：
//
//	HTTP 端点        WS 传输        协议分派            引擎适配
//	/json/version → net/http → ws 子包 → JSON-RPC → Adapter（宿主实现）
//	                                    + 会话/目标       → webkit.WebView
//
// Adapter 由宿主实现（见 app/cdp.go），engine 层因此不依赖宿主包
// （分层方向：webkit → engine，engine 不回指 webkit）。
package cdp

// Adapter 是引擎适配面：CDP 域方法落到具体页面（target）上。
//
// targetID 是 CDP 的页面标识；单页面宿主可以忽略它，多页面宿主用它路由到
// 对应的 WebView 实例。
type Adapter interface {
	// Targets 返回当前可附着的页面目标（至少一个；无页面时返回空切片）。
	Targets() []Target

	// ── Runtime ──
	// Evaluate 在目标页面上执行 JS，返回可序列化的结果（主机侧负责把 JS 值
	// 转成 RemoteValue：类型 + 值/描述）。
	Evaluate(targetID, expression string) (RemoteValue, error)
	// EvaluateHandle 求值并**总是**为对象/函数/数组返回句柄（objectId），供 CDP 的
	// `returnByValue=false` 路径使用（S2：DevTools 拿到 objectId 才会去 getProperties）。
	// 标量结果与 Evaluate 相同（只有 value，没有 objectId）。
	EvaluateHandle(targetID, expression string) (RemoteValue, error)
	// CallFunctionOn 调用一个函数声明并传参（CDP 的 callFunctionOn；本实现用
	// JSON 参数而非 objectId 句柄，见 session 注释）。
	CallFunctionOn(targetID, declaration string, args []any) (RemoteValue, error)
	// CallFunctionOnObject 以 objectID 为 this 调用函数声明（CDP callFunctionOn 的
	// objectId 形态；`Runtime.getProperties` 之后客户端就在这条路上）。
	CallFunctionOnObject(targetID, objectID, declaration string, args []any) (RemoteValue, error)
	// GetProperties 返回 objectID 的自身属性（CDP Runtime.getProperties）。
	GetProperties(targetID, objectID string) ([]PropertyDescriptor, error)
	// ReleaseObject 释放句柄（CDP Runtime.releaseObject；句柄表由宿主维护）。
	ReleaseObject(targetID, objectID string) error

	// ── Page ──
	// Screenshot 返回页面截图的 **PNG 字节**（引擎给的是像素缓冲，宿主编码 PNG）。
	Screenshot(targetID string) ([]byte, error)
	Navigate(targetID, url string) error
	Reload(targetID string) error
	// Resize 改变视口尺寸（CDP Emulation.setDeviceMetricsOverride 的最小实现）。
	Resize(targetID string, width, height int) error
	// SetDeviceMetrics 应用设备指标覆盖（CDP Emulation.setDeviceMetricsOverride）：
	// 视口 width×height（CSS 像素）+ deviceScaleFactor（设备像素比）。
	// 适配层自己记住「覆盖前」的真实视口，供 ClearDeviceMetrics 恢复。
	SetDeviceMetrics(targetID string, width, height int, dsf float64) error
	// ClearDeviceMetrics 取消设备指标覆盖（CDP Emulation.clearDeviceMetricsOverride）：
	// 回到覆盖前的视口尺寸，设备像素比回 1。
	ClearDeviceMetrics(targetID string) error

	// ── Input ──
	DispatchMouse(targetID string, ev MouseEvent) error
	DispatchKey(targetID string, ev KeyEvent) error

	// ── DOM ──
	DOMDocument(targetID string) (*DOMNode, error)
	DOMQuery(targetID, selector string) (*DOMNode, error)
	// DOMQueryAll 返回选择器命中的全部节点（S2：DevTools Elements 的搜索/多节点）。
	DOMQueryAll(targetID, selector string) ([]*DOMNode, error)
	DOMBoxModel(targetID, backendID string) (*BoxModel, error)
	// DOMOuterHTML 返回节点的 outerHTML（Elements 面板显示源码）。
	DOMOuterHTML(targetID, backendID string) (string, error)
	// DOMAttributes 返回节点的属性对。
	DOMAttributes(targetID, backendID string) ([][2]string, error)
	// DOMSetAttribute / DOMRemoveAttribute 是 Elements 面板的**编辑**路径。
	DOMSetAttribute(targetID, backendID, name, value string) error
	DOMRemoveAttribute(targetID, backendID, name string) error
	// DOMElementObjectID 把节点变成 JS 引用（CDP DOM.resolveNode → RemoteObject）。
	DOMElementObjectID(targetID, backendID string) (string, error)

	// ── CSS（S2） ──
	// ComputedStyle 返回元素的计算样式（CDP CSS.getComputedStyleForNode）。
	ComputedStyle(targetID, backendID string) ([]CSSProperty, error)
	// InlineStyle 返回元素的内联样式（CDP CSS.getInlineStylesForNode 的一半）。
	InlineStyle(targetID, backendID string) (*CSSStyleInfo, error)
	// MatchedRules 返回匹配该元素作者样式规则（CDP CSS.getMatchedStylesForNode 的一半）。
	MatchedRules(targetID, backendID string) ([]CSSRuleInfo, error)

	// ── Log ──
	// TakeConsoleEntries 取「自上次调用以来」的新控制台条目（增量语义：宿主
	// 记录游标，调用后清空）。没有新条目时返回 nil。
	TakeConsoleEntries(targetID string) []ConsoleEntry
}

// Target 是一个可附着的页面目标。
type Target struct {
	ID    string
	Type  string // "page"（Worker 目标是将来项，见 docs/implementation-path.md §3.2）
	Title string
	URL   string
}

// RemoteValue 是 JS 值的序列化形态（CDP RemoteObject 的数据部分）。
type RemoteValue struct {
	Type        string // undefined/object/boolean/number/string/function/symbol/bigint
	Subtype     string // null/array/error/…（可空）
	Value       any    // returnByValue 时的 JSON 值（可空）
	Description string // 值的字符串描述（Console 展示用，可空）
	// Exception 表示该值来自 JS 异常（求值抛错）。CDP 的 Runtime.evaluate 对
	// 表达式抛错**仍回成功的 result**，只额外给 exceptionDetails——回错误码会
	// 让 DevTools 前端把普通脚本错误当成协议故障。分派层据此决定是否附加该字段。
	Exception bool
	// ObjectID 是宿主分配的**不透明句柄**（CDP 的 objectId）：只有对象/函数才有，
	// 后续 getProperties / callFunctionOn(objectId) / releaseObject 用它回查。
	ObjectID string
}

// PropertyDescriptor 是 Runtime.getProperties 的一条属性（CDP PropertyDescriptor 的
// 最小子集：名字 + 值 + 三个标志位）。
type PropertyDescriptor struct {
	Name         string
	Value        RemoteValue
	Writable     bool
	Enumerable   bool
	Configurable bool
}

// CSSProperty 是一条 CSS 属性（CDP CSS.CSSProperty 的最小子集）。
type CSSProperty struct {
	Name  string
	Value string
	// Implicit 表示该属性不是作者显式声明的（computed style 里全为 true，
	// DevTools 据此把它显示为浅色）。
	Implicit bool
}

// CSSStyleInfo 是一份样式（CDP CSS.CSSStyle 的最小子集）。StyleSheetID 为空 =
// 内联样式（与 CDP 一致：内联样式没有 stylesheetId）。
type CSSStyleInfo struct {
	CSSText      string
	Properties   []CSSProperty
	StyleSheetID string
}

// CSSRuleInfo 是一条匹配到元素的样式规则（CDP CSS.CSSRule 的最小子集）。
type CSSRuleInfo struct {
	Selector     string
	Origin       string // "regular" = 作者样式
	StyleSheetID string
	Properties   []CSSProperty
	SourceURL    string
	Line         int
	Column       int
}

// DOMNode 是 DOM 树快照节点。BackendID 是宿主侧稳定标识——**约定为「可用于回查
// 该节点的 CSS 选择器路径」**（如 `body > div:nth-of-type(2)`）：CDP 层把它映射成
// 整数 nodeId，后续 describeNode/getBoxModel 用 BackendID 当选择器回查。
type DOMNode struct {
	BackendID string
	Tag       string
	Attrs     [][2]string
	Text      string
	Children  []*DOMNode
}

// BoxModel 是元素盒几何（CSS 像素，与 CDP 一致）。
type BoxModel struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// ConsoleEntry 是一条页面控制台输出。
type ConsoleEntry struct {
	Level string // log/info/warn/error/debug
	Text  string
}

// MouseEvent 是 CDP Input.dispatchMouseEvent 的参数（去掉协议细节后的形态）。
type MouseEvent struct {
	Type       string // mousePressed/mouseReleased/mouseMoved/mouseWheel
	X, Y       float64
	Button     string // left/right/middle/none
	Buttons    int
	ClickCount int
	DeltaX     float64
	DeltaY     float64
	Modifiers  int
}

// KeyEvent 是 CDP Input.dispatchKeyEvent 的参数。
type KeyEvent struct {
	Type                  string // keyDown/keyUp/rawKeyDown/char
	Key                   string
	Code                  string
	Text                  string
	WindowsVirtualKeyCode int
	Modifiers             int
}

// PageLoadWatcher 是 Adapter 的可选扩展：宿主若能报「页面累计加载完成次数」
// （单调递增），分派层就在客户端 enable Page 后据此推 Page.loadEventFired。
// 没实现该接口时只是不推这个事件，其余功能不受影响。
type PageLoadWatcher interface {
	PageLoadCount(targetID string) int
}

// UnimplementedAdapter 是 Adapter 的空实现：所有方法返回「未实现」。宿主可嵌入
// 它再只覆盖自己支持的域（P0/P1 阶段很实用：文档承诺的域按阶段落地）。
type UnimplementedAdapter struct{}

// NotImplementedError 表示「该域/方法在当前阶段未实现」——分派层据此回
// JSON-RPC 的 -32601（method not found），而不是 500。
type NotImplementedError struct{ Method string }

func (e NotImplementedError) Error() string {
	return "cdp: 未实现的方法 " + e.Method
}

func (UnimplementedAdapter) Targets() []Target { return nil }

func (UnimplementedAdapter) Evaluate(targetID, expression string) (RemoteValue, error) {
	return RemoteValue{}, NotImplementedError{"Runtime.evaluate"}
}

func (UnimplementedAdapter) EvaluateHandle(targetID, expression string) (RemoteValue, error) {
	return RemoteValue{}, NotImplementedError{"Runtime.evaluate"}
}

func (UnimplementedAdapter) CallFunctionOnObject(targetID, objectID, declaration string, args []any) (RemoteValue, error) {
	return RemoteValue{}, NotImplementedError{"Runtime.callFunctionOn"}
}

func (UnimplementedAdapter) GetProperties(targetID, objectID string) ([]PropertyDescriptor, error) {
	return nil, NotImplementedError{"Runtime.getProperties"}
}

func (UnimplementedAdapter) ReleaseObject(targetID, objectID string) error {
	return NotImplementedError{"Runtime.releaseObject"}
}

func (UnimplementedAdapter) DOMQueryAll(targetID, selector string) ([]*DOMNode, error) {
	return nil, NotImplementedError{"DOM.querySelectorAll"}
}

func (UnimplementedAdapter) DOMOuterHTML(targetID, backendID string) (string, error) {
	return "", NotImplementedError{"DOM.getOuterHTML"}
}

func (UnimplementedAdapter) DOMAttributes(targetID, backendID string) ([][2]string, error) {
	return nil, NotImplementedError{"DOM.getAttributes"}
}

func (UnimplementedAdapter) DOMSetAttribute(targetID, backendID, name, value string) error {
	return NotImplementedError{"DOM.setAttributeValue"}
}

func (UnimplementedAdapter) DOMRemoveAttribute(targetID, backendID, name string) error {
	return NotImplementedError{"DOM.removeAttribute"}
}

func (UnimplementedAdapter) DOMElementObjectID(targetID, backendID string) (string, error) {
	return "", NotImplementedError{"DOM.resolveNode"}
}

func (UnimplementedAdapter) ComputedStyle(targetID, backendID string) ([]CSSProperty, error) {
	return nil, NotImplementedError{"CSS.getComputedStyleForNode"}
}

func (UnimplementedAdapter) InlineStyle(targetID, backendID string) (*CSSStyleInfo, error) {
	return nil, NotImplementedError{"CSS.getInlineStylesForNode"}
}

func (UnimplementedAdapter) MatchedRules(targetID, backendID string) ([]CSSRuleInfo, error) {
	return nil, NotImplementedError{"CSS.getMatchedStylesForNode"}
}

func (UnimplementedAdapter) CallFunctionOn(targetID, declaration string, args []any) (RemoteValue, error) {
	return RemoteValue{}, NotImplementedError{"Runtime.callFunctionOn"}
}

func (UnimplementedAdapter) Screenshot(targetID string) ([]byte, error) {
	return nil, NotImplementedError{"Page.captureScreenshot"}
}

func (UnimplementedAdapter) Navigate(targetID, url string) error {
	return NotImplementedError{"Page.navigate"}
}

func (UnimplementedAdapter) Reload(targetID string) error {
	return NotImplementedError{"Page.reload"}
}

func (UnimplementedAdapter) Resize(targetID string, width, height int) error {
	return NotImplementedError{"Emulation.setDeviceMetricsOverride"}
}

func (UnimplementedAdapter) SetDeviceMetrics(targetID string, width, height int, dsf float64) error {
	return NotImplementedError{"Emulation.setDeviceMetricsOverride"}
}

func (UnimplementedAdapter) ClearDeviceMetrics(targetID string) error {
	return NotImplementedError{"Emulation.clearDeviceMetricsOverride"}
}

func (UnimplementedAdapter) DispatchMouse(targetID string, ev MouseEvent) error {
	return NotImplementedError{"Input.dispatchMouseEvent"}
}

func (UnimplementedAdapter) DispatchKey(targetID string, ev KeyEvent) error {
	return NotImplementedError{"Input.dispatchKeyEvent"}
}

func (UnimplementedAdapter) DOMDocument(targetID string) (*DOMNode, error) {
	return nil, NotImplementedError{"DOM.getDocument"}
}

func (UnimplementedAdapter) DOMQuery(targetID, selector string) (*DOMNode, error) {
	return nil, NotImplementedError{"DOM.querySelector"}
}

func (UnimplementedAdapter) DOMBoxModel(targetID, backendID string) (*BoxModel, error) {
	return nil, NotImplementedError{"DOM.getBoxModel"}
}

func (UnimplementedAdapter) TakeConsoleEntries(targetID string) []ConsoleEntry { return nil }
