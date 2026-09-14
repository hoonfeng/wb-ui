// Package bindings — 媒体文本轨道模型（HTML §4.8.11）。
//
// 覆盖：<track> → HTMLTrackElement.track（TextTrack）、HTMLMediaElement.textTracks
// （live TextTrackList）、TextTrack.cues（TextTrackCueList）、VTTCue，
// 以及 <track src="data:..."> 的 WebVTT 加载与 load/error 事件。
//
// 标识语义（框架与测试都依赖，也是 media-text-track 夹具的断言点）：
//   - 同一 <track> 元素始终返回同一个 TextTrack 对象
//   - video.textTracks[i] 与对应 <track> 元素的 .track 是同一对象
//   - video.textTracks 的 length/索引是 live 的（按子 <track> 元素顺序）
//
// 加载语义：data: URL 视为本地即时资源，在首次访问 .track 时同步解析完成
// （规范上资源加载是异步的，但同步解析对调用方是等价或更强的保证：脚本在
// 首个同步阶段就能读到 cues）；load / error 事件按规范异步派发（经
// EventLoop 宏任务）。非 data: URL 的资源没有网络栈，直接派发 error。
package bindings

import (
	"strconv"
	"strings"
	"sync"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// textTrackState 是 <track> 元素对应的 TextTrack 状态（每个元素一份）。
type textTrackState struct {
	interp  *jsc.Interpreter
	el      *dom.Element
	obj     *jsc.JSObject // TextTrack
	list    *jsc.JSObject // TextTrackCueList
	cues    []*jsc.JSObject
	count   int // list 上已设置的索引个数（清理多余索引用）
	loaded  bool
	loading bool
	failed  bool
	// regions 是该轨道 <track> 资源里 REGION 块解析出的区域（cue.region 绑定用）。
	regions []vttRegion

	kind     string
	label    string
	language string
	id       string
	mode     string
}

// textTrackListState 是 <video>/<audio> 的 TextTrackList（live 集合）状态。
type textTrackListState struct {
	interp *jsc.Interpreter
	el     *dom.Element
	obj    *jsc.JSObject
	count  int
}

// mediaProtoset 是每个解释器一份的媒体原型集合（跨 runtime 共享 goja 对象
// 会触发 "Illegal runtime transition"）。
type mediaProtoset struct {
	track     *jsc.JSObject
	cue       *jsc.JSObject
	cueList   *jsc.JSObject
	trackList *jsc.JSObject
	region    *jsc.JSObject
}

var (
	trackCacheMu   sync.Mutex
	trackCache     = map[*dom.Element]*textTrackState{}
	trackListCache = map[*dom.Element]*textTrackListState{}

	mediaProtoMu sync.Mutex
	mediaProtos  = map[*jsc.Interpreter]*mediaProtoset{}
)

// protosetFor 取该解释器的媒体原型集合。
func protosetFor(in *jsc.Interpreter) *mediaProtoset {
	mediaProtoMu.Lock()
	defer mediaProtoMu.Unlock()
	return mediaProtos[in]
}

// clearMediaCachesFor 清除属于指定文档（或指定解释器）的媒体缓存。
// WebView.Destroy → ClearPageBindingsFor 调用，避免旧文档/解释器被缓存引用。
func clearMediaCachesFor(interp *jsc.Interpreter, doc *dom.Document) {
	trackCacheMu.Lock()
	for el := range trackCache {
		if (doc != nil && el.OwnerDocument() == doc) || (doc == nil && interp == nil) {
			delete(trackCache, el)
		}
	}
	for el := range trackListCache {
		if (doc != nil && el.OwnerDocument() == doc) || (doc == nil && interp == nil) {
			delete(trackListCache, el)
		}
	}
	trackCacheMu.Unlock()

	if interp != nil {
		mediaProtoMu.Lock()
		delete(mediaProtos, interp)
		mediaProtoMu.Unlock()
	}

	// 媒体元素状态（engine/js/bindings/media_element.go：HTMLMediaElement 家族）同样按
	// 文档清理——否则旧文档的元素会一直被状态表引用。
	clearMediaElementCacheFor(doc)
}

// ─── 注册（构造器 + 原型）────────────────────────────────

// registerMediaTypes 注册 TextTrack / VTTCue / TextTrackCueList / TextTrackList
// 构造器与原型（由 RegisterDOMBindings 调用）。
func registerMediaTypes(rt *jsc.Interpreter, g *jsc.JSObject) {
	set := &mediaProtoset{}
	mediaProtoMu.Lock()
	mediaProtos[rt] = set
	mediaProtoMu.Unlock()

	// ── VTTCue ──
	cueCtor := rt.NewConstructor("VTTCue", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		cue := vttCue{}
		if len(args) > 0 {
			cue.Start = args[0].ToNumber()
		}
		if len(args) > 1 {
			cue.End = args[1].ToNumber()
		}
		if len(args) > 2 {
			cue.Text = args[2].ToString()
		}
		return newVTTCueObject(in, set, cue, nil)
	})
	set.cue = jsc.FunctionValue(cueCtor).AsObject().GetStr("prototype").AsObject()
	g.Set("VTTCue", jsc.FunctionValue(cueCtor))
	// VTTCue.prototype.getCueAsHTML()：按 WebVTT §6.4 把 cue 正文（含内嵌标记）
	// 解析为 DocumentFragment。+ toString() 返回原始 cue 文本（规范）。
	set.cue.Set("getCueAsHTML", jsc.FunctionValue(jsc.NewNativeFunction("getCueAsHTML",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			o := this.AsObject()
			if o == nil {
				return jsc.Undefined()
			}
			text := ""
			if v, ok := o.GetByKey("text"); ok {
				text = v.ToString()
			}
			doc := documentOfInterpreter(in)
			if doc == nil {
				return jsc.Undefined()
			}
			return jsc.ObjectValue(wrapDocFrag(in, buildCueFragment(doc, parseVTTCueMarkup(text))))
		}, 0)))
	set.cue.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			o := this.AsObject()
			if o == nil {
				return jsc.StringValue("")
			}
			if v, ok := o.GetByKey("text"); ok {
				return jsc.StringValue(v.ToString())
			}
			return jsc.StringValue("")
		}, 0)))

	// ── VTTRegion（WebVTT §4.4）──
	regionCtor := rt.NewConstructor("VTTRegion", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		return newVTTRegionObject(in, set, vttRegion{
			Width: 100, Lines: 3, RegionAnchorY: 100, ViewportAnchorY: 100,
		})
	})
	set.region = jsc.FunctionValue(regionCtor).AsObject().GetStr("prototype").AsObject()
	g.Set("VTTRegion", jsc.FunctionValue(regionCtor))

	// ── TextTrackCueList（不可 new，仅用于原型链与 instanceof）──
	cueListCtor := rt.NewConstructor("TextTrackCueList", func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		o := this.AsObject()
		if o == nil {
			o = jsc.NewObject(nil)
		}
		return o
	})
	set.cueList = jsc.FunctionValue(cueListCtor).AsObject().GetStr("prototype").AsObject()
	g.Set("TextTrackCueList", jsc.FunctionValue(cueListCtor))
	set.cueList.Set("getCueById", jsc.FunctionValue(jsc.NewNativeFunction("getCueById",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := textTrackStateOf(this)
			if st == nil || len(args) == 0 {
				return jsc.Null()
			}
			want := args[0].ToString()
			for _, cue := range st.cues {
				if v, ok := cue.GetByKey("id"); ok && v.ToString() == want {
					return jsc.ObjectValue(cue)
				}
			}
			return jsc.Null()
		}, 1)))

	// ── TextTrackList（不可 new，live 集合）──
	trackListCtor := rt.NewConstructor("TextTrackList", func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		o := this.AsObject()
		if o == nil {
			o = jsc.NewObject(nil)
		}
		return o
	})
	set.trackList = jsc.FunctionValue(trackListCtor).AsObject().GetStr("prototype").AsObject()
	g.Set("TextTrackList", jsc.FunctionValue(trackListCtor))
	set.trackList.Set("getTrackById", jsc.FunctionValue(jsc.NewNativeFunction("getTrackById",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := textTrackListStateOf(this)
			if st == nil || len(args) == 0 {
				return jsc.Null()
			}
			want := args[0].ToString()
			for _, child := range st.el.ChildNodes() {
				el, ok := child.(*dom.Element)
				if !ok || el.LocalName() != "track" {
					continue
				}
				obj := textTrackForElement(st.interp, el)
				if obj != nil && el.GetAttribute("id") == want {
					return jsc.ObjectValue(obj)
				}
			}
			return jsc.Null()
		}, 1)))

	// ── TextTrack（可 new：脚本可自建轨道，规范允许）──
	trackCtor := rt.NewConstructor("TextTrack", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		st := &textTrackState{interp: in, mode: "disabled", kind: "subtitles"}
		st.buildObjects(set, in)
		return st.obj
	})
	set.track = jsc.FunctionValue(trackCtor).AsObject().GetStr("prototype").AsObject()
	g.Set("TextTrack", jsc.FunctionValue(trackCtor))
	set.track.Set("addCue", jsc.FunctionValue(jsc.NewNativeFunction("addCue",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := textTrackStateOf(this)
			if st == nil || len(args) == 0 {
				return jsc.Undefined()
			}
			if cue := args[0].AsObject(); cue != nil {
				st.cues = append(st.cues, cue)
				st.refreshCueList()
			}
			return jsc.Undefined()
		}, 1)))
	set.track.Set("removeCue", jsc.FunctionValue(jsc.NewNativeFunction("removeCue",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := textTrackStateOf(this)
			if st == nil || len(args) == 0 {
				return jsc.Undefined()
			}
			// 按 JS 对象标识比较（*JSObject 是包装器，指针不可用于身份判断）
			target := args[0]
			for i, cue := range st.cues {
				if target.SameAs(jsc.ObjectValue(cue)) {
					st.cues = append(st.cues[:i], st.cues[i+1:]...)
					st.refreshCueList()
					break
				}
			}
			return jsc.Undefined()
		}, 1)))
}

