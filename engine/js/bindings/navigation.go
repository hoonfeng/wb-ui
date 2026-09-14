package bindings

// 文档级导航与 window.history 的宿主接线。
//
// 浏览器里 `location.assign/replace/href=...` 会**换文档**、`history.back()`
// 会遍历到上一个文档，且这些都反映在 `history.length` 上。引擎此前这三件事都
// 没有实现：
//   - `location.assign/replace/reload` 是返回 undefined 的空桩（静默无效），
//     `location.href` 只有 getter（赋值静默丢弃）；
//   - `history` 只记录脚本自己的 pushState/replaceState，宿主的 LoadHTML/
//     LoadURL 装配**不进历史栈** → `history.length` 恒为 1、back() 永远无操作。
//
// 这里提供宿主（webkit.WebView）需要实现的两个出口（NavigationRequest /
// ReloadRequest），以及宿主装配完成后回写历史的入口
// （NoteDocumentNavigation）。

import (
	"net/url"
	"strings"
	"sync"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// NavKind 是导航请求的种类（决定历史栈如何变化）。
type NavKind int

const (
	// NavPush：新导航（location.assign / location.href 赋值）——往历史栈
	// 追加条目（并丢弃 forward 栈）。
	NavPush NavKind = iota
	// NavReplace：替换当前条目（location.replace / location.reload）——
	// 历史栈长度不变。
	NavReplace
	// NavTraverse：历史遍历（history.back/forward/go 跨文档）——条目已存在，
	// 宿主加载目标文档后只需移动指针，**不得**再追加条目。
	NavTraverse
)

// NavigationRequest 由宿主注册：页面脚本要求换文档时调用。url 已按文档基准
// （document.baseURI）解析成绝对 URL、且已排除「与当前文档相同」的情形。
// 返回 true 表示宿主接受了这次导航（正在/已经换文档）。
//
// 未注册（纯 bindings 用法）或宿主拒绝（UI 库模式不允许导航）时返回 false，
// 调用方据此**不改变任何状态**——不会留下「URL 变了但内容没变」的假象。
var NavigationRequest func(in *jsc.Interpreter, url string, kind NavKind) bool

// ReloadRequest 由宿主注册：location.reload() 重新装配当前文档。没有来源 URL
// 的文档（LoadHTML 直出内容）宿主可用自己的内容源实现它。
var ReloadRequest func(in *jsc.Interpreter) bool

// FragmentNavigation 由宿主注册：**同文档**的 fragment 导航——`location.hash = …`、
// `location.href = "#x"`、`location.assign("#x")`，以及历史遍历到一个只有
// fragment 不同的条目。浏览器里这类导航**不重新加载文档**（HTML §7.4.2
// "navigate to a fragment"），只做三件事：更新 URL 的 fragment、滚动到锚点、
// 在 fragment 真的变化时派发 hashchange；历史条目仍按 kind 追加/替换
// （同文档导航在浏览器里同样产生新条目）。
//
// 返回 true 表示宿主接受了这次同文档导航。
var FragmentNavigation func(in *jsc.Interpreter, frag string, kind NavKind) bool

// fragmentOf 返回 URL 的 fragment（不含 "#"）。
func fragmentOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	return u.Fragment
}

// sameDocumentURL 报告两个 URL 是否指向**同一个文档**（去掉 fragment 后相等）：
// 这是「同文档导航」与「换文档导航」的判据。
func sameDocumentURL(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil || ua == nil {
		return a == b
	}
	ub, err := url.Parse(b)
	if err != nil || ub == nil {
		return a == b
	}
	ua.Fragment, ub.Fragment = "", ""
	ua.RawFragment, ub.RawFragment = "", ""
	return ua.String() == ub.String()
}

// navEntry 是历史栈的一个条目。
type navEntry struct {
	state map[string]interface{}
	title string
	// url 是条目对应的 URL：pushState 写的是脚本传入的（相对）串，宿主导航
	// 写的是绝对 URL。
	url string
	// hostNavigated 标记该条目来自**宿主文档导航**（LoadHTML/LoadURL 装配，
	// 含 location.assign 等触发的导航），而不是脚本的 pushState。历史遍历
	// 到这种条目时要请求宿主真的换文档（跨文档遍历），而 pushState 条目只需
	// 移动指针 + 派发 popstate（同文档遍历）。
	hostNavigated bool
}

// navPopListener 是 popstate 监听器（由 addEventListener 注册）。
type navPopListener struct {
	fn      jsc.JSValue
	capture bool
	// eventType 区分 popstate 与 hashchange：两者共用一份监听器表（都只在
	// 导航时派发），但派发时必须给事件正确的 type——此前一律派成 "popstate"，
	// 于是 window.addEventListener("hashchange", f) 注册的 f 收到的是
	// type="popstate" 的事件（`if (e.type === "hashchange")` 分支永远不成立）。
	eventType string
}

// navState 是每个解释器（= 每个 window）的历史栈。
type navState struct {
	entries      []navEntry
	index        int
	popListeners []navPopListener
	// hist 与 refresh 由 RegisterDOMBindings 装配：宿主导航完成后要更新
	// history.length / history.state（它们是 JS 对象上的属性，需要回写）。
	hist    *jsc.JSObject
	refresh func()
	// dispatchHash 由 RegisterDOMBindings 装配：宿主完成同文档 fragment 导航后
	// 派发 hashchange（包含 oldURL/newURL）。
	dispatchHash func(oldURL, newURL string)
}

