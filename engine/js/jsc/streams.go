// Package jsc — Streams 实现（ReadableStream / TransformStream /
// TextEncoderStream / TextDecoderStream / WritableStream 可写端）。
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
// 已实现的规范语义：
//   - 队列与背压：QueuingStrategy（highWaterMark / size）真实生效，
//     controller.desiredSize = highWaterMark - 队列总大小（流结束/出错后为 null）
//   - pull 调度：desiredSize > 0 且流已被读取时，在微任务中调用
//     underlyingSource.pull(controller)（按需生产，不预取）
//   - 锁：getReader() 锁流（locked 属性、二次 getReader 抛 TypeError）、
//     reader.releaseLock()（挂起读请求被 reject）、reader.closed Promise
//   - cancel(reason)：结清读请求、调用 underlyingSource.cancel(reason)、
//     已连接下游时中止下游
//   - tee()：把流分成两个分支（原流被锁定；按两分支的需求背压拉取）
//   - pipeThrough/pipeTo：本实现的 TransformStream 走同步泵；其它任意
//     {readable, writable} 或鸭子类型可写端（getWriter()/write）走 then 链泵，
//     支持 {preventClose, preventAbort, preventCancel}，返回 Promise
//
// 仍不实现（本引擎场景不需要；如需请在此扩展）：
//   - BYOB reader（type:'bytes' 的流按普通读处理，不分离 ArrayBuffer）
//   - 可转移（transferable）流与 structuredClone 的 stream 支持
//   - ReadableStream.from / asyncIterator 协议
package jsc

import (
	"sync"

	"wb-ui/engine/js/goja"
)

// ─── 内部状态 ────────────────────────────────────────────

// streamChunk 是队列里的一个分块：值 + 按 size 策略算出的权重（背压用）。
type streamChunk struct {
	value goja.Value
	size  float64
}

// streamReadRequest 是一次挂起的 read()：流内暂无数据时排队，等 enqueue /
// close / error 结清。
type streamReadRequest struct {
	resolve func(interface{}) error
	reject  func(interface{}) error
}

// readableState 是 ReadableStream（及其派生流）的内部状态。
type readableState struct {
	interp  *Interpreter
	vm      *goja.Runtime
	obj     *goja.Object
	items   []streamChunk
	done    bool
	err     goja.Value
	pending []*streamReadRequest
	// down 是 pipeTo/pipeThrough 连接的下游 TransformStream：非 nil 时 enqueue
	// 直接推给下游（同步泵，确定性最好）。
	down *transformState

	// 队列策略（背压）
	highWaterMark float64
	sizeFn        goja.Value
	queueSize     float64

	// underlyingSource 钩子
	pullFn   goja.Value
	cancelFn goja.Value
	pulling  bool

	// ctrl 是 ReadableStreamDefaultController（同一流始终同一对象：start 与
	// pull 收到的必须 ===）。
	ctrl *goja.Object

	// reader 是当前持有的读锁（locked 属性 = reader != nil）。
	reader *readerState
}

// readerState 是 ReadableStreamDefaultReader 的内部状态。
type readerState struct {
	interp        *Interpreter
	st            *readableState
	obj           *goja.Object
	closedPromise goja.Value
	closedResolve func(interface{}) error
	closedReject  func(interface{}) error
}

// transformState 是 TransformStream 的内部状态，同时充当它的可写端
// （pipeTo 的目标）。
type transformState struct {
	interp      *Interpreter
	vm          *goja.Runtime
	readable    *readableState
	readableObj *goja.Object
	writableObj *goja.Object
	obj         *goja.Object
	transform   goja.Value
	flush       goja.Value
	closed      bool
	// writer 是当前持有的写锁（getWriter 后 writable.locked = true）。
	writer *goja.Object
	// onClose 在可写端关闭后调用（pipeTo 的完成通知）。
	onClose func()
}

// pipeOptions 是 pipeTo/pipeThrough 的选项（对应规范的 PreventClose 等）。
type pipeOptions struct {
	preventClose  bool
	preventAbort  bool
	preventCancel bool
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

// ─── 微任务调度 ──────────────────────────────────────────

// queueMicrotask 把一个 Go 闭包排入 goja 的 Promise 微任务队列（由宿主的
// RunJobs 驱动）。没有 Promise 时退化为同步调用（保证功能不丢）。
func (r *Interpreter) queueMicrotask(fn func()) {
	p, resolve, _ := r.vm.NewPromise()
	_ = resolve(goja.Undefined())
	obj := p.PromiseObj()
	then, ok := goja.AssertFunction(obj.Get("then"))
	if !ok {
		fn()
		return
	}
	cb := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		fn()
		return goja.Undefined()
	})
	if _, err := then(obj, cb); err != nil {
		fn()
	}
}