// ─── 构造对象 ────────────────────────────────────────────

// newVTTCueObject 构造一条 VTTCue：规范默认值 + WebVTT settings 覆盖。
// regions 是该轨道解析出的 REGION 块（settings 里的 region:<id> 会绑定到对应
// 的 VTTRegion 对象；id 未声明时按规范保留为 null）。
func newVTTCueObject(in *jsc.Interpreter, set *mediaProtoset, cue vttCue, regions []vttRegion) *jsc.JSObject {
	var proto *jsc.JSObject
	if set != nil {
		proto = set.cue
	}
	obj := jsc.NewObject(proto)
	// 规范默认值（VTTCue IDL）
	obj.Set("id", jsc.StringValue(cue.ID))
	obj.Set("startTime", jsc.NumberValue(cue.Start))
	obj.Set("endTime", jsc.NumberValue(cue.End))
	obj.Set("pauseOnExit", jsc.BooleanValue(false))
	obj.Set("line", jsc.StringValue("auto"))
	obj.Set("lineAlign", jsc.StringValue("start"))
	obj.Set("position", jsc.StringValue("auto"))
	obj.Set("positionAlign", jsc.StringValue("auto"))
	obj.Set("size", jsc.NumberValue(100))
	obj.Set("align", jsc.StringValue("center"))
	obj.Set("vertical", jsc.StringValue(""))
	obj.Set("snapToLines", jsc.BooleanValue(true))
	obj.Set("region", jsc.Null())
	obj.Set("text", jsc.StringValue(cue.Text))
	obj.SetClassName("VTTCue")
	applyVTTSettings(obj, cue.Settings)
	if id, ok := cue.Settings["region"]; ok && id != "" {
		var regionObj *jsc.JSObject
		for i := range regions {
			if regions[i].ID == id {
				regionObj = newVTTRegionObject(in, set, regions[i])
				break
			}
		}
		if regionObj != nil {
			obj.Set("region", jsc.ObjectValue(regionObj))
		}
	}
	return obj
}

