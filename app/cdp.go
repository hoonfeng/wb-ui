package app

// CDP 适配层（实现路径主线 B）：把 engine/devtools/cdp 的域方法映射到
// webkit.WebView。每一行的引擎侧落点见 docs/implementation-path.md §3.3。
//
// 分工：协议/会话/传输在 cdp 包（纯 Go、不依赖宿主）；这里只出现宿主依赖
// （渲染、输入、JS 求值、控制台缓冲），因此 engine 层不会反向依赖 webkit。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"

	"wb-ui/engine/devtools/cdp"
	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// cdpTargetID 是单页面宿主的 target 标识：引擎里一个 WebView 就是一个页面，
// 没有「多标签」语义（多 WebView 宿主可在此基础上按实例编号扩展）。
const cdpTargetID = "page-1"

// cdpDOMNodeBudget 是 DOM 快照的节点上限：DevTools 打开 Elements 面板时会对大
// 页面调 getDocument，不设上限会把整棵树的 JSON 拉进内存（数万节点）。
const cdpDOMNodeBudget = 4000

type cdpAdapter struct {
	cdp.UnimplementedAdapter
	wv *webkit.WebView

	// 控制台增量的消费游标：引擎的 ConsoleEntries() 是全量**带级别**条目（append-only），
	// 用「已消费条目数」得到增量（S2：级别保真，见 jsc.BufferLogger.Entries）。
	consoleMu       sync.Mutex
	consoleConsumed int

	// loadCount 由宿主在页面加载完成时递增（推 Page.loadEventFired）。
	loadMu    sync.Mutex
	loadCount int

	// 设备指标覆盖（CDP Emulation.setDeviceMetricsOverride）的状态：
	// metricsOverridden 为真时 priorWidth/priorHeight 是「覆盖前」的真实视口，
	// clearDeviceMetricsOverride 据此恢复（单页面宿主没有「真实窗口尺寸」的
	// 独立来源，记录覆盖前的值是唯一可靠的恢复依据）。
	metricsMu         sync.Mutex
	metricsOverridden bool
	priorWidth        int
	priorHeight       int
}

// NewCDPAdapter 构造 CDP 适配层。
func NewCDPAdapter(wv *webkit.WebView) cdp.Adapter {
	return &cdpAdapter{wv: wv}
}

// ─── 目标 ────────────────────────────────────────────────

func (a *cdpAdapter) Targets() []cdp.Target {
	if a.wv == nil {
		return nil
	}
	title := ""
	if v, err := a.wv.EvalJS("document.title"); err == nil {
		title = v.ToString()
	}
	return []cdp.Target{{ID: cdpTargetID, Type: "page", Title: title, URL: a.wv.DocumentBaseURL()}}
}

// ─── Runtime ─────────────────────────────────────────────

func (a *cdpAdapter) Evaluate(targetID, expression string) (cdp.RemoteValue, error) {
	if a.wv == nil {
		return cdp.RemoteValue{}, errors.New("cdp adapter: 没有 WebView")
	}
	v, err := a.wv.EvalJS(expression)
	if err != nil {
		// 引擎级失败（已销毁 / JS 被禁用）：这是协议错误，不是脚本错误。
		if errors.Is(err, webkit.ErrDestroyed) || errors.Is(err, webkit.ErrJavaScriptDisabled) {
			return cdp.RemoteValue{}, err
		}
		// 脚本抛错：CDP 语义是「成功的 evaluate + exceptionDetails」。
		return cdp.RemoteValue{
			Type: "object", Subtype: "error", Description: err.Error(), Exception: true,
		}, nil
	}
	return remoteValueOf(v), nil
}

func (a *cdpAdapter) CallFunctionOn(targetID, declaration string, args []any) (cdp.RemoteValue, error) {
	argJSON, err := json.Marshal(args)
	if err != nil {
		return cdp.RemoteValue{}, fmt.Errorf("cdp adapter: 参数序列化失败: %w", err)
	}
	expr := "(function(){ var __args = " + string(argJSON) + "; var __fn = (" + declaration +
		"); return __fn.apply(globalThis, __args); })()"
	return a.Evaluate(targetID, expr)
}

// remoteValueOf 把引擎 JS 值转成 CDP RemoteObject 的数据形态。
func remoteValueOf(v jsc.JSValue) cdp.RemoteValue {
	switch {
	case v.IsUndefined():
		return cdp.RemoteValue{Type: "undefined"}
	case v.IsNull():
		return cdp.RemoteValue{Type: "object", Subtype: "null"}
	case v.IsBoolean():
		return cdp.RemoteValue{Type: "boolean", Value: v.ToBoolean()}
	case v.IsNumber():
		n := v.ToNumber()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			// JSON 无法表达 NaN/Infinity：CDP 用 description 承载。
			return cdp.RemoteValue{Type: "number", Description: strconv.FormatFloat(n, 'g', -1, 64)}
		}
		return cdp.RemoteValue{Type: "number", Value: n}
	case v.IsString():
		return cdp.RemoteValue{Type: "string", Value: v.ToString()}
	}
	desc := v.ToString()
	if desc == "[object Object]" {
		desc = ""
	}
	typ, subtype := "object", ""
	if v.IsFunction() {
		typ = "function"
	}
	exported := v.Export()
	if _, isArr := exported.([]any); isArr {
		subtype = "array"
	}
	if clean, ok := jsonSafeValue(exported); ok {
		return cdp.RemoteValue{Type: typ, Subtype: subtype, Value: clean, Description: desc}
	}
	return cdp.RemoteValue{Type: typ, Subtype: subtype, Description: desc}
}

