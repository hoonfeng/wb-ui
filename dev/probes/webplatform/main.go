// Command webplatform 实测 wb-ui 引擎的「Web 平台 API 面」覆盖矩阵。
//
// 动机：React 19 在引擎里静默空白，根因不是"跑得慢"，而是 commit 阶段的
// `element instanceof window.HTMLIFrameElement` 因该构造器不存在而抛错
// （异常发生在调度回调里，被引擎吞掉）。也就是说，「剩余未完成工作」的相当一部分
// 不是大模块，而是 Web 平台 API 面的细节缺口 —— 本探针把它测成可追踪的矩阵。
//
// 用法（仓库根，CGO 环境）：
//
//	go run ./dev/probes/webplatform
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

const pageHTML = `<!DOCTYPE html><html><head><title>webplatform-probe</title></head>
<body><div id="root"><p class="a">x</p></div></body></html>`

// jsProbe 探测三类东西：
//  1. window 上的**全局构造器/对象**是否存在（决定 instanceof / new / typeof 检测的行为）
//  2. document 的**属性与方法**是否存在（属性缺失同样是硬缺口，如 document.scripts）
//  3. 真实元素的**实例方法**是否存在（DOM 行为面）
//
// 输出统一为 {missing:[...], present:N, total:M} 的紧凑 JSON。
const jsProbe = `(function () {
  // 三种检测口径不能混用，否则会把「值合法为 null」的属性（firstChild/parentNode/
  // offsetParent…）误判成缺失：
  //   global —— typeof !== 'undefined'（全局绑定是否存在）
  //   fn     —— typeof === 'function' （方法是否可调用）
  //   prop   —— in 运算符（属性是否存在于自身或原型链，不受 null 值影响）
  function chkBy(scope, names, mode) {
    var missing = [], present = 0;
    for (var i = 0; i < names.length; i++) {
      var n = names[i], ok = false;
      try {
        if (mode === 'global') { ok = typeof scope[n] !== 'undefined'; }
        else if (mode === 'fn') { ok = typeof scope[n] === 'function'; }
        else { ok = (n in scope); }
      } catch (e) { ok = false; }
      if (ok) { present++; } else { missing.push(n); }
    }
    return { present: present, total: names.length, missing: missing };
  }
  function chkGlobals(s, n) { return chkBy(s, n, 'global'); }
  function chkFns(s, n) { return chkBy(s, n, 'fn'); }
  function chkProps(s, n) { return chkBy(s, n, 'prop'); }

  var globals = [
    // ES / 语言运行时
    'Object','Function','Array','String','Number','Boolean','Symbol','BigInt','Math','JSON','Date','RegExp','Error','TypeError','RangeError','SyntaxError','EvalError','URIError','AggregateError','Promise','Proxy','Reflect','Map','Set','WeakMap','WeakSet','WeakRef','FinalizationRegistry','ArrayBuffer','SharedArrayBuffer','DataView','Int8Array','Uint8Array','Uint8ClampedArray','Int16Array','Uint16Array','Int32Array','Uint32Array','Float32Array','Float64Array','BigInt64Array','BigUint64Array','Atomics','WebAssembly','Intl','GeneratorFunction','AsyncFunction','AsyncGeneratorFunction','eval','parseInt','parseFloat','isNaN','isFinite','encodeURIComponent','decodeURIComponent','escape','unescape','globalThis','NaN','Infinity',
    // 事件循环 / 调度
    'setTimeout','clearTimeout','setInterval','clearInterval','setImmediate','clearImmediate','queueMicrotask','requestAnimationFrame','cancelAnimationFrame','requestIdleCallback','cancelIdleCallback','MessageChannel','MessagePort','BroadcastChannel','structuredClone','postMessage','fetch','Request','Response','Headers','AbortController','AbortSignal','WebSocket','EventSource','XMLHttpRequest','URL','URLSearchParams','Blob','File','FileReader','FormData','TextEncoder','TextDecoder','DOMParser','XMLSerializer',
    // 存储
    'localStorage','sessionStorage','indexedDB','caches','CookieStore','Notification','Clipboard','Geolocation','crypto','atob','btoa','performance','console',
    // 观察者
    'MutationObserver','IntersectionObserver','ResizeObserver','PerformanceObserver','ReportingObserver',
    // 事件
    'Event','CustomEvent','UIEvent','MouseEvent','PointerEvent','KeyboardEvent','InputEvent','WheelEvent','DragEvent','FocusEvent','TouchEvent','ClipboardEvent','AnimationEvent','TransitionEvent','ErrorEvent','MessageEvent','PromiseRejectionEvent','PopStateEvent','HashChangeEvent','BeforeUnloadEvent','PageTransitionEvent','StorageEvent','SubmitEvent','EventTarget',
    // DOM 核心
    'Node','NodeList','NodeFilter','NodeIterator','TreeWalker','Element','Attr','NamedNodeMap','HTMLCollection','DOMTokenList','DOMRect','DOMRectList','DOMRectReadOnly','DOMPoint','DOMMatrix','DOMQuad','Range','Selection','Document','HTMLDocument','DocumentFragment','ShadowRoot','Text','Comment','CDATASection','ProcessingInstruction','DocumentType','XMLDocument','DOMImplementation','DocumentTimeline','Animation','KeyframeEffect','CSSStyleSheet','CSSStyleRule','CSSRule','CSSRuleList','CSSStyleDeclaration','CSSKeyframesRule','CSSMediaRule','CSSGroupingRule','CSSConditionRule','CSSSupportsRule','CSSFontFaceRule','MediaQueryList','MediaQueryListEvent','StyleSheetList','XPathResult','XPathExpression','FontFace','FontFaceSet','VisualViewport','CaretPosition',
    // HTML 元素构造器（React 的 instanceof 检查直接依赖它们）
    'HTMLElement','HTMLUnknownElement','HTMLDivElement','HTMLSpanElement','HTMLParagraphElement','HTMLAnchorElement','HTMLAreaElement','HTMLImageElement','HTMLCanvasElement','HTMLVideoElement','HTMLAudioElement','HTMLMediaElement','HTMLSourceElement','HTMLTrackElement','HTMLInputElement','HTMLTextAreaElement','HTMLButtonElement','HTMLSelectElement','HTMLOptionElement','HTMLOptGroupElement','HTMLDataListElement','HTMLOutputElement','HTMLFormElement','HTMLFieldSetElement','HTMLLegendElement','HTMLLabelElement','HTMLIFrameElement','HTMLFrameElement','HTMLFrameSetElement','HTMLObjectElement','HTMLEmbedElement','HTMLScriptElement','HTMLStyleElement','HTMLLinkElement','HTMLMetaElement','HTMLBaseElement','HTMLTitleElement','HTMLHeadElement','HTMLHtmlElement','HTMLBodyElement','HTMLTableElement','HTMLTableRowElement','HTMLTableCellElement','HTMLTableSectionElement','HTMLTableCaptionElement','HTMLTableColElement','HTMLUListElement','HTMLOListElement','HTMLLIElement','HTMLDListElement','HTMLMenuElement','HTMLBRElement','HTMLHRElement','HTMLPreElement','HTMLQuoteElement','HTMLModElement','HTMLTemplateElement','HTMLSlotElement','HTMLDialogElement','HTMLDetailsElement','HTMLSummaryElement','HTMLProgressElement','HTMLMeterElement','HTMLPictureElement','HTMLMapElement','HTMLTimeElement','HTMLDataElement','HTMLFontElement','HTMLDirectoryElement','HTMLMarqueeElement',
    // SVG / 其他命名空间
    'SVGElement','SVGSVGElement','SVGGraphicsElement','SVGGeometryElement','SVGPathElement','SVGTextElement','SVGImageElement','SVGUseElement','SVGForeignObjectElement','MathMLElement','Image','Audio','Option','Path2D','ImageData','OffscreenCanvas','CanvasRenderingContext2D','WebGLRenderingContext','WebGL2RenderingContext','ImageBitmap','createImageBitmap','customElements','CustomElementRegistry','matchMedia','getComputedStyle','getSelection','scroll','scrollTo','scrollBy','open','close','focus','blur','print','alert','confirm','prompt','stop','find','moveTo','resizeTo','getScreenDetails','showOpenFilePicker','showSaveFilePicker','EyeDropper','Scheduler','scheduler','TaskController','TaskPriorityChangeEvent','CSS','CSSStyleValue','CSSUnitValue','CSSTransformValue','CSSImageValue','CSSKeywordValue','CSSNumericValue','Highlight','HighlightRegistry','navigation','Navigation','ViewTransition','documentPictureInPicture','LaunchQueue','IdleDetector','WakeLock','screen','history','location','navigator','document','window','self','top','parent','frames','length','name','origin','isSecureContext','crossOriginIsolated','devicePixelRatio','innerWidth','innerHeight','outerWidth','outerHeight','scrollX','scrollY','pageXOffset','pageYOffset','visualViewport','menubar','toolbar','statusbar','locationbar','personalbar'
  ];

  var docProps = ['documentElement','body','head','title','cookie','URL','documentURI','baseURI','referrer','readyState','characterSet','charset','inputEncoding','contentType','compatMode','activeElement','scripts','forms','images','links','embeds','plugins','styleSheets','adoptedStyleSheets','fonts','fullscreenElement','fullscreenEnabled','pointerLockElement','visibilityState','hidden','defaultView','implementation','children','childNodes','firstChild','lastChild','firstElementChild','lastElementChild','childElementCount','nodeType','nodeName','ownerDocument','scrollingElement','timeline','currentScript','pictureInPictureElement','designMode','dir','domain','location','anchors','applets','all'];

  var docMethods = ['createElement','createElementNS','createTextNode','createComment','createDocumentFragment','createAttribute','createProcessingInstruction','createCDATASection','createRange','createTreeWalker','createNodeIterator','createEvent','createExpression','createNSResolver','evaluate','importNode','adoptNode','getElementById','getElementsByName','getElementsByTagName','getElementsByTagNameNS','getElementsByClassName','querySelector','querySelectorAll','hasFocus','elementFromPoint','elementsFromPoint','caretRangeFromPoint','caretPositionFromPoint','execCommand','queryCommandSupported','queryCommandEnabled','queryCommandState','queryCommandValue','write','writeln','open','close','startViewTransition','exitFullscreen','exitPointerLock','getAnimations','getBoxQuads','convertPointFromNode','convertPointToNode','replaceChildren','append','prepend','getSelection'];

  var elMethods = ['setAttribute','setAttributeNS','getAttribute','getAttributeNS','getAttributeNames','removeAttribute','removeAttributeNS','hasAttribute','hasAttributeNS','hasAttributes','toggleAttribute','appendChild','removeChild','replaceChild','insertBefore','cloneNode','contains','isEqualNode','isSameNode','compareDocumentPosition','getRootNode','append','prepend','before','after','remove','replaceWith','replaceChildren','insertAdjacentElement','insertAdjacentHTML','insertAdjacentText','querySelector','querySelectorAll','getElementsByTagName','getElementsByClassName','closest','matches','webkitMatchesSelector','getBoundingClientRect','getClientRects','scrollIntoView','scrollIntoViewIfNeeded','scroll','scrollTo','scrollBy','animate','getAnimations','attachShadow','checkVisibility','requestFullscreen','focus','blur','click','dispatchEvent','addEventListener','removeEventListener','setPointerCapture','releasePointerCapture','hasPointerCapture','setHTML'];

  var elProps = ['id','className','classList','dataset','style','attributes','tagName','localName','namespaceURI','prefix','nodeValue','textContent','innerHTML','outerHTML','innerText','outerText','children','childNodes','parentNode','parentElement','firstChild','lastChild','nextSibling','previousSibling','firstElementChild','lastElementChild','nextElementSibling','previousElementSibling','childElementCount','nodeType','nodeName','ownerDocument','shadowRoot','part','slot','assignedSlot','isConnected','offsetWidth','offsetHeight','offsetTop','offsetLeft','offsetParent','clientWidth','clientHeight','clientTop','clientLeft','scrollWidth','scrollHeight','scrollTop','scrollLeft','hidden','inert','nonce','autofocus','tabIndex','translate','contentEditable','isContentEditable','draggable','spellcheck','lang','dir','title','accessKey','popover'];

  var el = (function () { try { return document.createElement('div'); } catch (e) { return {}; } })();
  var svg = (function () { try { return document.createElementNS('http://www.w3.org/2000/svg', 'svg'); } catch (e) { return {}; } })();

  // ── ★ 第 19 轮：globals 分层（core / optional / excluded）───────────────
  // 收敛判据：**globalsCore 的 missing 必须为 0**（有界、可复算）；
  // globalsOptional / globalsExcluded 只报数、不参与收敛判定。
  // excluded 是**经监督确认的不追集合**（WORKITEMS §18-6 / §19）：
  //   A 图形/GPU 与 DOM 几何、B WASM 与并发隔离、C Intl、D 调度/导航/系统集成、
  //   E Typed OM 与 Animation、J 网络/持久化/系统权限、K 观察者。
  // globals（全量清单）保持原样以延续历史批次对比；partition 检查报告
  // 「三组是否恰好覆盖全量」（dup = 重复项，notCovered = 未被分层覆盖项）。
  var globalsCore = [
    // ES / 语言运行时（core：JS 代码的基本前提）
    'Object','Function','Array','String','Number','Boolean','Symbol','BigInt','Math','JSON','Date','RegExp','Error','TypeError','RangeError','SyntaxError','EvalError','URIError','AggregateError','Promise','Proxy','Reflect','Map','Set','WeakMap','WeakSet','WeakRef','FinalizationRegistry','ArrayBuffer','DataView','Int8Array','Uint8Array','Uint8ClampedArray','Int16Array','Uint16Array','Int32Array','Uint32Array','Float32Array','Float64Array','BigInt64Array','BigUint64Array','eval','parseInt','parseFloat','isNaN','isFinite','encodeURIComponent','decodeURIComponent','escape','unescape','globalThis','NaN','Infinity',
    // 事件循环 / 计时（core）
    'setTimeout','clearTimeout','setInterval','clearInterval','setImmediate','clearImmediate','queueMicrotask','requestAnimationFrame','cancelAnimationFrame','requestIdleCallback','cancelIdleCallback','MessageChannel','MessagePort','structuredClone',
    // 核心 Web API（core）
    'fetch','AbortController','AbortSignal','WebSocket','XMLHttpRequest','URL','URLSearchParams','Blob','FileReader','TextEncoder','TextDecoder','DOMParser','XMLSerializer','atob','btoa','crypto','performance','console','localStorage','sessionStorage',
    // Window 身份与几何（core）
    'window','self','top','parent','frames','length','name','origin','isSecureContext','document','screen','history','location','navigator','devicePixelRatio','innerWidth','innerHeight','outerWidth','outerHeight','scrollX','scrollY','pageXOffset','pageYOffset','visualViewport','getComputedStyle','getSelection','matchMedia','Image','Audio','Option',
    // 观察者（core：框架渲染/尺寸依赖）
    'MutationObserver','IntersectionObserver','ResizeObserver',
    // 事件接口（core，含本轮注册的 17 个 Event 子类）
    'EventTarget','Event','CustomEvent','UIEvent','MouseEvent','PointerEvent','KeyboardEvent','InputEvent','WheelEvent','DragEvent','FocusEvent','TouchEvent','ClipboardEvent','AnimationEvent','TransitionEvent','ErrorEvent','MessageEvent','PromiseRejectionEvent','PopStateEvent','HashChangeEvent','BeforeUnloadEvent','PageTransitionEvent','StorageEvent','SubmitEvent',
    // DOM 核心接口（core，含本轮注册的 I 组）
    'Node','NodeList','NodeFilter','NodeIterator','TreeWalker','Element','Attr','NamedNodeMap','HTMLCollection','DOMTokenList','DOMRect','DOMRectList','DOMRectReadOnly','Range','Selection','Document','HTMLDocument','DocumentFragment','ShadowRoot','Text','Comment','CDATASection','ProcessingInstruction','DocumentType','XMLDocument','DOMImplementation',
    // HTML 元素接口（core：React/Vue 的 instanceof 检查直接依赖）
    'HTMLElement','HTMLUnknownElement','HTMLDivElement','HTMLSpanElement','HTMLParagraphElement','HTMLHeadingElement','HTMLAnchorElement','HTMLAreaElement','HTMLImageElement','HTMLCanvasElement','HTMLVideoElement','HTMLAudioElement','HTMLMediaElement','HTMLSourceElement','HTMLTrackElement','HTMLInputElement','HTMLTextAreaElement','HTMLButtonElement','HTMLSelectElement','HTMLOptionElement','HTMLOptGroupElement','HTMLDataListElement','HTMLOutputElement','HTMLFormElement','HTMLFieldSetElement','HTMLLegendElement','HTMLLabelElement','HTMLIFrameElement','HTMLFrameElement','HTMLFrameSetElement','HTMLObjectElement','HTMLEmbedElement','HTMLParamElement','HTMLScriptElement','HTMLStyleElement','HTMLLinkElement','HTMLMetaElement','HTMLBaseElement','HTMLTitleElement','HTMLHeadElement','HTMLHtmlElement','HTMLBodyElement','HTMLTableElement','HTMLTableRowElement','HTMLTableCellElement','HTMLTableSectionElement','HTMLTableCaptionElement','HTMLTableColElement','HTMLUListElement','HTMLOListElement','HTMLLIElement','HTMLDListElement','HTMLMenuElement','HTMLBRElement','HTMLHRElement','HTMLPreElement','HTMLQuoteElement','HTMLModElement','HTMLTemplateElement','HTMLSlotElement','HTMLDialogElement','HTMLDetailsElement','HTMLSummaryElement','HTMLProgressElement','HTMLMeterElement','HTMLPictureElement','HTMLMapElement','HTMLTimeElement','HTMLDataElement','HTMLFontElement','HTMLDirectoryElement','HTMLMarqueeElement',
    // SVG / MathML 接口（core，含本轮注册的 H 组 9 项）
    'SVGElement','SVGSVGElement','SVGGraphicsElement','SVGGeometryElement','SVGPathElement','SVGTextElement','SVGImageElement','SVGUseElement','SVGForeignObjectElement','MathMLElement'
  ];

  // optional：可做子系统（非渲染必需；已在本引擎能力边界内或有明确实现路径）。
  var globalsOptional = [
    // 网络 / 持久化 / 表单（可做，但轻量引擎非必需）
    'Request','Response','Headers','EventSource','File','FormData','indexedDB','caches','CookieStore','Notification','Clipboard','Geolocation','BroadcastChannel','postMessage',
    // 自定义元素
    'customElements','CustomElementRegistry',
    // CSS 对象模型（规则 / 媒体查询 / 样式表）
    'CSSStyleSheet','CSSStyleRule','CSSRule','CSSRuleList','CSSStyleDeclaration','CSSKeyframesRule','CSSMediaRule','CSSGroupingRule','CSSConditionRule','CSSSupportsRule','CSSFontFaceRule','MediaQueryList','MediaQueryListEvent','StyleSheetList','CSS',
    // XPath
    'XPathResult','XPathExpression',
    // 字体 / 视口 / 插入符
    'FontFace','FontFaceSet','VisualViewport','CaretPosition',
    // 高亮
    'Highlight','HighlightRegistry',
    // Window 方法（无嵌套滚动/多窗口语义 → no-op 或未做）
    'scroll','scrollTo','scrollBy','open','close','focus','blur','print','alert','confirm','prompt','stop','find','moveTo','resizeTo',
    // BarProp 对象
    'menubar','toolbar','statusbar','locationbar','personalbar'
  ];

  // excluded：**经监督确认的不追集合**（固化于此，不计入 core 分母）。
  var globalsExcluded = [
    // A 图形 / GPU / DOM 几何
    'Path2D','ImageData','OffscreenCanvas','CanvasRenderingContext2D','WebGLRenderingContext','WebGL2RenderingContext','ImageBitmap','createImageBitmap','DOMPoint','DOMMatrix','DOMQuad',
    // B WASM / 并发隔离
    'SharedArrayBuffer','Atomics','WebAssembly','crossOriginIsolated',
    // C Intl
    'Intl',
    // 宿主内建函数（浏览器全局同样不可见：typeof === 'undefined'）
    'GeneratorFunction','AsyncFunction','AsyncGeneratorFunction',
    // D 调度 / 导航 / 系统集成
    'Scheduler','scheduler','TaskController','TaskPriorityChangeEvent','navigation','Navigation','ViewTransition','documentPictureInPicture','LaunchQueue','IdleDetector','WakeLock','getScreenDetails','showOpenFilePicker','showSaveFilePicker','EyeDropper',
    // E Typed OM / Animation
    'CSSStyleValue','CSSUnitValue','CSSTransformValue','CSSImageValue','CSSKeywordValue','CSSNumericValue','DocumentTimeline','Animation','KeyframeEffect',
    // K 观察者（性能 / 上报）
    'PerformanceObserver','ReportingObserver'
  ];

  // 分层完备性检查：三组并集应恰好覆盖 globals 全量（无重复、无遗漏）。
  var __seen = {}, __dup = [], __notCovered = [], __all = globalsCore.concat(globalsOptional, globalsExcluded);
  for (var __i = 0; __i < __all.length; __i++) {
    if (__seen[__all[__i]]) { __dup.push(__all[__i]); }
    __seen[__all[__i]] = 1;
  }
  for (var __j = 0; __j < globals.length; __j++) {
    if (!__seen[globals[__j]]) { __notCovered.push(globals[__j]); }
  }

  var out = {
    globals: chkGlobals(window, globals),
    globalsCore: chkGlobals(window, globalsCore),
    globalsOptional: chkGlobals(window, globalsOptional),
    globalsExcluded: chkGlobals(window, globalsExcluded),
    documentProps: chkProps(document, docProps),
    documentMethods: chkFns(document, docMethods),
    elementMethods: chkFns(el, elMethods),
    elementProps: chkProps(el, elProps),
  };
  out.svgTagName = (function () { try { return String(svg.tagName); } catch (e) { return 'ERR'; } })();
  // ★ 第 19 轮：分层完备性（dup = 三组内重复项；notCovered = 全量里未被任何组覆盖的项）。
  out.partition = { dup: __dup, notCovered: __notCovered };
  return JSON.stringify(out);
})()`

