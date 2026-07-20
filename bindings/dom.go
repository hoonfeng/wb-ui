// Package bindings implements Go <-> JS <-> DOM bridges for wb-ui.
// Completeness: 70% — adds full style/classList/traversal/event for SPA support.
package bindings

import (
	"fmt"
	"strings"

	"wb-ui/dom"
	"wb-ui/jsc"
)

// OnStyleNodeAdded is an optional callback invoked when a <style> element is
// dynamically added to the DOM (via appendChild/insertBefore). The bindings
// set this from webkit.WebView so the frame can re-extract and apply the new
// styles. When nil, dynamic <style> injection is silently ignored.
var OnStyleNodeAdded func(node dom.Node)

func RegisterDOMBindings(rt *jsc.Interpreter, document *dom.Document) {
	docObj := wrapDocument(rt, document)
	rt.GlobalObject().Set("document", jsc.ObjectValue(docObj))

	// window / self / globalThis → 全局对象
	g := rt.GlobalObject()
	g.Set("window", jsc.ObjectValue(g))
	g.Set("self", jsc.ObjectValue(g))
	g.Set("globalThis", jsc.ObjectValue(g))

	// window.location 桩
	loc := jsc.NewObject(rt.ObjectPrototype())
	loc.Set("href", jsc.StringValue("about:blank"))
	loc.Set("origin", jsc.StringValue(""))
	loc.Set("hostname", jsc.StringValue(""))
	loc.Set("pathname", jsc.StringValue("/"))
	loc.Set("search", jsc.StringValue(""))
	loc.Set("hash", jsc.StringValue(""))
	loc.Set("protocol", jsc.StringValue("file:"))
	loc.Set("assign", jsc.FunctionValue(jsc.NewNativeFunction("assign",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	loc.Set("replace", jsc.FunctionValue(jsc.NewNativeFunction("replace",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	loc.Set("reload", jsc.FunctionValue(jsc.NewNativeFunction("reload",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	g.Set("location", jsc.ObjectValue(loc))

	// window.history 桩 (vue-router 需要 pushState/replaceState)
	hist := jsc.NewObject(rt.ObjectPrototype())
	hist.Set("length", jsc.NumberValue(1))
	hist.Set("state", jsc.Null())
	hist.Set("pushState", jsc.FunctionValue(jsc.NewNativeFunction("pushState",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 3)))
	hist.Set("replaceState", jsc.FunctionValue(jsc.NewNativeFunction("replaceState",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 3)))
	hist.Set("go", jsc.FunctionValue(jsc.NewNativeFunction("go",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 1)))
	hist.Set("back", jsc.FunctionValue(jsc.NewNativeFunction("back",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	hist.Set("forward", jsc.FunctionValue(jsc.NewNativeFunction("forward",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))
	g.Set("history", jsc.ObjectValue(hist))

	// window.navigator 桩
	nav := jsc.NewObject(rt.ObjectPrototype())
	nav.Set("userAgent", jsc.StringValue("wb-ui"))
	nav.Set("platform", jsc.StringValue("Go"))
	nav.Set("language", jsc.StringValue("zh-CN"))
	nav.Set("languages", jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.StringValue("zh-CN"), jsc.StringValue("en")})))
	g.Set("navigator", jsc.ObjectValue(nav))

	// window.screen 桩
	screen := jsc.NewObject(rt.ObjectPrototype())
	screen.Set("width", jsc.NumberValue(1280))
	screen.Set("height", jsc.NumberValue(800))
	g.Set("screen", jsc.ObjectValue(screen))

// window.console 由 SetupGlobal 设置

	// window.matchMedia 桩
	g.Set("matchMedia", jsc.FunctionValue(jsc.NewNativeFunction("matchMedia",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			mq := jsc.NewObject(in.ObjectPrototype())
			mq.Set("matches", jsc.BooleanValue(false))
			mq.Set("media", jsc.StringValue(""))
			mq.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 2)))
			mq.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 2)))
			mq.Set("addListener", jsc.FunctionValue(jsc.NewNativeFunction("addListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 1)))
			mq.Set("removeListener", jsc.FunctionValue(jsc.NewNativeFunction("removeListener",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.Undefined()
				}, 1)))
			return jsc.ObjectValue(mq)
		}, 1)))

	// setTimeout / setInterval / clearTimeout / clearInterval（事件循环驱动）
	g.Set("setTimeout", jsc.FunctionValue(jsc.NewNativeFunction("setTimeout",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			el := in.EnsureEventLoop()
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			delayMs := int64(0)
			if len(args) >= 2 && args[1].IsNumber() {
				delayMs = int64(args[1].ToNumber())
			}
			id := el.SetTimeout(args[0], delayMs)
			return jsc.NumberValue(float64(id))
		}, 2)))
	g.Set("setInterval", jsc.FunctionValue(jsc.NewNativeFunction("setInterval",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			el := in.EnsureEventLoop()
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			intervalMs := int64(0)
			if len(args) >= 2 && args[1].IsNumber() {
				intervalMs = int64(args[1].ToNumber())
			}
			if intervalMs < 4 {
				intervalMs = 4 // 浏览器最小间隔 4ms
			}
			id := el.SetInterval(args[0], intervalMs)
			return jsc.NumberValue(float64(id))
		}, 2)))
	g.Set("clearTimeout", jsc.FunctionValue(jsc.NewNativeFunction("clearTimeout",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearTimeout(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))
	g.Set("clearInterval", jsc.FunctionValue(jsc.NewNativeFunction("clearInterval",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.ClearInterval(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))

	// requestAnimationFrame / cancelAnimationFrame（事件循环驱动）
	g.Set("requestAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("requestAnimationFrame",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			el := in.EnsureEventLoop()
			id := el.RequestAnimationFrame(args[0])
			return jsc.NumberValue(float64(id))
		}, 1)))
	g.Set("cancelAnimationFrame", jsc.FunctionValue(jsc.NewNativeFunction("cancelAnimationFrame",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if el := in.GetEventLoop(); el != nil && len(args) > 0 && args[0].IsNumber() {
				el.CancelAnimationFrame(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))

	// queueMicrotask（事件循环驱动）
	g.Set("queueMicrotask", jsc.FunctionValue(jsc.NewNativeFunction("queueMicrotask",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.Undefined()
			}
			el := in.EnsureEventLoop()
			el.QueueMicrotask(args[0])
			return jsc.Undefined()
		}, 1)))

	// window.addEventListener / removeEventListener (vue-router 需要 'popstate')
	g.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 2)))
	g.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 2)))

	// MutationObserver 构造函数
	g.Set("MutationObserver", jsc.FunctionValue(rt.NewConstructor("MutationObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			mo := dom.NewMutationObserver(func(records []*dom.MutationRecord, observer *dom.MutationObserver) {
				// 将 Go 记录转换为 JS 对象并调用 JS 回调
				jsRecords := make([]jsc.JSValue, len(records))
				for i, r := range records {
					jsRecords[i] = jsc.ObjectValue(mutationRecordToJS(in, r))
				}
				moObj := jsc.NewObject(in.ObjectPrototype())
				moObj.SetInternal(observer)
				// 在事件循环的微任务中投递（已在 dom.FlushMutationObservers 中调用）
				in.Call(cb, jsc.Undefined(), []jsc.JSValue{
					jsc.ObjectValue(jsc.NewArray(nil, jsRecords)),
					jsc.ObjectValue(moObj),
				})
			})
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.SetInternal(mo)
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					opts := &dom.MutationObserverOptions{}
					if len(a) >= 2 && a[1].IsObject() {
						optObj := a[1].AsObject()
						if optObj != nil {
							if v, ok := optObj.GetByKey("childList"); ok { opts.ChildList = v.ToBoolean() }
							if v, ok := optObj.GetByKey("attributes"); ok { opts.Attributes = v.ToBoolean() }
							if v, ok := optObj.GetByKey("characterData"); ok { opts.CharacterData = v.ToBoolean() }
							if v, ok := optObj.GetByKey("subtree"); ok { opts.Subtree = v.ToBoolean() }
							if v, ok := optObj.GetByKey("attributeOldValue"); ok { opts.AttributeOldValue = v.ToBoolean() }
							if v, ok := optObj.GetByKey("characterDataOldValue"); ok { opts.CharacterDataOldValue = v.ToBoolean() }
							if filter, ok := optObj.GetByKey("attributeFilter"); ok && filter.IsObject() {
								arr := filter.AsObject()
								if arr != nil {
									for _, k := range arr.Keys() {
										if v, ok := arr.GetByKey(k); ok {
											opts.AttributeFilter = append(opts.AttributeFilter, v.ToString())
										}
									}
								}
							}
						}
					}
					mo.Observe(target, opts)
					return jsc.Undefined()
				}, 2)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					mo.Disconnect()
					return jsc.Undefined()
				}, 0)))
			obj.Set("takeRecords", jsc.FunctionValue(jsc.NewNativeFunction("takeRecords",
				func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					records := mo.TakeRecords()
					jsRecords := make([]jsc.JSValue, len(records))
					for i, r := range records {
						jsRecords[i] = jsc.ObjectValue(mutationRecordToJS(in, r))
					}
					return jsc.ObjectValue(jsc.NewArray(nil, jsRecords))
				}, 0)))
			return obj
		})))

	// IntersectionObserver 构造函数（GUI 模式下所有元素视为 100% 可见）
	g.Set("IntersectionObserver", jsc.FunctionValue(rt.NewConstructor("IntersectionObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			var observed []*dom.Element
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					observed = append(observed, target)
					// 立即通过微任务通知 100% 可见（对标浏览器首次 observe 行为）
					el := in.EnsureEventLoop()
					el.QueueMicrotask(jsc.FunctionValue(jsc.NewNativeFunction("io-cb",
						func(interp2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
							entry := jsc.NewObject(interp2.ObjectPrototype())
							entry.Set("isIntersecting", jsc.BooleanValue(true))
							entry.Set("intersectionRatio", jsc.NumberValue(1.0))
							entry.Set("target", jsc.ObjectValue(wrapElement(interp2, target)))
							entry.Set("boundingClientRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							entry.Set("intersectionRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							entry.Set("rootBounds", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							_, _ = interp2.Call(cb, jsc.Undefined(), []jsc.JSValue{
								jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.ObjectValue(entry)})),
							})
							return jsc.Undefined()
						}, 0)))
					return jsc.Undefined()
				}, 1)))
			obj.Set("unobserve", jsc.FunctionValue(jsc.NewNativeFunction("unobserve",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					for i, el := range observed {
						if el == target {
							observed = append(observed[:i], observed[i+1:]...)
							break
						}
					}
					return jsc.Undefined()
				}, 1)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					observed = nil
					return jsc.Undefined()
				}, 0)))
			obj.Set("takeRecords", jsc.FunctionValue(jsc.NewNativeFunction("takeRecords",
				func(in2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return jsc.ObjectValue(jsc.NewArray(nil, nil))
				}, 0)))
			return obj
		})))

	// ResizeObserver 构造函数
	g.Set("ResizeObserver", jsc.FunctionValue(rt.NewConstructor("ResizeObserver",
		func(in *jsc.Interpreter, thisVal jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			if len(args) < 1 || !args[0].IsCallable() {
				return nil
			}
			cb := args[0]
			var observed []*dom.Element
			obj := jsc.NewObject(in.ObjectPrototype())
			obj.Set("observe", jsc.FunctionValue(jsc.NewNativeFunction("observe",
				func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					if target == nil { return jsc.Undefined() }
					observed = append(observed, target)
					// 通过微任务通知初始尺寸
					el := in.EnsureEventLoop()
					el.QueueMicrotask(jsc.FunctionValue(jsc.NewNativeFunction("ro-cb",
						func(interp2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
							entry := jsc.NewObject(interp2.ObjectPrototype())
							entry.Set("target", jsc.ObjectValue(wrapElement(interp2, target)))
							entry.Set("contentRect", jsc.ObjectValue(makeDOMRect(interp2, 0, 0, 0, 0)))
							_, _ = interp2.Call(cb, jsc.Undefined(), []jsc.JSValue{
								jsc.ObjectValue(jsc.NewArray(nil, []jsc.JSValue{jsc.ObjectValue(entry)})),
							})
							return jsc.Undefined()
						}, 0)))
					return jsc.Undefined()
				}, 1)))
			obj.Set("unobserve", jsc.FunctionValue(jsc.NewNativeFunction("unobserve",
				func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
					if len(a) < 1 { return jsc.Undefined() }
					targetObj := a[0].AsObject()
					if targetObj == nil { return jsc.Undefined() }
					target, _ := targetObj.Internal().(*dom.Element)
					for i, el := range observed {
						if el == target {
							observed = append(observed[:i], observed[i+1:]...)
							break
						}
					}
					return jsc.Undefined()
				}, 1)))
			obj.Set("disconnect", jsc.FunctionValue(jsc.NewNativeFunction("disconnect",
				func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					observed = nil
					return jsc.Undefined()
				}, 0)))
			return obj
		})))
}