// jsonSafeValue 判定导出值能否进 JSON（对象图里的函数/循环引用都不行）。
func jsonSafeValue(v any) (any, bool) {
	switch t := v.(type) {
	case nil:
		return nil, true
	case bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return t, true
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return nil, false
		}
		return t, true
	case []any:
		out := make([]any, 0, len(t))
		for _, el := range t {
			clean, ok := jsonSafeValue(el)
			if !ok {
				return nil, false
			}
			out = append(out, clean)
		}
		return out, true
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, el := range t {
			clean, ok := jsonSafeValue(el)
			if !ok {
				return nil, false
			}
			out[k] = clean
		}
		return out, true
	default:
		return nil, false
	}
}

// ─── Page ────────────────────────────────────────────────

// Screenshot 把 Render() 的像素缓冲编码成 PNG（与 psai 的截图同一条路径：预乘
// alpha 还原为直通 alpha，否则半透明像素在 PNG 里会偏暗）。
func (a *cdpAdapter) Screenshot(targetID string) ([]byte, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	pixels, err := a.wv.Render()
	if err != nil {
		return nil, err
	}
	w, h := a.wv.Width(), a.wv.Height()
	if w <= 0 || h <= 0 || len(pixels) < w*h*4 {
		return nil, fmt.Errorf("cdp adapter: 像素缓冲不足（%d 字节 < %dx%d）", len(pixels), w, h)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	for i := 0; i < len(img.Pix); i += 4 {
		alpha := img.Pix[i+3]
		if alpha == 0 || alpha == 255 {
			continue
		}
		for c := 0; c < 3; c++ {
			v := int(img.Pix[i+c]) * 255 / int(alpha)
			if v > 255 {
				v = 255
			}
			img.Pix[i+c] = uint8(v)
		}
	}
	// ★ S3：设备像素比 > 1 时按 CDP 语义输出「设备像素尺寸」的图片
	//   （Chrome 的截图尺寸 = CSS 视口 × deviceScaleFactor）。
	//   ★ 边界（如实记录，见 docs/TECH_DEBT.md）：这里是**放大**（CSS 像素
	//   光栅化后按比插值），不是「按设备像素重绘」——引擎没有独立的高分辨率
	//   渲染通道；DSF=1（默认）时输出与 Render() 逐像素一致（判据 4 不变）。
	if dsf := a.wv.DeviceScaleFactor(); dsf > 1 {
		img = scaleNRGBA(img, dsf)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// scaleNRGBA 把图像按 factor 双线性放大（factor<=1 时原样返回）。用于 CDP 截图
// 在 deviceScaleFactor>1 时输出设备像素尺寸的位图。
func scaleNRGBA(src *image.NRGBA, factor float64) *image.NRGBA {
	if src == nil || factor <= 1 {
		return src
	}
	b := src.Bounds()
	dw := int(math.Round(float64(b.Dx()) * factor))
	dh := int(math.Round(float64(b.Dy()) * factor))
	if dw <= 0 || dh <= 0 {
		return src
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		// 按**像素中心**映射采样点（避免整体偏移半像素）。
		sy := (float64(y)+0.5)/factor - 0.5
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		if y0 < 0 {
			y0, fy = 0, 0
		}
		y1 := y0 + 1
		if y1 > b.Dy()-1 {
			y1 = b.Dy() - 1
		}
		for x := 0; x < dw; x++ {
			sx := (float64(x)+0.5)/factor - 0.5
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			if x0 < 0 {
				x0, fx = 0, 0
			}
			x1 := x0 + 1
			if x1 > b.Dx()-1 {
				x1 = b.Dx() - 1
			}
			src0 := src.PixOffset(x0, y0)
			src1 := src.PixOffset(x1, y0)
			src2 := src.PixOffset(x0, y1)
			src3 := src.PixOffset(x1, y1)
			for c := 0; c < 4; c++ {
				p00 := float64(src.Pix[src0+c])
				p10 := float64(src.Pix[src1+c])
				p01 := float64(src.Pix[src2+c])
				p11 := float64(src.Pix[src3+c])
				v := p00*(1-fx)*(1-fy) + p10*fx*(1-fy) + p01*(1-fx)*fy + p11*fx*fy
				if v < 0 {
					v = 0
				}
				if v > 255 {
					v = 255
				}
				dst.Pix[dst.PixOffset(x, y)+c] = uint8(math.Round(v))
			}
		}
	}
	return dst
}

func (a *cdpAdapter) Navigate(targetID, url string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	// ★ ModeToolkit 下外部资源受资源策略限制（DenyExternal 时外部资源全拦），
	// 导航能否成功取决于宿主模式与 resolver——失败原因由引擎返回，原样上报。
	return a.wv.LoadURL(url)
}

func (a *cdpAdapter) Reload(targetID string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	u := a.wv.DocumentBaseURL()
	if strings.TrimSpace(u) == "" {
		return errors.New("cdp adapter: 当前文档没有来源 URL（LoadHTML 直出内容），无法重新加载")
	}
	return a.wv.LoadURL(u)
}

func (a *cdpAdapter) Resize(targetID string, width, height int) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("cdp adapter: 非法视口尺寸 %dx%d", width, height)
	}
	a.wv.Resize(width, height)
	return nil
}

// SetDeviceMetrics 应用设备指标覆盖（CDP Emulation.setDeviceMetricsOverride）：
// 视口 width×height（CSS 像素）+ deviceScaleFactor（设备像素比，<=0 按 1）。
//
// 首次覆盖时记录当前视口，供 ClearDeviceMetrics 恢复。
func (a *cdpAdapter) SetDeviceMetrics(targetID string, width, height int, dsf float64) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("cdp adapter: 非法视口尺寸 %dx%d", width, height)
	}
	if dsf <= 0 {
		dsf = 1
	}
	a.metricsMu.Lock()
	if !a.metricsOverridden {
		a.priorWidth, a.priorHeight = a.wv.Width(), a.wv.Height()
		a.metricsOverridden = true
	}
	a.metricsMu.Unlock()
	a.wv.Resize(width, height)
	a.wv.SetDeviceScaleFactor(dsf)
	return nil
}

