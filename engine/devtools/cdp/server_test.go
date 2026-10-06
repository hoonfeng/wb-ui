package cdp

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"wb-ui/engine/devtools/cdp/ws"
)

// ─── 测试用适配器 ────────────────────────────────────────

// fakeAdapter 是一个可预测的引擎适配面：求值返回固定值（含异常分支）、截图返回
// 一张 4x3 的 PNG、DOM 返回一棵两层小树。协议层的正确性不该依赖真引擎。
// metricsCall 记录一次设备指标覆盖（S3：Emulation.setDeviceMetricsOverride）。
type metricsCall struct {
	W, H int
	DSF  float64
}

type fakeAdapter struct {
	UnimplementedAdapter
	evals     []string
	consoleQ  []ConsoleEntry
	loads     int
	resized   [2]int
	mouse     []MouseEvent
	keys      []KeyEvent
	navigated []string
	metrics   []metricsCall
	cleared   int
}

func (f *fakeAdapter) Targets() []Target {
	return []Target{{ID: "page-1", Type: "page", Title: "测试页", URL: "file:///tmp/index.html"}}
}

func (f *fakeAdapter) Evaluate(targetID, expression string) (RemoteValue, error) {
	f.evals = append(f.evals, expression)
	if strings.Contains(expression, "boom") {
		return RemoteValue{Type: "object", Subtype: "error", Description: "Error: boom", Exception: true}, nil
	}
	if strings.Contains(expression, "innerWidth") {
		return RemoteValue{Type: "object", Value: map[string]any{"iw": 800.0, "ih": 600.0, "sw": 800.0, "sh": 1200.0}}, nil
	}
	return RemoteValue{Type: "number", Value: float64(2)}, nil
}

func (f *fakeAdapter) CallFunctionOn(targetID, declaration string, args []any) (RemoteValue, error) {
	return RemoteValue{Type: "string", Value: declaration, Description: declaration}, nil
}

func (f *fakeAdapter) Screenshot(targetID string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *fakeAdapter) Navigate(targetID, u string) error {
	f.navigated = append(f.navigated, u)
	return nil
}

func (f *fakeAdapter) Reload(targetID string) error { return nil }

func (f *fakeAdapter) Resize(targetID string, w, h int) error {
	f.resized = [2]int{w, h}
	return nil
}

func (f *fakeAdapter) SetDeviceMetrics(targetID string, w, h int, dsf float64) error {
	f.metrics = append(f.metrics, metricsCall{W: w, H: h, DSF: dsf})
	f.resized = [2]int{w, h}
	return nil
}

func (f *fakeAdapter) ClearDeviceMetrics(targetID string) error {
	f.cleared++
	return nil
}

func (f *fakeAdapter) DispatchMouse(targetID string, ev MouseEvent) error {
	f.mouse = append(f.mouse, ev)
	return nil
}

func (f *fakeAdapter) DispatchKey(targetID string, ev KeyEvent) error {
	f.keys = append(f.keys, ev)
	return nil
}

func (f *fakeAdapter) DOMDocument(targetID string) (*DOMNode, error) {
	body := &DOMNode{BackendID: "body", Tag: "body", Children: []*DOMNode{
		{BackendID: "body > div", Tag: "div", Attrs: [][2]string{{"id", "app"}}, Text: "hi"},
	}}
	return &DOMNode{BackendID: "html", Tag: "html", Children: []*DOMNode{body}}, nil
}

func (f *fakeAdapter) DOMQuery(targetID, selector string) (*DOMNode, error) {
	if selector == "body > div" || selector == "#app" {
		return &DOMNode{BackendID: "body > div", Tag: "div", Attrs: [][2]string{{"id", "app"}}, Text: "hi"}, nil
	}
	return nil, nil
}

func (f *fakeAdapter) DOMBoxModel(targetID, backendID string) (*BoxModel, error) {
	return &BoxModel{X: 10, Y: 20, Width: 100, Height: 40}, nil
}

func (f *fakeAdapter) TakeConsoleEntries(targetID string) []ConsoleEntry {
	out := f.consoleQ
	f.consoleQ = nil
	return out
}

func (f *fakeAdapter) PageLoadCount(targetID string) int { return f.loads }

// ─── 测试用 WS 客户端 ────────────────────────────────────

type testClient struct {
	t    *testing.T
	conn net.Conn
	br   *bufio.Reader
	next int
}

