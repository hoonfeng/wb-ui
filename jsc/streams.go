// Package jsc — Streams 标准最小实现（ReadableStream / TransformStream /
// TextEncoderStream / TextDecoderStream）。
//
// 为什么需要：现代 SSR/水合框架在客户端用流 API 解析服务端分块载荷——
//
//	new ReadableStream({start(controller){…}})
//	  .pipeThrough(new TextEncoderStream())
//	  .pipeThrough(new TransformStream({transform, flush}))
//	  .getReader().read()
//
// 缺 ReadableStream 时 `new ReadableStream(...)` 抛 ReferenceError，整段水合
// 脚本中断（页面停在服务端 HTML 状态、水合事件处理器全部缺失）。
//
// 相对 Streams 标准的简化（本引擎的调用场景不需要的部分）：
//   - 无真正背压：desiredSize 恒为 1，enqueue 立即推给下游（pipeTo 已连接时）
//     或入队等待读
//   - 无 async pull 调度：源头数据同步入队；read() 始终返回 Promise，值在
//     goja 微任务队列中被 await 消费（宿主 RunJobs / ProcessTasks 驱动）
//   - 未实现 locked / tee / transfer / ByteStream 专用读控制器
//   - pipeTo 的目标必须是本实现创建的 TransformStream 可写端（同一 realm）；
//     其他目标退化为立即 resolve（不抛错，避免中断调用方脚本）
package jsc

import (
	"sync"

	"wb-ui/goja"
)

// ─── 内部状态 ────────────────────────────────────────────

// streamReadRequest 是一次挂起的 read()：流内暂无数据时排队，等 enqueue /
// close / error 结清。
type streamReadRequest struct {
	resolve func(interface{}) error
	reject  func(interface{}) error
}

// readableState 是 ReadableStream（及其派生流）的内部状态。
type readableState struct {
	vm      *goja.Runtime
	obj     *goja.Object
	items   []goja.Value
	done    bool
	err     goja.Value
	pending []*streamReadRequest
	// down 是 pipeTo/pipeThrough 连接的下游 TransformStream：非 nil 时
	// enqueue 直接推给下游，本流不再缓冲（简化背压）。
	down *transformState
}

// readerState 是 ReadableStreamDefaultReader 的内部状态。
type readerState struct {
	st *readableState
}

// transformState 是 TransformStream 的内部状态，同时充当它的可写端
// （pipeTo 的目标）。
type transformState struct {
	vm          *goja.Runtime
	readable    *readableState
	readableObj *goja.Object
	writableObj *goja.Object
	obj         *goja.Object
	transform   goja.Value
	flush       goja.Value
	closed      bool
}

// streamRealm 是每个 goja.Runtime 一份的 Streams 原型/构造器集合。
// 原型挂在构造器的 prototype 上，实例经 __proto__ 继承（instanceof 成立）。
type streamRealm struct {
	readableProto  *goja.Object
	transformProto *goja.Object
	encoderProto   *goja.Object
	decoderProto   *goja.Object
	writableProto  *goja.Object
	readerProto    *goja.Object
	writerProto    *goja.Object
}

var (
	streamRealmsMu sync.Mutex
	streamRealms   = map[*goja.Runtime]*streamRealm{}
)

// ─── 注册 ────────────────────────────────────────────────

// registerStreams 注册 ReadableStream / TransformStream / TextEncoderStream /
// TextDecoderStream 及其原型。由 RegisterWebAPIs 调用。
func (r *Interpreter) registerStreams() {
	realm := &streamRealm{}
	streamRealmsMu.Lock()
	streamRealms[r.vm] = realm
	streamRealmsMu.Unlock()

	r.registerReadableStream(realm)
	r.registerTransformStream(realm)
	r.registerTextEncoderStream(realm)
	r.registerTextDecoderStream(realm)
}

