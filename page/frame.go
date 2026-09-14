// Translation of: Source/WebCore/page/Frame.h
//                  Source/WebCore/page/LocalFrame.h
//                  Source/WebCore/page/LocalFrame.cpp
// Completeness: 85%
// Simplifications:
//   - no frame tree (single frame per page); the Local/Remote frame split is omitted
//   - no NavigationScheduler / FrameLoaderClient / WindowProxy
//   - no owner element / sandbox flags / opener relationship
//   - LoadHTML drives HTML parse -> Document -> style resolve -> RenderTree build
//     directly, with no incremental parsing or script execution

package page

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"time"

	"wb-ui/bindings"
	"wb-ui/css"
	"wb-ui/debugenv"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/rendering"
	"wb-ui/style"
)

// Frame is the Go translation of WebCore::LocalFrame. A Frame is the rendering
// context for a single Document: it owns the document, the style resolver and the
// RenderView built from that document, plus the FrameView that manages the viewport
// and layout. In WebKit a Frame is part of a frame tree and is associated with a
// loader, a window proxy and an owner element; this port models a single
// standalone frame attached to a Page.
type Frame struct {
	page *Page

	// document is the currently-loaded DOM Document, mirroring
	// LocalFrame::document().
	document *dom.Document

	// renderView is the root of the render tree built from document, mirroring
	// the render view reached via LocalFrame::contentRenderer() / FrameView.
	renderView *rendering.RenderView

	// view is the FrameView managing the viewport and layout, mirroring
	// LocalFrame::view().
	view *FrameView

	// resolver resolves ComputedStyles for the document's elements. It is reused
	// across render-tree rebuilds so that added style sheets persist.
	resolver *style.Resolver

	// styleSheets tracks the CSSStyleSheets extracted from <style> elements
	// in the current document. They are removed from the resolver before a
	// fresh extraction on RebuildRenderTree, preventing stale rules from
	// accumulating when style content changes dynamically.
	styleSheets []*css.CSSStyleSheet

	// ScriptEngine is an optional callback for executing JavaScript. When set,
	// executeInlineScripts calls it for each inline <script> element found in
	// the document. The WebView sets this to its EvalJS method so that inline
	// scripts run after the document is parsed and styles are extracted.
	ScriptEngine func(code string) error

	// StyleSheetLoader is an optional callback for loading external stylesheets
	// referenced by <link rel="stylesheet" href="..."> elements. It receives the
	// href URL and should return the CSS text, or an error. When nil, external
	// stylesheets are silently skipped UNLESS a ResourceLoader is set. The WebView
	// sets this to a file-based loader that reads from the local filesystem.
	StyleSheetLoader func(href string) (string, error)

	// ScriptLoader is an optional callback for loading external scripts
	// referenced by <script src="..."> elements. It receives the src URL and
	// should return the JS text, or an error. When nil AND ResourceLoader is
	// also nil, external scripts are silently skipped. The WebView sets this
	// to a file-based loader that reads from the local filesystem, matching
	// the same pattern as StyleSheetLoader.
	ScriptLoader func(src string) (string, error)

	ResourceLoader *CachedResourceLoader

	// needsRenderTreeRebuild is set by MarkRenderTreeDirty when DOM mutations
	// (appendChild / removeChild / style changes) occur. The flag is flushed
	// at the start of the next layout pass, batching multiple mutations into
	// a single full render-tree rebuild.
	needsRenderTreeRebuild bool

	// rebuildCooldown 是重建降频计数器（变更风暴防护）：xterm 等库渲染
	// 大量 DOM（每块 PTY 输出 appendChild 数百 span）时每帧 MarkRenderTreeDirty
	// → 每帧全树重建（复杂页面 100-200ms）→ 主循环卡死（「频繁无响应」）。
	// MarkRenderTreeDirty 重置 cooldown=2：连续变更期间只推迟重建（保持
	// dirty），变更停歇 2 帧后才重建一次——输出风暴合并为一次重建。
	// GetElementBoxRect 等强制路径直接调 RebuildRenderTree（绕过 cooldown）。
	rebuildCooldown int

	// lastTreeDirtyAt 上次 MarkRenderTreeDirty 时刻：两次变更间隔超过
	// treeDirtyBurstWindow 说明是低频变化（时钟/歌单每秒文本更新），
	// 非变更风暴——立即重建（cooldown=0），否则低频更新被 cooldown
	// 无限推迟（每秒 1 次时 cooldown 永不耗尽，渲染树永停留旧结构）。
	lastTreeDirtyAt time.Time

	// styleFP caches the fingerprint of all <style> textContent + <link> href
	// seen at the last style extraction. RebuildRenderTree skips the expensive
	// full style re-extraction (re-parsing the whole Vue bundle CSS) when the
	// fingerprint is unchanged — pure DOM mutations (workspace switch, list
	// updates) never touch <style>, so re-parsing thousands of rules on every
	// rebuild is pure waste (measured: tens of ms on each rebuild).
	styleFP string
}