var (
	navStatesMu sync.Mutex
	navStates   = map[*jsc.Interpreter]*navState{}
)

// navStateFor 返回（必要时创建）解释器的历史栈。初始 entries 为空：第一个条目
// 由宿主装配完成后的 NoteDocumentNavigation 写入——这样 `history.length` 在
// 文档装配后就是 1（与浏览器一致），而不是引擎此前恒为 1 的占位条目。
func navStateFor(rt *jsc.Interpreter) *navState {
	navStatesMu.Lock()
	defer navStatesMu.Unlock()
	st := navStates[rt]
	if st == nil {
		st = &navState{index: -1}
		navStates[rt] = st
	}
	return st
}

// navStateIfAny 返回解释器已有的历史栈（不存在时返回 nil，不创建）。
func navStateIfAny(rt *jsc.Interpreter) *navState {
	navStatesMu.Lock()
	defer navStatesMu.Unlock()
	return navStates[rt]
}

// NoteDocumentNavigation 记录一次**文档级导航**（宿主装配完成新文档，或
// location.assign/replace 触发的导航）。浏览器语义：文档导航追加一条历史条目
// （replace/traverse 除外），丢弃 forward 栈，history.length 与 back/forward
// 的可达范围随之变化。
//
// url 为空（LoadHTML 直出内容，没有来源 URL）也记录：条目本身有效（length
// 语义正确），只是它不能作为历史遍历的目标。
func NoteDocumentNavigation(rt *jsc.Interpreter, url string, kind NavKind) {
	st := navStateIfAny(rt)
	if st == nil {
		return
	}
	switch kind {
	case NavTraverse:
		// 历史遍历：条目已经在栈里，按 URL 找到它并移动指针（宿主加载完成后
		// 回写，因此这里不能追加条目——否则每次 back/forward 都会让栈变长）。
		for i := range st.entries {
			if st.entries[i].hostNavigated && st.entries[i].url == url {
				st.index = i
				break
			}
		}
	case NavReplace:
		if st.index >= 0 && st.index < len(st.entries) {
			st.entries[st.index] = navEntry{url: url, hostNavigated: true}
		} else {
			st.entries = append(st.entries, navEntry{url: url, hostNavigated: true})
			st.index = len(st.entries) - 1
		}
	default: // NavPush
		if st.index >= 0 && st.index < len(st.entries) {
			// 截断 forward 栈（浏览器：新导航会丢弃 forward 历史）。
			st.entries = st.entries[:st.index+1]
		} else {
			st.entries = st.entries[:0]
		}
		st.entries = append(st.entries, navEntry{url: url, hostNavigated: true})
		st.index = len(st.entries) - 1
	}
	if st.refresh != nil {
		st.refresh()
	}
}

// ResetNavigationStates 摘除解释器的历史栈（WebView 销毁时调用：全局表持有
// JS 值/闭包，不摘除会让已销毁的 WebView 无法回收）。
func ResetNavigationStates(rt *jsc.Interpreter) {
	if rt == nil {
		return
	}
	navStatesMu.Lock()
	delete(navStates, rt)
	navStatesMu.Unlock()
}

// DispatchHashChange 由宿主在**同文档 fragment 导航**真正改变了 fragment 之后
// 调用：向 window 上注册的 hashchange 监听器派发事件（事件对象带 oldURL/newURL，
// 与浏览器一致）。同文档历史遍历（history.back 回到不同的 fragment）浏览器同样
// 派发 hashchange——宿主在 FragmentNavigation 里统一处理即可。
func DispatchHashChange(rt *jsc.Interpreter, oldURL, newURL string) {
	st := navStateIfAny(rt)
	if st == nil || st.dispatchHash == nil {
		return
	}
	st.dispatchHash(oldURL, newURL)
}

// requestNavigation 处理脚本发起的文档导航（location.assign/replace、
// location.href 赋值）：按文档基准解析成绝对 URL，交给宿主。
func requestNavigation(in *jsc.Interpreter, doc *dom.Document, raw string, kind NavKind) {
	if doc == nil || raw == "" {
		return
	}
	abs := dom.ResolveURL(doc.BaseURL(), raw)
	if abs == "" {
		return
	}
	if sameDocumentURL(abs, doc.URL()) {
		// 只有 fragment 不同 → 同文档导航：不重新加载文档，交给宿主更新 URL、
		// 滚动到锚点、派发 hashchange（见 FragmentNavigation）。fragment 与
		// 当前完全相同时（`location.href = location.href`）宿主只重滚动、
		// 不追加条目也不派发 hashchange——与浏览器一致。
		if FragmentNavigation != nil {
			FragmentNavigation(in, fragmentOf(abs), kind)
		}
		return
	}
	if NavigationRequest == nil {
		return
	}
	NavigationRequest(in, abs, kind)
}

// requestFragmentNavigation 处理 `location.hash = …`（同文档导航）。
// raw 是赋给 hash 的串：可能带前导 "#"（`location.hash = "#x"`），也可能是
// 裸片段名（`location.hash = "x"` → 浏览器得到 "#x"）；空串清除 fragment。
func requestFragmentNavigation(in *jsc.Interpreter, doc *dom.Document, raw string) {
	if doc == nil || FragmentNavigation == nil {
		return
	}
	frag := strings.TrimPrefix(raw, "#")
	FragmentNavigation(in, frag, NavPush)
}

// requestReload 处理 location.reload()：交给宿主重新装配当前文档。
func requestReload(in *jsc.Interpreter) {
	if ReloadRequest == nil {
		return
	}
	ReloadRequest(in)
}
