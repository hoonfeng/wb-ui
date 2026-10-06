package main

// CDP 自检（实现路径主线 B）：用**真实的 HTTP + WebSocket 客户端**连本进程起的
// 调试服务，验证「外部工具能通过统一协议驱动引擎」。这是 S1 场景（自家测试工装
// 用 CDP 替代一次性 API 调用）的样板，也是 docs/implementation-path.md §3.6
// 验收判据 1-4、6、7 的落地。
//
// 客户端手写而不引三方库：和「自研最小 WS 服务端」同一个理由——CDP 客户端只需
// 掩码文本帧 + ping/pong/close 这一小块，手写能把真实帧格式当被测输入。

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png" // PNG 解码（截图比对）
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wb-ui/app"
	"wb-ui/webkit"
)

// devtoolsSelfCheck 是主线 B0/B1 的端到端验收。dt 为 nil（未开端口）时打印跳过。
func devtoolsSelfCheck(dt *app.DevTools, wv *webkit.WebView) {
	fmt.Println("\n=== 无头自检：CDP 调试协议（主线 B0/B1）===")
	if dt == nil || dt.Server == nil {
		fmt.Println("  跳过：未开启 -remote-debugging-port（默认关闭、零开销）")
		return
	}
	base := "http://" + dt.Server.Addr()
	fmt.Printf("  服务地址：%s（只绑本机回环）\n", dt.Server.Addr())

	// ── 判据 7：只监听 loopback ──
	if host, _, err := net.SplitHostPort(dt.Server.Addr()); err != nil || host != "127.0.0.1" {
		fmt.Printf("  ✗ 判据 7：监听地址不是 127.0.0.1（%s）\n", dt.Server.Addr())
	} else {
		fmt.Println("  ✓ 判据 7：仅监听 127.0.0.1（非 0.0.0.0）")
	}

	// ── 判据 1：/json/version + /json/list ──
	version, err := httpGetJSON(base + "/json/version")
	if err != nil {
		fmt.Printf("  ✗ 判据 1：GET /json/version 失败: %v\n", err)
		return
	}
	wsBrowser, _ := version["webSocketDebuggerUrl"].(string)
	if version["Browser"] == nil || !strings.HasPrefix(wsBrowser, "ws://127.0.0.1:") {
		fmt.Printf("  ✗ 判据 1：/json/version 字段不符: %v\n", version)
	} else {
		fmt.Printf("  ✓ 判据 1：/json/version Browser=%v、webSocketDebuggerUrl=%s\n", version["Browser"], wsBrowser)
	}
	listRaw, err := httpGetJSONList(base + "/json/list")
	if err != nil || len(listRaw) == 0 {
		fmt.Printf("  ✗ 判据 1：/json/list 无页面 target（err=%v）\n", err)
		return
	}
	targetID, _ := listRaw[0]["id"].(string)
	pageWS, _ := listRaw[0]["webSocketDebuggerUrl"].(string)
	engineTitle, err := wv.EvalJS("document.title")
	titleWant := ""
	if err == nil {
		titleWant = engineTitle.ToString()
	}
	titleGot, _ := listRaw[0]["title"].(string)
	if titleGot != titleWant {
		fmt.Printf("  ✗ /json/list 的 title 与引擎不一致：CDP=%q 引擎=%q\n", titleGot, titleWant)
	} else {
		fmt.Printf("  ✓ 判据 1：/json/list 含页面 target id=%s title=%q（与引擎一致）\n", targetID, titleGot)
	}

	// ── 建立 WS 连接 ──
	conn, err := dialCDPClient(pageWS)
	if err != nil {
		fmt.Printf("  ✗ WS 连接失败: %v\n", err)
		return
	}
	defer conn.close()
	fmt.Printf("  ✓ WS 已连接 %s\n", pageWS)

	// ── 判据 2：Runtime.evaluate 1+1 ──
	res, err := conn.call("Runtime.evaluate", map[string]any{"expression": "1+1", "returnByValue": true})
	if err != nil {
		fmt.Printf("  ✗ 判据 2：Runtime.evaluate 失败: %v\n", err)
	} else if v := remoteValueOfResult(res); v != float64(2) {
		fmt.Printf("  ✗ 判据 2：1+1 = %v，want 2（原始响应 %v）\n", v, res)
	} else {
		fmt.Println("  ✓ 判据 2：Runtime.evaluate「1+1」→ result.value = 2")
	}

	// ── 判据 3：CDP 求值结果与 wv.EvalJS 一致 ──
	res, err = conn.call("Runtime.evaluate", map[string]any{"expression": "document.title", "returnByValue": true})
	if err != nil {
		fmt.Printf("  ✗ 判据 3：Runtime.evaluate 失败: %v\n", err)
	} else if got, _ := remoteValueOfResult(res).(string); got != titleWant {
		fmt.Printf("  ✗ 判据 3：CDP=%q 引擎 EvalJS=%q\n", got, titleWant)
	} else {
		fmt.Printf("  ✓ 判据 3：Runtime.evaluate「document.title」与 wv.EvalJS 一致（%q）\n", titleWant)
	}

	// ── 判据 6（等价形式）：DOM.getDocument 的节点数与页面侧计数一致 ──
	// 原判据用 __devtools.tree(0) 计数；psai 无 Host（不注入 __devtools），故与
	// document.getElementsByTagName('*').length + 1（含 documentElement 本身）比较。
	res, err = conn.call("Runtime.evaluate", map[string]any{
		"expression": "document.getElementsByTagName('*').length", "returnByValue": true,
	})
	jsCount, _ := remoteValueOfResult(res).(float64)
	res, err = conn.call("DOM.getDocument", map[string]any{"depth": -1})
	if err != nil {
		fmt.Printf("  ✗ 判据 6：DOM.getDocument 失败: %v\n", err)
	} else if root, ok := resultOf(res)["root"].(map[string]any); !ok {
		fmt.Printf("  ✗ 判据 6：DOM.getDocument 无 root（%v）\n", res)
	} else {
		elemCount := countElementNodes(root)
		if jsCount > 0 && float64(elemCount) != jsCount {
			fmt.Printf("  ✗ 判据 6：CDP 元素节点数=%d，页面侧计数=%v\n", elemCount, jsCount)
		} else {
			fmt.Printf("  ✓ 判据 6：DOM.getDocument 元素节点数=%d（与页面计数 %v 一致）\n", elemCount, jsCount)
		}
	}

	// ── 判据 6b：DOM.querySelector 找到的节点可算几何（Elements 面板高亮的基础）──
	res, err = conn.call("DOM.querySelector", map[string]any{"nodeId": 1, "selector": "body"})
	var nodeID float64
	if err == nil {
		nodeID, _ = resultOf(res)["nodeId"].(float64)
	}
	if nodeID == 0 {
		fmt.Printf("  ✗ DOM.querySelector('body') 未命中（%v）\n", res)
	} else if res2, err2 := conn.call("DOM.getBoxModel", map[string]any{"nodeId": nodeID}); err2 != nil {
		fmt.Printf("  ✗ DOM.getBoxModel 失败: %v\n", err2)
	} else if model, ok := resultOf(res2)["model"].(map[string]any); !ok || model["width"] == nil {
		fmt.Printf("  ✗ DOM.getBoxModel 缺 model（%v）\n", res2)
	} else {
		fmt.Printf("  ✓ DOM.querySelector('body') → nodeId=%d，getBoxModel 宽=%v 高=%v\n", int(nodeID), model["width"], model["height"])
	}

	// ── 判据 8（S2）：Runtime 句柄表 —— 对象求值回 objectId，getProperties 与
	//    callFunctionOn(objectId) 能用它回查（DevTools Console 展开对象的基础）──
	res, err = conn.call("Runtime.evaluate", map[string]any{
		"expression": "({ a: 1, b: '两' })", // 缺省 returnByValue=false → 应回 objectId
	})
	// 响应形状：result.result = RemoteObject（objectId 在**内层** result 里）。
	objID := ""
	if ro, ok := resultOf(res)["result"].(map[string]any); ok {
		objID, _ = ro["objectId"].(string)
	}
	switch {
	case err != nil || objID == "":
		fmt.Printf("  ✗ 判据 8：Runtime.evaluate 未回 objectId（err=%v res=%v）\n", err, res)
	default:
		propsRes, perr := conn.call("Runtime.getProperties", map[string]any{"objectId": objID})
		callRes, cerr := conn.call("Runtime.callFunctionOn", map[string]any{
			"objectId":            objID,
			"functionDeclaration": "function(){ return this.b; }",
			"returnByValue":       true,
		})
		_, rerr := conn.call("Runtime.releaseObject", map[string]any{"objectId": objID})
		got, _ := remoteValueOfResult(callRes).(string)
		switch {
		case perr != nil || !hasProperty(propsRes, "a") || !hasProperty(propsRes, "b"):
			fmt.Printf("  ✗ 判据 8：getProperties 缺 a/b（err=%v res=%v）\n", perr, propsRes)
		case cerr != nil || got != "两":
			fmt.Printf("  ✗ 判据 8：callFunctionOn(this.b) = %q，want 「两」（err=%v res=%v）\n", got, cerr, callRes)
		case rerr != nil:
			fmt.Printf("  ✗ 判据 8：releaseObject 失败: %v\n", rerr)
		default:
			fmt.Printf("  ✓ 判据 8：对象求值 → objectId=%s；getProperties 列出 a/b；callFunctionOn(this.b)=「两」；releaseObject 成功\n", objID)
		}
	}

	// ── 判据 9（S2）：DOM 扩展 —— querySelectorAll / getAttributes / getOuterHTML /
	//    resolveNode（Elements 面板的数据来源）──
	res, err = conn.call("DOM.querySelectorAll", map[string]any{"nodeId": 1, "selector": "div"})
	ids := float64Slice(resultOf(res)["nodeIds"])
	if err != nil || len(ids) == 0 {
		fmt.Printf("  ✗ 判据 9：DOM.querySelectorAll('div') 无结果（err=%v res=%v）\n", err, res)
	} else {
		probeID := int(ids[0])
		attrRes, aerr := conn.call("DOM.getAttributes", map[string]any{"nodeId": probeID})
		htmlRes, herr := conn.call("DOM.getOuterHTML", map[string]any{"nodeId": probeID})
		nodeRes, nerr := conn.call("DOM.resolveNode", map[string]any{"nodeId": probeID})
		html, _ := resultOf(htmlRes)["outerHTML"].(string)
		attrs, _ := resultOf(attrRes)["attributes"].([]any)
		obj, _ := resultOf(nodeRes)["object"].(map[string]any)
		switch {
		case aerr != nil || herr != nil || nerr != nil:
			fmt.Printf("  ✗ 判据 9：getAttributes/getOuterHTML/resolveNode 失败（%v / %v / %v）\n", aerr, herr, nerr)
		case !strings.HasPrefix(html, "<div"):
			fmt.Printf("  ✗ 判据 9：outerHTML = %q（应以 <div 开头）\n", html)
		case obj == nil || obj["objectId"] == nil:
			fmt.Printf("  ✗ 判据 9：resolveNode 未回 objectId（%v）\n", nodeRes)
		default:
			fmt.Printf("  ✓ 判据 9：querySelectorAll('div') → %d 个节点（首节点 nodeId=%d）；getAttributes=%d 项；outerHTML=%d 字节；resolveNode → objectId\n",
				len(ids), probeID, len(attrs), len(html))
		}
	}

	// ── 判据 10（S2）：DOM 编辑 —— setAttributeValue / removeAttribute 落到页面 ──
	res, err = conn.call("DOM.querySelector", map[string]any{"nodeId": 1, "selector": "body"})
	bodyID := 0
	if err == nil {
		if f, ok := resultOf(res)["nodeId"].(float64); ok {
			bodyID = int(f)
		}
	}
	if bodyID == 0 {
		fmt.Printf("  ✗ 判据 10：找不到 body（%v）\n", res)
	} else if _, err = conn.call("DOM.setAttributeValue", map[string]any{
		"nodeId": bodyID, "name": "data-cdp-probe", "value": "42",
	}); err != nil {
		fmt.Printf("  ✗ 判据 10：DOM.setAttributeValue 失败: %v\n", err)
	} else if gotRes, gerr := conn.call("Runtime.evaluate", map[string]any{
		"expression":    `document.body.getAttribute("data-cdp-probe")`,
		"returnByValue": true,
	}); gerr != nil {
		fmt.Printf("  ✗ 判据 10：页面复核失败: %v\n", gerr)
	} else if got, _ := remoteValueOfResult(gotRes).(string); got != "42" {
		fmt.Printf("  ✗ 判据 10：页面读到 %q，want 42（%v）\n", got, gotRes)
	} else if _, err = conn.call("DOM.removeAttribute", map[string]any{"nodeId": bodyID, "name": "data-cdp-probe"}); err != nil {
		fmt.Printf("  ✗ 判据 10：DOM.removeAttribute 失败: %v\n", err)
	} else if afterRes, aerr := conn.call("Runtime.evaluate", map[string]any{
		"expression":    `document.body.getAttribute("data-cdp-probe")`,
		"returnByValue": true,
	}); aerr != nil {
		fmt.Printf("  ✗ 判据 10：删除后复核失败: %v\n", aerr)
	} else if got, _ := remoteValueOfResult(afterRes).(string); got == "42" {
		fmt.Printf("  ✗ 判据 10：删除后仍读到 42\n")
	} else {
		// 注：引擎的 getAttribute 在属性缺失时回空串（浏览器回 null，见 TECH_DEBT），
		// 所以这里只断言「不再等于 42」。
		fmt.Printf("  ✓ 判据 10：DOM.setAttributeValue → 页面读到 42；DOM.removeAttribute → 属性不再存在（nodeId=%d）\n", bodyID)
	}

	// ── 判据 11（S2）：CSS 域 —— computed / inline / matched styles ──
	res, err = conn.call("Runtime.evaluate", map[string]any{
		"expression": `(function(){
			var d = document.createElement("div");
			d.id = "cdp-css-probe";
			d.setAttribute("style", "color: rgb(1, 2, 3); margin-top: 7px");
			document.body.appendChild(d);
			var st = document.createElement("style");
			st.textContent = "#cdp-css-probe { padding-left: 9px }";
			document.head.appendChild(st);
			return 1;
		})()`,
		"returnByValue": true,
	})
	res, err = conn.call("DOM.querySelector", map[string]any{"nodeId": 1, "selector": "#cdp-css-probe"})
	probeNode := 0
	if err == nil {
		if f, ok := resultOf(res)["nodeId"].(float64); ok {
			probeNode = int(f)
		}
	}
	if probeNode == 0 {
		fmt.Printf("  ✗ 判据 11：探针元素未建立（%v）\n", res)
	} else {
		csRes, cserr := conn.call("CSS.getComputedStyleForNode", map[string]any{"nodeId": probeNode})
		inRes, inerr := conn.call("CSS.getInlineStylesForNode", map[string]any{"nodeId": probeNode})
		mtRes, mterr := conn.call("CSS.getMatchedStylesForNode", map[string]any{"nodeId": probeNode})
		cs, _ := resultOf(csRes)["computedStyle"].([]any)
		inline, _ := resultOf(inRes)["inlineStyle"].(map[string]any)
		matched, _ := resultOf(mtRes)["matchedCSSRules"].([]any)
		switch {
		case cserr != nil || inerr != nil || mterr != nil:
			fmt.Printf("  ✗ 判据 11：CSS 域调用失败（%v / %v / %v）\n", cserr, inerr, mterr)
		case len(cs) == 0:
			fmt.Printf("  ✗ 判据 11：computedStyle 为空（%v）\n", csRes)
		case inline == nil:
			fmt.Printf("  ✗ 判据 11：inlineStyle 为 null（该元素有 style 属性，%v）\n", inRes)
		case len(matched) == 0:
			fmt.Printf("  ✗ 判据 11：matchedCSSRules 为空（注入的 #cdp-css-probe 规则没匹配上，%v）\n", mtRes)
		default:
			fmt.Printf("  ✓ 判据 11：computedStyle %d 项、inlineStyle %v 项、matchedCSSRules %d 条（含注入规则）\n",
				len(cs), len(inline["cssProperties"].([]any)), len(matched))
		}
	}

	// ── 判据 12（S2）：Log 分级 —— console.error/warn 以 error/warn 推给客户端 ──
	if _, err = conn.call("Log.enable", nil); err != nil {
		fmt.Printf("  ✗ 判据 12：Log.enable 失败: %v\n", err)
	} else if _, err = conn.call("Runtime.enable", nil); err != nil {
		fmt.Printf("  ✗ 判据 12：Runtime.enable 失败: %v\n", err)
	} else {
		// ① 清掉 enable 后的**历史存量**：Log/Runtime 是增量通道，enable 的第一批
		//    是页面启动期就产生的日志——断言新探针之前必须先推平游标。
		conn.drainEvents(400 * time.Millisecond)
		// ② 注入探针日志
		_, _ = conn.call("Runtime.evaluate", map[string]any{
			"expression":    `console.error("cdp-error-probe"); console.warn("cdp-warn-probe");`,
			"returnByValue": true,
		})
		// ③ 按**探针文本**等到属于它的事件：存量里本来就有 error/warn，只看「首条」
		//    会把旧日志当成本次探针的结果（首版判据就栽在这里）。
		errEvt := conn.waitEventWhere("Log.entryAdded",
			func(e map[string]any) bool { return entryText(e) == "cdp-error-probe" }, 4*time.Second)
		warnEvt := conn.waitEventWhere("Log.entryAdded",
			func(e map[string]any) bool { return entryText(e) == "cdp-warn-probe" }, 2*time.Second)
		switch {
		case errEvt == nil:
			fmt.Printf("  ✗ 判据 12：4s 内没收到 console.error 对应的 Log.entryAdded（事件泵未推或文本不匹配）\n")
		case entryLevel(errEvt) != "error":
			fmt.Printf("  ✗ 判据 12：console.error 的 entry.level = %q，want error（分级没传到客户端）\n", entryLevel(errEvt))
		case warnEvt == nil:
			fmt.Printf("  ✗ 判据 12：没收到 console.warn 对应的 Log.entryAdded\n")
		case entryLevel(warnEvt) != "warn":
			fmt.Printf("  ✗ 判据 12：console.warn 的 entry.level = %q，want warn\n", entryLevel(warnEvt))
		default:
			fmt.Printf("  ✓ 判据 12：console.error/warn → Log.entryAdded level=error/warn（分级保真，存量已推平）\n")
		}
	}

	// ── 判据 4：Page.captureScreenshot 与 wv.Render() 逐像素一致 ──
	res, err = conn.call("Page.captureScreenshot", nil)
	if err != nil {
		fmt.Printf("  ✗ 判据 4：Page.captureScreenshot 失败: %v\n", err)
	} else if data, _ := resultOf(res)["data"].(string); data == "" {
		fmt.Printf("  ✗ 判据 4：截图无 data（%v）\n", res)
	} else if raw, derr := base64.StdEncoding.DecodeString(data); derr != nil {
		fmt.Printf("  ✗ 判据 4：base64 解码失败: %v\n", derr)
	} else {
		diff, total, cerr := compareScreenshotWithRender(raw, wv)
		if cerr != nil {
			fmt.Printf("  ✗ 判据 4：%v\n", cerr)
		} else if diff != 0 {
			fmt.Printf("  ✗ 判据 4：截图与 Render() 有 %d/%d 个不透明像素不一致\n", diff, total)
		} else {
			fmt.Printf("  ✓ 判据 4：截图与 wv.Render() 逐像素一致（%d 个不透明像素全等）\n", total)
		}
	}

	// ── 附加：Target 会话（flatten）与未知方法错误码 ──
	if res, err := conn.call("Target.getTargets", nil); err == nil {
		if infos, ok := resultOf(res)["targetInfos"].([]any); ok && len(infos) > 0 {
			fmt.Printf("  ✓ Target.getTargets：%d 个 target\n", len(infos))
		}
	}

	// ── 判据 5（等价形式）：Input.dispatchMouseEvent 经引擎交互管线落到页面 ──
	// 原判据用 __devtools.pick(x,y) 比对命中共对象；psai 无 Host（不注入
	// __devtools），改为「点元素 → 该元素 click 计数递增」——走的是同一条命中
	// 测试 + 事件派发路径（webkit.HandleMouseButton → Interaction）。
	setup := `(function(){
	  var el = document.querySelector('.d-main_toolstrip > *');
	  if (!el) return '';
	  window.__cdpClicks = 0;
	  el.addEventListener('click', function(){ window.__cdpClicks++; });
	  var r = el.getBoundingClientRect();
	  return JSON.stringify({ x: r.left + r.width / 2, y: r.top + r.height / 2 });
	})()`
	res, err = conn.call("Runtime.evaluate", map[string]any{"expression": setup, "returnByValue": true})
	ptRaw, _ := remoteValueOfResult(res).(string)
	if strings.TrimSpace(ptRaw) == "" {
		fmt.Printf("  ⚠ 判据 5：页面里没有 .d-main_toolstrip > * 可点（跳过）\n")
	} else {
		var pt struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		}
		if jerr := json.Unmarshal([]byte(ptRaw), &pt); jerr != nil {
			fmt.Printf("  ✗ 判据 5：坐标解析失败: %v（%s）\n", jerr, ptRaw)
		} else {
			_, _ = conn.call("Input.dispatchMouseEvent", map[string]any{
				"type": "mousePressed", "x": pt.X, "y": pt.Y, "button": "left", "clickCount": 1,
			})
			_, _ = conn.call("Input.dispatchMouseEvent", map[string]any{
				"type": "mouseReleased", "x": pt.X, "y": pt.Y, "button": "left", "clickCount": 1,
			})
			res, _ = conn.call("Runtime.evaluate", map[string]any{
				"expression": "window.__cdpClicks || 0", "returnByValue": true,
			})
			if n, _ := remoteValueOfResult(res).(float64); n >= 1 {
				fmt.Printf("  ✓ 判据 5：CDP 在 (%.0f,%.0f) 按下+释放 → 目标元素 click 计数 = %d\n", pt.X, pt.Y, int(n))
			} else {
				fmt.Printf("  ✗ 判据 5：点击未落到页面（click 计数 = %v）\n", remoteValueOfResult(res))
			}
		}
	}

	// ── 附加：Input.dispatchKeyEvent 走引擎编辑管线（仅当页面里有文本控件）──
	kindExpr := `(function(){
	  var el = document.querySelector('.d-ai-prompt');
	  return el ? el.tagName.toLowerCase() : '';
	})()`
	res, _ = conn.call("Runtime.evaluate", map[string]any{"expression": kindExpr, "returnByValue": true})
	kind, _ := remoteValueOfResult(res).(string)
	if kind != "input" && kind != "textarea" {
		fmt.Printf("  ⚠ 键盘：页面里没有可用文本控件（.d-ai-prompt 是 %q），跳过\n", kind)
	} else {
		// 点输入框聚焦 → 清空 → 逐字符发 keyDown（带 text），读回 value。
		focusScript := `(function(){
		  var el = document.querySelector('.d-ai-prompt');
		  el.value = '';
		  var r = el.getBoundingClientRect();
		  return JSON.stringify({ x: r.left + r.width / 2, y: r.top + r.height / 2 });
		})()`
		res, _ = conn.call("Runtime.evaluate", map[string]any{"expression": focusScript, "returnByValue": true})
		raw, _ := remoteValueOfResult(res).(string)
		var pt struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		}
		if jerr := json.Unmarshal([]byte(raw), &pt); jerr != nil {
			fmt.Printf("  ⚠ 键盘：焦点坐标解析失败（%v），跳过\n", jerr)
		} else {
			_, _ = conn.call("Input.dispatchMouseEvent", map[string]any{
				"type": "mousePressed", "x": pt.X, "y": pt.Y, "button": "left", "clickCount": 1,
			})
			_, _ = conn.call("Input.dispatchMouseEvent", map[string]any{
				"type": "mouseReleased", "x": pt.X, "y": pt.Y, "button": "left", "clickCount": 1,
			})
			for _, r := range "cdp" {
				_, _ = conn.call("Input.dispatchKeyEvent", map[string]any{
					"type": "keyDown", "key": string(r), "text": string(r),
				})
			}
			res, _ = conn.call("Runtime.evaluate", map[string]any{
				"expression": `(document.querySelector('.d-ai-prompt')||{}).value || ''`, "returnByValue": true,
			})
			got, _ := remoteValueOfResult(res).(string)
			if strings.HasPrefix(got, "cdp") {
				fmt.Printf("  ✓ 键盘：dispatchKeyEvent 输入 3 个字符 → 输入框 value = %q\n", got)
			} else {
				fmt.Printf("  ✗ 键盘：输入框 value = %q（want 前缀 \"cdp\"）\n", got)
			}
		}
	}

	if res, err := conn.call("Network.enable", nil); err == nil {
		if e, ok := res["error"].(map[string]any); ok {
			fmt.Printf("  ✓ 未实现方法回错误码 %v（%v）\n", e["code"], e["message"])
		}
	}
}