// thenValue 在 v 是 thenable 时按 then 链回调，否则立即调用 onOk。
// （pipeTo 的鸭子类型目标：write() 可能返回 Promise，也可能同步返回。）
func (r *Interpreter) thenValue(v goja.Value, onOk func(goja.Value), onErr func(goja.Value)) {
	obj, ok := v.(*goja.Object)
	if !ok {
		onOk(v)
		return
	}
	then, ok := goja.AssertFunction(obj.Get("then"))
	if !ok {
		onOk(v)
		return
	}
	okFn := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		onOk(call.Argument(0))
		return goja.Undefined()
	})
	errFn := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if onErr != nil {
			onErr(call.Argument(0))
		}
		return goja.Undefined()
	})
	if _, err := then(obj, okFn, errFn); err != nil && onErr != nil {
		onErr(r.vm.ToValue(err.Error()))
	}
}

// callOrIgnore 调用对象上的方法（忽略返回值与错误）。
func (r *Interpreter) callOrIgnore(obj goja.Value, method string, args ...goja.Value) {
	o, ok := obj.(*goja.Object)
	if !ok {
		return
	}
	fn, ok := goja.AssertFunction(o.Get(method))
	if !ok {
		return
	}
	_, _ = fn(o, args...)
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
		rs := readerStateOf(call.This)
		if rs == nil || rs.st == nil {
			return goja.Undefined()
		}
		st := rs.st
		// 规范：释放锁会让挂起的读请求以 TypeError 拒绝。
		if len(st.pending) > 0 {
			te := r.vm.NewTypeError("This reader has been released")
			for _, req := range st.pending {
				_ = req.reject(te)
			}
			st.pending = nil
		}
		if st.reader == rs {
			st.reader = nil
		}
		rs.st = nil
		return goja.Undefined()
	}))
	readerProto.Set("cancel", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if rs := readerStateOf(call.This); rs != nil && rs.st != nil {
			return rs.st.cancelStream(call.Argument(0))
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	_ = readerProto.DefineAccessorProperty("closed",
		r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if rs := readerStateOf(call.This); rs != nil {
				if rs.closedPromise != nil {
					return rs.closedPromise
				}
				return r.resolvedPromise(goja.Undefined())
			}
			return r.resolvedPromise(goja.Undefined())
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	// ReadableStream.prototype
	readableProto := r.vm.NewObject()
	realm.readableProto = readableProto

	readableProto.Set("getReader", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if st.reader != nil {
			panic(r.vm.NewTypeError("ReadableStream is locked"))
		}
		// mode:'byob' 不支持缓冲区分离语义（按默认读处理，见文件头说明）。
		return r.newReaderObject(realm, st)
	}))
	readableProto.Set("pipeThrough", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			return goja.Undefined()
		}
		arg := call.Argument(0)
		o, ok := arg.(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("pipeThrough expects a {readable, writable} pair"))
		}
		// 本实现的 TransformStream（含 TextEncoder/DecoderStream）：同步泵。
		if ts := transformStateOf(arg); ts != nil {
			st.pipeInto(ts)
			return ts.readableObj
		}
		// 任意 {readable, writable} 对（鸭子类型）。
		readable := o.Get("readable")
		writable := o.Get("writable")
		if writable == nil || goja.IsUndefined(writable) {
			panic(r.vm.NewTypeError("pipeThrough target has no writable"))
		}
		_ = r.pipeToDuck(st, writable, r.pipeOptionsFrom(call.Argument(1)))
		return readable
	}))
	readableProto.Set("pipeTo", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			return r.resolvedPromise(goja.Undefined())
		}
		target := call.Argument(0)
		if ts := transformStateOf(target); ts != nil {
			st.pipeInto(ts)
			return r.resolvedPromise(goja.Undefined())
		}
		return r.pipeToDuck(st, target, r.pipeOptionsFrom(call.Argument(1)))
	}))
	readableProto.Set("cancel", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			return r.resolvedPromise(goja.Undefined())
		}
		return st.cancelStream(call.Argument(0))
	}))
	readableProto.Set("tee", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		st := readableStateOf(call.This)
		if st == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		a, b := r.teeReadable(st, realm)
		return r.vm.NewArray(a, b)
	}))
	_ = readableProto.DefineAccessorProperty("locked",
		r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if st := readableStateOf(call.This); st != nil {
				return r.vm.ToValue(st.reader != nil)
			}
			return r.vm.ToValue(false)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

	// ReadableStream 构造器
	var ctorVal goja.Value
	ctorVal = r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		src := call.Argument(0)
		st := r.newReadableState(realm)
		st.applyStrategy(call.Argument(1))
		if o, ok := src.(*goja.Object); ok {
			st.pullFn = o.Get("pull")
			st.cancelFn = o.Get("cancel")
			if fn, ok2 := goja.AssertFunction(o.Get("start")); ok2 {
				if _, err := fn(goja.Undefined(), st.controller()); err != nil {
					st.errorStream(r.vm.ToValue(err.Error()))
				}
			}
		}
		// start 之后按需拉取（仅当流已被读取时才会真正调用 pull）。
		st.maybePull()
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
	st := &readableState{interp: r, vm: r.vm, highWaterMark: 1}
	obj := r.vm.NewObject()
	if realm != nil && realm.readableProto != nil {
		obj.Set("__proto__", realm.readableProto)
	}
	obj.Internal = st
	st.obj = obj
	return st
}