// realmOf 返回该运行时的 Streams realm（必已由 registerStreams 建好）。
func (r *Interpreter) realmOf() *streamRealm {
	streamRealmsMu.Lock()
	defer streamRealmsMu.Unlock()
	return streamRealms[r.vm]
}

// ─── ReadableStream ─────────────────────────────────────

func (r *Interpreter) registerReadableStream(realm *streamRealm) {
	// ReadableStreamDefaultReader.prototype
	readerProto := r.vm.NewObject()
	realm.readerProto = readerProto
	readerProto.Set("read", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if rs := readerStateOf(call.This); rs != nil && rs.st != nil {
			return rs.st.read()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	readerProto.Set("releaseLock", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}))
	readerProto.Set("cancel", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if rs := readerStateOf(call.This); rs != nil && rs.st != nil {
			rs.st.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	readerProto.Set("closed", r.resolvedPromise(goja.Undefined()))

	// ReadableStream.prototype
	readableProto := r.vm.NewObject()
	realm.readableProto = readableProto

	readableProto.Set("getReader", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			return goja.Undefined()
		}
		obj := r.vm.NewObject()
		obj.Set("__proto__", readerProto)
		obj.Internal = &readerState{st: st}
		return obj
	}))
	readableProto.Set("pipeThrough", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		ts := transformStateOf(call.Argument(0))
		if st == nil || ts == nil {
			return goja.Undefined()
		}
		st.pipeInto(ts)
		return ts.readableObj
	}))
	readableProto.Set("pipeTo", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		// 目标可以是 TransformStream 对象本身（取它的可写端）或可写端对象。
		ts := transformStateOf(call.Argument(0))
		if st != nil && ts != nil {
			st.pipeInto(ts)
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	readableProto.Set("cancel", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if st := readableStateOf(call.This); st != nil {
			st.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	readableProto.Set("locked", r.vm.ToValue(false))

	// ReadableStream 构造器
	var ctorVal goja.Value
	ctorVal = r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		src := call.Argument(0)
		st := r.newReadableState(realm)
		if o, ok := src.(*goja.Object); ok {
			if fn, ok2 := goja.AssertFunction(o.Get("start")); ok2 {
				if _, err := fn(goja.Undefined(), st.controllerObject()); err != nil {
					st.errorStream(r.vm.ToValue(err.Error()))
				}
			}
		}
		return st.obj
	})
	r.vm.Set("ReadableStream", ctorVal)
	// 让 `stream instanceof ReadableStream` 成立：构造器 prototype 指向
	// 实例实际继承的原型对象（goja 自动建的 prototype 会被这里覆盖）。
	if o, ok := ctorVal.(*goja.Object); ok {
		o.Set("prototype", readableProto)
	}
}

// newReadableState 创建一条可读流（含对外对象，__proto__ 指向可读流原型）。
func (r *Interpreter) newReadableState(realm *streamRealm) *readableState {
	st := &readableState{vm: r.vm}
	obj := r.vm.NewObject()
	if realm != nil && realm.readableProto != nil {
		obj.Set("__proto__", realm.readableProto)
	}
	obj.Internal = st
	st.obj = obj
	return st
}

// controllerObject 创建 ReadableStreamDefaultController 形态的控制器
// （enqueue / close / error / desiredSize）。
func (s *readableState) controllerObject() goja.Value {
	c := s.vm.NewObject()
	c.Set("enqueue", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		s.enqueue(call.Argument(0))
		return goja.Undefined()
	}))
	c.Set("close", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		s.closeStream()
		return goja.Undefined()
	}))
	c.Set("error", s.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		s.errorStream(call.Argument(0))
		return goja.Undefined()
	}))
	// 简化：无真实背压，desiredSize 恒为 1（>0 表示可继续写入）。
	c.Set("desiredSize", s.vm.ToValue(int64(1)))
	return c
}

