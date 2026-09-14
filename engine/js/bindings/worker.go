package bindings

import (
	"sync"

	"wb-ui/engine/js/jsc"
	"wb-ui/engine/js/worker"
)

// workerScriptFetcher 是宿主注入的 worker 脚本加载器（默认 nil）。
//
// nil 时引擎只支持 data: URL 脚本（自包含、不联网）——这是桌面端的默认姿态：
// 引擎不会自己发起网络请求。宿主（app.Host / webkit.WebView）可注入一个走自身
// 资源加载管道的实现，把 http(s)/file:/相对路径交给自己的缓存与拦截规则。
var (
	workerFetcherMu     sync.Mutex
	workerScriptFetcher worker.ScriptFetcher
)

// SetWorkerScriptFetcher 安装 worker 脚本加载器（进程级；nil = 卸载，恢复为
// 「只支持 data: URL」）。
func SetWorkerScriptFetcher(f worker.ScriptFetcher) {
	workerFetcherMu.Lock()
	defer workerFetcherMu.Unlock()
	workerScriptFetcher = f
}

func currentWorkerScriptFetcher() worker.ScriptFetcher {
	workerFetcherMu.Lock()
	defer workerFetcherMu.Unlock()
	return workerScriptFetcher
}

// installWorker 注册并发脚本执行相关的 Web API：Worker（classic）与 MessageEvent。
//
// 线程模型（重要）：每个 Worker 跑在**自己的 goroutine + 自己的 goja 运行时**
// 上（见 wb-ui/engine/js/worker 包），两个运行时之间只交换 JSON 文本。主运行时的 JS 只在
// 主线程 tick（EventLoop.ProcessTasks）里被触碰——worker 回传的消息由本层入队，
// 再在主事件循环上排一个宏任务派发（同一批多条消息合并为一次派发）。
//
// 已实现：new Worker(url, {name}) / postMessage / onmessage / onerror /
// addEventListener / removeEventListener / terminate；脚本支持 data: URL 或宿主
// 注入的 Fetcher。未实现（有意保留，见 docs/CALIB.md）：module worker、Blob URL
// 脚本、transferable、MessageChannel/MessagePort、SharedWorker/ServiceWorker。
func installWorker(rt *jsc.Interpreter, g *jsc.JSObject) {
	// MessageEvent（worker / 消息类事件的标准事件接口）。
	g.Set("MessageEvent", jsc.FunctionValue(rt.NewConstructor("MessageEvent",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.SetClassName("MessageEvent")
			evType := "message"
			if len(args) >= 1 && args[0].Export() != nil {
				evType = args[0].ToString()
			}
			var data jsc.JSValue = jsc.Null()
			origin, lastEventID := "", ""
			ports := jsc.NewArrayForInterp(in, nil)
			if len(args) >= 2 && args[1].IsObject() {
				init := args[1].AsObject()
				if v, ok := init.GetByKey("data"); ok {
					data = v
				}
				if v, ok := init.GetByKey("origin"); ok {
					origin = v.ToString()
				}
				if v, ok := init.GetByKey("lastEventId"); ok {
					lastEventID = v.ToString()
				}
				if v, ok := init.GetByKey("ports"); ok && v.IsObject() {
					ports = v.AsObject()
				}
			}
			ev.Set("type", jsc.StringValue(evType))
			ev.Set("data", data)
			ev.Set("origin", jsc.StringValue(origin))
			ev.Set("lastEventId", jsc.StringValue(lastEventID))
			ev.Set("source", jsc.Null())
			ev.Set("ports", jsc.ObjectValue(ports))
			return ev
		})))

	// Worker 构造器。
	g.Set("Worker", jsc.FunctionValue(rt.NewConstructor("Worker", newWorker)))
}

