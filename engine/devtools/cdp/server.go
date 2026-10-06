package cdp

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"wb-ui/engine/devtools/cdp/ws"
)

// Options 是服务端配置。
type Options struct {
	// Adapter 是引擎适配面（宿主实现；缺省为 UnimplementedAdapter）。
	Adapter Adapter
	// Product 是 /json/version 的 Browser 字段，如 "wb-ui/0.1.0"。
	Product string
	// ProtocolVersion 是 CDP 协议版本（Chrome 用 "1.3"）。
	ProtocolVersion string
	// Revision / UserAgent / WebKitVersion 是 /json/version 的兼容字段。
	Revision      string
	UserAgent     string
	WebKitVersion string
	// Logf 打印服务日志（宿主传 log.Printf；nil = 静默）。
	Logf func(format string, args ...any)
}

// Server 是 CDP 服务端：HTTP 端点 + WebSocket 入口 + 会话注册表。
type Server struct {
	opt       Options
	httpSrv   *http.Server
	listener  net.Listener
	browserID string

	mu    sync.Mutex
	conns map[*wsConn]struct{}
}

// New 建服务端（不监听）。browserID 用于 /devtools/browser/<id> 路径段。
func New(opt Options) *Server {
	if opt.Adapter == nil {
		opt.Adapter = UnimplementedAdapter{}
	}
	if opt.Product == "" {
		opt.Product = "wb-ui"
	}
	if opt.ProtocolVersion == "" {
		opt.ProtocolVersion = "1.3"
	}
	if opt.Logf == nil {
		opt.Logf = func(string, ...any) {}
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "wb-ui"
	}
	return &Server{
		opt:       opt,
		browserID: fmt.Sprintf("%s-%d", host, os.Getpid()),
		conns:     map[*wsConn]struct{}{},
	}
}

// Start 在 **127.0.0.1:port** 上启动调试服务。port<=0 返回 (nil,"",nil)——宿主据此
// 实现「默认关闭、零开销」（docs/implementation-path.md §3.5）。返回值第二项是
// WebSocket 入口 URL，宿主照 Chrome 的格式打印 "DevTools listening on …"。
//
// ★ 只绑 loopback：调试协议等于把「执行任意 JS + 读页面内容」暴露给连接方，
// 绝不能监听 0.0.0.0。
func Start(opt Options, port int) (*Server, string, error) {
	if port <= 0 {
		return nil, "", nil
	}
	s := New(opt)
	if _, err := s.Listen(net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err != nil {
		// Windows 上端口处于 TIME_WAIT 时无法立即重绑（监听套接字是独占的），
		// 重新启动宿主时容易撞上——提示换端口，而不是只抛 "address in use"。
		return nil, "", fmt.Errorf("cdp: 监听 127.0.0.1:%d 失败（端口被占用或处于 TIME_WAIT；"+
			"可换端口重试）: %w", port, err)
	}
	return s, s.BrowserWSURL(), nil
}

// Listen 在 addr 上启动服务并返回实际监听地址（测试与自定义绑定用；生产路径是
// Start 的 loopback 包装——调试协议绝不能监听 0.0.0.0）。
func (s *Server) Listen(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s.listener = ln
	s.httpSrv = &http.Server{Handler: s.Handler()}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.opt.Logf("cdp: 服务退出: %v", err)
		}
	}()
	return ln.Addr().String(), nil
}

// Addr 返回实际监听地址（端口传 0 时由系统分配）。
func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// BrowserWSURL 返回浏览器级 WebSocket 入口。
func (s *Server) BrowserWSURL() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return "ws://" + s.listener.Addr().String() + "/devtools/browser/" + s.browserID
}

// PageWSURL 返回页面级 WebSocket 入口。
func (s *Server) PageWSURL(targetID string) string {
	if s == nil || s.listener == nil {
		return ""
	}
	return "ws://" + s.listener.Addr().String() + "/devtools/page/" + targetID
}

// Close 关停服务与所有连接。
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	conns := make([]*wsConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.c.Close()
	}
	if s.httpSrv != nil {
		return s.httpSrv.Close()
	}
	return nil
}

// ─── HTTP 端点 ───────────────────────────────────────────