// enqueue 入队一个 chunk：已连接下游则直接推给下游（含 transform 链），
// 否则满足挂起的 read()，再否则缓冲。
func (s *readableState) enqueue(chunk goja.Value) {
	if s.done || s.err != nil {
		return
	}
	if s.down != nil {
		s.down.write(chunk)
		return
	}
	if len(s.pending) > 0 {
		req := s.pending[0]
		s.pending = s.pending[1:]
		_ = req.resolve(s.readResult(chunk, false))
		return
	}
	s.items = append(s.items, chunk)
}

// readResult 构造 read() 的结清值 {value, done}。
func (s *readableState) readResult(v goja.Value, done bool) goja.Value {
	o := s.vm.NewObject()
	o.Set("value", v)
	o.Set("done", s.vm.ToValue(done))
	return o
}

// closeStream 关闭流：已连接下游则向下游传播（触发 flush → 下游关闭）。
func (s *readableState) closeStream() {
	if s.done || s.err != nil {
		return
	}
	if s.down != nil {
		s.down.closeStream()
		return
	}
	s.done = true
	for _, req := range s.pending {
		_ = req.resolve(s.readResult(goja.Undefined(), true))
	}
	s.pending = nil
}

// errorStream 以错误结束流：挂起的 read() 全部 reject。
func (s *readableState) errorStream(e goja.Value) {
	if s.done || s.err != nil {
		return
	}
	if e == nil {
		e = s.vm.ToValue("stream error")
	}
	s.err = e
	for _, req := range s.pending {
		_ = req.reject(e)
	}
	s.pending = nil
	s.down = nil
}

// read 返回 Promise<{value, done}>：有缓冲立即结清，否则挂起等数据。
func (s *readableState) read() goja.Value {
	p, resolve, reject := s.vm.NewPromise()
	if s.err != nil {
		_ = reject(s.err)
		return p.PromiseObj()
	}
	if len(s.items) > 0 {
		chunk := s.items[0]
		s.items = s.items[1:]
		_ = resolve(s.readResult(chunk, false))
		return p.PromiseObj()
	}
	if s.done {
		_ = resolve(s.readResult(goja.Undefined(), true))
		return p.PromiseObj()
	}
	s.pending = append(s.pending, &streamReadRequest{resolve: resolve, reject: reject})
	return p.PromiseObj()
}

// pipeInto 把本流接到下游 TransformStream 的可写端：已缓冲的数据立即泵入，
// 之后 enqueue/close/error 直接传播（不阻塞调用方脚本）。
func (s *readableState) pipeInto(t *transformState) {
	if t == nil || s.down != nil {
		return
	}
	s.down = t
	buffered := s.items
	s.items = nil
	for _, chunk := range buffered {
		t.write(chunk)
	}
	if s.err != nil {
		t.readable.errorStream(s.err)
		return
	}
	if s.done {
		t.closeStream()
	}
}

// ─── TransformStream ────────────────────────────────────

func (r *Interpreter) registerTransformStream(realm *streamRealm) {
	// WritableStream.prototype（getWriter 形态；write/close/abort 简化实现）
	writableProto := r.vm.NewObject()
	realm.writableProto = writableProto
	writableProto.Set("getWriter", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		ts := transformStateOf(call.This)
		if ts == nil {
			return goja.Undefined()
		}
		obj := r.vm.NewObject()
		obj.Set("__proto__", realm.writerProto)
		obj.Internal = ts
		return obj
	}))
	writableProto.Set("abort", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writableProto.Set("close", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writableProto.Set("locked", r.vm.ToValue(false))

	writerProto := r.vm.NewObject()
	realm.writerProto = writerProto
	writerProto.Set("write", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.write(call.Argument(0))
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writerProto.Set("close", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writerProto.Set("abort", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writerProto.Set("releaseLock", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}))
	writerProto.Set("ready", r.resolvedPromise(goja.Undefined()))
	writerProto.Set("closed", r.resolvedPromise(goja.Undefined()))

	transformProto := r.vm.NewObject()
	realm.transformProto = transformProto

	var ctorVal goja.Value
	ctorVal = r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		ts := r.newTransformState(realm, transformProto, call.Argument(0))
		return ts.obj
	})
	r.vm.Set("TransformStream", ctorVal)
	if o, ok := ctorVal.(*goja.Object); ok {
		o.Set("prototype", transformProto)
	}
}

