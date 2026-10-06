package cdp

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// dispatch 把一条消息分派到对应域方法。未实现的方法统一回 -32601（方法不存在），
// 让客户端能区分「这台引擎没实现」与「调用出错」。
func (c *wsConn) dispatch(s *session, req request) (any, *protocolError) {
	switch req.Method {

	// ── Browser ─────────────────────────────────────────
	case "Browser.getVersion":
		return c.browserVersion(), nil
	case "Browser.getBrowserCommandLine":
		return map[string]any{"arguments": []string{}}, nil

	// ── Target ──────────────────────────────────────────
	case "Target.getTargets":
		return c.getTargets(), nil
	case "Target.getTargetInfo":
		var p struct {
			TargetID string `json:"targetId"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		for _, t := range c.srv.opt.Adapter.Targets() {
			if p.TargetID == "" || t.ID == p.TargetID {
				return map[string]any{"targetInfo": c.targetInfoJSON(t)}, nil
			}
		}
		return nil, &protocolError{Code: errServer, Message: "cdp: 未知 targetId " + p.TargetID}
	case "Target.setDiscoverTargets":
		var p struct {
			Discover bool `json:"discover"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		c.enableDomain(s, "Target")
		if p.Discover {
			// Chrome 语义：discover 打开时把**当前已知**的目标推一遍
			// targetCreated（之后新目标增量推）；客户端（Puppeteer 的
			// browser-level 连接）据此发现页面。
			for _, t := range c.srv.opt.Adapter.Targets() {
				c.queueEvent(s.id, "Target.targetCreated", map[string]any{
					"targetInfo": c.targetInfoJSON(t),
				})
			}
		}
		return map[string]any{}, nil
	case "Target.setAutoAttach":
		// 单页面宿主的自动附加：autoAttach=true 时把当前页面目标附加给
		// 该连接，并推 Target.attachedToTarget（Puppeteer 的 browser-level
		// 连接走这条路径拿 sessionId，之后所有域调用带 sessionId）。
		var p struct {
			AutoAttach             bool `json:"autoAttach"`
			WaitForDebuggerOnStart bool `json:"waitForDebuggerOnStart"`
			Flatten                bool `json:"flatten"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if p.AutoAttach && !p.Flatten {
			return nil, &protocolError{Code: errServer, Message: "cdp: 只支持 flatten=true（会话内联在消息的 sessionId 字段）"}
		}
		c.enableDomain(s, "Target")
		if p.AutoAttach {
			for _, t := range c.srv.opt.Adapter.Targets() {
				sess, created := c.attachNew(t.ID)
				if !created {
					continue // 已附加过：不重复推事件（否则客户端会拿到重复会话）
				}
				c.queueEvent(s.id, "Target.attachedToTarget", map[string]any{
					"sessionId":          sess.id,
					"targetInfo":         c.targetInfoJSON(t),
					"waitingForDebugger": false,
				})
			}
		}
		return map[string]any{}, nil
	case "Target.getBrowserContexts":
		// 引擎是单页面、单上下文宿主（没有「无痕 / 多用户」语义）。Chrome 的
		// 默认上下文**不在**该列表里（列表只含新建的额外上下文）→ 空数组。
		return map[string]any{"browserContextIds": []any{}}, nil
	case "Target.attachToTarget":
		var p struct {
			TargetID string `json:"targetId"`
			Flatten  bool   `json:"flatten"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		// 非 flatten 模式要求为每个会话开独立隧道，本实现不做——明确回错而不是
		// 静默降级（客户端据此知道要用 flatten:true）。
		if !p.Flatten {
			return nil, &protocolError{Code: errServer, Message: "cdp: 只支持 flatten=true（会话内联在消息的 sessionId 字段）"}
		}
		if !c.targetExists(p.TargetID) {
			return nil, &protocolError{Code: errServer, Message: "cdp: 未知 targetId " + p.TargetID}
		}
		return map[string]any{"sessionId": c.attach(p.TargetID).id}, nil
	case "Target.detachFromTarget":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		c.detach(p.SessionID)
		return map[string]any{}, nil
	case "Target.activateTarget":
		return map[string]any{}, nil

	// ── Runtime ─────────────────────────────────────────
	case "Runtime.enable":
		c.enableDomain(s, "Runtime")
		return map[string]any{}, nil
	case "Runtime.evaluate":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Expression            string `json:"expression"`
			ReturnByValue         bool   `json:"returnByValue"`
			AwaitPromise          bool   `json:"awaitPromise"`
			IncludeCommandLineAPI bool   `json:"includeCommandLineAPI"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if strings.TrimSpace(p.Expression) == "" {
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: expression 不能为空"}
		}
		// returnByValue=true 回 JSON 值；缺省（false）回 **objectId**（DevTools 拿到
		// objectId 才会去 getProperties 展开对象）。宿主未实现句柄路径时退回值路径，
		// 保证 P0/P1 的客户端不受影响。
		var v RemoteValue
		var err error
		if p.ReturnByValue {
			v, err = c.srv.opt.Adapter.Evaluate(targetID, p.Expression)
		} else {
			v, err = c.srv.opt.Adapter.EvaluateHandle(targetID, p.Expression)
			var ni NotImplementedError
			if errors.As(err, &ni) {
				v, err = c.srv.opt.Adapter.Evaluate(targetID, p.Expression)
			}
		}
		if err != nil {
			return nil, adapterError(err, "Runtime.evaluate")
		}
		res := map[string]any{"result": remoteObjectOf(v)}
		if v.Exception {
			// CDP 语义：脚本抛错仍是一次成功的 evaluate，异常信息在
			// exceptionDetails 里（DevTools Console 靠它显示红字）。
			res["exceptionDetails"] = map[string]any{
				"text": v.Description,
				"exception": remoteObjectOf(RemoteValue{
					Type: "object", Subtype: "error", Description: v.Description,
				}),
			}
		}
		// awaitPromise 未实现（引擎求值是同步的，Promise 由事件循环推进）：
		// 文档 §3.3 已标注；这里不轮询等待，避免把阻塞带进分派层。
		_ = p.AwaitPromise
		return res, nil
	case "Runtime.callFunctionOn":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			FunctionDeclaration string `json:"functionDeclaration"`
			ObjectID            string `json:"objectId"`
			Arguments           []struct {
				Value any `json:"value"`
			} `json:"arguments"`
			ReturnByValue bool `json:"returnByValue"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if strings.TrimSpace(p.FunctionDeclaration) == "" {
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: functionDeclaration 不能为空"}
		}
		args := make([]any, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			args = append(args, a.Value)
		}
		// objectId 形态（S2）：以该对象为 this 调用——DevTools 的 Console 与 Elements
		// 面板都走这条路（`$0.classList.add(...)` 之类）。
		var v RemoteValue
		var err error
		if strings.TrimSpace(p.ObjectID) != "" {
			v, err = c.srv.opt.Adapter.CallFunctionOnObject(targetID, p.ObjectID, p.FunctionDeclaration, args)
		} else {
			v, err = c.srv.opt.Adapter.CallFunctionOn(targetID, p.FunctionDeclaration, args)
		}
		if err != nil {
			return nil, adapterError(err, "Runtime.callFunctionOn")
		}
		return map[string]any{"result": remoteObjectOf(v)}, nil
	case "Runtime.releaseObject", "Runtime.releaseObjectGroup":
		// 句柄表由宿主维护（S2）：释放请求透传；宿主未实现时按 no-op 处理（句柄随
		// 页面/连接生命周期回收，不会泄漏 goroutine）。
		var p struct {
			ObjectID string `json:"objectId"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if strings.TrimSpace(p.ObjectID) != "" {
			targetID, perr := requireTarget(s)
			if perr == nil {
				if err := c.srv.opt.Adapter.ReleaseObject(targetID, p.ObjectID); err != nil {
					var ni NotImplementedError
					if !errors.As(err, &ni) {
						return nil, adapterError(err, "Runtime.releaseObject")
					}
				}
			}
		}
		return map[string]any{}, nil
	case "Runtime.getProperties":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			ObjectID      string `json:"objectId"`
			OwnProperties bool   `json:"ownProperties"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if strings.TrimSpace(p.ObjectID) == "" {
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: Runtime.getProperties 需要 objectId"}
		}
		props, err := c.srv.opt.Adapter.GetProperties(targetID, p.ObjectID)
		if err != nil {
			return nil, adapterError(err, "Runtime.getProperties")
		}
		list := make([]map[string]any, 0, len(props))
		for _, pd := range props {
			list = append(list, map[string]any{
				"name":         pd.Name,
				"value":        remoteObjectOf(pd.Value),
				"writable":     pd.Writable,
				"enumerable":   pd.Enumerable,
				"configurable": pd.Configurable,
				"isOwn":        true, // 宿主只回自身属性（CDP 的 ownProperties 语义）
			})
		}
		return map[string]any{"result": list, "internalProperties": []any{}}, nil

	// ── Page ────────────────────────────────────────────
	case "Page.enable":
		c.enableDomain(s, "Page")
		return map[string]any{}, nil
	case "Page.disable":
		return map[string]any{}, nil
	case "Page.navigate":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			URL            string `json:"url"`
			FrameID        string `json:"frameId"`
			TransitionType string `json:"transitionType"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		if strings.TrimSpace(p.URL) == "" {
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: url 不能为空"}
		}
		if err := c.srv.opt.Adapter.Navigate(targetID, p.URL); err != nil {
			return nil, adapterError(err, "Page.navigate")
		}
		// ★ S3：navigate 要回 loaderId（客户端据此区分两次导航），并在响应之后
		// 推 Page.frameNavigated——导航已完成（宿主 LoadURL 是同步的），
		// 顺序按 Chrome：响应 → frameNavigated → loadEventFired（事件泵）。
		loaderID := c.nextLoaderID(targetID)
		c.queueEvent(s.id, "Page.frameNavigated", map[string]any{
			"frame": map[string]any{
				"id":             targetID,
				"loaderId":       loaderID,
				"url":            p.URL,
				"name":           "",
				"securityOrigin": securityOriginOf(p.URL),
				// 引擎只装载 HTML 文档（LoadURL 取到内容后按 HTML 解析）。
				"mimeType": "text/html",
			},
			"type": "Navigation",
		})
		return map[string]any{"frameId": targetID, "loaderId": loaderID}, nil
	case "Page.reload":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		if err := c.srv.opt.Adapter.Reload(targetID); err != nil {
			return nil, adapterError(err, "Page.reload")
		}
		return map[string]any{}, nil
	case "Page.captureScreenshot":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Format string `json:"format"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		format := strings.ToLower(strings.TrimSpace(p.Format))
		if format != "" && format != "png" {
			// 引擎只产出 PNG（宿主把 Render() 的像素缓冲编码 PNG）；明确回错，
			// 免得客户端拿到「声称 jpeg 实为 png」的脏数据。
			return nil, &protocolError{Code: errServer, Message: "cdp: 只支持 format=png（收到 " + p.Format + "）"}
		}
		png, err := c.srv.opt.Adapter.Screenshot(targetID)
		if err != nil {
			return nil, adapterError(err, "Page.captureScreenshot")
		}
		return map[string]any{"data": base64.StdEncoding.EncodeToString(png)}, nil
	case "Page.getLayoutMetrics":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		// 视口/文档尺寸走一次求值（引擎的 innerWidth/innerHeight 与文档滚动尺寸）。
		v, err := c.srv.opt.Adapter.Evaluate(targetID,
			`(function(){return {iw:window.innerWidth,ih:window.innerHeight,sw:document.documentElement?document.documentElement.scrollWidth:0,sh:document.documentElement?document.documentElement.scrollHeight:0};})()`)
		if err != nil {
			return nil, adapterError(err, "Page.getLayoutMetrics")
		}
		m, _ := v.Value.(map[string]any)
		num := func(key string) float64 {
			if m == nil {
				return 0
			}
			if f, ok := m[key].(float64); ok {
				return f
			}
			return 0
		}
		iw, ih, sw, sh := num("iw"), num("ih"), num("sw"), num("sh")
		return map[string]any{
			"layoutViewport": map[string]any{"pageX": 0, "pageY": 0, "clientWidth": iw, "clientHeight": ih},
			"visualViewport": map[string]any{"offsetX": 0, "offsetY": 0, "pageX": 0, "pageY": 0, "clientWidth": iw, "clientHeight": ih, "scale": 1},
			"contentSize":    map[string]any{"x": 0, "y": 0, "width": sw, "height": sh},
		}, nil

	// ── DOM（读） ────────────────────────────────────────
	case "DOM.enable":
		c.enableDomain(s, "DOM")
		return map[string]any{}, nil
	case "DOM.getDocument":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Depth *int `json:"depth"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		depth := 1 // CDP 缺省：根 + 直接子节点
		if p.Depth != nil {
			depth = *p.Depth
		}
		root, err := c.srv.opt.Adapter.DOMDocument(targetID)
		if err != nil {
			return nil, adapterError(err, "DOM.getDocument")
		}
		return map[string]any{"root": c.cdpNode(s, root, depth)}, nil
	case "DOM.describeNode":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		node, err := c.srv.opt.Adapter.DOMQuery(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "DOM.describeNode")
		}
		if node == nil {
			return nil, &protocolError{Code: errServer, Message: "cdp: 找不到节点 " + backendID}
		}
		return map[string]any{"node": c.cdpNode(s, node, 0)}, nil
	case "DOM.querySelector":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			NodeID   int    `json:"nodeId"`
			Selector string `json:"selector"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		node, err := c.srv.opt.Adapter.DOMQuery(targetID, p.Selector)
		if err != nil {
			return nil, adapterError(err, "DOM.querySelector")
		}
		if node == nil {
			return map[string]any{"nodeId": 0}, nil // CDP 约定：0 = 未找到
		}
		return map[string]any{"nodeId": s.nodeIDFor(node.BackendID)}, nil
	case "DOM.getBoxModel":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		box, err := c.srv.opt.Adapter.DOMBoxModel(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "DOM.getBoxModel")
		}
		if box == nil {
			return nil, &protocolError{Code: errServer, Message: "cdp: 找不到节点 " + backendID}
		}
		quad := []float64{box.X, box.Y, box.X + box.Width, box.Y, box.X + box.Width, box.Y + box.Height, box.X, box.Y + box.Height}
		return map[string]any{"model": map[string]any{
			"content": quad, "padding": quad, "border": quad, "margin": quad,
			"width": box.Width, "height": box.Height,
		}}, nil

	case "DOM.querySelectorAll":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			NodeID   int    `json:"nodeId"`
			Selector string `json:"selector"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		nodes, err := c.srv.opt.Adapter.DOMQueryAll(targetID, p.Selector)
		if err != nil {
			return nil, adapterError(err, "DOM.querySelectorAll")
		}
		ids := make([]int, 0, len(nodes))
		for _, n := range nodes {
			if n != nil {
				ids = append(ids, s.nodeIDFor(n.BackendID))
			}
		}
		return map[string]any{"nodeIds": ids}, nil
	case "DOM.getAttributes":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		attrs, err := c.srv.opt.Adapter.DOMAttributes(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "DOM.getAttributes")
		}
		return map[string]any{"attributes": flattenAttrs(attrs)}, nil
	case "DOM.getOuterHTML":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		html, err := c.srv.opt.Adapter.DOMOuterHTML(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "DOM.getOuterHTML")
		}
		return map[string]any{"outerHTML": html}, nil
	case "DOM.setAttributeValue":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			NodeID int    `json:"nodeId"`
			Name   string `json:"name"`
			Value  string `json:"value"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		backendID, ok := s.backendIDOf(p.NodeID)
		if !ok {
			return nil, &protocolError{Code: errServer, Message: "cdp: 未知 nodeId（先调 DOM.getDocument 建立映射）"}
		}
		if err := c.srv.opt.Adapter.DOMSetAttribute(targetID, backendID, p.Name, p.Value); err != nil {
			return nil, adapterError(err, "DOM.setAttributeValue")
		}
		return map[string]any{}, nil
	case "DOM.removeAttribute":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			NodeID int    `json:"nodeId"`
			Name   string `json:"name"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		backendID, ok := s.backendIDOf(p.NodeID)
		if !ok {
			return nil, &protocolError{Code: errServer, Message: "cdp: 未知 nodeId（先调 DOM.getDocument 建立映射）"}
		}
		if err := c.srv.opt.Adapter.DOMRemoveAttribute(targetID, backendID, p.Name); err != nil {
			return nil, adapterError(err, "DOM.removeAttribute")
		}
		return map[string]any{}, nil
	case "DOM.resolveNode":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		objectID, err := c.srv.opt.Adapter.DOMElementObjectID(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "DOM.resolveNode")
		}
		return map[string]any{"object": map[string]any{
			"type": "object", "subtype": "node", "objectId": objectID,
		}}, nil
	case "DOM.requestChildNodes":
		// 子节点：getDocument(depth=-1) 已经给全树，nodeId 映射也由它建立；这里
		// 只需一个成功的空结果（客户端据此认为该子树可展开）。
		return map[string]any{}, nil

	// ── CSS（S2：Elements 面板的 Styles / Computed 标签） ──
	case "CSS.enable":
		c.enableDomain(s, "CSS")
		return map[string]any{}, nil
	case "CSS.disable":
		return map[string]any{}, nil
	case "CSS.getComputedStyleForNode":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		props, err := c.srv.opt.Adapter.ComputedStyle(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "CSS.getComputedStyleForNode")
		}
		list := make([]map[string]any, 0, len(props))
		for _, p := range props {
			list = append(list, map[string]any{"name": p.Name, "value": p.Value, "implicit": p.Implicit})
		}
		return map[string]any{"computedStyle": list}, nil
	case "CSS.getInlineStylesForNode":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		info, err := c.srv.opt.Adapter.InlineStyle(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "CSS.getInlineStylesForNode")
		}
		return map[string]any{"inlineStyle": cdpStyle(info)}, nil
	case "CSS.getMatchedStylesForNode":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		backendID, perr := c.backendIDFromParams(s, req)
		if perr != nil {
			return nil, perr
		}
		rules, err := c.srv.opt.Adapter.MatchedRules(targetID, backendID)
		if err != nil {
			return nil, adapterError(err, "CSS.getMatchedStylesForNode")
		}
		matched := make([]map[string]any, 0, len(rules))
		for _, r := range rules {
			rule := map[string]any{
				"selectorList": map[string]any{
					"selectors": []map[string]any{{"text": r.Selector}},
					"text":      r.Selector,
				},
				"origin": r.Origin,
				"style":  cdpStyle(&CSSStyleInfo{StyleSheetID: r.StyleSheetID, Properties: r.Properties}),
			}
			matched = append(matched, map[string]any{"rule": rule})
		}
		return map[string]any{"matchedCSSRules": matched}, nil

	// ── Input ───────────────────────────────────────────
	case "Input.dispatchMouseEvent":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Type       string  `json:"type"`
			X          float64 `json:"x"`
			Y          float64 `json:"y"`
			Button     string  `json:"button"`
			Buttons    int     `json:"buttons"`
			ClickCount int     `json:"clickCount"`
			DeltaX     float64 `json:"deltaX"`
			DeltaY     float64 `json:"deltaY"`
			Modifiers  int     `json:"modifiers"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		ev := MouseEvent{
			Type: p.Type, X: p.X, Y: p.Y, Button: p.Button, Buttons: p.Buttons,
			ClickCount: p.ClickCount, DeltaX: p.DeltaX, DeltaY: p.DeltaY, Modifiers: p.Modifiers,
		}
		switch ev.Type {
		case "mousePressed", "mouseReleased", "mouseMoved", "mouseWheel":
		default:
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: 不支持的鼠标事件类型 " + ev.Type}
		}
		if err := c.srv.opt.Adapter.DispatchMouse(targetID, ev); err != nil {
			return nil, adapterError(err, "Input.dispatchMouseEvent")
		}
		return map[string]any{}, nil
	case "Input.dispatchKeyEvent":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Type                  string `json:"type"`
			Key                   string `json:"key"`
			Code                  string `json:"code"`
			Text                  string `json:"text"`
			WindowsVirtualKeyCode int    `json:"windowsVirtualKeyCode"`
			Modifiers             int    `json:"modifiers"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		ev := KeyEvent{
			Type: p.Type, Key: p.Key, Code: p.Code, Text: p.Text,
			WindowsVirtualKeyCode: p.WindowsVirtualKeyCode, Modifiers: p.Modifiers,
		}
		switch ev.Type {
		case "keyDown", "keyUp", "rawKeyDown", "char":
		default:
			return nil, &protocolError{Code: errInvalidParams, Message: "cdp: 不支持的按键事件类型 " + ev.Type}
		}
		if err := c.srv.opt.Adapter.DispatchKey(targetID, ev); err != nil {
			return nil, adapterError(err, "Input.dispatchKeyEvent")
		}
		return map[string]any{}, nil

	// ── Log ─────────────────────────────────────────────
	case "Log.enable":
		c.enableDomain(s, "Log")
		return map[string]any{}, nil
	case "Log.disable", "Log.clear":
		return map[string]any{}, nil

	// ── Emulation ───────────────────────────────────────
	case "Emulation.setDeviceMetricsOverride":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		var p struct {
			Width             int     `json:"width"`
			Height            int     `json:"height"`
			DeviceScaleFactor float64 `json:"deviceScaleFactor"`
			Mobile            bool    `json:"mobile"`
			ScreenOrientation any     `json:"screenOrientation"`
		}
		if perr := decodeParams(req, &p); perr != nil {
			return nil, perr
		}
		// ★ S3：deviceScaleFactor 已支持（引擎侧 SetDeviceScaleFactor）——它影响
		// window.devicePixelRatio、CSS 媒体查询与截图输出分辨率（见 TECH_DEBT 的
		// 语义边界）。width/height 是 **CSS 像素**视口尺寸；<=0 视为非法
		// （静默当成 0 会让页面几何全错，且客户端以为是协议问题）。
		dsf := p.DeviceScaleFactor
		if dsf <= 0 {
			dsf = 1 // CDP 里缺省表示「跟随设备」，本引擎的设备比为 1
		}
		if p.Width <= 0 || p.Height <= 0 {
			return nil, &protocolError{Code: errInvalidParams,
				Message: fmt.Sprintf("cdp: width/height 必须为正（收到 %dx%d）", p.Width, p.Height)}
		}
		if err := c.srv.opt.Adapter.SetDeviceMetrics(targetID, p.Width, p.Height, dsf); err != nil {
			return nil, adapterError(err, "Emulation.setDeviceMetricsOverride")
		}
		return map[string]any{}, nil
	case "Emulation.clearDeviceMetricsOverride":
		targetID, perr := requireTarget(s)
		if perr != nil {
			return nil, perr
		}
		if err := c.srv.opt.Adapter.ClearDeviceMetrics(targetID); err != nil {
			return nil, adapterError(err, "Emulation.clearDeviceMetricsOverride")
		}
		return map[string]any{}, nil
	}
	return nil, &protocolError{Code: errMethodNotFound, Message: "cdp: 未实现的方法 " + req.Method}
}

// ─── 辅助 ────────────────────────────────────────────────

// decodeParams 解出 params（缺省时保持零值）。
func decodeParams(req request, out any) *protocolError {
	if len(req.Params) == 0 || string(req.Params) == "null" {
		return nil
	}
	if err := json.Unmarshal(req.Params, out); err != nil {
		return &protocolError{Code: errInvalidParams, Message: "cdp: params 解析失败: " + err.Error()}
	}
	return nil
}

// requireTarget 返回会话目标；没有目标时给出明确错误（客户端据此知道「引擎里
// 没有页面」，而不是收到一个空结果误判成功）。
func requireTarget(s *session) (string, *protocolError) {
	if s == nil || s.targetID == "" {
		return "", &protocolError{Code: errServer, Message: "cdp: 没有可用的页面目标（页面尚未装载？）"}
	}
	return s.targetID, nil
}

// adapterError 把宿主错误转成协议错误：未实现 → -32601，其余 → -32000。
func adapterError(err error, method string) *protocolError {
	var ni NotImplementedError
	if errors.As(err, &ni) {
		return &protocolError{Code: errMethodNotFound, Message: "cdp: " + method + " 未实现（引擎适配层：" + ni.Method + "）"}
	}
	return &protocolError{Code: errServer, Message: method + " 失败: " + err.Error()}
}

// remoteObjectOf 把 RemoteValue 转成 CDP RemoteObject（省略空字段）。
func remoteObjectOf(v RemoteValue) map[string]any {
	t := strings.TrimSpace(v.Type)
	if t == "" {
		t = "undefined"
	}
	out := map[string]any{"type": t}
	if v.ObjectID != "" {
		out["objectId"] = v.ObjectID
	}
	if v.Subtype != "" {
		out["subtype"] = v.Subtype
	}
	if v.Value != nil {
		out["value"] = v.Value
	}
	if v.Description != "" {
		out["description"] = v.Description
	}
	return out
}

// targetInfoJSON 组装 CDP TargetInfo。
func (c *wsConn) targetInfoJSON(t Target) map[string]any {
	c.mu.Lock()
	_, attached := c.attached[t.ID]
	c.mu.Unlock()
	return map[string]any{
		"targetId": t.ID, "type": t.Type, "title": t.Title, "url": t.URL,
		"attached": attached, "canAccessOpener": false,
	}
}

// getTargets 返回全部目标。
func (c *wsConn) getTargets() map[string]any {
	infos := []map[string]any{}
	for _, t := range c.srv.opt.Adapter.Targets() {
		infos = append(infos, c.targetInfoJSON(t))
	}
	return map[string]any{"targetInfos": infos}
}

// targetExists 判定 targetId 是否有效。
func (c *wsConn) targetExists(id string) bool {
	for _, t := range c.srv.opt.Adapter.Targets() {
		if t.ID == id {
			return true
		}
	}
	return false
}

// securityOriginOf 从 URL 取安全源（CDP Page.Frame 的 securityOrigin）：
// http(s)/file 等带 host 的源给 "scheme://host"；不透明源（data: 等）按浏览器
// 语义给 "null"（不是空串——DevTools 前端会显示它）。
func securityOriginOf(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	if strings.HasPrefix(raw, "file:") {
		return "file://"
	}
	return "null"
}

// browserVersion 组装 Browser.getVersion 结果。
func (c *wsConn) browserVersion() map[string]any {
	o := c.srv.opt
	return map[string]any{
		"protocolVersion": o.ProtocolVersion,
		"product":         o.Product,
		"revision":        o.Revision,
		"userAgent":       o.UserAgent,
		"jsVersion":       "",
	}
}

// enableDomain 记录客户端 enable 的域（事件推送按此过滤）。
func (c *wsConn) enableDomain(s *session, name string) {
	if s == nil {
		return
	}
	if s.enabled == nil {
		s.enabled = map[string]bool{}
	}
	s.enabled[name] = true
}

// backendIDFromParams 从 params 里取出 nodeId（或 backendNodeId）并换成宿主
// backendID。没有 nodeId 时回错误——几何查询必须指明节点。
func (c *wsConn) backendIDFromParams(s *session, req request) (string, *protocolError) {
	var p struct {
		NodeID        int    `json:"nodeId"`
		BackendNodeID int    `json:"backendNodeId"`
		ObjectID      string `json:"objectId"`
	}
	if perr := decodeParams(req, &p); perr != nil {
		return "", perr
	}
	id := p.NodeID
	if id == 0 {
		id = p.BackendNodeID
	}
	if id == 0 {
		return "", &protocolError{Code: errInvalidParams, Message: "cdp: 需要 nodeId"}
	}
	backendID, ok := s.backendIDOf(id)
	if !ok {
		return "", &protocolError{Code: errServer, Message: "cdp: 未知 nodeId（先调 DOM.getDocument/querySelector 建立映射）"}
	}
	return backendID, nil
}

// cdpNode 把宿主 DOM 快照转成 CDP DOM.Node（depth<0 = 全树；0 = 只本节点；
// n>0 = 往下展开 n 层）。
func (c *wsConn) cdpNode(s *session, n *DOMNode, depth int) map[string]any {
	if n == nil {
		return nil
	}
	id := s.nodeIDFor(n.BackendID)
	children := []map[string]any{}
	if n.Text != "" {
		children = append(children, map[string]any{
			"nodeId":         s.nodeIDFor(n.BackendID + "#text"),
			"parentId":       id,
			"nodeType":       3,
			"nodeName":       "#text",
			"nodeValue":      n.Text,
			"childNodeCount": 0,
		})
	}
	if depth != 0 {
		next := depth - 1
		for _, ch := range n.Children {
			if cnode := c.cdpNode(s, ch, next); cnode != nil {
				cnode["parentId"] = id
				children = append(children, cnode)
			}
		}
	}
	return map[string]any{
		"nodeId":         id,
		"backendNodeId":  id,
		"nodeType":       1,
		"nodeName":       strings.ToUpper(n.Tag),
		"localName":      n.Tag,
		"nodeValue":      "",
		"attributes":     flattenAttrs(n.Attrs),
		"childNodeCount": childCount(n),
		"children":       children,
	}
}

// childCount 计算快照节点的子节点数（元素子 + 文本子，与 CDP 的语义一致）。
func childCount(n *DOMNode) int {
	cnt := len(n.Children)
	if n.Text != "" {
		cnt++
	}
	return cnt
}

// flattenAttrs 把属性对拍平成 CDP 的 [name, value, name, value…] 形式。
func flattenAttrs(attrs [][2]string) []string {
	out := make([]string, 0, len(attrs)*2)
	for _, kv := range attrs {
		out = append(out, kv[0], kv[1])
	}
	return out
}

// cdpStyle 把宿主的样式信息转成 CDP 的 CSS.CSSStyle（nil 入参回 nil：CDP 里
// `inlineStyle: null` 表示该元素没有内联样式，与空样式不同）。
func cdpStyle(info *CSSStyleInfo) map[string]any {
	if info == nil {
		return nil
	}
	props := make([]map[string]any, 0, len(info.Properties))
	for _, p := range info.Properties {
		props = append(props, map[string]any{"name": p.Name, "value": p.Value, "implicit": p.Implicit})
	}
	out := map[string]any{
		"cssProperties":     props,
		"shorthandEntries":  []any{},
		"range":             map[string]any{"startLine": 0, "startColumn": 0, "endLine": 0, "endColumn": 0},
	}
	if info.StyleSheetID != "" {
		out["styleSheetId"] = info.StyleSheetID
	}
	if info.CSSText != "" {
		out["cssText"] = info.CSSText
	}
	return out
}

// ─── 事件泵 ──────────────────────────────────────────────

// eventPumpInterval 是事件轮询间隔：CDP 事件在 Chrome 里是即时推送的，本实现
// 用「宿主增量缓冲 + 轮询」把控制台与加载完成转成事件。150ms 对调试/工装足够，
// 且不必把引擎的日志管道改成复杂的推送回调（引擎侧改动面最小）。
const eventPumpInterval = 150 * time.Millisecond

// eventPump 把宿主侧的新日志/加载完成转成 CDP 事件，直到连接关闭。
func (c *wsConn) eventPump() {
	ticker := time.NewTicker(eventPumpInterval)
	defer ticker.Stop()
	loadCounts := map[string]int{}
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
		}
		for _, s := range c.sessionList() {
			c.pumpConsole(s)
			c.pumpLoad(s, loadCounts)
		}
	}
}

// pumpConsole 推送控制台增量（Runtime.consoleAPICalled / Log.entryAdded）。
func (c *wsConn) pumpConsole(s *session) {
	if !s.domainEnabled("Runtime") && !s.domainEnabled("Log") {
		return
	}
	entries := c.srv.opt.Adapter.TakeConsoleEntries(s.targetID)
	for _, e := range entries {
		ts := float64(time.Now().UnixMilli())
		if s.domainEnabled("Runtime") {
			c.sendEvent(s.id, "Runtime.consoleAPICalled", map[string]any{
				"type":               e.Level,
				"args":               []map[string]any{{"type": "string", "value": e.Text}},
				"executionContextId": 1,
				"timestamp":          ts,
			})
		}
		if s.domainEnabled("Log") {
			c.sendEvent(s.id, "Log.entryAdded", map[string]any{
				"entry": map[string]any{
					"source": "javascript", "level": e.Level, "text": e.Text, "timestamp": ts,
				},
			})
		}
	}
}

// pumpLoad 推送 Page.loadEventFired（宿主实现 PageLoadWatcher 时）。
func (c *wsConn) pumpLoad(s *session, counts map[string]int) {
	if !s.domainEnabled("Page") {
		return
	}
	w, ok := c.srv.opt.Adapter.(PageLoadWatcher)
	if !ok {
		return
	}
	n := w.PageLoadCount(s.targetID)
	if prev, seen := counts[s.targetID]; seen && n > prev {
		c.sendEvent(s.id, "Page.loadEventFired", map[string]any{
			"timestamp": float64(time.Now().UnixMilli()),
		})
	}
	counts[s.targetID] = n
}