type ElementWrapper struct {
	JS  *jsc.JSObject
	DOM *dom.Element
}

// ─── Document ──────────────────────────────────────────

func wrapDocument(rt *jsc.Interpreter, doc *dom.Document) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Document")
obj.SetInternal(doc)

	obj.Set("getElementById", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		if el := doc.GetElementById(arg); el != nil {
			return jsc.ObjectValue(wrapElement(in, el))
		}
		return jsc.Null()
	})))
	obj.Set("createElement", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createElementNS", funcVal(fn2(func(in *jsc.Interpreter, ns, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, doc.CreateElement(arg)))
	})))
	obj.Set("createTextNode", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapText(in, doc.CreateTextNode(arg)))
	})))
	obj.Set("createComment", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.ObjectValue(wrapComment(in, doc.CreateComment(arg)))
	})))
	obj.Set("createDocumentFragment", funcVal(fn0(func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.ObjectValue(wrapDocFrag(in, doc.CreateDocumentFragment()))
	})))
	obj.Set("createEvent", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		return eventToJS(in, doc.CreateEvent(arg))
	})))
	obj.Set("querySelector", funcVal(fn1(func(in *jsc.Interpreter, sel string) jsc.JSValue {
		// 使用完整 CSS 选择器引擎
		if found := DocumentQuerySelector(doc, sel); found != nil {
			return jsc.ObjectValue(wrapElement(in, found))
		}
		return jsc.Null()
	})))
	obj.Set("querySelectorAll", funcVal(fn1(func(in *jsc.Interpreter, sel string) jsc.JSValue {
		return arrElem(in, DocumentQuerySelectorAll(doc, sel))
	})))
	obj.Set("getElementsByTagName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByTagName(arg)
		return arrJS(in, els)
	})))
	obj.Set("getElementsByClassName", funcVal(fn1(func(in *jsc.Interpreter, arg string) jsc.JSValue {
		els := doc.GetElementsByClassName(arg)
		return arrJS(in, els)
	})))
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(doc)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(doc)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(doc)))

	// Accessors
	obj.SetAccessor("body", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if b := doc.Body(); b != nil { return jsc.ObjectValue(wrapElement(in, b)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("head", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if h := doc.Head(); h != nil { return jsc.ObjectValue(wrapElement(in, h)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("documentElement", getter(func(in *jsc.Interpreter) jsc.JSValue {
		if de := doc.DocumentElement(); de != nil { return jsc.ObjectValue(wrapElement(in, de)) }
		return jsc.Null()
	}), nil)
	obj.SetAccessor("title",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(doc.Title()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { doc.SetTitle(v.ToString()) })
	obj.SetAccessor("URL", strAcc(doc.URL()), nil)
	obj.SetAccessor("cookie", strAcc(""), nil)

	return obj
}

// ─── Element wrapper cache ─────────────────────────────
// Ensures the same Go *dom.Element always maps to the same JS wrapper,
// so JS-side properties (__vue_app__, _vnode) set on one wrapper are
// visible through all DOM access methods (querySelector, getElementById, etc.)
var elementWrapperCache = make(map[*dom.Element]*jsc.JSObject)

func clearElementCache() {
	elementWrapperCache = make(map[*dom.Element]*jsc.JSObject)
}

// isStyleElement reports whether n is an HTML <style> element.
func isStyleElement(n dom.Node) bool {
	if n == nil { return false }
	el, ok := n.(*dom.Element)
	return ok && strings.EqualFold(el.TagName(), "style")
}

// mutationRecordToJS 将 Go MutationRecord 转换为 JS 对象。
func mutationRecordToJS(in *jsc.Interpreter, r *dom.MutationRecord) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.Set("type", jsc.StringValue(string(r.Type)))
	obj.Set("target", jsc.ObjectValue(wrapElement(in, r.Target.(*dom.Element))))
	if r.AddedNodes != nil {
		jsAdded := make([]jsc.JSValue, len(r.AddedNodes))
		for i, n := range r.AddedNodes {
			jsAdded[i] = nodeToJS(in, n)
		}
		obj.Set("addedNodes", jsc.ObjectValue(jsc.NewArray(nil, jsAdded)))
	}
	if r.RemovedNodes != nil {
		jsRemoved := make([]jsc.JSValue, len(r.RemovedNodes))
		for i, n := range r.RemovedNodes {
			jsRemoved[i] = nodeToJS(in, n)
		}
		obj.Set("removedNodes", jsc.ObjectValue(jsc.NewArray(nil, jsRemoved)))
	}
	if r.PreviousSibling != nil {
		obj.Set("previousSibling", nodeToJS(in, r.PreviousSibling))
	}
	if r.NextSibling != nil {
		obj.Set("nextSibling", nodeToJS(in, r.NextSibling))
	}
	if r.AttributeName != "" {
		obj.Set("attributeName", jsc.StringValue(r.AttributeName))
		obj.Set("attributeNamespace", jsc.Null())
	}
	if r.OldValue != "" {
		obj.Set("oldValue", jsc.StringValue(r.OldValue))
	}
	return obj
}

// nodeToJS 将 dom.Node 转换为对应的 JS 对象。
func nodeToJS(in *jsc.Interpreter, n dom.Node) jsc.JSValue {
	switch v := n.(type) {
	case *dom.Element:
		return jsc.ObjectValue(wrapElement(in, v))
	case *dom.Text:
		return jsc.ObjectValue(wrapText(in, v))
	case *dom.Comment:
		return jsc.ObjectValue(wrapComment(in, v))
	default:
		return jsc.Null()
	}
}

// makeDOMRect 创建一个 DOMRect 对象。
func makeDOMRect(in *jsc.Interpreter, x, y, w, h float64) *jsc.JSObject {
	r := jsc.NewObject(in.ObjectPrototype())
	r.Set("x", jsc.NumberValue(x))
	r.Set("y", jsc.NumberValue(y))
	r.Set("width", jsc.NumberValue(w))
	r.Set("height", jsc.NumberValue(h))
	r.Set("top", jsc.NumberValue(y))
	r.Set("right", jsc.NumberValue(x+w))
	r.Set("bottom", jsc.NumberValue(y+h))
	r.Set("left", jsc.NumberValue(x))
	return r
}

// ─── Element ───────────────────────────────────────────

func wrapElement(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	// Return cached wrapper if available
	if cached, ok := elementWrapperCache[el]; ok {
		return cached
	}
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Element")
obj.SetInternal(el)
	// Cache before returning
	elementWrapperCache[el] = obj

	// Attributes
	obj.Set("getAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.StringValue(el.GetAttribute(arg))
	})))
	obj.Set("setAttribute", funcVal(fn2(func(_ *jsc.Interpreter, a, b string) jsc.JSValue {
		el.SetAttribute(a, b)
		return jsc.Undefined()
	})))
	obj.Set("hasAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		return jsc.BooleanValue(el.HasAttribute(arg))
	})))
	obj.Set("removeAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		el.RemoveAttribute(arg)
		return jsc.Undefined()
	})))
	obj.Set("toggleAttribute", funcVal(fn1(func(_ *jsc.Interpreter, arg string) jsc.JSValue {
		if el.HasAttribute(arg) { el.RemoveAttribute(arg); return jsc.BooleanValue(false) }
		el.SetAttribute(arg, "")
		return jsc.BooleanValue(true)
	})))

	// Node tree
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.AppendChild(n)
		return a
	})))
	obj.Set("removeChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		el.RemoveChild(n)
		return a
	})))
	obj.Set("insertBefore", funcVal(fn2Node(func(in *jsc.Interpreter, nc, rc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil { return jsc.Null() }
		// DocumentFragment: insert all children individually.
		if frag, ok := nc.(*dom.DocumentFragment); ok {
			for c := frag.FirstChild(); c != nil; c = frag.FirstChild() {
				frag.RemoveChild(c)
				el.InsertBefore(c, rc)
			}
			return a0
		}
		el.InsertBefore(nc, rc)
		return a0
	})))
	obj.Set("replaceChild", funcVal(fn2Node(func(_ *jsc.Interpreter, nc, oc dom.Node, a0, a1 jsc.JSValue) jsc.JSValue {
		if nc == nil || oc == nil { return jsc.Null() }
		el.ReplaceChild(nc, oc)
		return a1
	})))
	obj.Set("contains", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, _ jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.BooleanValue(false) }
		return jsc.BooleanValue(el.Contains(n))
	})))
	obj.Set("cloneNode", jsc.FunctionValue(jsc.NewNativeFunction("cloneNode",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			deep := len(args) > 0 && args[0].ToBoolean()
			switch v := el.CloneNode(deep).(type) {
			case *dom.Element:
				return jsc.ObjectValue(wrapElement(in, v))
			case *dom.Text:
				return jsc.ObjectValue(wrapText(in, v))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("hasChildNodes", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.HasChildNodes())
	})))
	obj.Set("isConnected", funcVal(fn0(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.BooleanValue(el.IsConnected())
	})))

	// CSS 选择器匹配（对标浏览器）
	obj.Set("matches", jsc.FunctionValue(jsc.NewNativeFunction("matches",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			return jsc.BooleanValue(ElementMatches(el, args[0].ToString()))
		}, 1)))
	obj.Set("closest", jsc.FunctionValue(jsc.NewNativeFunction("closest",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			if found := ElementClosest(el, args[0].ToString()); found != nil {
				return jsc.ObjectValue(wrapElement(in, found))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("querySelector", jsc.FunctionValue(jsc.NewNativeFunction("querySelector",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			if found := ElementQuerySelector(el, args[0].ToString()); found != nil {
				return jsc.ObjectValue(wrapElement(in, found))
			}
			return jsc.Null()
		}, 1)))
	obj.Set("querySelectorAll", jsc.FunctionValue(jsc.NewNativeFunction("querySelectorAll",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return arrElem(in, nil) }
			return arrElem(in, ElementQuerySelectorAll(el, args[0].ToString()))
		}, 1)))
	// insertAdjacentHTML
	obj.Set("insertAdjacentHTML", jsc.FunctionValue(jsc.NewNativeFunction("insertAdjacentHTML",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 { return jsc.Undefined() }
			el.InsertAdjacentHTML(args[0].ToString(), args[1].ToString())
			if OnStyleNodeAdded != nil {
				// Check for newly added <style> elements
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if isStyleElement(c) { OnStyleNodeAdded(c) }
				}
			}
			return jsc.Undefined()
		}, 2)))
	// dataset — DOMStringMap 代理 data-* 属性
	obj.Set("dataset", jsc.ObjectValue(makeDataset(rt, el)))

	// classList
	obj.Set("classList", jsc.ObjectValue(makeClassList(rt, el)))

	// style — a live object that reads/writes the style attribute
	obj.Set("style", jsc.ObjectValue(makeStyleObject(rt, el)))

	// Events
	obj.Set("addEventListener", jsc.FunctionValue(makeAddEventListener(el)))
	obj.Set("removeEventListener", jsc.FunctionValue(makeRemoveEventListener(el)))
	obj.Set("dispatchEvent", jsc.FunctionValue(makeDispatchEvent(el)))

	// Tree traversal — dynamic getters so they reflect live DOM tree
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return el.ParentNode() }), nil)
	obj.SetAccessor("parentElement", nodeAccFn(rt, func() dom.Node { return el.ParentElement() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return el.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return el.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return el.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return el.LastChild() }), nil)
	obj.SetAccessor("childElementCount", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		n := 0
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if _, ok := c.(*dom.Element); ok { n++ }
		}
		return jsc.NumberValue(float64(n))
	}), nil)
	obj.SetAccessor("children", getter(func(in *jsc.Interpreter) jsc.JSValue {
		var els []*dom.Element
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok { els = append(els, e) }
		}
		return arrElem(in, els)
	}), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, el.ChildNodes())
	}), nil)
	// ownerDocument — needed by Vue 3 when checking element's document
	obj.SetAccessor("ownerDocument", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return in.GlobalObject().GetOrZero("document")
	}), nil)

	// Position / dimension stubs (Vue needs these)
	obj.Set("getBoundingClientRect", jsc.FunctionValue(jsc.NewNativeFunction("getBoundingClientRect",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			r := jsc.NewObject(in.ObjectPrototype())
			r.Set("x", jsc.NumberValue(0))
			r.Set("y", jsc.NumberValue(0))
			r.Set("width", jsc.NumberValue(0))
			r.Set("height", jsc.NumberValue(0))
			r.Set("top", jsc.NumberValue(0))
			r.Set("right", jsc.NumberValue(0))
			r.Set("bottom", jsc.NumberValue(0))
			r.Set("left", jsc.NumberValue(0))
			return jsc.ObjectValue(r)
		}, 0)))
	obj.Set("scrollIntoView", jsc.FunctionValue(jsc.NewNativeFunction("scrollIntoView",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))

	// Accessors for string properties
	obj.SetAccessor("tagName", strAcc(el.TagName()), nil)
	obj.SetAccessor("nodeName", strAcc(el.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(el.NodeType()))
	}), nil)
	obj.SetAccessor("id",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetId()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetId(v.ToString()) })
	obj.SetAccessor("className",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetClassName()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	obj.SetAccessor("innerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetInnerHTML()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetInnerHTML(v.ToString()) })
	obj.SetAccessor("outerHTML",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetOuterHTML()) }), nil)
	obj.SetAccessor("textContent",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.TextContent()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetTextContent(v.ToString()) })

	// <template> elements need .content returning a DocumentFragment
	// (Vue 3 + createStaticVNode depends on this).
	if strings.EqualFold(el.LocalName(), "template") {
		obj.SetAccessor("content",
			getter(func(in *jsc.Interpreter) jsc.JSValue {
				frag := el.OwnerDocument().CreateDocumentFragment()
				// Move all child nodes into the fragment
				for c := el.FirstChild(); c != nil; c = el.FirstChild() {
					frag.AppendChild(c)
				}
				return jsc.ObjectValue(wrapDocFrag(in, frag))
			}), nil)
	}

	return obj
}