// NewFrame constructs a Frame attached to the given Page. The frame is given a
// default 800x600 FrameView so that LoadHTML followed by Layout works without
// further setup, mirroring how WebKit creates a LocalFrameView for a new
// LocalFrame.
func NewFrame(page *Page) *Frame {
	f := &Frame{page: page}
	f.view = NewFrameView(f, 1280, 800)
	return f
}

// Page returns the owning Page, mirroring Frame::page().
func (f *Frame) Page() *Page { return f.page }

// Document returns the currently-loaded Document, mirroring
// LocalFrame::document(). It returns nil until LoadHTML or SetDocument is called.
func (f *Frame) Document() *dom.Document { return f.document }

// RenderView returns the root render object for the frame's document, mirroring
// the render view obtained via FrameView::renderView() in WebKit.
func (f *Frame) RenderView() *rendering.RenderView { return f.renderView }

// View returns the FrameView, mirroring LocalFrame::view().
func (f *Frame) View() *FrameView { return f.view }

// Resolver returns the style resolver used to build this frame's render tree.
// Callers may add style sheets to it before (re)loading a document so that the
// next render-tree build takes them into account.
func (f *Frame) Resolver() *style.Resolver { return f.resolver }

// SetView installs a new FrameView, mirroring LocalFrame::setView(). The previous
// view is replaced; the new view's frame pointer is set to this frame.
func (f *Frame) SetView(v *FrameView) {
	f.view = v
	if v != nil {
		v.frame = f
	}
}

// LoadHTML parses an HTML source string, creates a Document and builds the render
// tree for it. It is the Go translation of the WebKit load pipeline
// (FrameLoader::load -> HTMLDocumentParser -> Document -> attachRenderTree) for the
// common case of loading an inline HTML string. After it returns, Document() and
// RenderView() are populated and the frame's view is marked as needing layout.
func (f *Frame) LoadHTML(src string) error {
	if src == "" {
		return errors.New("page: empty HTML input")
	}
	Logf("LoadHTML", "start input_size=%d", len(src))
	doc, err := html.Parse(src)
	if err != nil {
		return err
	}
	Logf("LoadHTML", "parsed tagCount=%d elementCount=%d", countTags(doc), len(doc.GetElementsByTagName("*")))
	f.SetDocument(doc)
	return nil
}

// SetDocument installs a Document on the frame and (re)builds the render tree,
// mirroring the document-attach step of the load pipeline
// (FrameLoader::clear / setDocument -> FrameView::setContents). It creates a style
// resolver if none exists yet, extracts CSS from <style> elements, and constructs
// the render tree via rendering.RenderTreeBuilder. The Page's main-frame RenderView
// is kept in sync.
func (f *Frame) SetDocument(doc *dom.Document) {
	Logf("SetDocument", "start hasResolver=%v", f.resolver != nil)
	f.document = doc
	if f.resolver == nil {
		f.resolver = style.NewResolver()
		f.resolver.AddStyleSheet(html5.NewUAStyleSheet())
		Logf("SetDocument", "created new resolver + UA sheet")
	} else {
		f.resolver.ClearCache()
		Logf("SetDocument", "cleared resolver cache")
	}

	f.extractAndAddStyles()
	// ★ 指纹同步：SetDocument 已按当前文档提取过样式，这里记下指纹，
	//   否则紧随其后的首次 RebuildRenderTree 会判定「指纹变化」（styleFP
	//   初始为空）而**再全量重扫一次**——<style> 重复解析、<link> 重复
	//   加载（宿主 StyleSheetLoader / ResourceResolver 被重复调用）。
	f.styleFP = f.styleFingerprint()
	f.syncMediaQueryViewport()
	builder := rendering.NewRenderTreeBuilder(f.resolver)
	f.renderView = builder.Build(doc)
	objCount := 0
	if f.renderView != nil {
		objCount = countRenderObjects(rendering.RenderObject(f.renderView))
	}
	Logf("SetDocument", "renderObjectCount=%d", objCount)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	Logf("SetDocument", "done")
}