// Handler 返回调试服务的 HTTP 处理器（端点与 Chrome 同形）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", s.handleVersion)
	mux.HandleFunc("/json/list", s.handleList)
	mux.HandleFunc("/json", s.handleList)
	mux.HandleFunc("/json/protocol", s.handleProtocol)
	mux.HandleFunc("/json/new", s.handleNew)
	mux.HandleFunc("/devtools/browser/", s.handleWS)
	mux.HandleFunc("/devtools/page/", s.handleWS)
	return mux
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"Browser":              s.opt.Product,
		"Protocol-Version":     s.opt.ProtocolVersion,
		"User-Agent":           s.opt.UserAgent,
		"WebKit-Version":       s.opt.WebKitVersion,
		"V8-Version":           "",
		"webSocketDebuggerUrl": s.BrowserWSURL(),
	})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	targets := s.opt.Adapter.Targets()
	out := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		out = append(out, map[string]any{
			"id":                   t.ID,
			"type":                 t.Type,
			"title":                t.Title,
			"url":                  t.URL,
			"webSocketDebuggerUrl": s.PageWSURL(t.ID),
			"devtoolsFrontendUrl":  "/devtools/inspector.html?ws=" + s.PageWSURL(t.ID),
		})
	}
	writeJSON(w, out)
}

// handleProtocol 返回**本实现**的支持面描述（不是 Chrome 的完整协议描述）：
// 客户端据此探测可用域，也方便人读「这台引擎支持哪些 CDP 方法」。
func (s *Server) handleProtocol(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, protocolDescription())
}

// handleNew 是「新建 target」：wb-ui 是单页面宿主（一个 WebView = 一个页面），
// 没有「新建标签页」语义，因此明确回 501，而不是伪造一个 target 让客户端误判。
func (s *Server) handleNew(w http.ResponseWriter, r *http.Request) {
	http.Error(w, `{"error":"wb-ui 宿主为单页面引擎，不支持新建 target"}`, http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "   ")
	_ = enc.Encode(v)
}

// isLoopback 判定请求来自本机（监听已限定 loopback；这里再挡一层代理/转发）。
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// ─── WebSocket 入口与会话 ────────────────────────────────

// handleWS 处理 /devtools/browser/<id>（浏览器级）与 /devtools/page/<targetId>
// （页面级，无 sessionId 的消息直接路由到该页面）。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		http.Error(w, "cdp: 只接受本机连接", http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/devtools/")
	var defaultTarget string
	if strings.HasPrefix(path, "page/") {
		defaultTarget = strings.TrimPrefix(path, "page/")
	}
	conn, err := ws.Upgrade(w, r)
	if err != nil {
		return // Upgrade 已写错误响应
	}
	c := &wsConn{
		srv:           s,
		c:             conn,
		sessions:      map[string]*session{},
		attached:      map[string]string{},
		done:          make(chan struct{}),
		defaultTarget: defaultTarget,
	}
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		close(c.done)
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	s.opt.Logf("cdp: 客户端已连接 %s（路径 %s）", conn.RemoteAddr(), r.URL.Path)
	go c.eventPump()
	c.loop()
	s.opt.Logf("cdp: 客户端已断开 %s", conn.RemoteAddr())
}

// session 是一条已附加的 CDP 会话（flatten 模式：sessionId 在消息里传递）。
type session struct {
	id       string
	targetID string
	// enabled 记录客户端 enable 过的域（事件推送按此过滤）。
	enabled map[string]bool
	// nodeId ↔ 宿主 backendID 双向表（CDP 的 nodeId 是会话级整数；DOM 域用）。
	nodeIDs    map[int]string
	backendIDs map[string]int
	nextNodeID int
}

func (s *session) domainEnabled(name string) bool { return s != nil && s.enabled[name] }

// nodeIDFor 返回 backendID 对应的 CDP nodeId（首次见到时分配）。
func (s *session) nodeIDFor(backendID string) int {
	if s.backendIDs == nil {
		s.backendIDs = map[string]int{}
	}
	if id, ok := s.backendIDs[backendID]; ok {
		return id
	}
	s.nextNodeID++
	id := s.nextNodeID
	if s.nodeIDs == nil {
		s.nodeIDs = map[int]string{}
	}
	s.nodeIDs[id] = backendID
	s.backendIDs[backendID] = id
	return id
}

// backendIDOf 把 CDP nodeId 换回宿主 backendID。
func (s *session) backendIDOf(nodeID int) (string, bool) {
	if s == nil || s.nodeIDs == nil {
		return "", false
	}
	id, ok := s.nodeIDs[nodeID]
	return id, ok
}