// ClearDeviceMetrics 取消设备指标覆盖：视口回到覆盖前尺寸，DSF 回 1。
func (a *cdpAdapter) ClearDeviceMetrics(targetID string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	a.metricsMu.Lock()
	restore := a.metricsOverridden
	w, h := a.priorWidth, a.priorHeight
	a.metricsOverridden = false
	a.metricsMu.Unlock()
	if restore && w > 0 && h > 0 {
		a.wv.Resize(w, h)
	}
	a.wv.SetDeviceScaleFactor(1)
	return nil
}

// ─── Input ───────────────────────────────────────────────

func (a *cdpAdapter) DispatchMouse(targetID string, ev cdp.MouseEvent) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	switch ev.Type {
	case "mouseMoved":
		a.wv.HandleMouseMove(ev.X, ev.Y)
	case "mousePressed", "mouseReleased":
		action := 0 // 0 = Press
		if ev.Type == "mouseReleased" {
			action = 1
		}
		button := 0 // 0 = 左键（引擎约定，见 webkit.HandleMouseButton）
		switch strings.ToLower(strings.TrimSpace(ev.Button)) {
		case "", "left", "none":
			button = 0
		case "right":
			button = 2
		case "middle":
			// 引擎的鼠标管线只处理左键点击与右键 contextmenu，中键没有落点：
			// 明确回错，避免客户端以为中键点击生效了。
			return errors.New("cdp adapter: 引擎暂不支持中键点击")
		default:
			return fmt.Errorf("cdp adapter: 不支持的鼠标按钮 %q", ev.Button)
		}
		a.wv.HandleMouseButton(ev.X, ev.Y, button, action)
	case "mouseWheel":
		// CDP：deltaY 正 = 向下滚动；引擎 HandleWheel 的语义是 Win32 滚轮增量
		// （正 = 向上）——符号取反。
		a.wv.HandleWheel(-ev.DeltaY)
	default:
		return fmt.Errorf("cdp adapter: 不支持的鼠标事件类型 %q", ev.Type)
	}
	return nil
}

// cdpEditableKeys 是引擎 FormFocus.KeyInput 支持的命名键（CDP 的 key 字段命名
// 与引擎一致，直接转交；其余键当前没有处理管线——不是漏做，是引擎未实现）。
var cdpEditableKeys = map[string]bool{
	"Backspace": true, "Delete": true, "ArrowLeft": true, "ArrowRight": true,
	"Home": true, "End": true, "Enter": true, "Escape": true,
}

func (a *cdpAdapter) DispatchKey(targetID string, ev cdp.KeyEvent) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	if ev.Type == "keyUp" {
		// 引擎的编辑在 keyDown/char 上完成，没有独立的 keyup 管线：no-op 而不是
		// 报错（客户端成对发 down/up，报错会打断它的输入流程）。
		return nil
	}
	ff := a.wv.FormFocus()
	if ff == nil {
		return nil
	}
	if cdpEditableKeys[ev.Key] {
		ff.KeyInput(ev.Key)
		return nil
	}
	if ev.Text != "" {
		for _, r := range ev.Text {
			if r < 0x20 {
				continue // 控制字符（\r 已由 Enter 路径处理）
			}
			ff.CharInput(r)
		}
	}
	return nil
}