// ExecuteScripts walks the document's <script> elements and executes both
// inline and external scripts via the ScriptEngine callback. Callers should
// invoke this AFTER RegisterDOMBindings so the JS runtime has access to the
// complete DOM API (document / window / Element etc.).
func (f *Frame) ExecuteScripts() {
	f.executeInlineScripts()
}

// RebuildRenderTree reconstructs the render tree from the current DOM without
// re-parsing stylesheets. This mirrors the style-recalc + attach phase in
// WebKit (Document::resolveStyle -> Style::TreeResolver::createRenderTree) that
// fires after DOM mutations outside of parsing. Embedders should call this
// after mutating the DOM (SetTextContent / SetAttribute / AppendChild) so the
// next layout pass sees an up-to-date render tree. It also clears the style
// resolver cache so new/changed inline styles take effect.
func (f *Frame) RebuildRenderTree() {
	Logf("RebuildRenderTree", "start hasDocument=%v hasResolver=%v", f.document != nil, f.resolver != nil)
	if f.document == nil {
		Logf("RebuildRenderTree", "skip: no document")
		return
	}
	// ★ style 指纹缓存：<style>/<link> 内容未变时跳过 extractAndAddStyles +
	//   resolver.ClearCache（全量重扫+重解析整个 Vue bundle CSS）。
	//   切换工作区等纯 DOM 变化场景 style 未变，跳过可省几十 ms/次重建。
	//   styleFP 为空表示从未提取过（首次必须提取）。
	if f.resolver != nil {
		fp := f.styleFingerprint()
		if fp != f.styleFP {
			f.extractAndAddStyles()
			f.resolver.ClearCache()
			f.styleFP = fp
		} else {
			Logf("RebuildRenderTree", "style fingerprint unchanged, skip re-extract (len=%d)", len(fp))
		}
	}
	f.syncMediaQueryViewport()
	builder := rendering.NewRenderTreeBuilder(f.resolver)
	oldRV := f.renderView
	f.renderView = builder.Build(f.document)
	objCount := 0
	if f.renderView != nil {
		// The rebuilt tree has brand-new RenderBox objects and an empty
		// scroll-offset map. Carry per-box scroll offsets across by DOM
		// node so a keystroke (which rebuilds the tree) does not reset
		// vertical scroll — otherwise auto-scroll yanks the content to
		// the caret row on every character ("content jumps out of view").
		before := 0
		if oldRV != nil {
			before = oldRV.ScrollOffsetCount()
		}
		f.renderView.RestoreScrollOffsetsFrom(oldRV)
		// ★ cursor 状态同 scroll offset 一样需要跨重建迁移：paintRangeSlider
		//   的 hover 分开（悬停圆 vs 悬停条）读 RenderView.CursorPos 判定。
		//   Rebuild 生成全新 RenderView（cursor 复位 0,0），若鼠标正悬停在
		//   range 上且 DOM 变化触发重建（拖动改 value 等），paint 会读到
		//   (0,0) → 误判「悬停到条」→ 圆不变暗。旧 cursor 一并带过去。
		if ox, oy := oldRV.CursorPos(); ox != 0 || oy != 0 {
			f.renderView.SetCursorPos(ox, oy)
		}
		if debugenv.Enabled("WB_SCROLL_DEBUG") {
			Logf("RebuildRenderTree", "migrated scroll offsets old=%d new=%d", before, f.renderView.ScrollOffsetCount())
		}
		objCount = countRenderObjects(rendering.RenderObject(f.renderView))
	}
	Logf("RebuildRenderTree", "renderObjectCount=%d", objCount)
	if f.page != nil {
		f.page.setRenderView(f.renderView)
	}
	// ★ 手动重建已完成：清除挂起标记（否则后续 Layout 的
	// RebuildRenderTreeIfNeeded 判定「重建被推迟」而 defer 布局——
	// 渲染树已新但布局永远不执行，画面停旧几何）。
	f.needsRenderTreeRebuild = false
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	Logf("RebuildRenderTree", "done")
}