// devtoolsS3SelfCheck 覆盖 S3：真实客户端（Puppeteer/Playwright）连的是 **browser-level**
// 端点，靠 Target.setAutoAttach 拿 sessionId，再用**扁平会话**驱动页面、导航与设备仿真。
// 前面的判据 1-12 走的是 /devtools/page/<id> 直连（隐含会话），覆盖不到这条路。
//
//	判据 13：Target 扁平会话 —— getBrowserContexts / setDiscoverTargets+targetCreated /
//	         setAutoAttach+attachedToTarget / 会话内命令路由与响应回带 sessionId /
//	         重复附加不重复推 / 非 flatten 与未知 targetId 报错 / detach 后会话失效。
//	判据 14：Page.navigate —— loaderId + frameNavigated + 真换文档 + 两次导航 loaderId
//	         不同 + 空 URL 报错。
//	判据 15：Emulation 含 deviceScaleFactor —— devicePixelRatio / resolution 媒体查询
//	         命中并重算样式 / 截图按 DSF 放大 / width<=0 报错 / clear 复位。
//
// ★ 放在所有其它判据**之后**调用（见 main.go）：判据 14 会把页面导航走（探针页），
// 末尾再导航回原文档——自检插桩的规矩是不改变其它判据的前置状态。
func devtoolsS3SelfCheck(dt *app.DevTools, wv *webkit.WebView) {
	fmt.Println("\n=== 无头自检：CDP S3（浏览器级连接 + 扁平会话 + 导航 + 设备仿真）===")
	if dt == nil || dt.Server == nil {
		fmt.Println("  跳过：未开启 -remote-debugging-port（默认关闭、零开销）")
		return
	}
	base := "http://" + dt.Server.Addr()
	version, err := httpGetJSON(base + "/json/version")
	if err != nil {
		fmt.Printf("  ✗ 判据 13：GET /json/version 失败: %v\n", err)
		return
	}
	browserWS, _ := version["webSocketDebuggerUrl"].(string)
	listRaw, err := httpGetJSONList(base + "/json/list")
	if err != nil || len(listRaw) == 0 {
		fmt.Printf("  ✗ 判据 13：/json/list 无页面 target（err=%v）\n", err)
		return
	}
	targetID, _ := listRaw[0]["id"].(string)
	bconn, err := dialCDPClient(browserWS)
	if err != nil {
		fmt.Printf("  ✗ 判据 13：连 browser-level WS 失败: %v\n", err)
		return
	}
	defer bconn.close()

	// 原文档信息（收尾要导航回来；判据 13-d 也要比对引擎与 CDP 读到的一致）。
	origURL := strings.TrimSpace(wv.DocumentBaseURL())
	titleWant := ""
	if v, terr := wv.EvalJS("document.title"); terr == nil {
		titleWant = v.ToString()
	}

	// ── 判据 13-a：单上下文宿主 → 额外上下文列表为空 ──
	res, err := bconn.call("Target.getBrowserContexts", nil)
	if err != nil {
		fmt.Printf("  ✗ 判据 13-a：Target.getBrowserContexts 失败: %v\n", err)
		return
	}
	if ctxs, ok := resultOf(res)["browserContextIds"].([]any); !ok || len(ctxs) != 0 {
		fmt.Printf("  ✗ 判据 13-a：browserContextIds = %v，want 空数组（默认上下文不在列表里）\n",
			resultOf(res)["browserContextIds"])
	} else {
		fmt.Println("  ✓ 判据 13-a：Target.getBrowserContexts → []（单上下文宿主）")
	}

	// ── 判据 13-b：setDiscoverTargets → 当前已知目标推 targetCreated ──
	if _, err := bconn.call("Target.setDiscoverTargets", map[string]any{"discover": true}); err != nil {
		fmt.Printf("  ✗ 判据 13-b：Target.setDiscoverTargets 失败: %v\n", err)
		return
	}
	created := bconn.waitEventWhere("Target.targetCreated", func(e map[string]any) bool {
		return eventTargetID(e) == targetID
	}, 3*time.Second)
	if created == nil {
		fmt.Println("  ✗ 判据 13-b：3s 内没收到 Target.targetCreated（discover 未推已有目标）")
	} else {
		fmt.Printf("  ✓ 判据 13-b：setDiscoverTargets → Target.targetCreated（targetId=%s）\n",
			eventTargetID(created))
	}

	// ── 判据 13-c：setAutoAttach(flatten) → attachedToTarget + sessionId ──
	autoRes, err := bconn.call("Target.setAutoAttach", map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true})
	if err != nil || autoRes["error"] != nil {
		fmt.Printf("  ✗ 判据 13-c：Target.setAutoAttach 失败: %v %v\n", err, autoRes["error"])
		return
	}
	attached := bconn.waitEventWhere("Target.attachedToTarget", func(e map[string]any) bool {
		return eventTargetID(e) == targetID
	}, 3*time.Second)
	if attached == nil {
		fmt.Println("  ✗ 判据 13-c：3s 内没收到 Target.attachedToTarget")
		return
	}
	sessID, _ := eventParams(attached)["sessionId"].(string)
	if sessID == "" {
		fmt.Println("  ✗ 判据 13-c：attachedToTarget 没带 sessionId")
		return
	}
	fmt.Printf("  ✓ 判据 13-c：setAutoAttach(flatten) → Target.attachedToTarget（sessionId=%s）\n", sessID)

	// ── 判据 13-d：会话内命令路由（响应回带同一 sessionId、值来自页面）──
	res, err = bconn.callSession("Runtime.evaluate", map[string]any{
		"expression": "document.title", "returnByValue": true}, sessID)
	if err != nil {
		fmt.Printf("  ✗ 判据 13-d：会话内 Runtime.evaluate 失败: %v\n", err)
		return
	}
	gotTitle, _ := remoteValueOfResult(res).(string)
	echoSess, _ := res["sessionId"].(string)
	switch {
	case echoSess != sessID:
		fmt.Printf("  ✗ 判据 13-d：响应没回带 sessionId（got %q want %q）\n", echoSess, sessID)
	case gotTitle != titleWant:
		fmt.Printf("  ✗ 判据 13-d：会话内求值 title=%q，引擎 EvalJS=%q（会话没落到页面）\n", gotTitle, titleWant)
	default:
		fmt.Printf("  ✓ 判据 13-d：带 sessionId 的 Runtime.evaluate → title=%q（响应回带同一 sessionId）\n", gotTitle)
	}

	// ── 判据 13-e：重复 setAutoAttach 不重复推事件（会话复用）──
	_, _ = bconn.call("Target.setAutoAttach", map[string]any{
		"autoAttach": true, "waitForDebuggerOnStart": false, "flatten": true})
	if dup := bconn.waitEventWhere("Target.attachedToTarget", func(e map[string]any) bool {
		return eventTargetID(e) == targetID
	}, 700*time.Millisecond); dup != nil {
		fmt.Printf("  ✗ 判据 13-e：重复 setAutoAttach 又推了一条 attachedToTarget（sessionId=%s）\n",
			eventParams(dup)["sessionId"])
	} else {
		fmt.Println("  ✓ 判据 13-e：重复 setAutoAttach 不重复推 attachedToTarget（复用已有会话）")
	}

	// ── 判据 13-f：协议边界报错（非 flatten / 未知 targetId）──
	bad1, _ := bconn.call("Target.setAutoAttach", map[string]any{"autoAttach": true, "flatten": false})
	bad2, _ := bconn.call("Target.attachToTarget", map[string]any{"targetId": "no-such-target", "flatten": true})
	if bad1["error"] == nil || bad2["error"] == nil {
		fmt.Printf("  ✗ 判据 13-f：非 flatten / 未知 targetId 应各回错误（got %v / %v）\n",
			bad1["error"], bad2["error"])
	} else {
		fmt.Printf("  ✓ 判据 13-f：非 flatten 报错（code=%v）、未知 targetId 报错（code=%v）——不静默降级\n",
			errCode(bad1), errCode(bad2))
	}

	// ── 判据 14：Page.navigate ──
	// 本宿主是 ModeToolkit（沙箱：不走网络/文件系统，内容由拦截器提供）——引擎在该
	// 模式下**不允许 LoadURL**（webkit/mode.go 的 allowsNavigation）。所以导航分两处
	// 验证，两处都是端到端（真引擎 + 真 CDP + 真 WS 客户端）：
	//   14-a..14-c（本宿主）：参数校验先于模式检查；模式不支持时如实回 -32000 且消息
	//        指明原因（不静默成功、不假装导航过）；被拒后页面保持原状。
	//   14-1..14-4（s3BrowserNavigateCheck）：另起一个 ModeBrowser 宿主（默认模式，
	//        允许导航）+ 独立调试服务，验证 loaderId / frameNavigated / 真换文档 /
	//        两次导航 loaderId 递增。
	if _, err := bconn.callSession("Page.enable", nil, sessID); err != nil {
		fmt.Printf("  ✗ 判据 14：Page.enable 失败: %v\n", err)
		return
	}
	emptyRes, _ := bconn.callSession("Page.navigate", map[string]any{"url": "   "}, sessID)
	if e, ok := emptyRes["error"].(map[string]any); !ok || e["code"] != float64(-32602) {
		fmt.Printf("  ✗ 判据 14-a：空 URL 应回 -32602（got %v）\n", emptyRes["error"])
	} else {
		fmt.Println("  ✓ 判据 14-a：Page.navigate 空 URL → -32602（参数校验先于模式检查）")
	}
	toolkitTarget := origURL
	if toolkitTarget == "" {
		toolkitTarget = "file:///s3-toolkit-nav-probe.html"
	}
	navRes, _ := bconn.callSession("Page.navigate", map[string]any{"url": toolkitTarget}, sessID)
	if e, ok := navRes["error"].(map[string]any); !ok || e["code"] != float64(-32000) {
		fmt.Printf("  ✗ 判据 14-b：Toolkit 宿主下 navigate 应回 -32000（got %v）\n", navRes)
	} else if msg := fmt.Sprint(e["message"]); !strings.Contains(msg, "not supported in this mode") {
		fmt.Printf("  ✗ 判据 14-b：错误消息没说清模式原因：%q\n", msg)
	} else {
		fmt.Println("  ✓ 判据 14-b：Toolkit 宿主下 Page.navigate → -32000 且消息指明模式限制（如实上报）")
	}
	if nowTitle, _ := s3EvalString(bconn, sessID, "document.title"); nowTitle != titleWant {
		fmt.Printf("  ✗ 判据 14-c：导航被拒后页面被破坏（title=%q，原 %q）\n", nowTitle, titleWant)
	} else {
		fmt.Printf("  ✓ 判据 14-c：导航被拒后页面保持原状（title=%q）\n", nowTitle)
	}
	s3BrowserNavigateCheck()

	// ── 判据 15：Emulation.setDeviceMetricsOverride（含 deviceScaleFactor）──
	// 探针样式与元素用 Runtime.evaluate 注入（本宿主不能导航，但 CDP 能在页面里建
	// 节点）——这样 DSF 判据在**当前文档**上就能端到端验证「DSF 变化 → resolution
	// 媒体查询命中 → 样式重算」，无需换文档。判据 15 收尾时把探针移除。
	probeStyle := "#s3-dsf-probe{color:rgb(1,2,3);width:40px;height:20px}" +
		"@media (min-resolution: 2dppx){#s3-dsf-probe{color:rgb(4,5,6)}}"
	inject := fmt.Sprintf(`(function(){
	  if (!document.getElementById("s3-dsf-probe")) {
	    var st = document.createElement("style");
	    st.id = "s3-dsf-style";
	    st.textContent = %s;
	    document.head.appendChild(st);
	    var d = document.createElement("div");
	    d.id = "s3-dsf-probe";
	    d.textContent = "dsf";
	    document.body.appendChild(d);
	  }
	  return "ok";
	})()`, jsQuote(probeStyle))
	if got, ierr := s3EvalString(bconn, sessID, inject); ierr != nil || got != "ok" {
		fmt.Printf("  ✗ 判据 15：注入 DSF 探针失败（%v / %q）\n", ierr, got)
		return
	}
	mRes, err := bconn.callSession("Emulation.setDeviceMetricsOverride", map[string]any{
		"width": 400, "height": 300, "deviceScaleFactor": 2, "mobile": false}, sessID)
	if err != nil || mRes["error"] != nil {
		fmt.Printf("  ✗ 判据 15：setDeviceMetricsOverride 失败: %v %v\n", err, mRes["error"])
		return
	}
	gotMetrics, _ := s3EvalString(bconn, sessID, `(function(){
	  var cs = getComputedStyle(document.getElementById("s3-dsf-probe"));
	  return [window.devicePixelRatio, window.innerWidth, window.innerHeight, cs.color,
	          matchMedia("(min-resolution: 2dppx)").matches,
	          matchMedia("(min-resolution: 96dpi)").matches,
	          matchMedia("(min-resolution: 3dppx)").matches].join("|");
	})()`)
	// 末两位是阈值方向：DSF=2 = 2dppx = 192dpi → 2dppx/96dpi 命中（>= 阈值），
	// 3dppx 不命中。写反了就会把「阈值比较反了」的 bug 放过去。
	wantMetrics := "2|400|300|rgb(4, 5, 6)|true|true|false"
	if gotMetrics != wantMetrics {
		fmt.Printf("  ✗ 判据 15-a：DSF=2 实测 %q，want %q\n", gotMetrics, wantMetrics)
	} else {
		fmt.Println("  ✓ 判据 15-a：DSF=2 → devicePixelRatio=2、视口 400×300（CSS px）、" +
			"2dppx 媒体查询命中并重算样式（color 1,2,3 → 4,5,6）")
	}
	shot, err := bconn.callSession("Page.captureScreenshot", map[string]any{"format": "png"}, sessID)
	switch {
	case err != nil:
		fmt.Printf("  ✗ 判据 15-b：captureScreenshot 失败: %v\n", err)
	default:
		data, _ := resultOf(shot)["data"].(string)
		raw, derr := base64.StdEncoding.DecodeString(data)
		cfg, _, ierr := image.DecodeConfig(bytes.NewReader(raw))
		switch {
		case data == "":
			fmt.Printf("  ✗ 判据 15-b：截图无 data（%v）\n", shot)
		case derr != nil:
			fmt.Printf("  ✗ 判据 15-b：base64 解码失败: %v\n", derr)
		case ierr != nil:
			fmt.Printf("  ✗ 判据 15-b：PNG 解码失败: %v\n", ierr)
		case cfg.Width != 800 || cfg.Height != 600:
			fmt.Printf("  ✗ 判据 15-b：截图 %dx%d，want 800x600（视口 400×300 × DSF 2）\n", cfg.Width, cfg.Height)
		default:
			fmt.Printf("  ✓ 判据 15-b：Page.captureScreenshot → PNG %dx%d（CSS 400×300 × DSF 2）\n",
				cfg.Width, cfg.Height)
		}
	}
	badRes, _ := bconn.callSession("Emulation.setDeviceMetricsOverride", map[string]any{
		"width": 0, "height": 300, "deviceScaleFactor": 1}, sessID)
	if e, ok := badRes["error"].(map[string]any); !ok || e["code"] != float64(-32602) {
		fmt.Printf("  ✗ 判据 15-c：width=0 应回 -32602（got %v）\n", badRes["error"])
	} else {
		fmt.Println("  ✓ 判据 15-c：width=0 → -32602（非法尺寸不静默吞掉）")
	}
	cRes, cerr := bconn.callSession("Emulation.clearDeviceMetricsOverride", nil, sessID)
	if cerr != nil || cRes["error"] != nil {
		fmt.Printf("  ✗ 判据 15-d：clearDeviceMetricsOverride 失败: %v %v\n", cerr, cRes["error"])
	} else {
		after, _ := s3EvalString(bconn, sessID, `(function(){
		  return [window.devicePixelRatio, window.innerWidth,
		          matchMedia("(min-resolution: 2dppx)").matches].join("|");
		})()`)
		wantAfter := fmt.Sprintf("1|%d|false", wv.Width())
		if after != wantAfter {
			fmt.Printf("  ✗ 判据 15-d：clear 后 %q，want %q\n", after, wantAfter)
		} else {
			fmt.Printf("  ✓ 判据 15-d：clearDeviceMetricsOverride → %s（DSF 回 1、视口回到覆盖前）\n", after)
		}
	}
	// 移除注入的探针（必须在判据 13-g detach 会话之前做）。
	cleanup := `(function(){
	  var s = document.getElementById("s3-dsf-style"); if (s && s.parentNode) { s.parentNode.removeChild(s); }
	  var d = document.getElementById("s3-dsf-probe"); if (d && d.parentNode) { d.parentNode.removeChild(d); }
	  return "ok";
	})()`
	if got, uerr := s3EvalString(bconn, sessID, cleanup); uerr != nil || got != "ok" {
		fmt.Printf("  ⚠ 判据 15 收尾：移除注入探针失败（%v / %q）\n", uerr, got)
	}

	// ── 判据 13-g：detach 后会话失效 ──
	dRes, derr := bconn.call("Target.detachFromTarget", map[string]any{"sessionId": sessID})
	if derr != nil || dRes["error"] != nil {
		fmt.Printf("  ✗ 判据 13-g：detachFromTarget 失败: %v %v\n", derr, dRes["error"])
	} else if gone, gerr := bconn.callSession("Runtime.evaluate", map[string]any{
		"expression": "1", "returnByValue": true}, sessID); gerr == nil && gone["error"] == nil {
		fmt.Println("  ✗ 判据 13-g：detach 后用旧 sessionId 仍能调用（会话应失效）")
	} else {
		fmt.Println("  ✓ 判据 13-g：detach 后旧 sessionId 回错（会话不再可达）")
	}

	// ── 收尾：sessID 已 detach，这里顺带验证「同一连接上会话内与会话外调用并存」，
	// 并复核页面仍处于自检前的状态（注入探针已移除、导航被拒不改变文档）。
	lastTitle, _ := bconn.call("Runtime.evaluate", map[string]any{
		"expression": "document.title", "returnByValue": true})
	gotLast, _ := remoteValueOfResult(lastTitle).(string)
	if gotLast != titleWant {
		fmt.Printf("  ✗ 收尾：页面 title=%q，原 %q（探针未清理干净）\n", gotLast, titleWant)
	} else {
		fmt.Printf("  ✓ 收尾：会话外调用仍可用，页面状态未变（title=%q）\n", gotLast)
	}
}