// newVTTRegionObject 构造一个 VTTRegion 对象（属性按 WebVTT §4.4 命名）。
func newVTTRegionObject(in *jsc.Interpreter, set *mediaProtoset, r vttRegion) *jsc.JSObject {
	var proto *jsc.JSObject
	if set != nil {
		proto = set.region
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("VTTRegion")
	obj.Set("id", jsc.StringValue(r.ID))
	obj.Set("width", jsc.NumberValue(r.Width))
	obj.Set("lines", jsc.NumberValue(r.Lines))
	obj.Set("regionAnchorX", jsc.NumberValue(r.RegionAnchorX))
	obj.Set("regionAnchorY", jsc.NumberValue(r.RegionAnchorY))
	obj.Set("viewportAnchorX", jsc.NumberValue(r.ViewportAnchorX))
	obj.Set("viewportAnchorY", jsc.NumberValue(r.ViewportAnchorY))
	obj.Set("scroll", jsc.StringValue(r.Scroll))
	return obj
}

// buildCueFragment 把 cue 正文节点树构造成 DocumentFragment：文本节点直挂，
// 标签节点按映射树创建元素（span 带 class/title/lang）。
func buildCueFragment(doc *dom.Document, nodes []*vttNode) *dom.DocumentFragment {
	frag := doc.CreateDocumentFragment()
	var add func(parent dom.Node, list []*vttNode)
	add = func(parent dom.Node, list []*vttNode) {
		for _, n := range list {
			if n.Name == "" {
				parent.AppendChild(doc.CreateTextNode(n.Text))
				continue
			}
			el := doc.CreateElement(n.Tag)
			for k, v := range n.Attrs {
				el.SetAttribute(k, v)
			}
			parent.AppendChild(el)
			add(el, n.Children)
		}
	}
	add(frag, nodes)
	return frag
}

// documentOfInterpreter 取解释器全局的 document（getCueAsHTML 需要建 DOM）。
func documentOfInterpreter(in *jsc.Interpreter) *dom.Document {
	if in == nil {
		return nil
	}
	g := in.GlobalObject()
	if g == nil {
		return nil
	}
	docVal := g.GetStr("document")
	if docVal.IsUndefined() {
		return nil
	}
	n := unwrapNode(docVal)
	if d, ok := n.(*dom.Document); ok {
		return d
	}
	return nil
}

// applyVTTSettings 把时间行的 settings 写进 cue 对象（数值类尽量转数字，
// 便于脚本直接做算术比较；解析失败则保留原字符串）。
func applyVTTSettings(obj *jsc.JSObject, settings map[string]string) {
	if len(settings) == 0 {
		return
	}
	numOr := func(obj *jsc.JSObject, key, raw string) {
		if n, err := strconv.ParseFloat(strings.TrimSuffix(raw, "%"), 64); err == nil {
			obj.Set(key, jsc.NumberValue(n))
			return
		}
		obj.Set(key, jsc.StringValue(raw))
	}
	if v, ok := settings["line"]; ok {
		numOr(obj, "line", v)
	}
	if v, ok := settings["position"]; ok {
		numOr(obj, "position", v)
	}
	if v, ok := settings["size"]; ok {
		numOr(obj, "size", v)
	}
	if v, ok := settings["align"]; ok {
		obj.Set("align", jsc.StringValue(v))
	}
	if v, ok := settings["vertical"]; ok {
		obj.Set("vertical", jsc.StringValue(v))
	}
	if v, ok := settings["lineAlign"]; ok {
		obj.Set("lineAlign", jsc.StringValue(v))
	}
	if v, ok := settings["positionAlign"]; ok {
		obj.Set("positionAlign", jsc.StringValue(v))
	}
	// region 的处理见 newVTTCueObject：要绑定到声明过的 VTTRegion 对象（规范）。
}

// buildObjects 创建 TextTrack 与其 TextTrackCueList 的对外对象，并挂上属性
// 访问器（accessor 闭包捕获 st，因此读到的永远是当前状态——live 语义）。
func (st *textTrackState) buildObjects(set *mediaProtoset, in *jsc.Interpreter) {
	var trackProto, listProto *jsc.JSObject
	if set != nil {
		trackProto, listProto = set.track, set.cueList
	}
	list := jsc.NewObject(listProto)
	list.SetInternal(st)
	list.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(st.cues)))
	}), nil)
	st.list = list

	obj := jsc.NewObject(trackProto)
	obj.SetInternal(st)
	obj.SetClassName("TextTrack")
	obj.SetAccessor("kind", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(st.kind)
	}), nil)
	obj.SetAccessor("label", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(st.label)
	}), nil)
	obj.SetAccessor("language", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(st.language)
	}), nil)
	obj.SetAccessor("id", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(st.id)
	}), nil)
	obj.SetAccessor("mode",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(st.mode) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { st.mode = v.ToString() })
	obj.SetAccessor("cues", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(st.list)
	}), nil)
	obj.SetAccessor("activeCues", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(st.list)
	}), nil)
	// readyState：0 NONE / 1 LOADING / 2 LOADED / 3 ERROR
	obj.SetAccessor("readyState", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		switch {
		case st.failed:
			return jsc.NumberValue(3)
		case st.loading:
			return jsc.NumberValue(1)
		case st.loaded:
			return jsc.NumberValue(2)
		default:
			return jsc.NumberValue(0)
		}
	}), nil)
	st.obj = obj
	_ = in
}

