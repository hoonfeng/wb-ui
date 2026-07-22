package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wb-ui/bindings"
	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)
	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		panic(err)
	}

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	fr := mf.Frame()
	fr.ScriptLoader = func(src string) (string, error) {
		clean := strings.TrimPrefix(src, "file://")
		clean = strings.TrimPrefix(clean, "/")
		data, err := os.ReadFile(filepath.Join(absDist, clean))
		return string(data), err
	}
	fr.StyleSheetLoader = func(href string) (string, error) {
		clean := strings.TrimPrefix(href, "file://")
		clean = strings.TrimPrefix(clean, "/")
		data, err := os.ReadFile(filepath.Join(absDist, clean))
		if err != nil {
			data2, _ := os.ReadFile(filepath.Join(absDist, "assets", clean))
			return string(data2), nil
		}
		re := regexp.MustCompile(`\[data-v-[a-f0-9]+\]`)
		return re.ReplaceAllString(string(data), ""), nil
	}

	// ── HEAVY DUTY POLYFILLS ──
	polyfills := []string{
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(typeof TextEncoder==='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`,
		`if(typeof TextDecoder==='undefined')TextDecoder=function(){this.decode=function(a){return String.fromCharCode.apply(null,a)}}`,
		`if(typeof structuredClone==='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`,
		`if(typeof crypto==='undefined')crypto={getRandomValues:function(arr){for(var i=0;i<arr.length;i++)arr[i]=Math.floor(Math.random()*256)}}`,
		`if(typeof fetch==='undefined')fetch=function(){return Promise.reject(new Error('fetch not available'))}`,
		`if(typeof WebSocket==='undefined')WebSocket=function(){}`,
		`if(typeof CustomEvent==='undefined')CustomEvent=function(){return{}}`,
		`if(typeof ResizeObserver==='undefined')ResizeObserver=function(){}`,
		`if(typeof MutationObserver==='undefined')MutationObserver=function(){}`,
		`if(typeof requestAnimationFrame==='undefined')requestAnimationFrame=function(fn){return setTimeout(fn,16)}`,
		`if(typeof cancelAnimationFrame==='undefined')cancelAnimationFrame=function(id){clearTimeout(id)}`,
		`if(typeof performance==='undefined')performance={now:function(){return Date.now()}}`,
		`if(typeof navigator==='undefined')navigator={userAgent:'PairCode Desktop'}`,
		`if(typeof queueMicrotask==='undefined')queueMicrotask=function(fn){Promise.resolve().then(fn)}`,
		// Fix Array.from
		`if(!Array.from)Array.from=function(a,fn,ctx){var r=[];for(var i=0;i<a.length;i++)r.push(fn?fn.call(ctx||null,a[i],i):a[i]);return r}`,
	}
	for _, p := range polyfills {
		wv.EvalJS(p)
	}

	// ── Error trap: override console to catch ALL errors ──
	wv.EvalJS(`(function(){
		var _log = console.log;
		var _err = console.error;
		console.error = function() {
			var parts = [];
			for (var i = 0; i < arguments.length; i++) {
				var a = arguments[i];
				if (a instanceof Error && a.stack) parts.push(a.stack);
				else if (typeof a === 'object') parts.push(JSON.stringify(a));
				else parts.push(String(a));
			}
			_log.call(console, 'ERR:', parts.join(' '));
		};
		// Also trap unhandled errors
		if (typeof window !== 'undefined') {
			var _oe = window.onerror;
			window.onerror = function(msg, src, line, col, err) {
				_log.call(console, 'WINDOW_ERR:', msg, 'line='+line, 'col='+col, 'stack='+(err&&err.stack?err.stack:''));
				return true; // prevent default
			};
		}
	})()`)

	// ── DEBUG: wrap Object methods to find what gets null/undefined ──
	wv.EvalJS(`(function(){
		var _getProto = Object.getPrototypeOf;
		Object.getPrototypeOf = function(o) {
			if (o === null || o === undefined) {
				console.log('Object.getPrototypeOf called with ' + o);
				throw new TypeError('Object.getPrototypeOf called with ' + o);
			}
			return _getProto(o);
		};
		var _keys = Object.keys;
		Object.keys = function(o) {
			if (o === null || o === undefined) {
				console.log('Object.keys called with ' + o);
				throw new TypeError('Object.keys called with ' + o);
			}
			return _keys(o);
		};
		var _assign = Object.assign;
		Object.assign = function(t) {
			if (t === null || t === undefined) {
				console.log('Object.assign called with target=' + t);
				throw new TypeError('Object.assign called with target=' + t);
			}
			var args = [t];
			for (var i = 1; i < arguments.length; i++) args.push(arguments[i]);
			return _assign.apply(null, args);
		};
		var _defineProp = Object.defineProperty;
		Object.defineProperty = function(o, p, d) {
			if (o === null || o === undefined) {
				console.log('Object.defineProperty called with obj=' + o + ' prop=' + p);
				throw new TypeError('Object.defineProperty called with obj=' + o);
			}
			return _defineProp(o, p, d);
		};
	})()`)

	// Load HTML
	s := string(htmlData)
	s = strings.Replace(s, `type="module"`, "", 1)
	s = strings.ReplaceAll(s, `crossorigin`, "")
	if err := wv.LoadHTML(s); err != nil {
		panic(err)
	}

	rt := wv.JSInterpreter()
	doc := mf.Document()
	bindings.RegisterDOMBindings(rt, doc)

	// Execute scripts
	fmt.Println("=== Executing scripts ===")
	fr.ExecuteScripts()

	// Check
	r, _ := wv.EvalJS(`(function(){
		var r=[];
		r.push('Vue='+typeof Vue);
		if(typeof Vue!=='undefined')r.push('version='+Vue.version);
		var a=document.getElementById('app');
		r.push('app.children='+a.childElementCount+' htmlLen='+a.innerHTML.length);
		return r.join('\n');
	})()`)
	fmt.Println(r)
	fmt.Println("\nConsole errors:")
	fmt.Println(wv.ConsoleOutput())
}
