package jsc

import (
	"testing"
)

// newStreamsInterp 建一个装好 Web API 与事件循环的解释器（流测试公用）。
func newStreamsInterp(t *testing.T) *Interpreter {
	t.Helper()
	in := NewInterpreter()
	in.SetupGlobal(&BufferLogger{})
	in.RegisterWebAPIs()
	in.EnsureEventLoop()
	return in
}

// runStreamsScript 执行脚本并驱动微任务，然后返回表达式的值。
func runStreamsScript(t *testing.T, in *Interpreter, script, expr string) interface{} {
	t.Helper()
	if _, err := in.RunJS(script); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	in.RunJobs()
	v, err := in.RunJS(expr)
	if err != nil {
		t.Fatalf("eval %q failed: %v", expr, err)
	}
	return v.Export()
}

// TestStreamsBackpressure 覆盖队列策略与 desiredSize：highWaterMark 生效、
// enqueue/read 影响 desiredSize、流结束后 desiredSize 为 null。
func TestStreamsBackpressure(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		const ctl = {};
		const rs = new ReadableStream({ start(c) { ctl.c = c; } }, { highWaterMark: 2 });
		globalThis.__steps = [];
		globalThis.__steps.push(ctl.c.desiredSize);
		ctl.c.enqueue("a");
		globalThis.__steps.push(ctl.c.desiredSize);
		ctl.c.enqueue("b");
		globalThis.__steps.push(ctl.c.desiredSize);
		globalThis.__controllerSame = true;
		const reader = rs.getReader();
		reader.read().then(() => {
			globalThis.__steps.push(ctl.c.desiredSize);
			ctl.c.close();
			globalThis.__steps.push(ctl.c.desiredSize);
		});
	`
	steps := runStreamsScript(t, in, script, `globalThis.__steps.join(",")`)
	// close 之后 desiredSize 为 null（Array.join 把 null 渲染成空串）。
	if steps != "2,1,0,1," {
		t.Fatalf("desiredSize 序列 = %v，want 2,1,0,1,(null)", steps)
	}
}

// TestStreamsQueuingStrategySize 覆盖 strategy.size：队列大小按 chunk 权重计算。
func TestStreamsQueuingStrategySize(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		const ctl = {};
		const rs = new ReadableStream({ start(c) { ctl.c = c; } }, {
			highWaterMark: 10,
			size(chunk) { return chunk.length; }
		});
		globalThis.__want = [ctl.c.desiredSize];
		ctl.c.enqueue("abc");
		globalThis.__want.push(ctl.c.desiredSize);
		ctl.c.enqueue("de");
		globalThis.__want.push(ctl.c.desiredSize);
	`
	got := runStreamsScript(t, in, script, `globalThis.__want.join(",")`)
	if got != "10,7,5" {
		t.Fatalf("按 size 策略的 desiredSize = %v，want 10,7,5", got)
	}
}

