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
	"sync"

	"wb-ui/bindings"
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