// newReaderObject 创建 reader 对象（锁流、建 closed Promise）。
func (r *Interpreter) newReaderObject(realm *streamRealm, st *readableState) goja.Value {
	p, resolve, reject := r.vm.NewPromise()
	rd := &readerState{
		interp:        r,
		st:            st,
		closedPromise: p.PromiseObj(),
		closedResolve: resolve,
		closedReject:  reject,
	}
	if st.done {
		_ = resolve(goja.Undefined())
	} else if st.err != nil {
		_ = reject(st.err)
	}
	obj := r.vm.NewObject()
	if realm != nil && realm.readerProto != nil {
		obj.Set("__proto__", realm.readerProto)
	}
	obj.Internal = rd
	rd.obj = obj
	st.reader = rd
	return obj
}

// applyStrategy 应用 QueuingStrategy（second argument）：highWaterMark / size。
func (s *readableState) applyStrategy(v goja.Value) {
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return
	}
	if hwm := o.Get("highWaterMark"); hwm != nil && !goja.IsUndefined(hwm) {
		if f := hwm.ToFloat(); f > 0 {
			s.highWaterMark = f
		}
	}
	if sz := o.Get("size"); sz != nil && !goja.IsUndefined(sz) {
		s.sizeFn = sz
	}
}

// chunkSize 返回一个 chunk 的队列大小（默认 1，可被 strategy.size 覆盖）。
func (s *readableState) chunkSize(chunk goja.Value) float64 {
	fn, ok := goja.AssertFunction(s.sizeFn)
	if !ok {
		return 1
	}
	out, err := fn(goja.Undefined(), chunk)
	if err != nil {
		return 1
	}
	f := out.ToFloat()
	if f <= 0 {
		return 1
	}
	return f
}

// desiredSize 是控制器可继续入队的空间；流结束/出错后为 null（规范语义）。
func (s *readableState) desiredSize() goja.Value {
	if s.done || s.err != nil {
		return goja.Null()
	}
	return s.vm.ToValue(s.highWaterMark - s.queueSize)
}

// controller 返回该流的 ReadableStreamDefaultController（同一流同一对象）。
func (s *readableState) controller() *goja.Object {
	if s.ctrl != nil {
		return s.ctrl
	}
	c := s.vm.NewObject()
	s.ctrl = c
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
	_ = c.DefineAccessorProperty("desiredSize",
		s.vm.ToValue(func(goja.FunctionCall) goja.Value { return s.desiredSize() }),
		goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	return c
}

// enqueue 入队一个 chunk：已连接下游则直接推给下游（含 transform 链），
// 否则满足挂起的 read()，再否则缓冲（计入队列大小/背压）。
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
		s.maybePull()
		return
	}
	s.items = append(s.items, streamChunk{value: chunk, size: s.chunkSize(chunk)})
	s.queueSize += s.items[len(s.items)-1].size
	s.maybePull()
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
	s.settleReaderClosed(nil)
}

// errorStream 以错误结束流：挂起的 read() 全部 reject，并向下游传播。
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
	if s.down != nil {
		down := s.down
		s.down = nil
		down.errorStream(e)
	}
	s.settleReaderClosed(e)
}