// MarkRenderTreeDirty marks the frame as needing a render tree rebuild in the
// next layout. Multiple DOM mutations are batched into a single full rebuild.
// ★ 重建降频：连续 DOM 变更（xterm 输出风暴）期间延迟重建（cooldown），
// 避免每帧全树重建（100-200ms）卡死主循环。cooldown 只在首次 dirty 时
// 设置并逐帧递减（不因持续 dirty 无限刷新）——持续变更时每 cooldown+1
// 帧强制重建一次（内容周期性更新，不会永久停留在旧树）。
func (f *Frame) MarkRenderTreeDirty() {
	// ★ 只在「干净→脏」状态转换时更新 cooldown：一次高层变更（textContent
	// 赋值）内部会触发多次通知（RemoveChild/AppendChild 结构回调 + JS 侧
	// OnNodeInserted 桥回调），若每次通知都重设 cooldown=2，低频更新会被
	// 无限推迟（每秒 1 次 textContent 时 cooldown 永不耗尽 → 渲染树停留
	// 旧结构 → 时钟/歌单画面不刷新——widget 桥此前靠手动 RebuildRenderTree
	// 绕过，配置窗口 JS 交互同理）。已是脏态的通知直接合并（不重置）。
	if f.needsRenderTreeRebuild {
		return
	}
	f.needsRenderTreeRebuild = true
	// 距上次 dirty 超过 burst 窗口 = 低频变更（交互/定时更新）：立即重建
	//（cooldown=0）；窗口内 = 变更风暴（xterm 输出）→ 降频稀释。
	if time.Since(f.lastTreeDirtyAt) > treeDirtyBurstWindow {
		f.rebuildCooldown = 0
	} else {
		f.rebuildCooldown = 2
	}
	f.lastTreeDirtyAt = time.Now()
}

// treeDirtyBurstWindow 判定「变更风暴」的间隔阈值：两次脏转换间隔超过
// 该值视为低频交互/定时更新（立即重建）；否则合并进 cooldown 降频批次。
const treeDirtyBurstWindow = 100 * time.Millisecond

// RebuildRenderTreeIfNeeded rebuilds the render tree if MarkRenderTreeDirty was
// called since the last check. It returns true when a rebuild was performed.
// FrameView.Layout() calls this automatically before laying out.
// ★ cooldown>0 时保持 dirty 但不重建（变更风暴降频）；cooldown 耗尽后强制
// 重建——否则持续 DOM 变更（xterm 渲染）会无限推迟重建，渲染树停留在
// 旧结构（如 resize 前 24 行 → 光标布局在视口外不可见）。
func (f *Frame) RebuildRenderTreeIfNeeded() bool {
	if !f.needsRenderTreeRebuild {
		return false
	}
	if f.rebuildCooldown > 0 {
		f.rebuildCooldown--
		return false
	}
	f.needsRenderTreeRebuild = false
	f.RebuildRenderTree()
	return true
}

// FlushRenderTreeDirty 立即重建渲染树（忽略 rebuildCooldown 降频）——
// 命中测试等交互前需要最新树：JS 改 DOM（弹窗 display 等）后用户立即
// 点击，cooldown 会让树停留在旧结构 → 点击命中穿透。交互是低频操作，
// 立即重建开销可忽略；连续渲染仍走批量降频路径。
func (f *Frame) FlushRenderTreeDirty() {
	f.rebuildCooldown = 0
	f.RebuildRenderTreeIfNeeded()
}