// newWorker 实现 new Worker(scriptURL, options)：
// 创建 JS 侧句柄（事件 + 方法）与 Go 侧 worker（独立运行时）并接线消息泵。
func newWorker(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("Worker")

	scriptURL := ""
	if len(args) >= 1 && args[0].Export() != nil {
		scriptURL = args[0].ToString()
	}
	name := ""
	if len(args) >= 2 && args[1].IsObject() {
		if v, ok := args[1].AsObject().GetByKey("name"); ok && v.Export() != nil {
			name = v.ToString()
		}
	}

	obj.Set("onmessage", jsc.Null())
	obj.Set("onmessageerror", jsc.Null())
	obj.Set("onerror", jsc.Null())

	listeners := map[string][]jsc.JSValue{}

	// 跨线程状态：worker goroutine 只做 append + 排泵（schedule 线程安全），
	// 真正的 JS 派发全部发生在主线程 tick 里。
	var mu sync.Mutex
	var pendingData []string
	var pendingErr []string
	pumpScheduled := false
	terminated := false

	// dispatch 在主线程派发事件（on<type> 属性回调 + addEventListener 回调）。
	dispatch := func(evType string, ev jsc.JSValue) {
		if v, ok := obj.GetByKey("on" + evType); ok && v.IsCallable() {
			_, _ = in.Call(v, jsc.ObjectValue(obj), []jsc.JSValue{ev})
		}
		for _, fn := range listeners[evType] {
			_, _ = in.Call(fn, jsc.ObjectValue(obj), []jsc.JSValue{ev})
		}
	}

	newMessageEvent := func(data jsc.JSValue) *jsc.JSObject {
		ev := jsc.NewObject(in.ObjectPrototype())
		ev.SetClassName("MessageEvent")
		ev.Set("type", jsc.StringValue("message"))
		ev.Set("data", data)
		ev.Set("origin", jsc.StringValue(""))
		ev.Set("lastEventId", jsc.StringValue(""))
		ev.Set("source", jsc.Null())
		ev.Set("ports", jsc.ObjectValue(jsc.NewArrayForInterp(in, nil)))
		ev.Set("target", jsc.ObjectValue(obj))
		ev.Set("currentTarget", jsc.ObjectValue(obj))
		return ev
	}

	newErrorEvent := func(message string) *jsc.JSObject {
		ev := jsc.NewObject(in.ObjectPrototype())
		ev.SetClassName("ErrorEvent")
		ev.Set("type", jsc.StringValue("error"))
		ev.Set("message", jsc.StringValue(message))
		ev.Set("filename", jsc.StringValue(scriptURL))
		ev.Set("lineno", jsc.NumberValue(0))
		ev.Set("colno", jsc.NumberValue(0))
		ev.Set("error", jsc.Null())
		ev.Set("target", jsc.ObjectValue(obj))
		ev.Set("currentTarget", jsc.ObjectValue(obj))
		return ev
	}

	// pumpFn 是排到主事件循环上的宏任务：把 worker 线程攒下的消息一次性派发。
	//
	// ★ 事件循环必须在**主线程**这里先取好：jsc.Interpreter.eventLoop 没有锁，
	//   若 worker 线程稍后调 in.EnsureEventLoop() 会与主线程的同类调用构成数据
	//   竞争（go test -race 实测）。worker 线程此后只使用 mainLoop.SetTimeout
	//   ——EventLoop 内部自带锁，且只入队不执行 JS。
	mainLoop := in.EnsureEventLoop()

	pumpFn := jsc.FunctionValue(in.NewNativeFunction("workerPump",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			mu.Lock()
			datas := pendingData
			errs := pendingErr
			pendingData, pendingErr = nil, nil
			pumpScheduled = false
			mu.Unlock()
			for _, data := range datas {
				dispatch("message", jsc.ObjectValue(newMessageEvent(worker.ParseJSON(in, data))))
			}
			for _, e := range errs {
				dispatch("error", jsc.ObjectValue(newErrorEvent(e)))
			}
			return jsc.Undefined()
		}, 0))

	// schedule 排一次派发（幂等：同一批消息只排一个宏任务）。
	// ★ 可从 worker goroutine 调用：EventLoop.SetTimeout 内部加锁，只做入队，
	// 不执行任何 JS。
	schedule := func() {
		mu.Lock()
		if pumpScheduled || terminated {
			mu.Unlock()
			return
		}
		pumpScheduled = true
		mu.Unlock()
		if mainLoop != nil {
			mainLoop.SetTimeout(pumpFn, 0)
		}
	}

	w := worker.New(worker.Options{
		URL:     scriptURL,
		Name:    name,
		Fetcher: currentWorkerScriptFetcher(),
		OnMessage: func(data string) {
			mu.Lock()
			if terminated {
				mu.Unlock()
				return
			}
			pendingData = append(pendingData, data)
			mu.Unlock()
			schedule()
		},
		OnError: func(_, message string) {
			mu.Lock()
			if terminated {
				mu.Unlock()
				return
			}
			// 事件 message 保持引擎原文（浏览器里 worker 的 error 事件 message
			// 同样是加载/执行错误的描述文本）。
			pendingErr = append(pendingErr, message)
			mu.Unlock()
			schedule()
		},
	})
	w.Start()

	obj.Set("postMessage", jsc.FunctionValue(in.NewNativeFunction("postMessage",
		func(interp *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			mu.Lock()
			dead := terminated
			mu.Unlock()
			if dead {
				return jsc.Undefined()
			}
			var v jsc.JSValue
			if len(a) >= 1 {
				v = a[0]
			}
			text, ok := worker.StringifyJSON(interp, v)
			if !ok {
				// 规范：不可克隆的值抛 DataCloneError。
				// ★ 必须 panic goja 原生对象（*goja.Object）：panic 一个
				//   jsc.JSValue 会穿透成 Go panic，脚本的 try/catch 抓不到
				//   （见 engine/js/bindings/popover.go: popoverErrorPanic 的同类注释）。
				o := interp.VM().NewObject()
				_ = o.Set("name", "DataCloneError")
				_ = o.Set("message", "Failed to execute 'postMessage' on 'Worker': The object could not be cloned.")
				panic(o)
			}
			w.PostMessage(text)
			return jsc.Undefined()
		}, 1)))

	obj.Set("terminate", jsc.FunctionValue(in.NewNativeFunction("terminate",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			mu.Lock()
			if terminated {
				mu.Unlock()
				return jsc.Undefined()
			}
			terminated = true
			pendingData, pendingErr = nil, nil
			mu.Unlock()
			w.Terminate()
			return jsc.Undefined()
		}, 0)))

	obj.Set("addEventListener", jsc.FunctionValue(in.NewNativeFunction("addEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) < 2 || !a[1].IsCallable() {
				return jsc.Undefined()
			}
			t := a[0].ToString()
			for _, fn := range listeners[t] {
				if fn.SameAs(a[1]) {
					return jsc.Undefined() // 重复注册同一回调：忽略
				}
			}
			listeners[t] = append(listeners[t], a[1])
			return jsc.Undefined()
		}, 2)))

	obj.Set("removeEventListener", jsc.FunctionValue(in.NewNativeFunction("removeEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
			if len(a) < 2 {
				return jsc.Undefined()
			}
			t := a[0].ToString()
			cur := listeners[t]
			out := cur[:0]
			for _, fn := range cur {
				if fn.SameAs(a[1]) {
					continue
				}
				out = append(out, fn)
			}
			listeners[t] = out
			return jsc.Undefined()
		}, 2)))

	return obj
}