func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

func main() {
	rounds := flag.Int("rounds", 20, "驱动轮数")
	out := flag.String("out", "", "把原始覆盖矩阵 JSON 写入该路径（便于归档与前后对比）")
	flag.Parse()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pageHTML)
	}))
	defer srv.Close()

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(1024, 768)
	wv.SetConsoleLogger(&jsc.BufferLogger{})
	if err := wv.LoadURL(srv.URL + "/"); err != nil {
		fmt.Printf("[FAIL] LoadURL: %v\n", err)
		return
	}
	for i := 0; i < *rounds; i++ {
		pumpFrame(wv)
	}

	v, err := wv.EvalJS(jsProbe)
	if err != nil {
		fmt.Printf("[FAIL] probe eval: %v\n", err)
		return
	}
	raw := v.ToString()

	type cover struct {
		Present int      `json:"present"`
		Total   int      `json:"total"`
		Missing []string `json:"missing"`
	}
	var whole map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &whole); err != nil {
		fmt.Println("原始输出:", raw)
		return
	}
	res := map[string]cover{}
	for k, v := range whole {
		var c cover
		if json.Unmarshal(v, &c) == nil && c.Total > 0 {
			res[k] = c
		}
	}
	svgTag := ""
	if v, ok := whole["svgTagName"]; ok {
		_ = json.Unmarshal(v, &svgTag)
	}
	// ★ 第 19 轮：globals 分层完备性（三组应恰好覆盖 globals 全量）。
	type partition struct {
		Dup        []string `json:"dup"`
		NotCovered []string `json:"notCovered"`
	}
	var part partition
	if v, ok := whole["partition"]; ok {
		_ = json.Unmarshal(v, &part)
	}

	cats := []string{"globals", "documentProps", "documentMethods", "elementMethods", "elementProps"}
	fmt.Println("=== Web 平台 API 面覆盖（wb-ui 引擎实测）===")
	tp, tt := 0, 0
	for _, c := range cats {
		r := res[c]
		pct := 0.0
		if r.Total > 0 {
			pct = float64(r.Present) * 100 / float64(r.Total)
		}
		fmt.Printf("%-16s %4d/%-4d  %5.1f%%\n", c, r.Present, r.Total, pct)
		tp += r.Present
		tt += r.Total
		// 全量缺失清单（按字母序，便于归档追踪）
		m := append([]string(nil), r.Missing...)
		sort.Strings(m)
		fmt.Printf("  缺失: %s\n\n", strings.Join(m, " "))
	}
	// ★ 第 19 轮：globals 分层报告（core 的 missing 是**有界收敛判据**；
	// optional / excluded 只报数，不参与收敛判定）。
	for _, c := range []string{"globalsCore", "globalsOptional", "globalsExcluded"} {
		r := res[c]
		pct := 0.0
		if r.Total > 0 {
			pct = float64(r.Present) * 100 / float64(r.Total)
		}
		fmt.Printf("%-16s %4d/%-4d  %5.1f%%", c, r.Present, r.Total, pct)
		if c == "globalsCore" {
			m := append([]string(nil), r.Missing...)
			sort.Strings(m)
			fmt.Printf("   ★ 收敛判据 missing（必须为 0）: [%s]", strings.Join(m, " "))
		} else {
			fmt.Print("   （不参与收敛判定）")
		}
		fmt.Println()
	}
	if len(part.Dup) == 0 && len(part.NotCovered) == 0 {
		fmt.Println("分层完备性: 三组恰好覆盖 globals 全量（dup=0, notCovered=0）")
	} else {
		fmt.Printf("分层完备性: dup=%v notCovered=%v\n", part.Dup, part.NotCovered)
	}
	fmt.Println()
	fmt.Printf("合计 %d/%d = %.1f%%\n", tp, tt, float64(tp)*100/float64(tt))
	if svgTag != "" {
		fmt.Printf("createElementNS('svg') tagName = %s\n", svgTag)
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(raw), 0o644); err == nil {
			fmt.Printf("原始矩阵已写入 %s\n", *out)
		}
	}
}