// refreshCueList 把 cue 列表同步到 TextTrackCueList 的数字索引上（清掉多余的）。
func (st *textTrackState) refreshCueList() {
	if st.list == nil {
		return
	}
	for i, cue := range st.cues {
		st.list.Set(strconv.Itoa(i), jsc.ObjectValue(cue))
	}
	for i := len(st.cues); i < st.count; i++ {
		st.list.Set(strconv.Itoa(i), jsc.Undefined())
	}
	st.count = len(st.cues)
}

// ─── 加载 ────────────────────────────────────────────────

// ensureLoaded 按 <track src> 加载并解析轨道（幂等：已加载/失败后直接返回）。
func (st *textTrackState) ensureLoaded() {
	if st.loaded || st.loading || st.failed || st.el == nil {
		return
	}
	src := st.el.GetAttribute("src")
	if src == "" {
		// 无 src：空轨道（readyState=NONE），不派发事件（规范如此）。
		st.loaded = true
		return
	}
	st.loading = true
	_, body, ok := decodeDataURL(src)
	if !ok {
		// 本引擎没有网络栈：仅支持 data: URL。其他资源按规范异步报 error
		// （脚本的 "track 加载失败" 分支能正常走到，而不是静默挂起）。
		st.loading = false
		st.failed = true
		dispatchTrackEventLater(st.interp, st.el, "error")
		return
	}
	set := protosetFor(st.interp)
	trackDoc := parseWebVTTDocument(body)
	st.regions = trackDoc.Regions
	for _, cue := range trackDoc.Cues {
		st.cues = append(st.cues, newVTTCueObject(st.interp, set, cue, st.regions))
	}
	st.refreshCueList()
	st.loading = false
	st.loaded = true
	// ★ loaded 必须先置位再派发：同步派发路径下回调可能立刻再访问 .track
	// （夹具的 complete() 就是），重入必须看到"已加载"而不是重新解析。
	dispatchTrackEventLater(st.interp, st.el, "load")
}

