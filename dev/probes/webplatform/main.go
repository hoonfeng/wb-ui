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

  var out = {
    globals: chkGlobals(window, globals),
    documentProps: chkProps(document, docProps),
    documentMethods: chkFns(document, docMethods),
    elementMethods: chkFns(el, elMethods),
    elementProps: chkProps(el, elProps),
  };
  out.svgTagName = (function () { try { return String(svg.tagName); } catch (e) { return 'ERR'; } })();
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