// ─── classList ──────────────────────────────────────────

func makeClassList(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	cls := jsc.NewObject(rt.ObjectPrototype())
	get := func() []string { return strings.Fields(el.GetClassName()) }
	set := func(c []string) { el.SetClassName(strings.Join(c, " ")) }

	cls.Set("add", jsc.FunctionValue(jsc.NewNativeFunction("add",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { m[a.ToString()] = true }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Undefined() }
			m := make(map[string]bool)
			for _, c := range get() { m[c] = true }
			for _, a := range args { delete(m, a.ToString()) }
			var r []string
			for c := range m { r = append(r, c) }
			set(r)
			return jsc.Undefined()
		}, 1)))
	cls.Set("contains", jsc.FunctionValue(jsc.NewNativeFunction("contains",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			for _, c := range get() { if c == n { return jsc.BooleanValue(true) } }
			return jsc.BooleanValue(false)
		}, 1)))
	cls.Set("toggle", jsc.FunctionValue(jsc.NewNativeFunction("toggle",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.BooleanValue(false) }
			n := args[0].ToString()
			force := len(args) >= 2
			cs := get()
			for i, c := range cs {
				if c == n {
					if force && args[1].ToBoolean() {
						return jsc.BooleanValue(true) // already there
					}
					cs = append(cs[:i], cs[i+1:]...)
					set(cs)
					return jsc.BooleanValue(false)
				}
			}
			if force && !args[1].ToBoolean() {
				return jsc.BooleanValue(false) // force-remove but not there
			}
			cs = append(cs, n)
			set(cs)
			return jsc.BooleanValue(true)
		}, 2)))
	cls.Set("item", jsc.FunctionValue(jsc.NewNativeFunction("item",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.Null() }
			idx := int(args[0].ToNumber())
			cs := get()
			if idx < 0 || idx >= len(cs) {
				return jsc.Null()
			}
			return jsc.StringValue(cs[idx])
		}, 1)))
	cls.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(len(get())))
	}), nil)
	cls.SetAccessor("value",
		getter(func(_ *jsc.Interpreter) jsc.JSValue { return jsc.StringValue(el.GetClassName()) }),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { el.SetClassName(v.ToString()) })
	return cls
}