// newTransformState 创建一条转换流：可读端（readable）+ 可写端（writable）。
// selfProto 是本体对象的原型（TransformStream 或 TextEncoderStream 等）；
// transformer 为 {transform, flush} 形态的字典（可为 undefined）。
func (r *Interpreter) newTransformState(realm *streamRealm, selfProto *goja.Object, transformer goja.Value) *transformState {
	readable := r.newReadableState(realm)
	ts := &transformState{vm: r.vm, readable: readable, readableObj: readable.obj}

	w := r.vm.NewObject()
	if realm != nil && realm.writableProto != nil {
		w.Set("__proto__", realm.writableProto)
	}
	w.Internal = ts
	ts.writableObj = w

	obj := r.vm.NewObject()
	if selfProto != nil {
		obj.Set("__proto__", selfProto)
	}
	obj.Internal = ts
	obj.Set("readable", ts.readableObj)
	obj.Set("writable", w)
	ts.obj = obj

	if o, ok := transformer.(*goja.Object); ok {
		ts.transform = o.Get("transform")
		ts.flush = o.Get("flush")
	}
	return ts
}

// write 把一个 chunk 写入转换流：调用 transform(chunk, controller)，
// controller.enqueue 写入可读端。无 transform 时透传。
func (t *transformState) write(chunk goja.Value) {
	if t.closed {
		return
	}
	fn, ok := goja.AssertFunction(t.transform)
	if !ok {
		t.readable.enqueue(chunk)
		return
	}
	if _, err := fn(goja.Undefined(), chunk, t.readable.controllerObject()); err != nil {
		t.readable.errorStream(t.vm.ToValue(err.Error()))
	}
}

// closeStream 关闭转换流：先调用 flush(controller)（尾部数据），再关闭可读端。
func (t *transformState) closeStream() {
	if t.closed {
		return
	}
	t.closed = true
	if fn, ok := goja.AssertFunction(t.flush); ok {
		if _, err := fn(goja.Undefined(), t.readable.controllerObject()); err != nil {
			t.readable.errorStream(t.vm.ToValue(err.Error()))
			return
		}
	}
	t.readable.closeStream()
}

// ─── TextEncoderStream / TextDecoderStream ──────────────

func (r *Interpreter) registerTextEncoderStream(realm *streamRealm) {
	proto := r.vm.NewObject()
	realm.encoderProto = proto
	var ctorVal goja.Value
	ctorVal = r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		ts := r.newTransformState(realm, proto, goja.Undefined())
		ts.transform = r.newTextEncoderTransform()
		return ts.obj
	})
	r.vm.Set("TextEncoderStream", ctorVal)
	if o, ok := ctorVal.(*goja.Object); ok {
		o.Set("prototype", proto)
	}
}

func (r *Interpreter) registerTextDecoderStream(realm *streamRealm) {
	proto := r.vm.NewObject()
	realm.decoderProto = proto
	var ctorVal goja.Value
	ctorVal = r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		ts := r.newTransformState(realm, proto, goja.Undefined())
		transform, flush := r.newTextDecoderTransforms()
		ts.transform = transform
		ts.flush = flush
		return ts.obj
	})
	r.vm.Set("TextDecoderStream", ctorVal)
	if o, ok := ctorVal.(*goja.Object); ok {
		o.Set("prototype", proto)
	}
}