// dispatchTrackEventLater 异步派发轨道事件（EventLoop 宏任务）。没有事件
// 循环时退化为同步派发，保证单测/无循环宿主也能观察到事件。
func dispatchTrackEventLater(in *jsc.Interpreter, el *dom.Element, evType string) {
	fire := func() {
		if el != nil {
			el.DispatchEvent(dom.NewEvent(evType, false, false, false))
		}
	}
	if in == nil {
		fire()
		return
	}
	if loop := in.EnsureEventLoop(); loop != nil {
		cb := in.NewNativeFunction("track_"+evType, func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			fire()
			return jsc.Undefined()
		}, 0)
		_ = loop.SetTimeout(jsc.FunctionValue(cb), 0)
		return
	}
	fire()
}

// ─── 元素接入 ────────────────────────────────────────────

// textTrackForElement 返回 <track> 元素的 TextTrack（同一元素同一实例），
// 首次访问时按 src 加载。
func textTrackForElement(in *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	if el == nil {
		return nil
	}
	trackCacheMu.Lock()
	st, ok := trackCache[el]
	if !ok {
		set := protosetFor(in)
		if set == nil {
			trackCacheMu.Unlock()
			return nil
		}
		st = &textTrackState{
			interp:   in,
			el:       el,
			mode:     "disabled",
			kind:     strings.ToLower(el.GetAttribute("kind")),
			label:    el.GetAttribute("label"),
			language: el.GetAttribute("srclang"),
			id:       el.GetAttribute("id"),
		}
		if st.kind == "" {
			st.kind = "subtitles" // HTML §4.8.11：kind 缺省值是 subtitles
		}
		st.buildObjects(set, in)
		trackCache[el] = st
	}
	trackCacheMu.Unlock()
	// 加载在锁外进行：解析/派发可能回调用户脚本（其再访问 .track 时不能死锁）。
	st.ensureLoaded()
	return st.obj
}