// ─── dataset (DOMStringMap) ────────────────────────────

func makeDataset(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	ds := jsc.NewObject(rt.ObjectPrototype())
	// DOMStringMap uses a proxy-like pattern: reading ds.key translates to
	// el.getAttribute("data-key"), writing translates to setAttribute.
	// Go's JSObject doesn't support full Proxy, so we provide direct accessor
	// methods and also attempt to pre-populate known data-* attrs.
	ds.Set("get", jsc.FunctionValue(jsc.NewNativeFunction("_get",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 { return jsc.StringValue("") }
			key := "data-" + camelToKebab(args[0].ToString())
			return jsc.StringValue(el.GetAttribute(key))
		}, 1)))
	ds.Set("set", jsc.FunctionValue(jsc.NewNativeFunction("_set",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 { return jsc.Undefined() }
			key := "data-" + camelToKebab(args[0].ToString())
			el.SetAttribute(key, args[1].ToString())
			return jsc.Undefined()
		}, 2)))
	// Pre-populate with existing data-* attributes
	for _, name := range el.AttributeNames() {
		if strings.HasPrefix(name, "data-") {
			camel := kebabToCamel(name[5:])
			val := el.GetAttribute(name)
			ds.Set(camel, jsc.StringValue(val))
		}
	}
	return ds
}