// dialCDP 手写 CDP 客户端握手（Go 标准库没有 WS 客户端；手写才能保证我们验证的
// 是真实帧格式）。
func dialCDP(t *testing.T, wsURL string) *testClient {
	t.Helper()
	u, err := url.Parse(wsURL)
	if err != nil {
		t.Fatalf("解析 ws URL 失败: %v", err)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	req := "GET " + u.Path + " HTTP/1.1\r\nHost: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("写握手失败: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("读握手响应失败: %v", err)
	}
	if resp.StatusCode != 101 {
		t.Fatalf("握手状态 = %d，want 101", resp.StatusCode)
	}
	return &testClient{t: t, conn: conn, br: br}
}

func (c *testClient) close() { _ = c.conn.Close() }

// sendFrame 发一个带掩码的文本帧。
func (c *testClient) sendFrame(payload []byte) {
	c.t.Helper()
	mask := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	var buf bytes.Buffer
	buf.WriteByte(0x81)
	n := len(payload)
	switch {
	case n < 126:
		buf.WriteByte(byte(n) | 0x80)
	case n <= 0xFFFF:
		buf.WriteByte(126 | 0x80)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		buf.Write(ext[:])
	default:
		buf.WriteByte(127 | 0x80)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		buf.Write(ext[:])
	}
	buf.Write(mask)
	for i, b := range payload {
		buf.WriteByte(b ^ mask[i%4])
	}
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		c.t.Fatalf("写帧失败: %v", err)
	}
}

// readMessage 读一条服务端消息（JSON）。
func (c *testClient) readMessage() map[string]any {
	c.t.Helper()
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		c.t.Fatalf("读帧头失败: %v", err)
	}
	op := head[0] & 0x0F
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.t.Fatalf("读扩展长度失败: %v", err)
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			c.t.Fatalf("读扩展长度失败: %v", err)
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		c.t.Fatalf("读负载失败: %v", err)
	}
	if op != ws.OpText {
		c.t.Fatalf("opcode = 0x%x，want 文本", op)
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		c.t.Fatalf("负载不是 JSON: %v（%s）", err, payload)
	}
	return m
}

// call 发一条请求并读回响应（自动跳过事件消息：事件没有 id）。
func (c *testClient) call(method string, params any, sessionID string) map[string]any {
	c.t.Helper()
	c.next++
	id := c.next
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("序列化请求失败: %v", err)
	}
	c.sendFrame(raw)
	for i := 0; i < 20; i++ {
		got := c.readMessage()
		if fid, ok := got["id"].(float64); ok && int(fid) == id {
			return got
		}
		// 事件（无 id）：测试里忽略，继续等响应。
	}
	c.t.Fatalf("%s 没有收到响应", method)
	return nil
}

// startTestServer 起一个带 fake adapter 的 CDP 服务。
func startTestServer(t *testing.T) (*Server, *fakeAdapter, string) {
	t.Helper()
	fa := &fakeAdapter{}
	s := New(Options{Adapter: fa, Product: "wb-ui/test", UserAgent: "wb-ui-test"})
	addr, err := s.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, fa, "http://" + addr
}

// TestJSONEndpoints 覆盖验收判据 1：/json/version 含 Browser 与 webSocketDebuggerUrl；
// /json/list 含页面 target。这两个端点是所有 CDP 客户端的第一跳。
func TestJSONEndpoints(t *testing.T) {
	_, _, base := startTestServer(t)

	resp, err := http.Get(base + "/json/version")
	if err != nil {
		t.Fatalf("GET /json/version 失败: %v", err)
	}
	defer resp.Body.Close()
	var version map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		t.Fatalf("解析 version 失败: %v", err)
	}
	if version["Browser"] != "wb-ui/test" {
		t.Errorf("Browser = %v", version["Browser"])
	}
	if ws, _ := version["webSocketDebuggerUrl"].(string); !strings.HasPrefix(ws, "ws://127.0.0.1:") {
		t.Errorf("webSocketDebuggerUrl = %q，want ws://127.0.0.1:…", ws)
	}
	if pv, _ := version["Protocol-Version"].(string); pv == "" {
		t.Error("Protocol-Version 不应为空")
	}

	resp2, err := http.Get(base + "/json/list")
	if err != nil {
		t.Fatalf("GET /json/list 失败: %v", err)
	}
	defer resp2.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&list); err != nil {
		t.Fatalf("解析 list 失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("target 数 = %d，want 1", len(list))
	}
	if list[0]["id"] != "page-1" || list[0]["type"] != "page" {
		t.Errorf("target = %v", list[0])
	}
	if ws, _ := list[0]["webSocketDebuggerUrl"].(string); !strings.Contains(ws, "/devtools/page/page-1") {
		t.Errorf("页面 ws URL = %q", ws)
	}
}