// cancelStream 取消流（readable.cancel(reason) / reader.cancel(reason)）：
// 中止已连接的下游、结清读请求、调用 underlyingSource.cancel(reason)。
func (s *readableState) cancelStream(reason goja.Value) goja.Value {
	p, resolve, _ := s.vm.NewPromise()
	if s.done || s.err != nil {
		_ = resolve(goja.Undefined())
		return p.PromiseObj()
	}
	if s.down != nil {
		down := s.down
		s.down = nil
		down.errorStream(reason)
	}
	s.done = true
	for _, req := range s.pending {
		_ = req.resolve(s.readResult(goja.Undefined(), true))
	}
	s.pending = nil
	s.settleReaderClosed(nil)
	if fn, ok := goja.AssertFunction(s.cancelFn); ok {
		if _, err := fn(goja.Undefined(), reason); err != nil {
			_ = resolve(goja.Undefined())
			return p.PromiseObj()
		}
	}
	_ = resolve(goja.Undefined())
	return p.PromiseObj()
}

// settleReaderClosed 结清 reader.closed（正常结束 resolve，出错 reject）。
func (s *readableState) settleReaderClosed(err goja.Value) {
	rd := s.reader
	if rd == nil || rd.closedResolve == nil {
		return
	}
	if err == nil {
		_ = rd.closedResolve(goja.Undefined())
	} else {
		_ = rd.closedReject(err)
	}
	rd.closedResolve = nil
	rd.closedReject = nil
}

// read 返回 Promise<{value, done}>：有缓冲立即结清，否则挂起等数据。
func (s *readableState) read() goja.Value {
	p, resolve, reject := s.vm.NewPromise()
	if s.err != nil {
		_ = reject(s.err)
		return p.PromiseObj()
	}
	if len(s.items) > 0 {
		c := s.items[0]
		s.items = s.items[1:]
		s.queueSize -= c.size
		if s.queueSize < 0 {
			s.queueSize = 0
		}
		_ = resolve(s.readResult(c.value, false))
		s.maybePull()
		return p.PromiseObj()
	}
	if s.done {
		_ = resolve(s.readResult(goja.Undefined(), true))
		return p.PromiseObj()
	}
	s.pending = append(s.pending, &streamReadRequest{resolve: resolve, reject: reject})
	s.maybePull()
	return p.PromiseObj()
}

// shouldPull 判定是否需要调用 underlyingSource.pull：有 pull、未结束、队列未满，
// 且流已被读取（规范 ShouldCallPull——没人读时不预取）。
func (s *readableState) shouldPull() bool {
	if s.done || s.err != nil || s.down != nil {
		return false
	}
	if s.pullFn == nil || goja.IsUndefined(s.pullFn) {
		return false
	}
	if s.reader == nil {
		return false
	}
	return s.highWaterMark-s.queueSize > 0
}

// maybePull 在微任务中调度一次 pull（已在调度中时跳过）。
func (s *readableState) maybePull() {
	if s.pulling || !s.shouldPull() {
		return
	}
	s.pulling = true
	s.interp.queueMicrotask(func() {
		s.pulling = false
		if !s.shouldPull() {
			return
		}
		fn, ok := goja.AssertFunction(s.pullFn)
		if !ok {
			return
		}
		if _, err := fn(goja.Undefined(), s.controller()); err != nil {
			s.errorStream(s.vm.ToValue(err.Error()))
		}
	})
}

// pipeInto 把本流接到下游 TransformStream 的可写端：已缓冲的数据立即泵入，
// 之后 enqueue/close/error 直接传播（同步泵，不阻塞调用方脚本）。
func (s *readableState) pipeInto(t *transformState) {
	if t == nil || s.down != nil {
		return
	}
	s.down = t
	t.onClose = nil
	buffered := s.items
	s.items = nil
	s.queueSize = 0
	for _, chunk := range buffered {
		t.write(chunk.value)
	}
	if s.err != nil {
		t.readable.errorStream(s.err)
		return
	}
	if s.done {
		t.closeStream()
	}
}

// ─── tee ────────────────────────────────────────────────

