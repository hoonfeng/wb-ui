// Package webkit — ES Module 运行时（宿主侧）。
//
// 与 engine/page/esm.go 的分工：
//   - page 包负责**语法转译**（import/export → __require/__exports），不碰网络；
//   - 本文件负责**模块图**：取源码（经 wv.loadExternalResource 的统一资源通道，
//     因此 ResourceResolver / http / file / data: 全部可用）、解析相对说明符、
//     处理循环依赖与执行顺序、缓存模块实例、实现动态 import。
//
// 语义约定（与浏览器对齐的部分）：
//   - 同一个模块 URL 只执行一次（模块图按文档生命周期缓存）；
//   - `import * as ns` 拿到的是模块的导出命名空间对象；
//   - `import.meta.url` 是被加载模块自身的绝对 URL（inline 模块是文档 URL）；
//   - `export` 用 getter 绑定，导出值随模块内变量更新（live binding）；
//   - 模块体在严格模式下执行（"use strict"，与真实 ESM 一致）。
//
// 已知简化（构建产物里罕见，见 page/esm.go 的文件头）：
//   - 循环依赖里，先执行的模块拿到的是尚未填充的导出对象（浏览器用 live binding
//     的 TDZ 检查，这里没有实现）；
//   - 顶层 await（TLA）模块按同步执行，会直接语法/运行错误；
//   - 不支持 import maps / bare specifier（node_modules）——构建产物已 bundle，
//     dev server 场景下浏览器原生行为也要 import map 或裸模块解析。
package webkit

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"wb-ui/engine/js/jsc"
	"wb-ui/engine/page"
)

type esmModuleState int

const (
	esmStateLoading esmModuleState = iota
	esmStateLoaded
	esmStateFailed
)

// esmModule 是模块图里的一个节点。
type esmModule struct {
	// url 是模块自身的绝对 URL（import.meta.url / 相对说明符解析的基准）。
	url     string
	exports *jsc.JSObject
	state   esmModuleState
	err     error
}

// esmLoader 是一个文档的模块图。
type esmLoader struct {
	wv     *WebView
	interp *jsc.Interpreter

	mu        sync.Mutex
	modules   map[string]*esmModule
	inlineSeq int
}

func newESMLoader(wv *WebView, interp *jsc.Interpreter) *esmLoader {
	return &esmLoader{
		wv:      wv,
		interp:  interp,
		modules: make(map[string]*esmModule),
	}
}

// resetESM 丢弃当前文档的模块图（文档重新加载时调用）。
func (wv *WebView) resetESM() {
	if wv == nil {
		return
	}
	wv.esm = nil
}

// esmLoaderFor 取当前文档的模块加载器（解释器换代时重建）。
func (wv *WebView) esmLoaderFor(interp *jsc.Interpreter) *esmLoader {
	if wv.esm == nil || wv.esm.interp != interp {
		wv.esm = newESMLoader(wv, interp)
	}
	return wv.esm
}

// RunModuleScript 执行一段 `<script type="module">` 脚本。
//
//	srcURL —— 模块的绝对 URL（外部脚本）或文档 URL（内联脚本）；
//	code   —— 源码；外部脚本传空串表示由加载器自己取；
//	inline —— 内联脚本：注册键加序号（同一文档里可以有多条内联模块），
//	          import.meta.url 仍是文档 URL。
func (wv *WebView) RunModuleScript(srcURL, code string, inline bool) error {
	if wv == nil || wv.destroyed {
		return nil
	}
	interp := wv.JSInterpreter()
	l := wv.esmLoaderFor(interp)
	key := srcURL
	if inline {
		l.mu.Lock()
		l.inlineSeq++
		key = fmt.Sprintf("%s#esm-inline-%d", srcURL, l.inlineSeq)
		l.mu.Unlock()
	}
	if _, err := l.load(key, srcURL, code); err != nil {
		page.Logf("Module", "exec FAIL: %s: %v", key, err)
		wv.reportModuleError(err)
		return err
	}
	interp.RunJobs()
	page.Logf("Module", "exec OK: %s", key)
	return nil
}

// reportModuleError 把模块加载/执行错误送到页面控制台（浏览器里是
// window 的 error 事件 + 控制台），方便宿主与探针定位。
func (wv *WebView) reportModuleError(err error) {
	interp := wv.JSInterpreter()
	if interp == nil {
		return
	}
	msg := jsc.StringValue("Uncaught (in module) " + err.Error())
	if e, eErr := interp.RunJS(`(function (m) { try { window.dispatchEvent(new ErrorEvent('error', { message: m })); } catch (_) {} return m; })`); eErr == nil && e.IsCallable() {
		_, _ = interp.Call(e, jsc.Undefined(), []jsc.JSValue{msg})
	}
}