// TestRuntimeEvaluateOverWS 覆盖验收判据 2 与 3（协议侧）：不带 sessionId 直接
// 发 Runtime.evaluate（S1 自家工装的最短路径）→ result.value 正确；重复调用
// 的表达式原样到达适配层（保证「协议层没吞参数」）。
func TestRuntimeEvaluateOverWS(t *testing.T) {
	s, fa, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()

	got := c.call("Runtime.evaluate", map[string]any{"expression": "1+1", "returnByValue": true}, "")
	if got["error"] != nil {
		t.Fatalf("意外错误: %v", got["error"])
	}
	result, _ := got["result"].(map[string]any)
	obj, _ := result["result"].(map[string]any)
	if obj["value"] != float64(2) || obj["type"] != "number" {
		t.Fatalf("result = %v", obj)
	}
	c.call("Runtime.evaluate", map[string]any{"expression": "document.title"}, "")
	if len(fa.evals) != 2 || fa.evals[1] != "document.title" {
		t.Fatalf("适配层收到的表达式 = %v", fa.evals)
	}
}

// TestEvaluateExceptionUsesExceptionDetails：脚本抛错必须回**成功响应 +
// exceptionDetails**（CDP 语义）。回协议错误会让 DevTools 把普通脚本错误显示成
// 协议故障。
func TestEvaluateExceptionUsesExceptionDetails(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()
	got := c.call("Runtime.evaluate", map[string]any{"expression": "boom()"}, "")
	if got["error"] != nil {
		t.Fatalf("不该是协议错误: %v", got["error"])
	}
	if _, ok := got["result"].(map[string]any)["exceptionDetails"]; !ok {
		t.Fatalf("缺少 exceptionDetails: %v", got["result"])
	}
}

// TestTargetAttachFlattenAndSessionRouting 覆盖 flatten 会话：attach 拿到
// sessionId 后，带 sessionId 的调用要落到同一 target；未 attach 的 targetId 要
// 明确报错；非 flatten 要明确拒绝。
func TestTargetAttachFlattenAndSessionRouting(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.BrowserWSURL())
	defer c.close()

	targets := c.call("Target.getTargets", nil, "")
	infos, _ := targets["result"].(map[string]any)["targetInfos"].([]any)
	if len(infos) != 1 {
		t.Fatalf("targetInfos = %v", targets)
	}

	attached := c.call("Target.attachToTarget", map[string]any{"targetId": "page-1", "flatten": true}, "")
	sess, _ := attached["result"].(map[string]any)["sessionId"].(string)
	if sess == "" {
		t.Fatalf("未拿到 sessionId: %v", attached)
	}
	got := c.call("Runtime.evaluate", map[string]any{"expression": "1+1"}, sess)
	if got["sessionId"] != sess {
		t.Errorf("响应未回带 sessionId: %v", got)
	}
	if got["error"] != nil {
		t.Fatalf("带 sessionId 的调用失败: %v", got["error"])
	}

	bad := c.call("Target.attachToTarget", map[string]any{"targetId": "nope", "flatten": true}, "")
	if bad["error"] == nil {
		t.Error("未知 targetId 应报错")
	}
	nonFlatten := c.call("Target.attachToTarget", map[string]any{"targetId": "page-1", "flatten": false}, "")
	if nonFlatten["error"] == nil {
		t.Error("flatten=false 应明确拒绝（本实现只支持 flatten）")
	}
}

// TestMethodNotFoundIsMinus32601：未实现的方法要回 -32601，客户端据此区分
// 「引擎没实现」与「调用失败」。
func TestMethodNotFoundIsMinus32601(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()
	got := c.call("Network.enable", nil, "")
	errObj, _ := got["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("应回错误: %v", got)
	}
	if errObj["code"] != float64(errMethodNotFound) {
		t.Errorf("错误码 = %v，want %d", errObj["code"], errMethodNotFound)
	}
}

