package webkit

// 页面脚本发起的文档导航（location.assign/replace/reload、history 遍历）与
// 引擎的接线。
//
// bindings 层只提供出口（NavigationRequest / ReloadRequest，见
// bindings/navigation.go）：引擎没有自己的网络层，换文档必须由宿主装配
// （LoadURL）。这里把出口落到 WebView，并在装配完成后把导航结果写回
// window.history（NoteDocumentNavigation）——history.length 因此反映真实
// 文档数、back/forward 能遍历到上一个文档。

import (
	neturl "net/url"
	"sync"

	"wb-ui/bindings"
	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/page"
)

var installNavigationOnce sync.Once

// installNavigationDispatch 注册 bindings 的导航出口（进程内一次：按解释器
// 反查 WebView，多 WebView 各走各的）。
func installNavigationDispatch() {
	installNavigationOnce.Do(func() {
		bindings.NavigationRequest = func(in *jsc.Interpreter, url string, kind bindings.NavKind) bool {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return false
			}
			return wv.navigateTo(url, kind)
		}
		bindings.ReloadRequest = func(in *jsc.Interpreter) bool {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return false
			}
			return wv.reloadDocument()
		}
		bindings.FragmentNavigation = func(in *jsc.Interpreter, frag string, kind bindings.NavKind) bool {
			wv := webViewForInterpreter(in)
			if wv == nil {
				return false
			}
			return wv.navigateToFragment(frag, kind)
		}
	})
}

// queuedNavigation 是装配期间收到的导航请求（见 navigateTo）。
type queuedNavigation struct {
	url  string
	kind bindings.NavKind
}

// finishAssembly 在文档装配结束后调用：复位装配标记，并执行装配期间排队的
// 导航请求（浏览器语义：脚本里发起的导航在当前脚本跑完后才发生）。
func (wv *WebView) finishAssembly() {
	wv.navMu.Lock()
	wv.assembling = false
	q := wv.queuedNav
	wv.queuedNav = nil
	wv.navMu.Unlock()
	if q == nil {
		return
	}
	if !wv.navigateTo(q.url, q.kind) {
		page.Logf("navigation", "排队的导航未能执行：%s", q.url)
	}
}

// navigateTo 处理脚本发起的文档导航：UI 库模式拒绝（导航是浏览器能力，页面
// 被替换会让 Go 侧构建的 UI 树与事件绑定悬空），浏览器模式交给 LoadURL。
// 返回 false 时 bindings 层不会改动任何历史/URL 状态。
func (wv *WebView) navigateTo(url string, kind bindings.NavKind) bool {
	if wv == nil || wv.destroyed {
		return false
	}
	if !wv.mode.allowsNavigation() {
		page.Logf("navigation", "location 导航在 %s 模式被拒绝：%s（宿主可用 LoadURL 显式导航）", wv.mode, url)
		wv.navMu.Lock()
		fn := wv.onNavigationBlocked
		wv.navMu.Unlock()
		if fn != nil {
			fn(url)
		}
		return false
	}
	// ★ 装配中（页面脚本执行阶段发起的导航）排队到装配结束再执行：浏览器里
	//   导航是异步的——当前脚本继续跑完才换文档。直接重入 LoadURL 会在
	//   LoadHTML/ExecuteScripts 尚未返回时替换 mainFrame 的文档与渲染树。
	wv.navMu.Lock()
	if wv.assembling {
		wv.queuedNav = &queuedNavigation{url: url, kind: kind}
		wv.navMu.Unlock()
		page.Logf("navigation", "装配中收到导航请求，排队到装配结束后执行：%s", url)
		return true
	}
	wv.navMu.Unlock()
	wv.setPendingNavKind(kind)
	if err := wv.LoadURL(url); err != nil {
		wv.clearPendingNavKind()
		page.Logf("navigation", "LoadURL(%q): %v", url, err)
		return false
	}
	return true
}

// reloadDocument 处理 location.reload()：重新装配当前文档。
//
// 有来源 URL（LoadURL 加载的文档）时重新取内容；没有来源 URL（LoadHTML 直出
// 内容）时引擎无处取内容——交给宿主注册的 SetReloadHandler（UI 库模式的宿主
// 自己持有模板/内容源）。
func (wv *WebView) reloadDocument() bool {
	if wv == nil || wv.destroyed {
		return false
	}
	if !wv.mode.allowsNavigation() {
		wv.navMu.Lock()
		fn := wv.onNavigationBlocked
		wv.navMu.Unlock()
		if fn != nil {
			fn("reload")
		}
		return false
	}
	u := wv.documentURL()
	if u == "" {
		wv.navMu.Lock()
		fn := wv.reloadHandler
		wv.navMu.Unlock()
		if fn == nil {
			page.Logf("navigation", "location.reload(): 文档没有来源 URL（LoadHTML 直出内容），宿主未注册 SetReloadHandler")
			return false
		}
		return fn()
	}
	// reload 替换当前历史条目（浏览器语义：不新增条目）。
	wv.setPendingNavKind(bindings.NavReplace)
	if err := wv.LoadURL(u); err != nil {
		wv.clearPendingNavKind()
		page.Logf("navigation", "reload(%q): %v", u, err)
		return false
	}
	return true
}