// camelToKebab converts "someProp" → "some-prop"
func camelToKebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + 32) // lowercase
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// kebabToCamel converts "some-prop" → "someProp"
func kebabToCamel(s string) string {
	var b strings.Builder
	upper := false
	for _, r := range s {
		if r == '-' {
			upper = true
			continue
		}
		if upper {
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r - 32)
			} else {
				b.WriteRune(r)
			}
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ─── style object ──────────────────────────────────────
// A live CSSStyleDeclaration that reads/writes the element's style attribute.

func makeStyleObject(rt *jsc.Interpreter, el *dom.Element) *jsc.JSObject {
	s := jsc.NewObject(rt.ObjectPrototype())
	s.SetClassName("CSSStyleDeclaration")
s.SetInternal(el)

	// cssText getter/setter
	s.SetAccessor("cssText",
		getter(func(_ *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(el.GetAttribute("style"))
		}),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			el.SetAttribute("style", v.ToString())
		})

		s.Set("setProperty", jsc.FunctionValue(jsc.NewNativeFunction("setProperty",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 2 { return jsc.Undefined() }
				cssText := el.GetAttribute("style")
				props := parseStyle(cssText)
				props[args[0].ToString()] = args[1].ToString()
				el.SetAttribute("style", joinStyle(props))
				return jsc.Undefined()
			}, 2)))
		s.Set("removeProperty", jsc.FunctionValue(jsc.NewNativeFunction("removeProperty",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 { return jsc.StringValue("") }
				cssText := el.GetAttribute("style")
				props := parseStyle(cssText)
				old := props[args[0].ToString()]
				delete(props, args[0].ToString())
				el.SetAttribute("style", joinStyle(props))
				return jsc.StringValue(old)
			}, 1)))

	return s
}