// teeReadable 把流分成两个独立分支（原流被锁定）。两分支都还有队列空间时才
// 继续向源流拉取——tee 的背压语义正是「最慢的分支决定拉取速度」。
func (r *Interpreter) teeReadable(src *readableState, realm *streamRealm) (goja.Value, goja.Value) {
	branchA := r.newReadableState(realm)
	branchB := r.newReadableState(realm)
	// tee 会锁住原流（规范：tee() 后原流 locked）。
	if src.reader == nil {
		src.reader = &readerState{interp: r, st: src}
	}

	branchWants := func(b *readableState) bool {
		if b.done || b.err != nil {
			return false
		}
		return b.highWaterMark-b.queueSize > 0
	}
	closeBoth := func(e goja.Value) {
		if e == nil {
			branchA.closeStream()
			branchB.closeStream()
			return
		}
		branchA.errorStream(e)
		branchB.errorStream(e)
	}

	var pump func()
	pump = func() {
		if !branchWants(branchA) || !branchWants(branchB) {
			return // 有分支队列已满：等它被读取后再继续（tee 的背压）
		}
		// 需要时再拉：分支被读走数据后由 maybePull 触发（见 pullFn 布置）
		res := src.read()
		r.thenValue(res, func(v goja.Value) {
			o, ok := v.(*goja.Object)
			if !ok {
				closeBoth(nil)
				return
			}
			if d := o.Get("done"); d != nil && d.ToBoolean() {
				closeBoth(nil)
				return
			}
			val := o.Get("value")
			branchA.enqueue(val)
			branchB.enqueue(val)
			pump()
		}, func(e goja.Value) {
			closeBoth(e)
		})
	}
	// 分支被读取/写入后继续泵（两分支各自成为驱动源）。
	installTeePump := func(b *readableState) {
		b.pullFn = r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			pump()
			return goja.Undefined()
		})
		b.highWaterMark = 1
	}
	installTeePump(branchA)
	installTeePump(branchB)
	pump()
	return branchA.obj, branchB.obj
}

// ─── pipeTo（鸭子类型可写端）─────────────────────────────

// pipeOptionsFrom 解析 pipeTo/pipeThrough 的选项对象。
func (r *Interpreter) pipeOptionsFrom(v goja.Value) pipeOptions {
	opts := pipeOptions{}
	o, ok := v.(*goja.Object)
	if !ok || o == nil {
		return opts
	}
	if p := o.Get("preventClose"); p != nil && !goja.IsUndefined(p) {
		opts.preventClose = p.ToBoolean()
	}
	if p := o.Get("preventAbort"); p != nil && !goja.IsUndefined(p) {
		opts.preventAbort = p.ToBoolean()
	}
	if p := o.Get("preventCancel"); p != nil && !goja.IsUndefined(p) {
		opts.preventCancel = p.ToBoolean()
	}
	return opts
}

// pipeWriter 是可写端的鸭子类型视图（write/close/abort 三个方法）。
type pipeWriter struct {
	obj   goja.Value
	write goja.Value
	close goja.Value
	abort goja.Value
}

// resolvePipeWriter 把 pipeTo 的目标解析成 writer：优先 getWriter()，其次目标
// 自身就是 writer（有 write 方法）。
func (r *Interpreter) resolvePipeWriter(target goja.Value) *pipeWriter {
	o, ok := target.(*goja.Object)
	if !ok || o == nil {
		return nil
	}
	if fn, ok := goja.AssertFunction(o.Get("getWriter")); ok {
		if w, err := fn(o); err == nil {
			if wo, ok := w.(*goja.Object); ok {
				return &pipeWriter{
					obj:   wo,
					write: wo.Get("write"),
					close: wo.Get("close"),
					abort: wo.Get("abort"),
				}
			}
		}
		return nil
	}
	if w := o.Get("write"); w != nil && !goja.IsUndefined(w) {
		return &pipeWriter{obj: o, write: w, close: o.Get("close"), abort: o.Get("abort")}
	}
	return nil
}