// s3BrowserNavigateCheck 是判据 14 的**正向**端到端。主自检宿主是 ModeToolkit
// （沙箱：不走网络/文件系统、不允许 LoadURL），所以这里另起一个 ModeBrowser 宿主
// （webkit.NewWebView 的默认模式，完整浏览器语义）+ 独立调试服务，用真实 WS 客户端
// 驱动 Page.navigate：
//
//	14-1：宿主 LoadURL 载入初始文档（真的走文件系统）
//	14-2：Page.navigate 响应给 frameId + loaderId
//	14-3：Page.frameNavigated 的 url/loaderId 与响应一致
//	14-4：导航后文档**真的换了**（title 由 A 变 B）
//	14-5：第二次导航给新 loaderId（客户端据此区分两次加载）
//
// 用完立即关停服务、销毁 WebView、删临时目录——不给自检留残留状态。
func s3BrowserNavigateCheck() {
	fmt.Println("  ── 判据 14（正向）：ModeBrowser 宿主上的真实导航（端到端）──")
	dir, err := os.MkdirTemp("", "s3nav")
	if err != nil {
		fmt.Printf("  ✗ 判据 14-1：建临时目录失败: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)
	urlA, urlB, werr := writeNavProbes(dir)
	if werr != nil {
		fmt.Printf("  ✗ 判据 14-1：写临时页面失败: %v\n", werr)
		return
	}
	navWV := webkit.NewWebView()
	defer navWV.Destroy()
	navWV.Resize(400, 300)
	if err := navWV.LoadURL(urlA); err != nil {
		fmt.Printf("  ✗ 判据 14-1：ModeBrowser 宿主 LoadURL 失败: %v\n", err)
		return
	}
	port, perr := freePort()
	if perr != nil {
		fmt.Printf("  ✗ 判据 14-1：取空闲端口失败: %v\n", perr)
		return
	}
	navDT, derr := app.StartDevTools(navWV, port)
	if derr != nil || navDT == nil {
		fmt.Printf("  ✗ 判据 14-1：起独立调试服务失败: %v\n", derr)
		return
	}
	defer navDT.Close()
	list, lerr := httpGetJSONList(fmt.Sprintf("http://127.0.0.1:%d/json/list", port))
	if lerr != nil || len(list) == 0 {
		fmt.Printf("  ✗ 判据 14-1：/json/list 无页面 target（err=%v）\n", lerr)
		return
	}
	pageWS, _ := list[0]["webSocketDebuggerUrl"].(string)
	targetID, _ := list[0]["id"].(string)
	navConn, cerr := dialCDPClient(pageWS)
	if cerr != nil {
		fmt.Printf("  ✗ 判据 14-1：连 WS 失败: %v\n", cerr)
		return
	}
	defer navConn.close()
	initTitle, _ := s3EvalString(navConn, "", "document.title")
	fmt.Printf("  ✓ 判据 14-1：ModeBrowser 宿主就绪（独立 CDP 端口 %d，targetId=%s，初始 title=%q）\n",
		port, targetID, initTitle)

	if _, err := navConn.call("Page.enable", nil); err != nil {
		fmt.Printf("  ✗ 判据 14-2：Page.enable 失败: %v\n", err)
		return
	}
	nav1, err := navConn.call("Page.navigate", map[string]any{"url": urlB})
	if err != nil || nav1["error"] != nil {
		fmt.Printf("  ✗ 判据 14-2：Page.navigate 失败: %v %v\n", err, nav1["error"])
		return
	}
	loader1, _ := resultOf(nav1)["loaderId"].(string)
	frameID, _ := resultOf(nav1)["frameId"].(string)
	if loader1 == "" || frameID != targetID {
		fmt.Printf("  ✗ 判据 14-2：响应不全（frameId=%q loaderId=%q；want frameId=%q 且 loaderId 非空）\n",
			frameID, loader1, targetID)
	} else {
		fmt.Printf("  ✓ 判据 14-2：Page.navigate → frameId=%s、loaderId=%s\n", frameID, loader1)
	}
	navEvt := navConn.waitEventWhere("Page.frameNavigated", func(e map[string]any) bool {
		f, _ := eventParams(e)["frame"].(map[string]any)
		return f["url"] == urlB
	}, 4*time.Second)
	if navEvt == nil {
		fmt.Println("  ✗ 判据 14-3：4s 内没收到 Page.frameNavigated（url 与请求一致的那条）")
	} else {
		f, _ := eventParams(navEvt)["frame"].(map[string]any)
		if evLoader, _ := f["loaderId"].(string); evLoader != loader1 {
			fmt.Printf("  ✗ 判据 14-3：frameNavigated 的 loaderId=%q 与响应 %q 不一致\n", evLoader, loader1)
		} else if evURL, _ := f["url"].(string); evURL != urlB {
			fmt.Printf("  ✗ 判据 14-3：frameNavigated 的 url=%q，want %q\n", evURL, urlB)
		} else {
			fmt.Println("  ✓ 判据 14-3：Page.frameNavigated（url/loaderId 与响应一致）")
		}
	}
	if titleB, _ := s3EvalString(navConn, "", "document.title"); titleB != "S3-NAV-B" {
		fmt.Printf("  ✗ 判据 14-4：导航后 title=%q，want \"S3-NAV-B\"（文档没真的换）\n", titleB)
	} else {
		fmt.Printf("  ✓ 判据 14-4：导航后文档已替换（title=%q）\n", titleB)
	}
	nav2, _ := navConn.call("Page.navigate", map[string]any{"url": urlA})
	loader2, _ := resultOf(nav2)["loaderId"].(string)
	if loader2 == "" || loader2 == loader1 {
		fmt.Printf("  ✗ 判据 14-5：第二次导航 loaderId=%q（首次 %q）——应每次导航都新分配\n", loader2, loader1)
	} else {
		fmt.Printf("  ✓ 判据 14-5：两次导航 loaderId 不同（%s → %s）\n", loader1, loader2)
	}
}

// writeNavProbes 写两个临时页面（a.html / b.html），返回各自的 file:// URL。
func writeNavProbes(dir string) (string, string, error) {
	page := func(title string) string {
		return `<!DOCTYPE html><html><head><meta charset="utf-8"><title>` + title +
			`</title></head><body style="margin:0"><div id="p">` + title + `</div></body></html>`
	}
	a := filepath.Join(dir, "a.html")
	b := filepath.Join(dir, "b.html")
	if err := os.WriteFile(a, []byte(page("S3-NAV-A")), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(b, []byte(page("S3-NAV-B")), 0o644); err != nil {
		return "", "", err
	}
	return fileURLOf(a), fileURLOf(b), nil
}

// freePort 取一个空闲的回环端口：StartDevTools 只接受具体端口（0 视为关闭），
// 所以先监听 0 拿到端口，再让独立自检服务占用它。
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// jsQuote 把 Go 字符串编码成 JS 字面量（复用 JSON 的转义规则）。
func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// s3EvalString 在指定扁平会话里求值一个返回字符串的表达式。
func s3EvalString(c *cdpClient, sessionID, expr string) (string, error) {
	res, err := c.callSession("Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true}, sessionID)
	if err != nil {
		return "", err
	}
	s, _ := remoteValueOfResult(res).(string)
	return s, nil
}

// eventParams 取事件的 params 对象（非对象时返回空 map 便于链式取值）。
func eventParams(evt map[string]any) map[string]any {
	if evt == nil {
		return map[string]any{}
	}
	if p, ok := evt["params"].(map[string]any); ok {
		return p
	}
	return map[string]any{}
}

// eventTargetID 取 Target 域事件里 targetInfo.targetId。
func eventTargetID(evt map[string]any) string {
	ti, _ := eventParams(evt)["targetInfo"].(map[string]any)
	id, _ := ti["targetId"].(string)
	return id
}

// errCode 取响应里 error.code（无错时 nil）。
func errCode(res map[string]any) any {
	if e, ok := res["error"].(map[string]any); ok {
		return e["code"]
	}
	return nil
}

// ─── 断言辅助 ────────────────────────────────────────────

// remoteValueOfResult 取出 Runtime.evaluate 响应里的 result.value。
// hasProperty 报告 Runtime.getProperties 的结果里是否有指定名字的属性。
func hasProperty(res map[string]any, name string) bool {
	list, _ := resultOf(res)["result"].([]any)
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			if n, _ := m["name"].(string); n == name {
				return true
			}
		}
	}
	return false
}

// float64Slice 把 CDP 的 nodeIds 数组（JSON 数组）转成 []float64。
func float64Slice(v any) []float64 {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, x := range arr {
		if f, ok := x.(float64); ok {
			out = append(out, f)
		}
	}
	return out
}

// entryText 取 Log.entryAdded 事件里的 entry.text（去掉尾部换行）。
func entryText(evt map[string]any) string {
	p, _ := evt["params"].(map[string]any)
	en, _ := p["entry"].(map[string]any)
	t, _ := en["text"].(string)
	return strings.TrimSpace(t)
}

// entryLevel 取 Log.entryAdded 事件里的 entry.level。
func entryLevel(evt map[string]any) string {
	p, _ := evt["params"].(map[string]any)
	en, _ := p["entry"].(map[string]any)
	lvl, _ := en["level"].(string)
	return lvl
}

func remoteValueOfResult(res map[string]any) any {
	ro, ok := resultOf(res)["result"].(map[string]any)
	if !ok {
		return nil
	}
	return ro["value"]
}

// resultOf 取出 CDP 响应的 result 对象（CDP 的成功响应是 {"id":…,"result":{…}}，
// 错误响应是 {"id":…,"error":{…}}——错误在顶层，结果在 result 里）。
func resultOf(res map[string]any) map[string]any {
	r, _ := res["result"].(map[string]any)
	return r
}

// countElementNodes 统计 CDP 节点树里的元素节点数（nodeType == 1）。
func countElementNodes(n map[string]any) int {
	if n == nil {
		return 0
	}
	n0 := 0
	if t, ok := n["nodeType"].(float64); ok && int(t) == 1 {
		n0 = 1
	}
	children, _ := n["children"].([]any)
	for _, ch := range children {
		if cm, ok := ch.(map[string]any); ok {
			n0 += countElementNodes(cm)
		}
	}
	return n0
}

// compareScreenshotWithRender 把 CDP 截图（PNG）与引擎的像素缓冲逐点比对，
// 返回不一致数与参与比对的像素数。只比对 alpha=255 的像素：半透明像素在 PNG
// 里是直通 alpha、而 Render() 给的是预乘 alpha，数值天然不同（不是实现缺陷）。
func compareScreenshotWithRender(pngBytes []byte, wv *webkit.WebView) (int, int, error) {
	img, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("截图不是可解码的图片: %w", err)
	}
	pixels, err := wv.Render()
	if err != nil {
		return 0, 0, fmt.Errorf("Render(): %w", err)
	}
	w, h := wv.Width(), wv.Height()
	if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		return 0, 0, fmt.Errorf("截图尺寸 %v 与视口 %dx%d 不符", img.Bounds(), w, h)
	}
	if len(pixels) < w*h*4 {
		return 0, 0, fmt.Errorf("像素缓冲 %d 字节 < %dx%d", len(pixels), w, h)
	}
	diff, total := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			if pixels[i+3] != 255 {
				continue // 半透明像素不参与（见函数注释）
			}
			total++
			r, g, b, _ := img.At(x, y).RGBA()
			if uint8(r>>8) != pixels[i] || uint8(g>>8) != pixels[i+1] || uint8(b>>8) != pixels[i+2] {
				diff++
			}
		}
	}
	return diff, total, nil
}