// parseStyle parses "color:red;font-size:16px" → map
func parseStyle(s string) map[string]string {
	m := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func joinStyle(m map[string]string) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ";")
}

// ─── DocumentFragment ──────────────────────────────────

func wrapDocFrag(rt *jsc.Interpreter, frag *dom.DocumentFragment) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("DocumentFragment")
obj.SetInternal(frag)
	obj.Set("appendChild", funcVal(fn1Node(func(_ *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue {
		if n == nil { return jsc.Null() }
		frag.AppendChild(n)
		if OnStyleNodeAdded != nil && isStyleElement(n) {
			OnStyleNodeAdded(n)
		}
		return a
	})))
	return obj
}

// ─── Text / Comment ────────────────────────────────────

func wrapText(rt *jsc.Interpreter, t *dom.Text) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Text")
obj.SetInternal(t)
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := t.ParentNode(); p != nil { p.RemoveChild(t) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		strAcc(t.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		strAcc(t.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { t.SetData(v.ToString()) })
	obj.SetAccessor("length", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.Length()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(t.NodeName()), nil)
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(t.NodeType()))
	}), nil)
	return obj
}

func wrapComment(rt *jsc.Interpreter, c *dom.Comment) *jsc.JSObject {
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("Comment")
	obj.SetInternal(c)
	obj.Set("remove", jsc.FunctionValue(jsc.NewNativeFunction("remove",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if p := c.ParentNode(); p != nil { p.RemoveChild(c) }
			return jsc.Undefined()
		}, 0)))
	obj.SetAccessor("data",
		strAcc(c.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("textContent",
		strAcc(c.Data()),
		func(_ *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { c.SetData(v.ToString()) })
	obj.SetAccessor("nodeType", getter(func(_ *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(float64(c.NodeType()))
	}), nil)
	obj.SetAccessor("nodeName", strAcc(c.NodeName()), nil)

	// 树遍历属性（Vue 3 渲染器需要）
	obj.SetAccessor("parentNode", nodeAccFn(rt, func() dom.Node { return c.ParentNode() }), nil)
	obj.SetAccessor("nextSibling", nodeAccFn(rt, func() dom.Node { return c.NextSibling() }), nil)
	obj.SetAccessor("previousSibling", nodeAccFn(rt, func() dom.Node { return c.PreviousSibling() }), nil)
	obj.SetAccessor("firstChild", nodeAccFn(rt, func() dom.Node { return c.FirstChild() }), nil)
	obj.SetAccessor("lastChild", nodeAccFn(rt, func() dom.Node { return c.LastChild() }), nil)
	obj.SetAccessor("childNodes", getter(func(in *jsc.Interpreter) jsc.JSValue {
		return arrNode(in, c.ChildNodes())
	}), nil)
	return obj
}

// ─── Helpers ───────────────────────────────────────────

func unwrapNode(v jsc.JSValue) dom.Node {
	if !v.IsObject() { return nil }
	if n, ok := v.AsObject().Internal().(dom.Node); ok { return n }
	return nil
}

// Accessor helpers
type getterFn = func(*jsc.Interpreter, jsc.JSValue) jsc.JSValue

func getter(fn func(*jsc.Interpreter) jsc.JSValue) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return fn(in) }
}