// textTracksForMediaElement 返回 <video>/<audio> 的 TextTrackList（live）：
// 每次访问都按当前子 <track> 元素刷新索引。
func textTracksForMediaElement(in *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	if el == nil {
		return nil
	}
	trackCacheMu.Lock()
	st, ok := trackListCache[el]
	if !ok {
		set := protosetFor(in)
		if set == nil {
			trackCacheMu.Unlock()
			return nil
		}
		obj := jsc.NewObject(set.trackList)
		st = &textTrackListState{interp: in, el: el, obj: obj}
		obj.SetInternal(st)
		obj.SetClassName("TextTrackList")
		obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.NumberValue(float64(countTrackChildren(el)))
		}), nil)
		trackListCache[el] = st
	}
	trackCacheMu.Unlock()

	refreshTextTrackList(st)
	return st.obj
}

// refreshTextTrackList 按子 <track> 元素的树序刷新列表索引（live 集合语义）。
func refreshTextTrackList(st *textTrackListState) {
	idx := 0
	for _, child := range st.el.ChildNodes() {
		trackEl, ok := child.(*dom.Element)
		if !ok || trackEl.LocalName() != "track" {
			continue
		}
		trackObj := textTrackForElement(st.interp, trackEl)
		if trackObj == nil {
			continue
		}
		st.obj.Set(strconv.Itoa(idx), jsc.ObjectValue(trackObj))
		idx++
	}
	for i := idx; i < st.count; i++ {
		st.obj.Set(strconv.Itoa(i), jsc.Undefined())
	}
	st.count = idx
}

// countTrackChildren 数子 <track> 元素个数（TextTrackList.length 用）。
func countTrackChildren(el *dom.Element) int {
	n := 0
	for _, child := range el.ChildNodes() {
		if c, ok := child.(*dom.Element); ok && c.LocalName() == "track" {
			n++
		}
	}
	return n
}

// ─── JS 值 → 状态 ────────────────────────────────────────

func textTrackStateOf(v jsc.JSValue) *textTrackState {
	o := v.AsObject()
	if o == nil {
		return nil
	}
	st, _ := o.Internal().(*textTrackState)
	return st
}

func textTrackListStateOf(v jsc.JSValue) *textTrackListState {
	o := v.AsObject()
	if o == nil {
		return nil
	}
	st, _ := o.Internal().(*textTrackListState)
	return st
}