// RebuildStyleForElement 增量更新单个元素的计算样式——拖拽热路径专用
// （textarea resize / range 拖动每帧触发 style 变更，全量 RebuildRenderTree
// 重建整棵渲染树在复杂页面下耗时 30ms+，是「拖拽不跟手」的主因）。
//
// style 属性变化通常只影响几何（height/width）：重新解析该元素的
// ComputedStyle 并同步到渲染树 RenderBox 与布局树 ElementBox，然后仅
// 标记需要布局（不重建渲染树）。若解析结果显示结构属性（display/
// position/float 等）变化——会改变渲染树形态——则回退到全量重建。
//
// 返回 true 表示已按增量路径处理；false 表示回退全量重建（调用方
// 需 MarkRenderTreeDirty）。找不到 RenderBox 或 resolver 缺失时同样回退。
func (f *Frame) RebuildStyleForElement(el *dom.Element) bool {
	if el == nil || f.renderView == nil || f.resolver == nil {
		return false
	}
	rb := f.renderView.FindRenderBoxForNode(el)
	if rb == nil {
		return false
	}
	cs := f.resolver.ResolveElement(el)
	if cs == nil {
		return false
	}
	// 结构属性变化必须全量重建渲染树（节点类型/子结构会变）。
	if old := rb.Style(); old != nil {
		if old.Display != cs.Display || old.Position != cs.Position ||
			old.Float != cs.Float || old.Clear != cs.Clear {
			return false
		}
	}
	rb.SetStyle(cs)
	if lb := rb.LayoutBox(); lb != nil {
		lb.SetStyle(cs)
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	return true
}

// ApplyTextChange 增量更新单个 Text 节点的内容——打字热路径专用
// （CM6 编辑器每敲一键触发 text data 变更，全量 RebuildRenderTree 重建
// 整棵渲染树 + 布局树 + 层树耗时 10ms+，是「打字卡顿」的主因）。
//
// 与 RebuildStyleForElement 同理：text 变更只影响所在 block 的 inline 布局，
// 重新同步 RenderText.text 与布局树 InlineTextBox.text，标记所在 block 的
// layout box dirty，然后仅标记需要布局（不重建渲染树）。
//
// 返回 true 表示已按增量路径处理；false 表示回退全量重建（调用方需
// MarkRenderTreeDirty）。找不到 RenderText / 所在 block 的 layout box 时回退。
func (f *Frame) ApplyTextChange(node dom.Node) bool {
	if node == nil || f.renderView == nil {
		return false
	}
	if !f.renderView.ApplyTextChange(node) {
		return false
	}
	if f.view != nil {
		f.view.SetNeedsLayout(true)
	}
	return true
}

// LayoutNow 强制立即布局（iframe 子文档绘制前调用；幂等——无待布局
// 标记时直接返回）。
func (f *Frame) LayoutNow() {
	if f.view != nil && f.view.NeedsLayout() {
		f.view.Layout()
	}
}

// syncMediaQueryViewport 把 FrameView 的视口尺寸交给样式解析器，供 @media
// 的 width/height 求值。必须在渲染树 Build（首次构建 / 重建都会重新解析
// 样式）**之前**调用：resolver 的媒体上下文默认是 0x0，不设置的话
// @media (min-width: …) 恒不匹配、@media (max-width: …) 恒匹配（0 <= 950），
// 即所有尺寸媒体查询都落在错误分支。SetViewportSize 在尺寸变化时会清
// ComputedStyle 缓存，因此重建后的解析必然使用新尺寸。
func (f *Frame) syncMediaQueryViewport() bool {
	if f.resolver == nil || f.view == nil {
		return false
	}
	return f.resolver.SetViewportSize(f.view.Width(), f.view.Height())
}

// ViewportWidth 返回子文档视口宽度（iframe 内容框宽度，CSS 像素）。
func (f *Frame) ViewportWidth() int {
	if f.view == nil {
		return 0
	}
	return f.view.Width()
}

// ViewportHeight 返回子文档视口高度（iframe 内容框高度，CSS 像素）。
func (f *Frame) ViewportHeight() int {
	if f.view == nil {
		return 0
	}
	return f.view.Height()
}

// NeedsLayout reports whether the frame's view requires a layout pass, mirroring
// the per-frame layout-pending check in WebKit (FrameView::needsLayout()).
func (f *Frame) NeedsLayout() bool {
	return f.view != nil && f.view.NeedsLayout()
}

// NeedsRenderTreeRebuild 报告是否有挂起的渲染树重建（含降频推迟中的）。
// 渲染循环（挂件/配置窗口）应在该标记或 NeedsLayout 成立时持续渲染——
// 否则重建被 cooldown 推迟的帧渲染循环早退，挂起重建永远不执行。
func (f *Frame) NeedsRenderTreeRebuild() bool {
	return f.needsRenderTreeRebuild
}

// SetNeedsLayout marks the frame's view as needing (or not needing) a layout
// pass, mirroring FrameView::setNeedsLayout(). Embedders should call this
// after mutating the DOM outside of a style recalc so the next EnsureLayout
// rebuilds/relayouts the render tree.
func (f *Frame) SetNeedsLayout(needs bool) {
	if f.view != nil {
		f.view.SetNeedsLayout(needs)
	}
}

// styleFingerprint 计算当前文档所有 <style> textContent 与 <link> href 的
// 组合指纹（FNV 哈希）。<style> 内容或外部样式表引用未变 → 指纹相同 →
// RebuildRenderTree 跳过全量重扫。Vue 打包的 scoped CSS 在构建时已固定，
// 运行时纯 DOM 变化（切换工作区/列表更新）不会改变 <style>。
func (f *Frame) styleFingerprint() string {
	if f.document == nil {
		return ""
	}
	h := fnv.New64a()
	var styleEls []*dom.Element
	dom.WalkComposedTree(f.document, func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && strings.EqualFold(el.LocalName(), "style") {
			styleEls = append(styleEls, el)
		}
	})
	for _, el := range styleEls {
		h.Write([]byte(el.TextContent()))
		h.Write([]byte{0})
	}
	linkEls := f.document.GetElementsByTagName("link")
	for _, el := range linkEls {
		if l, ok := html5.ToLinkElement(el); ok && l.IsStyleSheet() {
			h.Write([]byte(l.Href()))
			h.Write([]byte{0})
		}
	}
	return fmt.Sprintf("%x", h.Sum64())
}