// ─── DOM（读） ───────────────────────────────────────────

// cdpDOMHelpers 是页面侧 DOM 快照的公共片段：选择器路径 + 递归快照。用页面自己
// 的 DOM 生成快照（引擎 DOM 与页面 JS 看到的是同一棵树），路径可直接回喂
// document.querySelector —— CDP 的 backendNodeId 语义由此落地。
const cdpDOMHelpers = `
  function __cdpSel(el, stopAt) {
    if (el === stopAt) return el.tagName.toLowerCase();
    if (el.id) return '#' + el.id;
    var parts = [];
    var cur = el;
    while (cur && cur.nodeType === 1 && cur !== stopAt) {
      var tag = cur.tagName.toLowerCase();
      var idx = 1, sib = cur;
      while ((sib = sib.previousElementSibling)) { if (sib.tagName === cur.tagName) idx++; }
      parts.unshift(tag + ':nth-of-type(' + idx + ')');
      cur = cur.parentElement;
    }
    if (parts.length === 0) parts.push(el.tagName.toLowerCase());
    return parts.join(' > ');
  }
  function __cdpSnap(el, stopAt, budget) {
    budget.n--;
    var attrs = [];
    for (var i = 0; i < el.attributes.length; i++) attrs.push([el.attributes[i].name, el.attributes[i].value]);
    var text = '';
    if (el.childNodes.length === 1 && el.childNodes[0].nodeType === 3) {
      text = String(el.childNodes[0].nodeValue).replace(/\s+/g, ' ').slice(0, 120);
    }
    var kids = [];
    for (var j = 0; j < el.children.length && budget.n > 0; j++) {
      kids.push(__cdpSnap(el.children[j], stopAt, budget));
    }
    return { id: __cdpSel(el, stopAt), tag: el.tagName.toLowerCase(), attrs: attrs, text: text, children: kids };
  }
`

func (a *cdpAdapter) DOMDocument(targetID string) (*cdp.DOMNode, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){` + cdpDOMHelpers + `
    var root = document.documentElement;
    if (!root) return '';
    var budget = { n: ` + strconv.Itoa(cdpDOMNodeBudget) + ` };
    return JSON.stringify(__cdpSnap(root, root, budget));
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	return decodeDOMNodeJSON(v.ToString())
}