// TestStreamsPullScheduling 覆盖 pull 调度：没人读时不预取（shouldPull），
// 读取后在微任务中按需调用 pull，直到控制器 close。
func TestStreamsPullScheduling(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__pulls = 0;
		globalThis.__out = [];
		const ctl = {};
		const rs = new ReadableStream({
			start(c) { ctl.c = c; },
			pull(controller) {
				globalThis.__pulls++;
				controller.enqueue("chunk" + globalThis.__pulls);
				if (globalThis.__pulls >= 3) controller.close();
			}
		}, { highWaterMark: 1 });
		// 没有 reader：pull 不应被调用（不预取）
		globalThis.__pullsBeforeRead = globalThis.__pulls;
		(async () => {
			const reader = rs.getReader();
			let r = await reader.read();
			while (!r.done) {
				globalThis.__out.push(r.value);
				r = await reader.read();
			}
			globalThis.__readerDone = r.done === true && r.value === undefined;
		})().catch((e) => { globalThis.__pullErr = String(e); });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			pullsBeforeRead: globalThis.__pullsBeforeRead,
			pulls: globalThis.__pulls,
			out: globalThis.__out,
			done: globalThis.__readerDone,
			err: globalThis.__pullErr || null
		})
	`)
	want := `{"pullsBeforeRead":0,"pulls":3,"out":["chunk1","chunk2","chunk3"],"done":true,"err":null}`
	if got != want {
		t.Fatalf("pull 调度 = %v\nwant %v", got, want)
	}
}

// TestStreamsLockSemantics 覆盖锁：getReader 锁流、二次 getReader 抛 TypeError、
// releaseLock 解锁并以 TypeError 拒绝挂起的读请求。
func TestStreamsLockSemantics(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__lock = [];
		const ctl = {};
		const rs = new ReadableStream({ start(c) { ctl.c = c; } });
		globalThis.__lock.push(rs.locked);
		const reader = rs.getReader();
		globalThis.__lock.push(rs.locked);
		try {
			rs.getReader();
			globalThis.__lock.push("no-throw");
		} catch (e) {
			globalThis.__lock.push(e instanceof TypeError ? "TypeError" : "other:" + e);
		}
		// 挂起的读请求在 releaseLock 后必须以 TypeError 拒绝
		const pending = reader.read();
		reader.releaseLock();
		globalThis.__lock.push(rs.locked);
		pending.then(
			() => { globalThis.__lock.push("resolved"); },
			(e) => { globalThis.__lock.push(e instanceof TypeError ? "rejected-TypeError" : "rejected-other"); }
		);
		// 重新获取 reader 应可用
		const again = rs.getReader();
		globalThis.__lock.push(typeof again.read === "function");
	`
	got := runStreamsScript(t, in, script, `globalThis.__lock.join(",")`)
	// 最后一项是异步的（releaseLock 拒绝挂起读请求），排在重新获取 reader 之后。
	want := "false,true,TypeError,false,true,rejected-TypeError"
	if got != want {
		t.Fatalf("锁语义 = %v\nwant %v", got, want)
	}
}

// TestStreamsCancelSemantics 覆盖 cancel(reason)：调用 underlyingSource.cancel
// 并传入理由、挂起的读请求以 {done:true} 结清、reader.cancel 同样生效。
func TestStreamsCancelSemantics(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__cancel = {};
		const ctl = {};
		const rs = new ReadableStream({
			start(c) { ctl.c = c; },
			cancel(reason) { globalThis.__cancel.reason = reason; }
		});
		const reader = rs.getReader();
		const pending = reader.read().then((r) => {
			globalThis.__cancel.pendingValue = String(r.value);
			globalThis.__cancel.pendingDone = r.done;
		});
		reader.cancel("bye").then(() => { globalThis.__cancel.cancelled = true; });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			reason: globalThis.__cancel.reason,
			cancelled: globalThis.__cancel.cancelled === true,
			done: globalThis.__cancel.pendingDone === true,
			value: globalThis.__cancel.pendingValue
		})
	`)
	want := `{"reason":"bye","cancelled":true,"done":true,"value":"undefined"}`
	if got != want {
		t.Fatalf("cancel 语义 = %v\nwant %v", got, want)
	}
}

// TestStreamsTee 覆盖 tee()：原流被锁定、两分支各自收到完整数据、两分支互不
// 影响（各自独立读完后 done）。
func TestStreamsTee(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__tee = { a: [], b: [] };
		const rs = new ReadableStream({
			start(c) { c.enqueue(1); c.enqueue(2); c.enqueue(3); c.close(); }
		});
		const branches = rs.tee();
		globalThis.__tee.locked = rs.locked;
		const ra = branches[0].getReader();
		const rb = branches[1].getReader();
		// ★ tee 的背压：只有两个分支的队列都没满才继续向源流拉取，因此必须
		// 交替消费两个分支（先读完 A 再读 B 会在浏览器里同样挂住）。
		(async () => {
			let doneA = false, doneB = false;
			while (!doneA || !doneB) {
				if (!doneA) {
					const r = await ra.read();
					if (r.done) { doneA = true; } else { globalThis.__tee.a.push(r.value); }
				}
				if (!doneB) {
					const r2 = await rb.read();
					if (r2.done) { doneB = true; } else { globalThis.__tee.b.push(r2.value); }
				}
			}
			globalThis.__tee.done = true;
		})().catch((e) => { globalThis.__tee.err = String(e); });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			locked: globalThis.__tee.locked,
			a: globalThis.__tee.a,
			b: globalThis.__tee.b,
			done: globalThis.__tee.done === true,
			err: globalThis.__tee.err || null
		})
	`)
	want := `{"locked":true,"a":[1,2,3],"b":[1,2,3],"done":true,"err":null}`
	if got != want {
		t.Fatalf("tee = %v\nwant %v", got, want)
	}
}