// extractAndAddStyles finds all <style> elements in the current document,
// parses their CSS text, and adds the resulting stylesheets to the style
// resolver. Previously extracted stylesheets are removed first so that
// dynamically changed style content is reflected correctly.
func (f *Frame) extractAndAddStyles() {
	Logf("extractAndAddStyles", "start docOk=%v resolverOk=%v", f.document != nil, f.resolver != nil)
	if f.document == nil || f.resolver == nil {
		Logf("extractAndAddStyles", "skip: no doc/resolver")
		return
	}

	removed := len(f.styleSheets)
	for _, sheet := range f.styleSheets {
		f.resolver.RemoveStyleSheet(sheet)
	}
	f.styleSheets = nil
	Logf("extractAndAddStyles", "removedPrevSheets=%d", removed)
	// Keep Vue scoped [data-v-...] selectors as-is: the DOM elements carry the
	// matching data-v attribute (Vue 3.5 __scopeId lands on the element), so
	// stripping them flattened every scoped rule to global and leaked
	// same-name classes across components (e.g. PlanView's .plan-empty
	// margin-top:40px onto RightPanel's plan-container).
	// ★ 用 WalkComposedTree 而非 GetElementsByTagName：后者只遍历 light DOM，
	//   shadow tree 内的 <style> 会被漏掉。WalkComposedTree 进入 shadow tree，
	//   使 shadow 内样式表也能被提取（其作用域由 style 层的 scoping root 判定）。
	var styleElements []*dom.Element
	dom.WalkComposedTree(f.document, func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && strings.EqualFold(el.LocalName(), "style") {
			styleElements = append(styleElements, el)
		}
	})
	Logf("extractAndAddStyles", "styleElementCount=%d", len(styleElements))
	for i, styleEl := range styleElements {
		cssText := styleEl.TextContent()
		if strings.TrimSpace(cssText) == "" {
			Logf("extractAndAddStyles", "style[%d]: empty, skip", i)
			continue
		}
		sheet := css.NewCSSStyleSheetWithOwner(styleEl, "")
		p := css.NewParser(cssText)
		p.ParseStyleSheetInto(sheet)
		rules := sheet.Rules()
		f.resolver.AddStyleSheet(sheet)
		f.styleSheets = append(f.styleSheets, sheet)
		Logf("extractAndAddStyles", "style[%d]: cssLen=%d rules=%d", i, len(cssText), len(rules))
	}

	linkElements := f.document.GetElementsByTagName("link")
	linkCount := 0
	for _, linkEl := range linkElements {
		l, ok := html5.ToLinkElement(linkEl)
		if !ok || !l.IsStyleSheet() {
			continue
		}
		href := l.Href()
		if href == "" {
			continue
		}
		linkCount++
		Logf("extractAndAddStyles", "link[%d]: href=%q", linkCount, href)
		if f.ResourceLoader != nil {
			f.ResourceLoader.LoadStylesheet(href, &frameStyleSheetClient{
				frame: f,
				owner: linkEl,
				href:  href,
			})
			Logf("extractAndAddStyles", "link[%d]: async load queued", linkCount)
		} else if f.StyleSheetLoader != nil {
			cssText, err := f.StyleSheetLoader(href)
			if err != nil || strings.TrimSpace(cssText) == "" {
				Logf("extractAndAddStyles", "link[%d]: load failed err=%v", linkCount, err)
				continue
			}
			sheet := css.NewCSSStyleSheetWithOwner(linkEl, href)
			p := css.NewParser(cssText)
			p.ParseStyleSheetInto(sheet)
			f.resolver.AddStyleSheet(sheet)
			f.styleSheets = append(f.styleSheets, sheet)
			Logf("extractAndAddStyles", "link[%d]: sync loaded len=%d", linkCount, len(cssText))
		}
	}
	Logf("extractAndAddStyles", "done: totalStyleSheets=%d linkCount=%d", len(f.styleSheets), linkCount)
}