// TestCaptureScreenshotIsPNG 覆盖验收判据 4 的协议侧：data 是 base64 的 PNG，
// 解码后能被 image/png 解析；format=jpeg 要明确失败（引擎只有 PNG 编码路径）。
func TestCaptureScreenshotIsPNG(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()
	got := c.call("Page.captureScreenshot", nil, "")
	res, _ := got["result"].(map[string]any)
	data, _ := res["data"].(string)
	if data == "" {
		t.Fatalf("没有 data: %v", got)
	}
	raw, err := base64Decode(data)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil || format != "png" {
		t.Fatalf("截图不是 PNG: format=%q err=%v", format, err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Errorf("尺寸 = %v", img.Bounds())
	}
	jpeg := c.call("Page.captureScreenshot", map[string]any{"format": "jpeg"}, "")
	if jpeg["error"] == nil {
		t.Error("format=jpeg 应明确失败")
	}
}

// TestDOMGetDocumentAndBoxModel：DOM 读链路（getDocument → nodeId → getBoxModel）
// 与「未知 nodeId」的明确报错。
func TestDOMGetDocumentAndBoxModel(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()

	got := c.call("DOM.getDocument", map[string]any{"depth": -1}, "")
	if got["error"] != nil {
		t.Fatalf("DOM.getDocument 报错: %v", got["error"])
	}
	root, _ := got["result"].(map[string]any)["root"].(map[string]any)
	if root == nil || root["nodeName"] != "HTML" {
		t.Fatalf("root = %v", got)
	}
	bodyID := findNodeID(root, "BODY")
	if bodyID == 0 {
		t.Fatalf("没有找到 BODY 节点: %v", root)
	}
	box := c.call("DOM.getBoxModel", map[string]any{"nodeId": bodyID}, "")
	model, _ := box["result"].(map[string]any)["model"].(map[string]any)
	if model == nil || model["width"] != float64(100) {
		t.Fatalf("boxModel = %v", box)
	}
	bad := c.call("DOM.getBoxModel", map[string]any{"nodeId": 9999}, "")
	if bad["error"] == nil {
		t.Error("未知 nodeId 应报错")
	}
}

// findNodeID 在 CDP 节点树里按 nodeName 找 nodeId（忽略大小写）。
func findNodeID(n map[string]any, name string) int {
	if n == nil {
		return 0
	}
	if s, _ := n["nodeName"].(string); strings.EqualFold(s, name) {
		if id, ok := n["nodeId"].(float64); ok {
			return int(id)
		}
	}
	children, _ := n["children"].([]any)
	for _, ch := range children {
		if cm, ok := ch.(map[string]any); ok {
			if id := findNodeID(cm, name); id != 0 {
				return id
			}
		}
	}
	return 0
}

// TestInputDispatchReachesAdapter：Input 域把协议参数原样交给适配层（坐标、
// 按钮、按键字段都不能在协议层被吞掉）。
func TestInputDispatchReachesAdapter(t *testing.T) {
	s, fa, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()

	if got := c.call("Input.dispatchMouseEvent", map[string]any{
		"type": "mousePressed", "x": 12.5, "y": 34.5, "button": "left", "clickCount": 1,
	}, ""); got["error"] != nil {
		t.Fatalf("鼠标事件失败: %v", got["error"])
	}
	if len(fa.mouse) != 1 || fa.mouse[0].X != 12.5 || fa.mouse[0].Y != 34.5 || fa.mouse[0].Button != "left" {
		t.Fatalf("适配层收到的鼠标事件 = %+v", fa.mouse)
	}
	if got := c.call("Input.dispatchKeyEvent", map[string]any{
		"type": "keyDown", "key": "Enter", "text": "\r", "windowsVirtualKeyCode": 13,
	}, ""); got["error"] != nil {
		t.Fatalf("按键事件失败: %v", got["error"])
	}
	if len(fa.keys) != 1 || fa.keys[0].Key != "Enter" || fa.keys[0].WindowsVirtualKeyCode != 13 {
		t.Fatalf("适配层收到的按键事件 = %+v", fa.keys)
	}
	if got := c.call("Input.dispatchMouseEvent", map[string]any{"type": "bogus"}, ""); got["error"] == nil {
		t.Error("未知事件类型应报错")
	}
}

// TestEmulationDeviceMetrics 覆盖 S3：设备指标覆盖支持 deviceScaleFactor（引擎侧
// SetDeviceScaleFactor），缺省按 1，非法尺寸明确报错，clearDeviceMetricsOverride
// 落到适配层。
func TestEmulationDeviceMetrics(t *testing.T) {
	s, fa, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()

	got := c.call("Emulation.setDeviceMetricsOverride", map[string]any{
		"width": 800, "height": 600, "deviceScaleFactor": 2, "mobile": false,
	}, "")
	if got["error"] != nil {
		t.Fatalf("DSF=2 应成功（S3 起引擎支持设备像素比）: %v", got["error"])
	}
	if len(fa.metrics) != 1 || fa.metrics[0] != (metricsCall{W: 800, H: 600, DSF: 2}) {
		t.Fatalf("适配层收到的设备指标 = %+v", fa.metrics)
	}
	if got := c.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1024, "height": 768}, ""); got["error"] != nil {
		t.Fatalf("缺省 DSF 应成功: %v", got["error"])
	}
	if len(fa.metrics) != 2 || fa.metrics[1].DSF != 1 {
		t.Errorf("缺省 DSF = %+v，want 1", fa.metrics)
	}
	// 非法尺寸必须明确报错（静默当成 0 会让页面几何全错，且客户端以为是协议问题）。
	if bad := c.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 0, "height": 600}, ""); bad["error"] == nil {
		t.Error("width=0 应报错")
	}
	if got := c.call("Emulation.clearDeviceMetricsOverride", nil, ""); got["error"] != nil {
		t.Fatalf("clearDeviceMetricsOverride 失败: %v", got["error"])
	}
	if fa.cleared != 1 {
		t.Errorf("clear 次数 = %d，want 1", fa.cleared)
	}
}