// ─── HTTP 辅助 ───────────────────────────────────────────

func httpGetJSON(u string) (map[string]any, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func httpGetJSONList(u string) ([]map[string]any, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── 最小 CDP 客户端（WS） ───────────────────────────────

type cdpClient struct {
	conn net.Conn
	br   *bufio.Reader
	id   int

	// evts 收集引擎主动推的事件消息（没有 id 的消息）。此前 call 直接丢弃它们，
	// 于是「Log.entryAdded / Runtime.consoleAPICalled 是否真被推」无从断言。
	evMu sync.Mutex
	evts []map[string]any
	// checked 是 waitEventWhere/drainEvents 的「已检查游标」：同一条事件不会被
	// 重复匹配（否则 waitEventWhere 会反复命中第一条不满足条件的事件）。
	checked int
}

// findEvent 在已收集的事件里找第一条指定 method 的消息。
func (c *cdpClient) findEvent(method string) map[string]any {
	c.evMu.Lock()
	defer c.evMu.Unlock()
	for _, e := range c.evts {
		if m, _ := e["method"].(string); m == method {
			return e
		}
	}
	return nil
}

// waitEvent 等一条指定 method 的事件（最多 timeout）。读超时不算失败：事件由宿主
// 的 150ms 泵推送，客户端在这里轮询读取。
func (c *cdpClient) waitEvent(method string, timeout time.Duration) map[string]any {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if e := c.findEvent(method); e != nil {
			return e
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		op, payload, err := c.readFrame()
		if err != nil {
			continue // 读超时：继续等到总时限
		}
		if op != 0x1 {
			continue
		}
		var msg map[string]any
		if json.Unmarshal(payload, &msg) != nil {
			continue
		}
		if _, hasID := msg["id"]; hasID {
			continue // 无人认领的响应：忽略
		}
		c.evMu.Lock()
		c.evts = append(c.evts, msg)
		c.evMu.Unlock()
	}
	return c.findEvent(method)
}

// drainEvents 读掉当前已排队/即将到达的事件（最多 wait 时长），并把「已检查游标」
// 推到末尾。用途：Log/Runtime 是**增量**通道，enable 后的第一批事件是**历史存量**
// ——断言新探针事件之前必须先清掉它们，否则首条必然是旧的 console.log。
func (c *cdpClient) drainEvents(wait time.Duration) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		_ = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		op, payload, err := c.readFrame()
		if err != nil || op != 0x1 {
			continue
		}
		var msg map[string]any
		if json.Unmarshal(payload, &msg) != nil {
			continue
		}
		if _, hasID := msg["id"]; hasID {
			continue
		}
		c.evMu.Lock()
		c.evts = append(c.evts, msg)
		c.evMu.Unlock()
	}
	c.evMu.Lock()
	c.checked = len(c.evts)
	c.evMu.Unlock()
}