// TestStreamsPipeToDuckTyped 覆盖 pipeTo 的鸭子类型目标（有 write/close 的对象
// 而非本实现的 WritableStream）：数据按序写入、写完关闭目标、返回的 Promise
// 在完成后 resolve，{preventClose} 时不关闭目标。
func TestStreamsPipeToDuckTyped(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__pipe = {};
		function makeDest() {
			return {
				chunks: [],
				closed: false,
				write(chunk) { this.chunks.push(chunk); return Promise.resolve(); },
				close() { this.closed = true; }
			};
		}
		(async () => {
			const dest1 = makeDest();
			const src1 = new ReadableStream({ start(c) { c.enqueue("x"); c.enqueue("y"); c.close(); } });
			await src1.pipeTo(dest1);
			globalThis.__pipe.first = dest1.chunks.join("") + "|" + dest1.closed;

			const dest2 = makeDest();
			const src2 = new ReadableStream({ start(c) { c.enqueue("z"); c.close(); } });
			await src2.pipeTo(dest2, { preventClose: true });
			globalThis.__pipe.preventClose = dest2.chunks.join("") + "|" + dest2.closed;
		})().catch((e) => { globalThis.__pipe.err = String(e); });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			first: globalThis.__pipe.first,
			preventClose: globalThis.__pipe.preventClose,
			err: globalThis.__pipe.err || null
		})
	`)
	want := `{"first":"xy|true","preventClose":"z|false","err":null}`
	if got != want {
		t.Fatalf("pipeTo（鸭子类型）= %v\nwant %v", got, want)
	}
}

// TestStreamsPipeToErrorPropagation 覆盖错误传播：源流出错时 pipeTo 以该错误
// reject（并中止目标，除非 preventAbort）；目标 write 失败时 pipeTo reject。
func TestStreamsPipeToErrorPropagation(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__err = {};
		(async () => {
			// 源流出错 → 目标被 abort，pipeTo reject
			const dest = {
				aborted: null,
				write() { return Promise.resolve(); },
				close() {},
				abort(reason) { this.aborted = String(reason); }
			};
			const ctl = {};
			const src = new ReadableStream({ start(c) { ctl.c = c; } });
			const p = src.pipeTo(dest);
			ctl.c.enqueue("data");
			ctl.c.error("boom");
			try {
				await p;
				globalThis.__err.sourceResult = "resolved";
			} catch (e) {
				globalThis.__err.sourceResult = "rejected:" + e;
			}
			globalThis.__err.aborted = dest.aborted;

			// 目标 write 失败 → pipeTo reject
			const badDest = {
				write() { return Promise.reject(new Error("write failed")); },
				close() {}
			};
			const okSrc = new ReadableStream({ start(c) { c.enqueue(1); c.close(); } });
			try {
				await okSrc.pipeTo(badDest);
				globalThis.__err.writeResult = "resolved";
			} catch (e) {
				globalThis.__err.writeResult = "rejected:" + e.message;
			}
		})().catch((e) => { globalThis.__err.fatal = String(e); });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			sourceResult: globalThis.__err.sourceResult,
			aborted: globalThis.__err.aborted,
			writeResult: globalThis.__err.writeResult,
			fatal: globalThis.__err.fatal || null
		})
	`)
	want := `{"sourceResult":"rejected:boom","aborted":"boom","writeResult":"rejected:write failed","fatal":null}`
	if got != want {
		t.Fatalf("pipeTo 错误传播 = %v\nwant %v", got, want)
	}
}