// newTextEncoderTransform 返回 string → Uint8Array 的 transform 函数。
// 复用已注册的 TextEncoder（Go 原生），保持与 textEncoder.encode 完全一致。
func (r *Interpreter) newTextEncoderTransform() goja.Value {
	encCtor := r.vm.Get("TextEncoder")
	if encCtor == nil {
		return goja.Undefined()
	}
	encObj, err := r.vm.New(encCtor)
	if err != nil {
		return goja.Undefined()
	}
	encodeFn, ok := goja.AssertFunction(encObj.Get("encode"))
	if !ok {
		return goja.Undefined()
	}
	return r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		out, err := encodeFn(goja.Undefined(), call.Argument(0))
		if err != nil {
			return goja.Undefined()
		}
		r.callMethod(call.Argument(1), "enqueue", out)
		return goja.Undefined()
	})
}

// newTextDecoderTransforms 返回 Uint8Array → string 的 (transform, flush)。
// transform 用 {stream: true} 保留不完整的多字节序列，flush 结清残余。
func (r *Interpreter) newTextDecoderTransforms() (goja.Value, goja.Value) {
	decCtor := r.vm.Get("TextDecoder")
	if decCtor == nil {
		return goja.Undefined(), goja.Undefined()
	}
	decObj, err := r.vm.New(decCtor)
	if err != nil {
		return goja.Undefined(), goja.Undefined()
	}
	decodeFn, ok := goja.AssertFunction(decObj.Get("decode"))
	if !ok {
		return goja.Undefined(), goja.Undefined()
	}
	opts := r.vm.NewObject()
	opts.Set("stream", r.vm.ToValue(true))
	transform := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		out, err := decodeFn(goja.Undefined(), call.Argument(0), opts)
		if err != nil {
			return goja.Undefined()
		}
		if s := out.String(); s != "" {
			r.callMethod(call.Argument(1), "enqueue", out)
		}
		return goja.Undefined()
	})
	flush := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		out, err := decodeFn(goja.Undefined())
		if err != nil {
			return goja.Undefined()
		}
		if s := out.String(); s != "" {
			r.callMethod(call.Argument(0), "enqueue", out)
		}
		return goja.Undefined()
	})
	return transform, flush
}

// ─── 辅助 ────────────────────────────────────────────────

// resolvedPromise 返回已结清的 Promise（用于无真正异步性的 API 返回值）。
func (r *Interpreter) resolvedPromise(v goja.Value) goja.Value {
	p, resolve, _ := r.vm.NewPromise()
	_ = resolve(v)
	return p.PromiseObj()
}

// callMethod 调用对象的某个方法（this = 该对象），失败静默返回 undefined。
func (r *Interpreter) callMethod(obj goja.Value, method string, args ...goja.Value) goja.Value {
	o, ok := obj.(*goja.Object)
	if !ok {
		return goja.Undefined()
	}
	fn, ok := goja.AssertFunction(o.Get(method))
	if !ok {
		return goja.Undefined()
	}
	res, err := fn(o, args...)
	if err != nil {
		return goja.Undefined()
	}
	return res
}

// readableStateOf 从 this 取出可读流状态（非流对象返回 nil）。
func readableStateOf(this goja.Value) *readableState {
	o, ok := this.(*goja.Object)
	if !ok || o == nil {
		return nil
	}
	st, _ := o.Internal.(*readableState)
	return st
}

// readerStateOf 从 this 取出读控制器状态。
func readerStateOf(this goja.Value) *readerState {
	o, ok := this.(*goja.Object)
	if !ok || o == nil {
		return nil
	}
	rs, _ := o.Internal.(*readerState)
	return rs
}

// transformStateOf 从 JS 值取出转换流状态：既接受 TransformStream 本体对象，
// 也接受它的可读端/可写端（pipeTo 的目标常是 writable）。
func transformStateOf(v goja.Value) *transformState {
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return nil
	}
	if ts, ok := o.Internal.(*transformState); ok {
		return ts
	}
	if st, ok := o.Internal.(*readableState); ok {
		return st.down
	}
	return nil
}
