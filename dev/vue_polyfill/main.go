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

	// ── POLYFILLS ──
	for _, p := range []string{
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(typeof TextEncoder=='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`,
		`if(typeof TextDecoder=='undefined')TextDecoder=function(){this.decode=function(a){return String.fromCharCode.apply(null,a)}}`,
		`if(typeof structuredClone=='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`,
		`if(typeof crypto=='undefined')crypto={getRandomValues:function(arr){for(var i=0;i<arr.length;i++)arr[i]=Math.floor(Math.random()*256)}}`,
		`if(typeof fetch=='undefined')fetch=function(){return Promise.reject(new Error('fetch not available'))}`,
		`if(typeof WebSocket=='undefined')WebSocket=function(){}`,
		`if(typeof CustomEvent=='undefined')CustomEvent=function(){return{}}`,
		`if(typeof ResizeObserver=='undefined')ResizeObserver=function(){}`,
		`if(typeof MutationObserver=='undefined')MutationObserver=function(){}`,
		`if(typeof requestAnimationFrame=='undefined')requestAnimationFrame=function(fn){return setTimeout(fn,16)}`,
		`if(typeof cancelAnimationFrame=='undefined')cancelAnimationFrame=function(id){clearTimeout(id)}`,
		`if(typeof performance=='undefined')performance={now:function(){return Date.now()}}`,
		`if(typeof navigator=='undefined')navigator={userAgent:'PairCode Desktop'}`,
		`if(typeof queueMicrotask=='undefined')queueMicrotask=function(fn){Promise.resolve().then(fn)}`,
		`if(typeof Event=='undefined'){(function(){Event=function(t,e){this.type=t};Event.prototype={constructor:Event};window.Event=Event})()}`,
		`if(!Array.from)Array.from=function(a,fn,ctx){var r=[];for(var i=0;i<a.length;i++)r.push(fn?fn.call(ctx||null,a[i],i):a[i]);return r}`,
		`if(typeof window.getSelection=='undefined')window.getSelection=function(){return{anchorNode:null,anchorOffset:0,focusNode:null,focusOffset:0,rangeCount:0,getRangeAt:function(){return null},addRange:function(){},removeAllRanges:function(){}}}`,
		`if(typeof Range=='undefined')Range=function(){this.collapsed=true;this.startOffset=0;this.endOffset=0}`,
		`if(typeof document.createRange=='undefined')document.createRange=function(){return new Range()}`,
		// Error.stack support (goja doesn't set it automatically)
		`if(typeof Error.prototype.stack=='undefined'){Error.prototype.stack='';var _errCtor=Error;var newErr=function(m){var e=_errCtor(m);e.stack='';return e};window.Error=newErr}`,
	} {
		wv.EvalJS(p)
	}

	// DEBUG: catch Object method misuse
	wv.EvalJS(`(function(){
		var _gp=Object.getPrototypeOf;
		Object.getPrototypeOf=function(o){if(o===null||o===undefined){console.log('[DEBUG] getPrototypeOf('+o+')');throw new TypeError('getPrototypeOf('+o+')');}return _gp(o);};
		var _ks=Object.keys;
		Object.keys=function(o){if(o===null||o===undefined){console.log('[DEBUG] keys('+o+')');throw new TypeError('keys('+o+')');}return _ks(o);};
	})()`)

	// Trap console
	wv.EvalJS(`(function(){
		var _log=console.log;
		console.error=function(){var a=[];for(var i=0;i<arguments.length;i++){var v=arguments[i];if(v instanceof Error) a.push(v.message+'\n'+v.stack);else a.push(String(v));} _log.call(console,'ERR:',a.join(' '));};
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

	fmt.Println("=== Executing scripts ===")
	fr.ExecuteScripts()

	r, _ := wv.EvalJS(`(function(){
		var r=[];
		r.push('Vue='+typeof Vue);
		if(typeof Vue!='undefined')r.push('version='+Vue.version);
		var a=document.getElementById('app');
		r.push('app.children='+a.childElementCount+' htmlLen='+a.innerHTML.length);
		return r.join('\n');
	})()`)
	fmt.Println(r)
	fmt.Println("\n=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