// TestStreamsReaderClosed 覆盖 reader.closed：流正常关闭后 resolve，流出错后
// 以该错误 reject。
func TestStreamsReaderClosed(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__closed = {};
		const ctl = {};
		const rs = new ReadableStream({ start(c) { ctl.c = c; } });
		const reader = rs.getReader();
		reader.closed.then(
			() => { globalThis.__closed.ok = "resolved"; },
			(e) => { globalThis.__closed.ok = "rejected:" + e; }
		);
		ctl.c.close();

		const ctl2 = {};
		const rs2 = new ReadableStream({ start(c) { ctl2.c = c; } });
		const reader2 = rs2.getReader();
		reader2.closed.then(
			() => { globalThis.__closed.bad = "resolved"; },
			(e) => { globalThis.__closed.bad = "rejected:" + e; }
		);
		ctl2.c.error("nope");
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({ ok: globalThis.__closed.ok, bad: globalThis.__closed.bad })
	`)
	want := `{"ok":"resolved","bad":"rejected:nope"}`
	if got != want {
		t.Fatalf("reader.closed = %v\nwant %v", got, want)
	}
}

// TestStreamsWritableLock 覆盖 TransformStream 可写端的锁语义与 abort：
// writable.locked、二次 getWriter 抛 TypeError、abort 让可读端出错。
func TestStreamsWritableLock(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__wl = {};
		const ts = new TransformStream({ transform(c, ctrl) { ctrl.enqueue(c); } });
		globalThis.__wl.locked0 = ts.writable.locked;
		const w1 = ts.writable.getWriter();
		globalThis.__wl.locked1 = ts.writable.locked;
		try {
			ts.writable.getWriter();
			globalThis.__wl.second = "no-throw";
		} catch (e) {
			globalThis.__wl.second = e instanceof TypeError ? "TypeError" : "other";
		}
		w1.releaseLock();
		globalThis.__wl.locked2 = ts.writable.locked;

		const ts2 = new TransformStream();
		const reader2 = ts2.readable.getReader();
		ts2.writable.abort("stop");
		reader2.read().then(
			() => { globalThis.__wl.abortRead = "resolved"; },
			(e) => { globalThis.__wl.abortRead = "rejected:" + e; }
		);
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({
			locked0: globalThis.__wl.locked0,
			locked1: globalThis.__wl.locked1,
			second: globalThis.__wl.second,
			locked2: globalThis.__wl.locked2,
			abortRead: globalThis.__wl.abortRead
		})
	`)
	want := `{"locked0":false,"locked1":true,"second":"TypeError","locked2":false,"abortRead":"rejected:stop"}`
	if got != want {
		t.Fatalf("可写端锁/abort = %v\nwant %v", got, want)
	}
}

// TestStreamsPipeThroughCustomPair 覆盖 pipeThrough 接受任意 {readable, writable}
// 对（非本实现的 TransformStream）：返回其 readable、数据经自定义可写端处理。
func TestStreamsPipeThroughCustomPair(t *testing.T) {
	in := newStreamsInterp(t)

	script := `
		globalThis.__cp = {};
		const ctl = {};
		const out = new ReadableStream({ start(c) { globalThis.__cp.outCtl = c; } });
		const writable = {
			write(chunk) { globalThis.__cp.outCtl.enqueue(String(chunk).toUpperCase()); return Promise.resolve(); },
			close() { globalThis.__cp.outCtl.close(); }
		};
		const src = new ReadableStream({ start(c) { ctl.c = c; } });
		const piped = src.pipeThrough({ readable: out, writable: writable });
		ctl.c.enqueue("a");
		ctl.c.enqueue("b");
		ctl.c.close();
		(async () => {
			const reader = piped.getReader();
			const got = [];
			let r = await reader.read();
			while (!r.done) { got.push(r.value); r = await reader.read(); }
			globalThis.__cp.got = got;
		})().catch((e) => { globalThis.__cp.err = String(e); });
	`
	got := runStreamsScript(t, in, script, `
		JSON.stringify({ got: globalThis.__cp.got, err: globalThis.__cp.err || null })
	`)
	want := `{"got":["A","B"],"err":null}`
	if got != want {
		t.Fatalf("pipeThrough（自定义对）= %v\nwant %v", got, want)
	}
}