// navigateToFragment 处理**同文档**的 fragment 导航：`location.hash = "#x"`、
// `location.href = "#x"` / `location.assign("#x")`，以及历史遍历到一个只有
// fragment 不同的条目。浏览器语义（HTML §7.4.2 "navigate to a fragment"）：
// **不重新加载文档**，只做三件事——
//  1. 更新 URL 的 fragment（`document.URL` / `location.href` 随之变化）；
//  2. 滚动到锚点（id 优先，其次 `<a name>`；无匹配则回文档顶部）；
//  3. fragment 真的变化时派发 `hashchange`（带 oldURL/newURL）。
//
// 历史条目仍按 kind 追加/替换：同文档导航在浏览器里同样产生条目
// （`history.length` +1），`history.back()` 因此能回到上一个 fragment。
// `frag` 不含 "#"，空串表示清除 fragment。
func (wv *WebView) navigateToFragment(frag string, kind bindings.NavKind) bool {
	if wv == nil || wv.destroyed {
		return false
	}
	if !wv.mode.allowsNavigation() {
		page.Logf("navigation", "fragment 导航在 %s 模式被拒绝：#%s", wv.mode, frag)
		wv.navMu.Lock()
		fn := wv.onNavigationBlocked
		wv.navMu.Unlock()
		if fn != nil {
			fn("#" + frag)
		}
		return false
	}
	doc := wv.Document()
	if doc == nil {
		return false
	}
	old := doc.URL()
	oldFrag := fragmentOfURL(old)
	newURL := withFragment(old, frag)
	if frag == "" && oldFrag == "" {
		return true // 无 fragment 也未要求清除 fragment：无事可做（不滚动、不记条目）
	}
	if newURL != old {
		doc.SetURL(newURL)
		bindings.NoteDocumentNavigation(wv.jsInterpreter, newURL, kind)
		// ★ URL 变了 → 依赖 URL 的选择器必须重新匹配：`:target` 匹配「URL
		// fragment 指向的元素」（见 css/selectorchecker.go），是纯 CSS 的
		// hash 路由写法（`#tab1:target{display:block}`）。样式解析器按元素
		// 缓存 ComputedStyle，不清缓存就仍是旧匹配结果 → 清缓存 + 标记重建
		// （与 onClassChanged 的失效路径一致：祖先变化影响后代匹配）。
		if fr := wv.mainFrame.Frame(); fr != nil {
			if rsv := fr.Resolver(); rsv != nil {
				rsv.ClearCache()
			}
			fr.MarkRenderTreeDirty()
			fr.SetNeedsLayout(true)
		}
	}
	// ★ 即使 fragment 未变也要滚动（`location.hash = 同一个值` 在浏览器里
	// 只是重新滚动，不产生条目、不派发 hashchange）。
	wv.scrollToAnchor(frag)
	if oldFrag != frag {
		bindings.DispatchHashChange(wv.jsInterpreter, old, newURL)
	}
	return true
}

// scrollToAnchor 实现 HTML §7.4.2 的 "scroll to the fragment"：把锚点滚进视口
// （元素顶对齐视口顶）。fragment 为空或没有匹配元素时滚回文档顶部（浏览器行为）。
// 只处理**文档级**滚动（锚点落在内层滚动容器里时浏览器还会滚动其祖先容器，
// 这里不涉及）。
func (wv *WebView) scrollToAnchor(frag string) {
	view := wv.mainFrameView()
	if view == nil {
		return
	}
	// 页面级滚动有**两处**状态，必须一起设置：rendering.RenderView 的
	// scrollOffset 是渲染真正使用的（renderpipeline 用 view.ScrollOffset()
	// 平移 canvas），page.FrameView 的 scrollX/Y 是宿主内省与钳制用的
	// （MaxScrollY/ContentHeight 由它维护）。只设一个 → 「偏移量对了但画面
	// 没动」（或反之宿主读到 0）。
	applyScroll := func(y float64) {
		if y < 0 {
			y = 0
		}
		if maxY := view.MaxScrollY(); y > float64(maxY) {
			y = float64(maxY)
		}
		view.SetScrollOffset(0, int(y))
		if rv := wv.RenderView(); rv != nil {
			rv.SetScrollOffset(0, float64(view.ScrollY()))
		}
	}
	if frag == "" {
		applyScroll(0)
		return
	}
	wv.EnsureHitTestReady() // 定位前确保布局/渲染树就绪
	el := anchorElement(wv.Document(), frag)
	if el == nil {
		applyScroll(0) // 没有匹配的锚点：浏览器滚回文档顶部
		return
	}
	// EnsureHitTestReady 里的 Layout 可能重建渲染树（RenderView 换新实例），
	// 必须重新取。
	rv := wv.RenderView()
	if rv == nil {
		return
	}
	// 取锚点在文档中的 y。AbsoluteY 是渲染树坐标系里的绝对 y（文档原点为 0）；
	// 若它已含祖先滚动容器的偏移，减去以还原「元素在文档中的位置」。
	//
	// ★ 空 inline 锚点（`<a name="x"></a>`）在引擎里**不生成渲染盒**，但浏览器
	// 仍把 fragment 导航视为落在该处 → 退化为「DOM 顺序中该锚点之后第一个有
	// 渲染盒的节点」的 y（典型情况就是锚点后面的那个块，位置与浏览器一致）。
	// 此前直接 `box == nil → return` 会让 `<a name>` 锚点**静默不滚动**。
	for n := dom.Node(el); n != nil; n = nextNodeInDocOrder(n) {
		box := rv.FindRenderBoxForNode(n)
		if box == nil {
			continue
		}
		y := box.AbsoluteY()
		if _, sy := rv.ScrollStackOffsetFor(box); sy != 0 {
			y -= sy
		}
		applyScroll(y)
		return
	}
	applyScroll(0)
}