// TestPageNavigateReturnsLoaderIDAndFrameNavigated 覆盖 S3：navigate 回
// frameId + loaderId（每次导航唯一），并在**响应之后**推 Page.frameNavigated
// （Chrome 的事件顺序），空 url 明确报错。
func TestPageNavigateReturnsLoaderIDAndFrameNavigated(t *testing.T) {
	s, fa, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()

	const target = "data:text/html,<title>nav</title>"
	got := c.call("Page.navigate", map[string]any{"url": target}, "")
	if got["error"] != nil {
		t.Fatalf("Page.navigate 失败: %v", got["error"])
	}
	res, _ := got["result"].(map[string]any)
	if res["frameId"] != "page-1" {
		t.Errorf("frameId = %v，want page-1", res["frameId"])
	}
	loader, _ := res["loaderId"].(string)
	if loader == "" {
		t.Fatalf("loaderId 为空: %v", res)
	}
	if len(fa.navigated) != 1 || fa.navigated[0] != target {
		t.Fatalf("适配层收到的导航 = %v", fa.navigated)
	}
	// 事件排在响应之后（Chrome 顺序：响应 → frameNavigated）。
	msg := c.readMessage()
	if msg["method"] != "Page.frameNavigated" {
		t.Fatalf("首条事件 = %v，want Page.frameNavigated", msg["method"])
	}
	params, _ := msg["params"].(map[string]any)
	frame, _ := params["frame"].(map[string]any)
	if frame["loaderId"] != loader || frame["url"] != target || frame["id"] != "page-1" {
		t.Errorf("frameNavigated frame = %v", frame)
	}
	if frame["securityOrigin"] != "null" {
		t.Errorf("data: 的安全源应为 null，得到 %v", frame["securityOrigin"])
	}
	// 第二次导航给新 loaderId（客户端据此区分两次加载）。
	got2 := c.call("Page.navigate", map[string]any{"url": "about:blank"}, "")
	res2, _ := got2["result"].(map[string]any)
	if res2["loaderId"] == loader {
		t.Errorf("两次导航的 loaderId 相同（%v），应为新值", loader)
	}
	if bad := c.call("Page.navigate", map[string]any{"url": "   "}, ""); bad["error"] == nil {
		t.Error("空 url 应报错")
	}
}