func (a *cdpAdapter) DOMQuery(targetID, selector string) (*cdp.DOMNode, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	if strings.TrimSpace(selector) == "" {
		return nil, nil
	}
	script := `(function(){` + cdpDOMHelpers + `
    var el = document.querySelector(` + jsLiteral(selector) + `);
    if (!el) return '';
    var budget = { n: 1 };
    return JSON.stringify(__cdpSnap(el, document.documentElement, budget));
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	out := v.ToString()
	if strings.TrimSpace(out) == "" {
		return nil, nil // 选择器没有命中
	}
	return decodeDOMNodeJSON(out)
}

func (a *cdpAdapter) DOMBoxModel(targetID, backendID string) (*cdp.BoxModel, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	if strings.TrimSpace(backendID) == "" {
		return nil, errors.New("cdp adapter: 空的节点标识")
	}
	// 几何取自引擎真实布局（getBoundingClientRect → 渲染树几何），与 __devtools
	// 的几何同源 —— 这是 S2「Elements 面板高亮」的基础。
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return '';
    var r = el.getBoundingClientRect();
    return JSON.stringify({ x: r.left, y: r.top, w: r.width, h: r.height });
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(v.ToString())
	if raw == "" {
		return nil, nil
	}
	var box struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal([]byte(raw), &box); err != nil {
		return nil, fmt.Errorf("cdp adapter: 几何解析失败: %w", err)
	}
	return &cdp.BoxModel{X: box.X, Y: box.Y, Width: box.W, Height: box.H}, nil
}

// domNodeJSON 是页面侧快照的 JSON 形态。
type domNodeJSON struct {
	ID       string        `json:"id"`
	Tag      string        `json:"tag"`
	Text     string        `json:"text"`
	Attrs    [][]string    `json:"attrs"`
	Children []domNodeJSON `json:"children"`
}

func (n domNodeJSON) toCDP() *cdp.DOMNode {
	out := &cdp.DOMNode{BackendID: n.ID, Tag: n.Tag, Text: n.Text}
	for _, kv := range n.Attrs {
		if len(kv) == 2 {
			out.Attrs = append(out.Attrs, [2]string{kv[0], kv[1]})
		}
	}
	for _, ch := range n.Children {
		out.Children = append(out.Children, ch.toCDP())
	}
	return out
}

func decodeDOMNodeJSON(raw string) (*cdp.DOMNode, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var n domNodeJSON
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return nil, fmt.Errorf("cdp adapter: DOM 快照解析失败: %w", err)
	}
	return n.toCDP(), nil
}

// jsLiteral 把字符串编码成安全的 JS 字面量（用 JSON 编码，比 %q 更稳：反斜杠、
// 引号、非 ASCII 都覆盖）。
func jsLiteral(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// ─── DOM（S2 扩展）──────────────────────────────────────

// DOMQueryAll 返回选择器命中的全部节点（DevTools Elements 的搜索/多节点选中）。
func (a *cdpAdapter) DOMQueryAll(targetID, selector string) ([]*cdp.DOMNode, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	if strings.TrimSpace(selector) == "" {
		return nil, nil
	}
	script := `(function(){` + cdpDOMHelpers + `
    var list = document.querySelectorAll(` + jsLiteral(selector) + `);
    var out = [];
    for (var i = 0; i < list.length; i++) {
      var budget = { n: 1 };
      out.push(__cdpSnap(list[i], document.documentElement, budget));
    }
    return JSON.stringify(out);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	var raw []domNodeJSON
	if err := json.Unmarshal([]byte(v.ToString()), &raw); err != nil {
		return nil, fmt.Errorf("cdp adapter: 节点列表解析失败: %w", err)
	}
	out := make([]*cdp.DOMNode, 0, len(raw))
	for _, n := range raw {
		out = append(out, n.toCDP())
	}
	return out, nil
}

// DOMOuterHTML 返回节点的 outerHTML（Elements 面板的源码视图）。
func (a *cdpAdapter) DOMOuterHTML(targetID, backendID string) (string, error) {
	if a.wv == nil {
		return "", errors.New("cdp adapter: 没有 WebView")
	}
	if strings.TrimSpace(backendID) == "" {
		return "", errors.New("cdp adapter: 空的节点标识")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    return el ? el.outerHTML : "";
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

// DOMAttributes 返回节点的属性对（CDP 的扁平数组由分派层组装）。
func (a *cdpAdapter) DOMAttributes(targetID, backendID string) ([][2]string, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return "[]";
    var out = [];
    for (var i = 0; i < el.attributes.length; i++) {
      var at = el.attributes[i];
      out.push([at.name, at.value]);
    }
    return JSON.stringify(out);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	var raw [][]string
	if err := json.Unmarshal([]byte(v.ToString()), &raw); err != nil {
		return nil, fmt.Errorf("cdp adapter: 属性列表解析失败: %w", err)
	}
	out := make([][2]string, 0, len(raw))
	for _, kv := range raw {
		if len(kv) == 2 {
			out = append(out, [2]string{kv[0], kv[1]})
		}
	}
	return out, nil
}

// DOMSetAttribute / DOMRemoveAttribute 是 Elements 面板的属性编辑路径。
// ★ 走页面 API（setAttribute/removeAttribute）而不是绕过 bindings 直接改 dom 树：
// 引擎的失效链路（样式重算 + 布局标记 + 重绘）挂在 bindings 的写路径上。
func (a *cdpAdapter) DOMSetAttribute(targetID, backendID, name, value string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return false;
    el.setAttribute(` + jsLiteral(name) + `, ` + jsLiteral(value) + `);
    return true;
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return err
	}
	if v.ToBoolean() != true {
		return fmt.Errorf("cdp adapter: 找不到节点 %s", backendID)
	}
	return nil
}

func (a *cdpAdapter) DOMRemoveAttribute(targetID, backendID, name string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return false;
    el.removeAttribute(` + jsLiteral(name) + `);
    return true;
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return err
	}
	if v.ToBoolean() != true {
		return fmt.Errorf("cdp adapter: 找不到节点 %s", backendID)
	}
	return nil
}

// DOMElementObjectID 把节点变成 JS 引用（CDP DOM.resolveNode → RemoteObject）。
func (a *cdpAdapter) DOMElementObjectID(targetID, backendID string) (string, error) {
	v, err := a.EvaluateHandle(targetID, `document.querySelector(`+jsLiteral(backendID)+`)`)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(v.ObjectID) == "" {
		return "", fmt.Errorf("cdp adapter: 找不到节点 %s", backendID)
	}
	return v.ObjectID, nil
}

// ─── CSS（S2）───────────────────────────────────────────

// ComputedStyle 返回元素的计算样式（引擎的 getComputedStyle → 逐属性导出）。
func (a *cdpAdapter) ComputedStyle(targetID, backendID string) ([]cdp.CSSProperty, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return "[]";
    var cs = window.getComputedStyle(el);
    if (!cs) return "[]";
    var out = [];
    for (var i = 0; i < cs.length; i++) {
      var n = cs[i];
      out.push({ name: String(n), value: String(cs.getPropertyValue(n)), implicit: true });
    }
    return JSON.stringify(out);
  })()`
	return a.decodeCSSProperties(script)
}

// InlineStyle 返回元素的内联样式；没有任何内联属性时回 nil（CDP 里 `inlineStyle: null`
// 与「空样式」语义不同，前端据此显示「element.style { }」与否）。
func (a *cdpAdapter) InlineStyle(targetID, backendID string) (*cdp.CSSStyleInfo, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return "";
    var s = el.style;
    var props = [];
    if (s) {
      for (var i = 0; i < s.length; i++) {
        var n = s[i];
        props.push({ name: String(n), value: String(s.getPropertyValue(n)), implicit: false });
      }
    }
    return JSON.stringify({ text: el.getAttribute("style") || "", props: props });
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(v.ToString())
	if raw == "" {
		return nil, nil // 节点不存在
	}
	var parsed struct {
		Text  string `json:"text"`
		Props []struct {
			Name     string `json:"name"`
			Value    string `json:"value"`
			Implicit bool   `json:"implicit"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("cdp adapter: 内联样式解析失败: %w", err)
	}
	if strings.TrimSpace(parsed.Text) == "" && len(parsed.Props) == 0 {
		return nil, nil
	}
	info := &cdp.CSSStyleInfo{CSSText: parsed.Text}
	for _, p := range parsed.Props {
		info.Properties = append(info.Properties, cdp.CSSProperty{Name: p.Name, Value: p.Value, Implicit: p.Implicit})
	}
	return info, nil
}