// nextNodeInDocOrder 返回文档序中节点 n 之后的下一个节点（先深入子节点，再
// 后继兄弟，最后沿祖先链向上找后继兄弟）；遍历到末尾返回 nil。
func nextNodeInDocOrder(n dom.Node) dom.Node {
	if n == nil {
		return nil
	}
	if c := n.FirstChild(); c != nil {
		return c
	}
	for cur := n; cur != nil; {
		if s := cur.NextSibling(); s != nil {
			return s
		}
		cur = cur.ParentNode()
	}
	return nil
}

// anchorElement 按 HTML §7.4.2 的顺序解析锚点目标：先按 id 找，再找
// `<a name="…">`（旧式命名锚点）。fragment 已解码（`#a%20b` → `a b`）。
func anchorElement(doc *dom.Document, frag string) *dom.Element {
	if doc == nil || frag == "" {
		return nil
	}
	if el := doc.GetElementById(frag); el != nil {
		return el
	}
	for _, a := range doc.GetElementsByTagName("a") {
		if a.GetAttribute("name") == frag {
			return a
		}
	}
	return nil
}

// fragmentOfURL 返回 URL 的 fragment（已解码；""=无 fragment）。
func fragmentOfURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	// url.URL.Fragment 已是解码后的形式（`#a%20b` → `a b`），不要再解码一次
	// （`#%2520` 会被错误地解成空格）。
	return u.Fragment
}

// withFragment 把 frag 写进 URL 的 fragment（空串=清除），其余部分不变。
func withFragment(raw, frag string) string {
	u, err := neturl.Parse(raw)
	if err != nil || u == nil {
		if frag == "" {
			return raw
		}
		return raw + "#" + frag
	}
	u.Fragment = frag
	u.RawFragment = ""
	return u.String()
}

// mainFrameView 返回主框架的 FrameView（页面级滚动偏移在这里）。
func (wv *WebView) mainFrameView() *page.FrameView {
	if wv == nil || wv.page == nil || wv.page.MainFrame() == nil {
		return nil
	}
	return wv.page.MainFrame().View()
}

// SetReloadHandler 注册 location.reload() 的处理（返回 true 表示已重新装配）。
// 用得着它的场景：LoadHTML / LoadHTMLWithBaseURL 直出的文档没有来源 URL，
// 引擎无法自己重新取内容；宿主可在这里重新装配（UI 库模式的常见做法）。
func (wv *WebView) SetReloadHandler(fn func() bool) {
	if wv == nil {
		return
	}
	wv.navMu.Lock()
	wv.reloadHandler = fn
	wv.navMu.Unlock()
}

// SetOnNavigationBlocked 注册「导航被模式门禁拒绝」的回调（UI 库模式下的
// location.assign 等）：宿主可用它提示/记录，而不是让页面的跳转静默失效。
func (wv *WebView) SetOnNavigationBlocked(fn func(url string)) {
	if wv == nil {
		return
	}
	wv.navMu.Lock()
	wv.onNavigationBlocked = fn
	wv.navMu.Unlock()
}

// setPendingNavKind 记下本次导航的种类（LoadURL 装配完成后消费一次）。
func (wv *WebView) setPendingNavKind(kind bindings.NavKind) {
	wv.navMu.Lock()
	wv.pendingNavKind = kind
	wv.pendingNavSet = true
	wv.navMu.Unlock()
}

// takeNavKind 消费本次导航的种类；没有显式设置时按 NavPush（宿主直接调
// LoadURL = 新导航，浏览器里点链接同理）。
func (wv *WebView) takeNavKind() bindings.NavKind {
	wv.navMu.Lock()
	defer wv.navMu.Unlock()
	k := wv.pendingNavKind
	if !wv.pendingNavSet || k == 0 {
		k = bindings.NavPush
	}
	wv.pendingNavKind = bindings.NavPush
	wv.pendingNavSet = false
	return k
}

func (wv *WebView) clearPendingNavKind() {
	wv.navMu.Lock()
	wv.pendingNavKind = bindings.NavPush
	wv.pendingNavSet = false
	wv.navMu.Unlock()
}
