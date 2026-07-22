package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/webkit"
)

func main() {
	distDir := `F:\syproject\gou-ide\cmd\desktop\web-ui\dist`
	absDist, _ := filepath.Abs(distDir)

	wv := webkit.NewWebView()
	mf := wv.MainFrame()
	fr := mf.Frame()

	// ScriptLoader: load all JS files from the dist folder
	fr.ScriptLoader = func(src string) (string, error) {
		clean := strings.TrimPrefix(strings.TrimPrefix(src, "file://"), "./")
		path := filepath.Join(absDist, clean)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("[LOAD] FAILED: %s (%v)\n", src, err)
		} else {
			fmt.Printf("[LOAD] %s → %d bytes\n", src, len(data))
		}
		return string(data), err
	}

	// Polyfills
	for _, p := range []string{
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(typeof TextEncoder=='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`,
		`if(typeof TextDecoder=='undefined')TextDecoder=function(){this.decode=function(a){return String.fromCharCode.apply(null,a)}}`,
		`if(typeof structuredClone=='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`,
		`if(typeof crypto=='undefined')crypto={getRandomValues:function(arr){for(var i=0;i<arr.length;i++)arr[i]=Math.floor(Math.random()*256)}}`,
		`if(typeof fetch=='undefined')fetch=function(){return Promise.resolve({json:function(){return Promise.resolve({})},text:function(){return Promise.resolve('')}})}`,
		`if(typeof WebSocket=='undefined')WebSocket=function(){}`,
		`if(typeof CustomEvent=='undefined')CustomEvent=function(t,e){this.type=t||''}`,
		`if(typeof ResizeObserver=='undefined')ResizeObserver=function(cb){this.observe=function(){};this.disconnect=function(){}}`,
		`if(typeof MutationObserver=='undefined')MutationObserver=function(cb){this.observe=function(){};this.disconnect=function(){};this.takeRecords=function(){return[]}}`,
		`if(typeof requestAnimationFrame=='undefined')requestAnimationFrame=function(fn){return setTimeout(fn,16)}`,
		`if(typeof cancelAnimationFrame=='undefined')cancelAnimationFrame=function(id){clearTimeout(id)}`,
		`if(typeof performance=='undefined')performance={now:function(){return Date.now()}}`,
		`if(typeof navigator=='undefined')navigator={userAgent:'PairCode Desktop'}`,
		`if(typeof queueMicrotask=='undefined')queueMicrotask=function(fn){Promise.resolve().then(fn)}`,
		`if(typeof Event=='undefined'){Event=function(t,e){this.type=t||'';for(var k in(e||{}))this[k]=e[k]};window.Event=Event}`,
		`if(!Array.from)Array.from=function(a,fn,ctx){var r=[];for(var i=0;i<a.length;i++)r.push(fn?fn.call(ctx||null,a[i],i):a[i]);return r}`,
		`if(typeof window.getSelection=='undefined')window.getSelection=function(){return{anchorNode:null,anchorOffset:0,focusNode:null,focusOffset:0}}`,
		// setTimeout / setInterval — goja doesn't have these, Vue needs them for nextTick
		`if(typeof setTimeout=='undefined'){window.setTimeout=function(fn,delay){if(typeof fn=='function')fn();return 1};window.clearTimeout=function(){}}`,
		`if(typeof setInterval=='undefined'){window.setInterval=function(fn,delay){return 1};window.clearInterval=function(){}}`,
	} {
		wv.EvalJS(p)
	}

	// Load HTML
	htmlData, err := os.ReadFile(filepath.Join(distDir, "index.html"))
	if err != nil {
		panic(err)
	}
	s := string(htmlData)
	s = strings.Replace(s, `type="module"`, "", 1)
	s = strings.ReplaceAll(s, `crossorigin`, "")

	fmt.Printf("=== Loading HTML (%d bytes) ===\n", len(s))
	if err := wv.LoadHTML(s); err != nil {
		panic(err)
	}

	// After LoadHTML, bindings are registered and scripts executed.
	// Check results:
	r, _ := wv.EvalJS(`(function(){
		var r=[];
		r.push('Vue='+typeof Vue);
		if(typeof Vue!='undefined')r.push('version='+Vue.version);
		r.push('Pinia='+typeof Pinia);
		r.push('VueRouter='+typeof VueRouter);
		var a=document.getElementById('app');
		if(a)r.push('app.children='+a.childElementCount+' htmlLen='+a.innerHTML.length);
		else r.push('app=null');
		return r.join('\n');
	})()`)
	fmt.Println("=== Post-load ===")
	fmt.Println(r)
	fmt.Println("\n=== Console ===")
	fmt.Println(wv.ConsoleOutput())
}