// wsConn 是一条 WebSocket 连接及其会话表。
type wsConn struct {
	srv *Server
	c   *ws.Conn

	writeMu sync.Mutex

	mu             sync.Mutex
	seq            int
	sessions       map[string]*session // sessionId → session
	attached       map[string]string   // targetId → sessionId
	defaultTarget  string              // /devtools/page/<id> 直连时为该 target
	defaultSession *session            // 无 sessionId 的调用共用的稳定会话（见 sessionFor）
	// loaderSeq 给每次导航分配新的 loaderId（CDP 的 loaderId 是「本次加载的
	// 唯一标识」，客户端据此区分两次导航）。
	loaderSeq int
	// queuedEvents 是「排在当前响应之后发送」的事件（导航引发的
	// Page.frameNavigated：Chrome 的顺序是响应在前、事件在后）。
	queuedEvents []queuedEvent
	// done 在连接关闭时被关闭：事件泵据此退出（否则轮询 goroutine 会泄漏）。
	done chan struct{}
}

// loop 读消息并分派，直到对端关闭。
func (c *wsConn) loop() {
	for {
		op, payload, err := c.c.ReadMessage()
		if err != nil {
			return
		}
		if op != ws.OpText {
			continue
		}
		c.handleMessage(payload)
	}
}

// handleMessage 解析一条消息并回响应（解析失败回 parse error）。
func (c *wsConn) handleMessage(payload []byte) {
	var req request
	if err := json.Unmarshal(payload, &req); err != nil {
		c.sendError(0, "", errParse, "消息不是合法 JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Method) == "" {
		c.sendError(req.ID, req.SessionID, errInvalidRequest, "缺少 method")
		return
	}
	sess := c.sessionFor(req.SessionID)
	result, perr := c.dispatch(sess, req)
	if perr != nil {
		c.sendError(req.ID, req.SessionID, perr.Code, perr.Message)
		return
	}
	c.sendResult(req.ID, req.SessionID, result)
	// 响应之后才发的事件（Chrome 的顺序：navigate 响应 → frameNavigated → …）。
	c.flushQueuedEvents()
}

// sessionFor 按 sessionId 找会话；没有 sessionId 时退回「默认目标」的隐含会话
// （/devtools/page/<id> 直连、或浏览器级入口下的唯一页面）。这是 S1 自家工装
// 最常用的形态：连上就能发 Runtime.evaluate，不必先 attach。
func (c *wsConn) sessionFor(sessionID string) *session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sessionID != "" {
		return c.sessions[sessionID]
	}
	if c.defaultTarget == "" {
		for _, s := range c.sessions {
			return s
		}
		targets := c.srv.opt.Adapter.Targets()
		if len(targets) > 0 {
			c.defaultTarget = targets[0].ID
		}
	}
	if c.defaultTarget == "" {
		return nil
	}
	if sid, ok := c.attached[c.defaultTarget]; ok {
		return c.sessions[sid]
	}
	// 隐含会话：客户端没 attach 就直接调用（S1 工装的最短路径）。★ 必须缓存成
	// 一条稳定会话——nodeId 映射与 enable 状态要跨消息保持，否则 DOM.getDocument
	// 拿到的 nodeId 在下一条消息里就失效了。
	if c.defaultSession == nil || c.defaultSession.targetID != c.defaultTarget {
		c.defaultSession = &session{
			targetID:   c.defaultTarget,
			enabled:    map[string]bool{},
			nodeIDs:    map[int]string{},
			backendIDs: map[string]int{},
		}
	}
	return c.defaultSession
}

// attach 建一条会话（Target.attachToTarget flatten 模式）；已附加时复用。
func (c *wsConn) attach(targetID string) *session {
	s, _ := c.attachNew(targetID)
	return s
}

// attachNew 建会话并报告是否**新建**（Target.setAutoAttach 只对新附加的目标推
// attachedToTarget 事件，复用已有会话时不重复推）。
func (c *wsConn) attachNew(targetID string) (*session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sid, ok := c.attached[targetID]; ok {
		return c.sessions[sid], false
	}
	c.seq++
	s := &session{
		id:         fmt.Sprintf("%s.%d", c.srv.browserID, c.seq),
		targetID:   targetID,
		enabled:    map[string]bool{},
		nodeIDs:    map[int]string{},
		backendIDs: map[string]int{},
	}
	c.sessions[s.id] = s
	c.attached[targetID] = s.id
	return s, true
}

// queuedEvent 是一条「响应之后再发」的事件。
type queuedEvent struct {
	sessionID string
	method    string
	params    any
}