func strAcc(s string) getterFn {
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.StringValue(s) }
}

func nodeAcc(rt *jsc.Interpreter, n dom.Node) getterFn {
	if n == nil {
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
	}
	switch v := n.(type) {
	case *dom.Element:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapElement(rt, v))
		}
	case *dom.Text:
		return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(wrapText(rt, v))
		}
	}
	return func(_ *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue { return jsc.Null() }
}

// nodeAccFn returns an accessor getter that calls fn() each time it is read,
// so it stays in sync with the live DOM tree.
func nodeAccFn(rt *jsc.Interpreter, fn func() dom.Node) getterFn {
	return func(in *jsc.Interpreter, _ jsc.JSValue) jsc.JSValue {
		n := fn()
		if n == nil {
			return jsc.Null()
		}
		switch v := n.(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		case *dom.Comment:
			return jsc.ObjectValue(wrapComment(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		}
		return jsc.Null()
	}
}

func funcVal(fn *jsc.JSFunction) jsc.JSValue { return jsc.FunctionValue(fn) }

// fn0/fn1/fn2 helpers
func fn0(fn func(in *jsc.Interpreter) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		return fn(in)
	}, 0)
}

func fn1(fn func(in *jsc.Interpreter, arg string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, args[0].ToString())
	}, 1)
}