// TestTargetSetAutoAttach 覆盖 S3：browser-level 连接调 setAutoAttach(flatten)
// 拿到 sessionId（Target.attachedToTarget），并用该 sessionId 驱动同一个页面；
// 非 flatten 明确报错。
func TestTargetSetAutoAttach(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.BrowserWSURL())
	defer c.close()

	if got := c.call("Target.setAutoAttach", map[string]any{"autoAttach": true, "flatten": true}, ""); got["error"] != nil {
		t.Fatalf("setAutoAttach 失败: %v", got["error"])
	}
	msg := c.readMessage()
	if msg["method"] != "Target.attachedToTarget" {
		t.Fatalf("首条事件 = %v，want Target.attachedToTarget", msg["method"])
	}
	params, _ := msg["params"].(map[string]any)
	sess, _ := params["sessionId"].(string)
	if sess == "" {
		t.Fatalf("attachedToTarget 无 sessionId: %v", params)
	}
	info, _ := params["targetInfo"].(map[string]any)
	if info["targetId"] != "page-1" || info["attached"] != true {
		t.Errorf("targetInfo = %v", info)
	}
	// 带 sessionId 的域调用落到该页面（S3 的会话语义）。
	got := c.call("Runtime.evaluate", map[string]any{"expression": "1+1", "returnByValue": true}, sess)
	if got["error"] != nil {
		t.Fatalf("带 sessionId 的调用失败: %v", got["error"])
	}
	if sess2, _ := got["sessionId"].(string); sess2 != sess {
		t.Errorf("响应未回带 sessionId: %v", got["sessionId"])
	}
	// 重复 setAutoAttach 不再推重复事件（复用已有会话）。
	if got := c.call("Target.setAutoAttach", map[string]any{"autoAttach": true, "flatten": true}, ""); got["error"] != nil {
		t.Fatalf("重复 setAutoAttach 失败: %v", got["error"])
	}
	if bad := c.call("Target.setAutoAttach", map[string]any{"autoAttach": true, "flatten": false}, ""); bad["error"] == nil {
		t.Error("非 flatten 的 setAutoAttach 应报错")
	}
}

// TestTargetSetDiscoverTargets 覆盖 S3：discover 打开时推 Target.targetCreated；
// getBrowserContexts 在单上下文宿主里回空数组。
func TestTargetSetDiscoverTargets(t *testing.T) {
	s, _, _ := startTestServer(t)
	c := dialCDP(t, s.BrowserWSURL())
	defer c.close()

	if got := c.call("Target.setDiscoverTargets", map[string]any{"discover": true}, ""); got["error"] != nil {
		t.Fatalf("setDiscoverTargets 失败: %v", got["error"])
	}
	msg := c.readMessage()
	if msg["method"] != "Target.targetCreated" {
		t.Fatalf("首条事件 = %v，want Target.targetCreated", msg["method"])
	}
	params, _ := msg["params"].(map[string]any)
	info, _ := params["targetInfo"].(map[string]any)
	if info["targetId"] != "page-1" || info["type"] != "page" {
		t.Errorf("targetInfo = %v", info)
	}
	got := c.call("Target.getBrowserContexts", nil, "")
	if got["error"] != nil {
		t.Fatalf("getBrowserContexts 失败: %v", got["error"])
	}
	res, _ := got["result"].(map[string]any)
	if list, ok := res["browserContextIds"].([]any); !ok || len(list) != 0 {
		t.Errorf("browserContextIds = %v，want 空数组", res["browserContextIds"])
	}
}

// TestConsoleEventPump：enable Runtime 后，宿主侧的增量控制台条目要被推成
// Runtime.consoleAPICalled 事件（DevTools Console 面板的数据来源）。
func TestConsoleEventPump(t *testing.T) {
	s, fa, _ := startTestServer(t)
	c := dialCDP(t, s.PageWSURL("page-1"))
	defer c.close()
	if got := c.call("Runtime.enable", nil, ""); got["error"] != nil {
		t.Fatalf("Runtime.enable 失败: %v", got)
	}
	fa.consoleQ = []ConsoleEntry{{Level: "error", Text: "ReferenceError: x is not defined"}}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		msg := c.readMessage()
		if msg["method"] == "Runtime.consoleAPICalled" {
			params, _ := msg["params"].(map[string]any)
			if params["type"] != "error" {
				t.Errorf("type = %v", params["type"])
			}
			return
		}
	}
	t.Fatal("没有收到 Runtime.consoleAPICalled 事件")
}

// TestHandlersRejectNonLoopback 覆盖验收判据 7 的一半：判定函数本身要正确
// （监听已限 loopback，这里保证「来源判定」不是摆设）。
func TestHandlersRejectNonLoopback(t *testing.T) {
	req, err := http.NewRequest("GET", "http://example.com/devtools/page/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "10.0.0.5:1234"
	if isLoopback(req) {
		t.Error("非 loopback 来源应被判定为 false")
	}
	req.RemoteAddr = "127.0.0.1:1234"
	if !isLoopback(req) {
		t.Error("loopback 来源应被判定为 true")
	}
	req.RemoteAddr = "[::1]:1234"
	if !isLoopback(req) {
		t.Error("IPv6 loopback 应被判定为 true")
	}
}

// base64Decode 是 base64.StdEncoding.DecodeString 的薄包装（测试里保持 import
// 清单最小）。
func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