// load 加载并执行一个模块，返回模块节点。
//
//	key  —— 模块图里的唯一键（内联模块与外部模块共用一套表）；
//	base —— 模块自身 URL（相对说明符的基准）；
//	code —— 已取得的源码；空表示由加载器读取。
func (l *esmLoader) load(key, base, code string) (*esmModule, error) {
	l.mu.Lock()
	if m, ok := l.modules[key]; ok {
		l.mu.Unlock()
		if m.state == esmStateLoading {
			// 循环依赖：返回已经存在的导出对象（浏览器里是未初始化的
			// live binding，这里退化为「稍后可用的空对象」）。
			return m, nil
		}
		if m.state == esmStateFailed {
			return nil, m.err
		}
		return m, nil
	}
	m := &esmModule{url: base, exports: jsc.NewObject(l.interp.ObjectPrototype()), state: esmStateLoading}
	l.modules[key] = m
	l.mu.Unlock()

	if code == "" {
		src, err := l.wv.loadExternalResource(base, PurposeScript)
		if err != nil {
			return l.fail(m, fmt.Errorf("load %s: %w", base, err))
		}
		code = src
	}

	tr, err := page.TranslateModule(code, base, func(spec string) (string, error) {
		return resolveModuleURL(base, spec)
	})
	if err != nil {
		return l.fail(m, err)
	}

	// 先实例化并执行静态依赖（深度优先、后序执行——依赖先于使用者运行）。
	for _, dep := range tr.Deps {
		if _, err := l.load(dep, dep, ""); err != nil {
			return l.fail(m, fmt.Errorf("dependency %s: %w", dep, err))
		}
	}

	if err := l.execute(m, tr.Body); err != nil {
		return l.fail(m, err)
	}
	m.state = esmStateLoaded
	return m, nil
}

func (l *esmLoader) fail(m *esmModule, err error) (*esmModule, error) {
	m.state = esmStateFailed
	m.err = err
	return nil, err
}

// execute 把转译后的模块体包装成函数并调用。
func (l *esmLoader) execute(m *esmModule, body string) error {
	// __export 在模块作用域内定义：把导出名绑定到 getter（live binding）。
	// 参数顺序与 page.TranslateModule 的约定一致。
	wrapper := "(function (__exports, __module, __require, __importMeta, __importDynamic) {\n" +
		`"use strict";` +
		"var __export = function (__n, __get) { Object.defineProperty(__exports, __n, { get: __get, enumerable: true, configurable: true }); };\n" +
		body + "\n})"
	fnVal, err := l.interp.RunJS(wrapper)
	if err != nil {
		return fmt.Errorf("compile %s: %w", m.url, err)
	}
	if !fnVal.IsCallable() {
		return fmt.Errorf("module wrapper for %s is not callable", m.url)
	}

	exportsVal := jsc.ObjectValue(m.exports)
	moduleObj := jsc.NewObject(l.interp.ObjectPrototype())
	moduleObj.Set("exports", exportsVal)

	requireFn := l.interp.NewNativeFunction("require", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return jsc.Undefined()
		}
		depURL := args[0].ToString()
		dep, err := l.load(depURL, depURL, "")
		if err != nil {
			// require 发生在模块体里：抛 JS 异常比静默返回 undefined 更接近
			// 浏览器（模块链接失败会中止整个模块图）。
			panic(in.VM().NewGoError(fmt.Errorf("cannot import %s: %w", depURL, err)))
		}
		return jsc.ObjectValue(dep.exports)
	}, 1)

	importMeta := jsc.NewObject(l.interp.ObjectPrototype())
	importMeta.Set("url", jsc.StringValue(m.url))

	importDynamicFn := l.interp.NewNativeFunction("importDynamic", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) == 0 {
			return in.RejectPromise(jsc.StringValue("import(): missing module specifier"))
		}
		spec := args[0].ToString()
		abs, err := resolveModuleURL(m.url, spec)
		if err != nil {
			return in.RejectPromise(jsc.StringValue(err.Error()))
		}
		dep, err := l.load(abs, abs, "")
		if err != nil {
			return in.RejectPromise(jsc.StringValue(err.Error()))
		}
		return in.ResolvePromise(jsc.ObjectValue(dep.exports))
	}, 1)

	_, err = l.interp.Call(fnVal, jsc.Undefined(), []jsc.JSValue{
		exportsVal,
		jsc.ObjectValue(moduleObj),
		jsc.FunctionValue(requireFn),
		jsc.ObjectValue(importMeta),
		jsc.FunctionValue(importDynamicFn),
	})
	if err != nil {
		return fmt.Errorf("evaluate %s: %w", m.url, err)
	}
	return nil
}

// resolveModuleURL 把模块说明符解析成绝对 URL（ESM 说明符解析的子集）：
//   - 绝对 URL（带 scheme，含 app:/file:/data:）→ 原样；
//   - 以 "./""../""/" 开头 → 基于 base 解析；
//   - 其他（bare specifier）→ 报错（需要 import map / node_modules 解析，
//     构建产物已 bundle，不会出现）。
func resolveModuleURL(base, spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("empty module specifier")
	}
	// 协议相对（//cdn/x.js）与绝对 URL：原样返回。
	if strings.HasPrefix(spec, "//") {
		if b, err := url.Parse(base); err == nil && b.Scheme != "" && b.Scheme != "app" && b.Scheme != "file" {
			return b.Scheme + ":" + spec, nil
		}
		return "https:" + spec, nil
	}
	if u, err := url.Parse(spec); err == nil && u.IsAbs() {
		return spec, nil
	}
	relative := strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") ||
		spec == "." || spec == ".." || strings.HasPrefix(spec, "/")
	if !relative {
		return "", fmt.Errorf("bare module specifier %q (import maps are not supported)", spec)
	}
	b, err := url.Parse(base)
	if err != nil || b.Scheme == "" {
		// 没有绝对 base（LoadHTML 直出、无文档 URL）：只能返回原说明符。
		return spec, nil
	}
	ref, err := url.Parse(spec)
	if err != nil {
		return "", fmt.Errorf("invalid module specifier %q: %w", spec, err)
	}
	return b.ResolveReference(ref).String(), nil
}