func fn2(fn func(in *jsc.Interpreter, a, b string) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Undefined() }
		return fn(in, args[0].ToString(), args[1].ToString())
	}, 2)
}

func fn1Node(fn func(in *jsc.Interpreter, n dom.Node, a jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), args[0])
	}, 1)
}

func fn2Node(fn func(in *jsc.Interpreter, n1, n2 dom.Node, a0, a1 jsc.JSValue) jsc.JSValue) *jsc.JSFunction {
	return jsc.NewNativeFunction("fn", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 2 { return jsc.Null() }
		return fn(in, unwrapNode(args[0]), unwrapNode(args[1]), args[0], args[1])
	}, 2)
}

// arr helpers
func arrElem(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

func arrNode(in *jsc.Interpreter, nodes []dom.Node) jsc.JSValue {
	return arrayValue(in, len(nodes), func(i int) jsc.JSValue {
		switch v := nodes[i].(type) {
		case *dom.Element:
			return jsc.ObjectValue(wrapElement(in, v))
		case *dom.Text:
			return jsc.ObjectValue(wrapText(in, v))
		case *dom.Comment:
			return jsc.ObjectValue(wrapComment(in, v))
		case *dom.DocumentFragment:
			return jsc.ObjectValue(wrapDocFrag(in, v))
		}
		return jsc.Null()
	})
}
func arrayValue(in *jsc.Interpreter, n int, fn func(int) jsc.JSValue) jsc.JSValue {
	arr := make([]jsc.JSValue, n)
	for i := 0; i < n; i++ { arr[i] = fn(i) }
	return jsc.ObjectValue(jsc.NewArrayForInterp(in, arr))
}
func arrJS(in *jsc.Interpreter, els []*dom.Element) jsc.JSValue {
	return arrayValue(in, len(els), func(i int) jsc.JSValue {
		return jsc.ObjectValue(wrapElement(in, els[i]))
	})
}

// Silence unused import warning
var _ = fmt.Sprintf