// frameStyleSheetClient implements CachedResourceClient to handle the
// asynchronous delivery of an externally loaded stylesheet.
type frameStyleSheetClient struct {
	frame *Frame
	owner dom.Node
	href  string
}

func (c *frameStyleSheetClient) NotifyFinished(resource *CachedResource) {
	if resource.Status() != CachedResourceStatusLoaded {
		Logf("extractAndAddStyles", "async: href=%q status=%d (not loaded)", c.href, resource.Status())
		return
	}
	data := resource.Data()
	if len(data) == 0 {
		Logf("extractAndAddStyles", "async: href=%q empty data", c.href)
		return
	}
	cssText := string(data)
	if strings.TrimSpace(cssText) == "" {
		Logf("extractAndAddStyles", "async: href=%q whitespace only", c.href)
		return
	}

	sheet := css.NewCSSStyleSheetWithOwner(c.owner, c.href)
	p := css.NewParser(cssText)
	p.ParseStyleSheetInto(sheet)
	rules := sheet.Rules()
	c.frame.resolver.AddStyleSheet(sheet)
	c.frame.styleSheets = append(c.frame.styleSheets, sheet)
	Logf("extractAndAddStyles", "async: href=%q loaded len=%d rules=%d", c.href, len(cssText), len(rules))

	if c.frame.renderView != nil {
		c.frame.view.SetNeedsLayout(true)
		Logf("extractAndAddStyles", "async: setNeedsLayout")
	}
}

// executeInlineScripts finds all <script> elements and executes them.
// External scripts are loaded via ScriptLoader or ResourceLoader.
// Inline scripts are executed directly — NO try/catch wrapper (real errors
// propagate to the frame's error log).
func (f *Frame) executeInlineScripts() {
	if f.document == nil || f.ScriptEngine == nil {
		return
	}
	scriptElements := f.document.GetElementsByTagName("script")
	Logf("ScriptLoad", "start: scripts=%d", len(scriptElements))
	for i, el := range scriptElements {
		s, ok := html5.ToScriptElement(el)
		if !ok {
			continue
		}
		t := s.Type()
		if t != "" && t != "text/javascript" && t != "module" {
			Logf("ScriptLoad", "[%d] skip: type=%q", i, t)
			continue
		}

		if src := s.Src(); src != "" {
			Logf("ScriptLoad", "[%d] external: src=%q type=%q", i, src, t)
			if f.ResourceLoader != nil {
				f.ResourceLoader.LoadScript(src, &frameScriptClient{
					frame: f,
					src:   src,
					el:    el,
				})
				Logf("ScriptLoad", "[%d] async queued", i)
				continue
			}
			if f.ScriptLoader != nil {
				code, err := f.ScriptLoader(src)
				if err != nil {
					Logf("ScriptLoad", "[%d] LOAD FAIL: %v", i, err)
					continue
				}
				if strings.TrimSpace(code) == "" {
					Logf("ScriptLoad", "[%d] empty content", i)
					continue
				}
				Logf("ScriptLoad", "[%d] exec: src=%q len=%d", i, src, len(code))
				// Execute directly — no try/catch wrapper.
				if err := f.runScriptForElement(el, code); err != nil {
					Logf("ScriptLoad", "[%d] EXEC FAIL: %v", i, err)
				} else {
					Logf("ScriptLoad", "[%d] OK", i)
				}
				continue
			}
			Logf("ScriptLoad", "[%d] no loader, skip", i)
			continue
		}

		// Inline script: execute directly.
		code := s.Text()
		if strings.TrimSpace(code) == "" {
			Logf("ScriptLoad", "[%d] inline empty", i)
			continue
		}
		Logf("ScriptLoad", "[%d] inline: len=%d", i, len(code))
		if err := f.runScriptForElement(el, code); err != nil {
			Logf("ScriptLoad", "[%d] INLINE FAIL: %v", i, err)
		} else {
			Logf("ScriptLoad", "[%d] inline OK", i)
		}
	}
	Logf("ScriptLoad", "done")
}

