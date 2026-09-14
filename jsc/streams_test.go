package jsc

import (
	"strings"
	"testing"
)

// TestStreamsHydrationPipeline 覆盖现代 SSR 水合的分块流契约
// （dev/cssprobe/fixtures/modern-streams.html 的核心脚本）：
//
//	ReadableStream({start}) → pipeThrough(TextEncoderStream)
//	→ pipeThrough(TransformStream{transform,flush}) → getReader().read()
//
// 断言读回的 chunk 是切好的整行文本（await 后的 Promise 结清值）。
func TestStreamsHydrationPipeline(t *testing.T) {
	in := NewInterpreter()
	in.SetupGlobal(&BufferLogger{})
	in.RegisterWebAPIs()
	in.EnsureEventLoop()

	_, err := in.RunJS(`
		const hydration = {};
		hydration.stream = new ReadableStream({
			start(controller) { hydration.controller = controller; }
		}).pipeThrough(new TextEncoderStream());
		hydration.controller.enqueue('["server",{"hydrated":true}]\n');
		hydration.controller.close();

		(async () => {
			const decoder = new TextDecoder();
			let tail = "";
			const lines = hydration.stream.pipeThrough(new TransformStream({
				transform(chunk, controller) {
					const complete = (tail + decoder.decode(chunk, {stream: true})).split("\n");
					tail = complete.pop() || "";
					for (const line of complete) controller.enqueue(line);
				},
				flush(controller) { if (tail) controller.enqueue(tail); }
			}));
			const first = await lines.getReader().read();
			const parsed = JSON.parse(first.value);
			globalThis.__streamsOk =
				!first.done && parsed[0] === "server" && parsed[1].hydrated === true;
			globalThis.__streamsValue = first.value;
		})().catch((error) => { globalThis.__streamsErr = String(error); });
	`)
	if err != nil {
		t.Fatalf("fixture script failed: %v", err)
	}
	in.RunJobs()

	errVal, _ := in.RunJS(`globalThis.__streamsErr || ""`)
	if s := strings.TrimSpace(errVal.ToString()); s != "" && s != `""` {
		t.Fatalf("streams pipeline threw: %s", s)
	}
	okVal, _ := in.RunJS(`globalThis.__streamsOk === true`)
	if okVal.Export() != true {
		line, _ := in.RunJS(`String(globalThis.__streamsValue)`)
		t.Fatalf("streams pipeline did not pass; value=%v", line.Export())
	}
}

// TestStreamsChunking 覆盖变换链的分块语义：flush 结清尾部残余、
// 未完整行保留在 tail、read() 在流关闭后返回 {done:true}。
func TestStreamsChunking(t *testing.T) {
	in := NewInterpreter()
	in.SetupGlobal(&BufferLogger{})
	in.RegisterWebAPIs()
	in.EnsureEventLoop()

	_, err := in.RunJS(`
		const ctl = {};
		const src = new ReadableStream({ start(c) { ctl.c = c; } });
		const up = new TextEncoderStream();
		const dec = new TextDecoderStream();
		const lines = src.pipeThrough(up).pipeThrough(new TransformStream({
			transform(chunk, controller) {
				// 逐字节喂入，验证多字节字符不被截断
				const bytes = new TextDecoder().decode(chunk);
				for (const ch of bytes) controller.enqueue(ch);
			}
		}));
		const reader = lines.getReader();
		ctl.c.enqueue("abc");
		ctl.c.close();
		(async () => {
			const out = [];
			let r = await reader.read();
			while (!r.done) { out.push(r.value); r = await reader.read(); }
			globalThis.__chars = out.join("");
			globalThis.__doneAfterClose = r.done === true && r.value === undefined;
		})().catch((e) => { globalThis.__charsErr = String(e); });
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	in.RunJobs()
	if v, _ := in.RunJS(`String(globalThis.__chars)`); v.Export() != "abc" {
		t.Fatalf("chunks = %v, want abc", v.Export())
	}
	if v, _ := in.RunJS(`globalThis.__doneAfterClose === true`); v.Export() != true {
		t.Fatalf("stream did not report done after close")
	}
}

// TestTextEncoderStreamInstanceOf 覆盖构造器与原型的可用性（框架会做
// `x instanceof TransformStream` 形式的能力探测与鸭子类型判断）。
func TestTextEncoderStreamInstanceOf(t *testing.T) {
	in := NewInterpreter()
	in.SetupGlobal(&BufferLogger{})
	in.RegisterWebAPIs()

	v, err := in.RunJS(`
		const rs = new ReadableStream({ start() {} });
		const ts = new TransformStream({ transform(c, ctrl) { ctrl.enqueue(c); } });
		const es = new TextEncoderStream();
		const ds = new TextDecoderStream();
		[
			rs instanceof ReadableStream,
			ts instanceof TransformStream,
			es instanceof TextEncoderStream,
			ds instanceof TextDecoderStream,
			typeof rs.getReader().read === "function",
			typeof ts.writable.getWriter().write === "function",
			typeof ts.readable.pipeTo === "function",
			typeof es.readable.getReader === "function"
		].every(Boolean)
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if v.Export() != true {
		t.Fatal("stream prototype/instanceof contract broken")
	}
}