// waitEventWhere 等一条**满足条件**的事件：按已检查游标逐条推进（同一批里没命中的
// 事件不会被重复检查），直到命中或超时。
func (c *cdpClient) waitEventWhere(method string, pred func(map[string]any) bool, timeout time.Duration) map[string]any {
	deadline := time.Now().Add(timeout)
	for {
		c.evMu.Lock()
		for c.checked < len(c.evts) {
			e := c.evts[c.checked]
			c.checked++
			if m, _ := e["method"].(string); m == method && pred(e) {
				c.evMu.Unlock()
				return e
			}
		}
		c.evMu.Unlock()
		if time.Now().After(deadline) {
			return nil
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		op, payload, err := c.readFrame()
		if err != nil || op != 0x1 {
			continue
		}
		var msg map[string]any
		if json.Unmarshal(payload, &msg) != nil {
			continue
		}
		if _, hasID := msg["id"]; hasID {
			continue
		}
		c.evMu.Lock()
		c.evts = append(c.evts, msg)
		c.evMu.Unlock()
	}
}

// dialCDPClient 完成 WS 握手并校验 Sec-WebSocket-Accept（顺便验证服务端的
// accept 算法，而不是只看状态码）。
func dialCDPClient(wsURL string) (*cdpClient, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout("tcp", u.Host, 3*time.Second)
	if err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString([]byte("wb-ui-cdp-probe"))
	req := "GET " + u.Path + " HTTP/1.1\r\nHost: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode != 101 {
		_ = conn.Close()
		return nil, fmt.Errorf("握手状态 %d", resp.StatusCode)
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if want := base64.StdEncoding.EncodeToString(sum[:]); resp.Header.Get("Sec-WebSocket-Accept") != want {
		_ = conn.Close()
		return nil, fmt.Errorf("Sec-WebSocket-Accept 不符（got %q want %q）", resp.Header.Get("Sec-WebSocket-Accept"), want)
	}
	return &cdpClient{conn: conn, br: br}, nil
}

func (c *cdpClient) close() { _ = c.conn.Close() }

// call 发一条 CDP 请求并等响应（不带 sessionId：走隐含会话，S1 的最短路径）。
func (c *cdpClient) call(method string, params map[string]any) (map[string]any, error) {
	return c.callSession(method, params, "")
}

// callSession 发一条 CDP 请求并等响应；sessionID 非空时把请求路由到该扁平会话
// （flatten 模式下 sessionId 是消息的顶层字段，见 engine/devtools/cdp/protocol.go）。
//
// 读帧时**收下**没有 id 的事件而不是丢弃：Page.frameNavigated / Target.attachedToTarget
// 这类事件紧跟在响应之后推，丢了就再也断言不到（判据 13/14 依赖它们）。
func (c *cdpClient) callSession(method string, params map[string]any, sessionID string) (map[string]any, error) {
	c.id++
	msg := map[string]any{"id": c.id, "method": method}
	if sessionID != "" {
		msg["sessionId"] = sessionID
	}
	if params != nil {
		msg["params"] = params
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if err := c.writeText(raw); err != nil {
		return nil, err
	}
	// ★ waitEventWhere/drainEvents 会把连接读截止时间设成 150ms 的轮询值，而它们
	// 返回时那一刻通常**已经越过**该截止时间（最后一次 readFrame 正好耗掉 150ms）
	// → 发请求前必须重置读截止时间，否则随后的读帧立刻 i/o timeout。实测：判据
	// 13-e 的 700ms 轮询之后，13-f 与判据 14 全部因「读超时」假失败。
	_ = c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for i := 0; i < 200; i++ {
		op, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		if op != 0x1 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(payload, &m); err != nil {
			return nil, err
		}
		if id, ok := m["id"].(float64); ok {
			if int(id) == c.id {
				return m, nil
			}
			continue // 别的请求的响应（本客户端串行发送，正常不会出现）
		}
		c.evMu.Lock()
		c.evts = append(c.evts, m)
		c.evMu.Unlock()
	}
	return nil, fmt.Errorf("%s：超过 200 条消息仍未收到响应", method)
}

// writeText 发一个带掩码的文本帧（客户端帧必须掩码，RFC6455 §5.1）。
func (c *cdpClient) writeText(payload []byte) error {
	mask := []byte{0x5A, 0x5B, 0x5C, 0x5D}
	buf := bytes.NewBuffer(nil)
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
	masked := make([]byte, n)
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	buf.Write(masked)
	_, err := c.conn.Write(buf.Bytes())
	return err
}

// readFrame 读一个服务端帧（服务端帧无掩码）。
func (c *cdpClient) readFrame() (byte, []byte, error) {
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		return 0, nil, err
	}
	op := head[0] & 0x0F
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > math.MaxInt32 {
		return 0, nil, fmt.Errorf("帧过大: %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, err
	}
	return op, payload, nil
}