// queueEvent 把事件排到「当前响应之后」发送（见 handleMessage 的 flush）。
func (c *wsConn) queueEvent(sessionID, method string, params any) {
	c.mu.Lock()
	c.queuedEvents = append(c.queuedEvents, queuedEvent{sessionID: sessionID, method: method, params: params})
	c.mu.Unlock()
}

// flushQueuedEvents 发送并清空排队的事件。
func (c *wsConn) flushQueuedEvents() {
	c.mu.Lock()
	evs := c.queuedEvents
	c.queuedEvents = nil
	c.mu.Unlock()
	for _, e := range evs {
		c.sendEvent(e.sessionID, e.method, e.params)
	}
}

// nextLoaderID 分配一个新的 loaderId（每次导航一个）。
func (c *wsConn) nextLoaderID(targetID string) string {
	c.mu.Lock()
	c.loaderSeq++
	n := c.loaderSeq
	c.mu.Unlock()
	return fmt.Sprintf("%s.l%d", targetID, n)
}

// detach 结束一条会话。
func (c *wsConn) detach(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.sessions[sessionID]; ok {
		delete(c.attached, s.targetID)
		delete(c.sessions, sessionID)
	}
}

// sessionList 返回当前会话快照（含 targetId）。
func (c *wsConn) sessionList() []*session {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*session, 0, len(c.sessions)+1)
	for _, s := range c.sessions {
		out = append(out, s)
	}
	// ★ 隐含会话（无 attach 直接用）不在 sessions 表里，但事件泵必须能看到它，
	// 否则「连上就 Runtime.enable」的客户端永远收不到 consoleAPICalled。
	if c.defaultSession != nil {
		out = append(out, c.defaultSession)
	}
	return out
}

// sendResult 回一条成功响应。
func (c *wsConn) sendResult(id int, sessionID string, result any) {
	raw, err := json.Marshal(result)
	if err != nil {
		c.sendError(id, sessionID, errInternal, "结果序列化失败: "+err.Error())
		return
	}
	c.writeJSON(response{ID: id, Result: raw, SessionID: sessionID})
}

// sendError 回一条错误响应。
func (c *wsConn) sendError(id int, sessionID string, code int, msg string) {
	c.writeJSON(response{ID: id, Error: &protocolError{Code: code, Message: msg}, SessionID: sessionID})
}

// sendEvent 推送一条事件。
func (c *wsConn) sendEvent(sessionID, method string, params any) {
	c.writeJSON(event{Method: method, Params: params, SessionID: sessionID})
}

func (c *wsConn) writeJSON(v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.c.WriteText(payload)
}

// protocolDescription 是本实现的支持面（/json/protocol）。刻意只列我们真正
// 实现的方法：客户端（与人）据此判断可用范围，而不是拿到 Chrome 的全量清单再
// 在调用时才发现 -32601。
func protocolDescription() map[string]any {
	domains := map[string][]string{
		"Browser":   {"getVersion", "getBrowserCommandLine"},
		"Target":    {"getTargets", "getTargetInfo", "attachToTarget", "detachFromTarget", "setDiscoverTargets", "setAutoAttach", "getBrowserContexts", "activateTarget"},
		"Runtime":   {"enable", "evaluate", "callFunctionOn", "getProperties", "releaseObject", "releaseObjectGroup"},
		"Page":      {"enable", "navigate", "reload", "captureScreenshot", "getLayoutMetrics"},
		"DOM":       {"enable", "getDocument", "describeNode", "querySelector", "querySelectorAll", "getAttributes", "getOuterHTML", "getBoxModel", "resolveNode", "setAttributeValue", "removeAttribute", "requestChildNodes"},
		"CSS":       {"enable", "disable", "getComputedStyleForNode", "getInlineStylesForNode", "getMatchedStylesForNode"},
		"Input":     {"dispatchMouseEvent", "dispatchKeyEvent"},
		"Log":       {"enable", "disable", "clear"},
		"Emulation": {"setDeviceMetricsOverride", "clearDeviceMetricsOverride"},
	}
	names := make([]string, 0, len(domains))
	for d := range domains {
		names = append(names, d)
	}
	return map[string]any{
		"version": map[string]any{"major": "1", "minor": "3"},
		"domains": domains,
		"note": "本文件描述 wb-ui 引擎实现的 CDP 子集（非 Chrome 全量协议）：" +
			"详见 docs/implementation-path.md §3。未列出的方法返回 -32601。",
	}
}