// runScriptForElement 执行一个 <script> 的代码，并在执行期间把
// document.currentScript 指向该元素（HTML §4.12.1：脚本执行期间
// currentScript 指向该元素，执行结束后恢复）。React 19 的水合流程据此定位
// 正在执行的宿主脚本——读到 undefined 时 `head.appendChild(undefined)`
// 抛错，整个脚本中断（页面上看不到任何错误，只是后续语句不再执行）。
//
// 嵌套调用（脚本内动态插入并执行脚本）时恢复上一层元素而非直接清空。
func (f *Frame) runScriptForElement(el *dom.Element, code string) error {
	if f.ScriptEngine == nil {
		return nil
	}
	prev := bindings.CurrentScriptElement
	bindings.CurrentScriptElement = el
	err := f.ScriptEngine(code)
	bindings.CurrentScriptElement = prev
	return err
}

// frameScriptClient implements CachedResourceClient for async script loading.
type frameScriptClient struct {
	frame *Frame
	src   string
	// el 是加载中的 <script> 元素：异步脚本执行期间 document.currentScript
	// 必须指向它（与同步路径 runScriptForElement 一致）。
	el *dom.Element
}

func (c *frameScriptClient) NotifyFinished(resource *CachedResource) {
	if resource.Status() != CachedResourceStatusLoaded {
		Logf("ScriptLoad", "async FAIL: src=%q status=%d", c.src, resource.Status())
		return
	}
	data := resource.Data()
	if len(data) == 0 {
		Logf("ScriptLoad", "async empty: src=%q", c.src)
		return
	}
	code := string(data)
	if strings.TrimSpace(code) == "" {
		Logf("ScriptLoad", "async whitespace: src=%q", c.src)
		return
	}
	Logf("ScriptLoad", "async exec: src=%q len=%d", c.src, len(code))
	// ★ 走 runScriptForElement 而不是直接 ScriptEngine：外部脚本是异步执行的，
	// 期间 currentScript 必须指向该 <script>（React 19 的水合流程据此定位宿主
	// 脚本；读到 undefined 时 appendChild(undefined) 抛错、整段脚本中断）。
	var err error
	if c.el != nil {
		err = c.frame.runScriptForElement(c.el, code)
	} else {
		err = c.frame.ScriptEngine(code)
	}
	if err != nil {
		Logf("ScriptLoad", "async EXEC FAIL: src=%q %v", c.src, err)
	} else {
		Logf("ScriptLoad", "async OK: src=%q", c.src)
	}
}

// logError logs script/resource errors (legacy; prefer Logf).
func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[page] "+format+"\n", args...)
}

// Layout triggers a layout on the frame's view.
func (f *Frame) Layout() {
	if f.view != nil {
		f.view.Layout()
	}
}

// ─── Verbose logging helpers ─────────────────────────────

func countTags(doc *dom.Document) int {
	count := 0
	walk := func(n dom.Node) {
		if _, ok := n.(*dom.Element); ok {
			count++
		}
	}
	walkNode(doc, walk)
	return count
}

func countRenderObjects(ro rendering.RenderObject) int {
	count := 0
	var walkRO func(r rendering.RenderObject)
	walkRO = func(r rendering.RenderObject) {
		count++
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			walkRO(c)
		}
	}
	walkRO(ro)
	return count
}

func walkNode(n dom.Node, fn func(dom.Node)) {
	fn(n)
	switch v := n.(type) {
	case *dom.Document:
		if el := v.DocumentElement(); el != nil {
			walkNode(el, fn)
		}
		if b := v.Head(); b != nil {
			walkNode(b, fn)
		}
		if b := v.Body(); b != nil {
			walkNode(b, fn)
		}
	case *dom.Element:
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			walkNode(c, fn)
		}
	}
}

func trimForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