// MatchedRules 返回匹配该元素的作者样式规则（Elements 面板 Styles 标签的上半部分）。
//
// 实现方式是**在页面上遍历 document.styleSheets**、用 querySelectorAll 判定命中——
// 引擎侧没有暴露「元素 → 匹配规则」的内部接口（那要动样式解析层）。代价：规则多时
// 逐条查询较慢，且拿不到 specificity 与 source line（记为 TECH_DEBT 的已知边界）。
func (a *cdpAdapter) MatchedRules(targetID, backendID string) ([]cdp.CSSRuleInfo, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){
    var el = document.querySelector(` + jsLiteral(backendID) + `);
    if (!el) return "[]";
    var out = [];
    var sheets = document.styleSheets || [];
    for (var i = 0; i < sheets.length; i++) {
      var rules = null;
      try { rules = sheets[i].cssRules; } catch (e) { rules = null; }
      if (!rules) continue;
      for (var j = 0; j < rules.length; j++) {
        var r = rules[j];
        var sel = (r && r.selectorText) ? String(r.selectorText) : "";
        if (!sel) continue;
        var hit = false;
        try {
          var list = document.querySelectorAll(sel);
          for (var k = 0; k < list.length; k++) { if (list[k] === el) { hit = true; break; } }
        } catch (e) { hit = false; }
        if (!hit) continue;
        var props = [];
        var st = r.style;
        if (st) {
          for (var m = 0; m < st.length; m++) {
            var n = st[m];
            props.push({ name: String(n), value: String(st.getPropertyValue(n)), implicit: false });
          }
        }
        out.push({ selector: sel, href: String(sheets[i].href || ""), props: props });
      }
    }
    return JSON.stringify(out);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Selector string `json:"selector"`
		Href     string `json:"href"`
		Props    []struct {
			Name     string `json:"name"`
			Value    string `json:"value"`
			Implicit bool   `json:"implicit"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(v.ToString()), &raw); err != nil {
		return nil, fmt.Errorf("cdp adapter: 匹配规则解析失败: %w", err)
	}
	out := make([]cdp.CSSRuleInfo, 0, len(raw))
	for _, r := range raw {
		info := cdp.CSSRuleInfo{Selector: r.Selector, Origin: "regular", SourceURL: r.Href}
		for _, p := range r.Props {
			info.Properties = append(info.Properties, cdp.CSSProperty{Name: p.Name, Value: p.Value, Implicit: p.Implicit})
		}
		out = append(out, info)
	}
	return out, nil
}

// decodeCSSProperties 执行「返回 CSS 属性数组 JSON」的脚本。
func (a *cdpAdapter) decodeCSSProperties(script string) ([]cdp.CSSProperty, error) {
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Implicit bool   `json:"implicit"`
	}
	if err := json.Unmarshal([]byte(v.ToString()), &raw); err != nil {
		return nil, fmt.Errorf("cdp adapter: CSS 属性解析失败: %w", err)
	}
	out := make([]cdp.CSSProperty, 0, len(raw))
	for _, p := range raw {
		out = append(out, cdp.CSSProperty{Name: p.Name, Value: p.Value, Implicit: p.Implicit})
	}
	return out, nil
}

// ─── Runtime（S2：objectId 句柄表）───────────────────────

// cdpHandleHelpers 是页面侧的句柄表辅助（S2 的 objectId 实现）。
//
// 为什么把句柄放在页面里：CDP 的 objectId 只是「不透明句柄」，宿主本可以用 Go 侧表
// 持 jsc.JSValue，但那会让 Go 侧长期持有 goja 值（跨 goroutine 访问 runtime 需要
// 额外同步）；页面侧的数组天然与 JS 值同生命周期，求值/取属性都能直接用它。
const cdpHandleHelpers = `
  window.__cdpHandles = window.__cdpHandles || [];
  window.__cdpHandleGet = function(id){
    var i = parseInt(String(id).replace(/^cdp:/, ""), 10);
    if (!isFinite(i) || i < 0 || i >= window.__cdpHandles.length) return undefined;
    return window.__cdpHandles[i];
  };
  window.__cdpHandleDrop = function(id){
    var i = parseInt(String(id).replace(/^cdp:/, ""), 10);
    if (isFinite(i) && i >= 0 && i < window.__cdpHandles.length) window.__cdpHandles[i] = undefined;
    return true;
  };
`

// remoteValueJSON 是 EvaluateHandle 的页面侧结果（kind=handle 时带 objectId）。
type remoteValueJSON struct {
	Kind     string `json:"kind"`
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	Desc     string `json:"desc"`
	ID       string `json:"id"`
	Value    any    `json:"value"`
	HasValue bool   `json:"hasValue"`
}

// EvaluateHandle 求值并给对象/函数回**句柄**（CDP `returnByValue=false` 路径）。
// 标量与 Evaluate 一致（带 value、无 objectId）——DevTools 拿到 objectId 才会去展开
// 属性，此前一律回 JSON 值，Console 里点对象是展不开的。
func (a *cdpAdapter) EvaluateHandle(targetID, expression string) (cdp.RemoteValue, error) {
	if a.wv == nil {
		return cdp.RemoteValue{}, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){` + cdpHandleHelpers + `
    var v = (` + expression + `);
    var t = typeof v;
    var out = { type: t };
    if (v === null) { out.type = "object"; out.subtype = "null"; out.desc = "null"; return JSON.stringify(out); }
    if (t === "undefined") { out.desc = "undefined"; return JSON.stringify(out); }
    if (t === "object" || t === "function") {
      window.__cdpHandles.push(v);
      out.kind = "handle";
      out.id = "cdp:" + (window.__cdpHandles.length - 1);
      if (t === "function") { out.desc = "function " + (v.name || "") + "()"; }
      else {
        var tag = Object.prototype.toString.call(v);
        out.subtype = (tag === "[object Array]") ? "array" : "";
        out.desc = tag;
      }
      return JSON.stringify(out);
    }
    out.kind = "value";
    out.desc = String(v);
    if (t !== "bigint" && t !== "symbol") {
      try { JSON.stringify(v); out.value = v; out.hasValue = true; } catch (e) { out.hasValue = false; }
    }
    return JSON.stringify(out);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return cdp.RemoteValue{Type: "object", Subtype: "error", Description: err.Error(), Exception: true}, nil
	}
	var parsed remoteValueJSON
	if err := json.Unmarshal([]byte(v.ToString()), &parsed); err != nil {
		return cdp.RemoteValue{}, fmt.Errorf("cdp adapter: 求值结果解析失败: %w", err)
	}
	out := cdp.RemoteValue{Type: parsed.Type, Subtype: parsed.Subtype, Description: parsed.Desc}
	if parsed.Kind == "handle" {
		out.ObjectID = parsed.ID
	} else if parsed.HasValue {
		out.Value = parsed.Value
	}
	return out, nil
}

// CallFunctionOnObject 以 objectId 为 this 调用函数声明（CDP callFunctionOn 的
// objectId 形态：DevTools Console 的 `$0.classList.add("x")` 就走这条路）。
func (a *cdpAdapter) CallFunctionOnObject(targetID, objectID, declaration string, args []any) (cdp.RemoteValue, error) {
	if a.wv == nil {
		return cdp.RemoteValue{}, errors.New("cdp adapter: 没有 WebView")
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return cdp.RemoteValue{}, fmt.Errorf("cdp adapter: 参数序列化失败: %w", err)
	}
	script := `(function(){` + cdpHandleHelpers + `
    var fn = (` + declaration + `);
    if (typeof fn !== "function") return undefined;
    var args = JSON.parse(` + jsLiteral(string(argsJSON)) + `);
    var thisVal = window.__cdpHandleGet(` + jsLiteral(objectID) + `);
    return fn.apply(thisVal, args);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return cdp.RemoteValue{Type: "object", Subtype: "error", Description: err.Error(), Exception: true}, nil
	}
	return remoteValueOf(v), nil
}

// GetProperties 返回 objectId 的自身属性（CDP Runtime.getProperties）。
func (a *cdpAdapter) GetProperties(targetID, objectID string) ([]cdp.PropertyDescriptor, error) {
	if a.wv == nil {
		return nil, errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){` + cdpHandleHelpers + `
    var o = window.__cdpHandleGet(` + jsLiteral(objectID) + `);
    if (o === undefined || o === null) return "[]";
    var names = [];
    try { names = Object.getOwnPropertyNames(o); } catch (e) { names = []; }
    var out = [];
    for (var i = 0; i < names.length; i++) {
      var n = names[i];
      var item = { name: n, value: { type: "undefined" } };
      var d = null;
      try { d = Object.getOwnPropertyDescriptor(o, n); } catch (e) { d = null; }
      if (d) { item.writable = !!d.writable; item.enumerable = !!d.enumerable; item.configurable = !!d.configurable; }
      var v;
      try { v = o[n]; } catch (e) { v = undefined; }
      var t = typeof v;
      item.value.type = t;
      if (v === null) { item.value.type = "object"; item.value.subtype = "null"; item.value.description = "null"; }
      else if (t === "object" || t === "function") { item.value.description = String(v); }
      else {
        item.value.description = String(v);
        if (t !== "bigint" && t !== "symbol") { try { JSON.stringify(v); item.value.value = v; } catch (e) {} }
      }
      out.push(item);
    }
    return JSON.stringify(out);
  })()`
	v, err := a.wv.EvalJS(script)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Name  string `json:"name"`
		Value struct {
			Type        string `json:"type"`
			Subtype     string `json:"subtype"`
			Description string `json:"description"`
			Value       any    `json:"value"`
		} `json:"value"`
		Writable     bool `json:"writable"`
		Enumerable   bool `json:"enumerable"`
		Configurable bool `json:"configurable"`
	}
	if err := json.Unmarshal([]byte(v.ToString()), &raw); err != nil {
		return nil, fmt.Errorf("cdp adapter: 属性列表解析失败: %w", err)
	}
	out := make([]cdp.PropertyDescriptor, 0, len(raw))
	for _, r := range raw {
		out = append(out, cdp.PropertyDescriptor{
			Name: r.Name,
			Value: cdp.RemoteValue{
				Type: r.Value.Type, Subtype: r.Value.Subtype,
				Description: r.Value.Description, Value: r.Value.Value,
			},
			Writable: r.Writable, Enumerable: r.Enumerable, Configurable: r.Configurable,
		})
	}
	return out, nil
}

// ReleaseObject 释放句柄（把页面侧表项置为 undefined：JS 值随即可回收）。
func (a *cdpAdapter) ReleaseObject(targetID, objectID string) error {
	if a.wv == nil {
		return errors.New("cdp adapter: 没有 WebView")
	}
	script := `(function(){` + cdpHandleHelpers + `
    return window.__cdpHandleDrop(` + jsLiteral(objectID) + `);
  })()`
	_, err := a.wv.EvalJS(script)
	return err
}

// ─── Log ─────────────────────────────────────────────────

func (a *cdpAdapter) TakeConsoleEntries(targetID string) []cdp.ConsoleEntry {
	if a.wv == nil {
		return nil
	}
	entries := a.wv.ConsoleEntries()
	a.consoleMu.Lock()
	defer a.consoleMu.Unlock()
	if len(entries) <= a.consoleConsumed {
		return nil
	}
	chunk := entries[a.consoleConsumed:]
	a.consoleConsumed = len(entries)
	var out []cdp.ConsoleEntry
	for _, e := range chunk {
		text := strings.TrimRight(e.Text, "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		level := strings.TrimSpace(e.Level)
		if level == "" {
			level = "log"
		}
		out = append(out, cdp.ConsoleEntry{Level: level, Text: text})
	}
	return out
}

// ─── 页面加载事件 ────────────────────────────────────────

func (a *cdpAdapter) PageLoadCount(targetID string) int {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()
	return a.loadCount
}

// NotePageLoad 由宿主在页面完成一次加载时调用（推 Page.loadEventFired）。
func (a *cdpAdapter) NotePageLoad() {
	a.loadMu.Lock()
	a.loadCount++
	a.loadMu.Unlock()
}

// ─── 宿主入口 ────────────────────────────────────────────

// DevTools 是宿主持有的调试服务句柄。
type DevTools struct {
	Server  *cdp.Server
	adapter *cdpAdapter
}

// StartDevTools 在 127.0.0.1:port 上启动 CDP 服务并接到 wv 上；port<=0 时返回
// (nil, nil)——默认关闭、零开销（docs/implementation-path.md §3.5）。启动成功
// 后打印与 Chrome 同格式的一行 "DevTools listening on ws://…"，外部工具据此发现
// 入口。
func StartDevTools(wv *webkit.WebView, port int) (*DevTools, error) {
	if wv == nil || port <= 0 {
		return nil, nil
	}
	adapter := &cdpAdapter{wv: wv}
	srv, wsURL, err := cdp.Start(cdp.Options{
		Adapter:       adapter,
		Product:       "wb-ui/1.0 (Go WebKit port)",
		UserAgent:     "Mozilla/5.0 (compatible; wb-ui/1.0)",
		WebKitVersion: "wb-ui-engine",
		Logf:          log.Printf,
	}, port)
	if err != nil {
		return nil, err
	}
	// 初始记一次「已加载」：客户端连上时通常已经过了首次加载，先给个非零基线，
	// 之后的每次加载都会推 loadEventFired。
	adapter.NotePageLoad()
	log.Printf("DevTools listening on %s", wsURL)
	return &DevTools{Server: srv, adapter: adapter}, nil
}

// NotePageLoad 通知「页面已完成一次加载」（宿主在 LoadHTML/LoadURL 成功后调用）。
func (d *DevTools) NotePageLoad() {
	if d != nil && d.adapter != nil {
		d.adapter.NotePageLoad()
	}
}

// Close 关停调试服务。
func (d *DevTools) Close() error {
	if d == nil || d.Server == nil {
		return nil
	}
	return d.Server.Close()
}