// pipeToDuck 实现任意可写端的 pipeTo：then 链泵数据、结束关闭目标、错误双向往
// 传播，返回 Promise（写完/关闭后 resolve）。选项语义遵循规范。
func (r *Interpreter) pipeToDuck(st *readableState, target goja.Value, opts pipeOptions) goja.Value {
	p, resolve, reject := r.vm.NewPromise()
	w := r.resolvePipeWriter(target)
	if w == nil {
		_ = reject(r.vm.NewTypeError("pipeTo target is not a writable stream"))
		return p.PromiseObj()
	}
	// pipeTo 会锁定源流（规范语义）。
	if st.reader == nil {
		st.reader = &readerState{interp: r, st: st}
	}
	done := false
	finish := func(v goja.Value) {
		if done {
			return
		}
		done = true
		_ = resolve(v)
	}
	fail := func(e goja.Value) {
		if done {
			return
		}
		done = true
		_ = reject(e)
	}

	var pump func()
	pump = func() {
		if done {
			return
		}
		res := st.read()
		r.thenValue(res, func(v goja.Value) {
			o, ok := v.(*goja.Object)
			if !ok {
				// 非规范结清值：按关闭处理（不阻塞调用方）。
				if !opts.preventClose {
					r.callOrIgnore(w.obj, "close")
				}
				finish(goja.Undefined())
				return
			}
			if d := o.Get("done"); d != nil && d.ToBoolean() {
				if !opts.preventClose {
					r.callOrIgnore(w.obj, "close")
				}
				finish(goja.Undefined())
				return
			}
			// 写一个 chunk：目标 write 可能返回 Promise（等待它），也可能同步返回。
			fn, ok := goja.AssertFunction(w.write)
			if !ok {
				fail(r.vm.NewTypeError("pipeTo writable has no write()"))
				return
			}
			out, err := fn(w.obj, o.Get("value"))
			if err != nil {
				if !opts.preventCancel {
					st.cancelStream(r.vm.ToValue(err.Error()))
				}
				fail(r.vm.ToValue(err.Error()))
				return
			}
			r.thenValue(out, func(goja.Value) { pump() }, func(e goja.Value) {
				if !opts.preventCancel {
					st.cancelStream(e)
				}
				fail(e)
			})
		}, func(e goja.Value) {
			// 源流出错：中止目标（除非 preventAbort），pipeTo 以该错误 reject。
			if !opts.preventAbort {
				r.callOrIgnore(w.obj, "abort", e)
			}
			fail(e)
		})
	}
	pump()
	return p.PromiseObj()
}

// ─── TransformStream ────────────────────────────────────

func (r *Interpreter) registerTransformStream(realm *streamRealm) {
	// WritableStream.prototype（getWriter 形态）
	writableProto := r.vm.NewObject()
	realm.writableProto = writableProto
	writableProto.Set("getWriter", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		ts := transformStateOf(call.This)
		if ts == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if ts.writer != nil {
			panic(r.vm.NewTypeError("WritableStream is locked"))
		}
		obj := r.vm.NewObject()
		obj.Set("__proto__", realm.writerProto)
		obj.Internal = ts
		ts.writer = obj
		return obj
	}))
	writableProto.Set("abort", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.abortStream(call.Argument(0))
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writableProto.Set("close", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.closeStream()
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	_ = writableProto.DefineAccessorProperty("locked",
		r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if ts := transformStateOf(call.This); ts != nil {
				return r.vm.ToValue(ts.writer != nil)
			}
			return r.vm.ToValue(false)
		}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)

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
			ts.abortStream(call.Argument(0))
		}
		return r.resolvedPromise(goja.Undefined())
	}))
	writerProto.Set("releaseLock", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if ts := transformStateOf(call.This); ts != nil {
			ts.writer = nil
		}
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
	ts := &transformState{interp: r, vm: r.vm, readable: readable, readableObj: readable.obj}

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
	if _, err := fn(goja.Undefined(), chunk, t.readable.controller()); err != nil {
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
		if _, err := fn(goja.Undefined(), t.readable.controller()); err != nil {
			t.readable.errorStream(t.vm.ToValue(err.Error()))
			t.notifyClosed()
			return
		}
	}
	t.readable.closeStream()
	t.notifyClosed()
}

// abortStream 中止转换流：可读端以错误结束（规范：abort 让 readable 出错）。
func (t *transformState) abortStream(reason goja.Value) {
	if t.closed {
		return
	}
	t.closed = true
	if reason == nil {
		reason = t.vm.ToValue("stream aborted")
	}
	t.readable.errorStream(reason)
	t.notifyClosed()
}

// errorStream 把上游错误传播到转换流（可读端出错、可写端进入已关闭态）。
func (t *transformState) errorStream(e goja.Value) {
	if t.closed {
		return
	}
	t.closed = true
	t.readable.errorStream(e)
	t.notifyClosed()
}

func (t *transformState) notifyClosed() {
	if t.onClose != nil {
		cb := t.onClose
		t.onClose = nil
		cb()
	}
}

// ─── TextEncoderStream / TextDecoderStream ──────────────

func (r *Interpreter) registerTextEncoderStream(realm *streamRealm) {
	proto := r.vm.NewObject()
	realm.encoderProto = proto
	proto.Set("encoding", "utf-8")
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
	proto.Set("encoding", "utf-8")
	proto.Set("fatal", r.vm.ToValue(false))
	proto.Set("ignoreBOM", r.vm.ToValue(false))
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
