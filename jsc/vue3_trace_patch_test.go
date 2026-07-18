package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3TracePatch 在 bundle 的 patch 函数调用点注入诊断
func TestVue3TracePatch(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Minimal polyfills
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		// Essential polyfills only
		`Object.getOwnPropertyNames=function(o){if(!o)return[];var k=[];for(var n in o)k.push(n);return k}`,
		`Object.hasOwn=function(o,p){return Object.prototype.hasOwnProperty.call(o,p)}`,
		`Object.fromEntries=function(e){var r={};for(var i=0;e&&i<e.length;i++)if(e[i])r[e[i][0]]=e[i][1];return r}`,
		`if(!Array.prototype.flatMap)Array.prototype.flatMap=function(f){var r=[];for(var i=0;i<this.length;i++){var v=f(this[i],i,this);if(v&&v.length)for(var j=0;j<v.length;j++)r.push(v[j]);else r.push(v)}return r}`,
		`if(!Array.prototype.at)Array.prototype.at=function(i){var n=Number(i);if(isNaN(n))n=0;var l=this.length;n=n>=0?n:l+n;if(n<0||n>=l)return undefined;return this[n]}`,
		`if(!Object.isExtensible)Object.isExtensible=function(){return true}`,
		`if(!Object.isSealed)Object.isSealed=function(){return false}`,
		`if(!Object.isFrozen)Object.isFrozen=function(){return false}`,
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,
		`if(!Object.preventExtensions)Object.preventExtensions=function(o){return o}`,
		`if(!Object.seal)Object.seal=function(o){return o}`,
		// DOM with all needed methods
		`document={}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),childNodes:[],innerHTML:'',textContent:'',setAttribute:function(k,v){},getAttribute:function(k){return this[k]},appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t,nodeValue:t}}`,
		`document.createComment=function(t){return{nodeType:8,textContent:t,nodeValue:t}}`,
		`document.getElementById=function(s){if(!window.__el)window.__el={innerHTML:'<p>test</p>',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRACE_IB: c='+(c.tagName||'#text'));this.childNodes.push(c)},setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'};return window.__el}`,
		`document.querySelector=function(s){return document.getElementById(s)}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill: %v", err)
		}
	}

	// Read bundle
	distDir := "F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets"
	entries, err := os.ReadDir(distDir)
	if err != nil {
		t.Skip("dist not found:", err)
	}
	var bp string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			bp = distDir + "/" + e.Name()
			break
		}
	}
	if bp == "" {
		t.Skip("no bundle found")
	}
	data, err := os.ReadFile(bp)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	bundleSrc := string(data)

	// Strategy: modify the bundle to inject tracing at the mount call
	// 
	// The original end is:
	// console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");...
	//
	// We'll replace it to capture the mount return and do diagnostics
	oldEnd := `console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]"),console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"))})();`

	// Instead of running the bundle's mount, we:
	// 1. Keep everything until createApp
	// 2. Manually create the app and trace mount step by step
	newEnd := `
console.log("A");
var yl=hl(vl);
console.log("B");

// === MANUAL TRACE MOUNT ===
try {
	var Mi = document.querySelector('#app');
	console.log('MI_TYPE: '+(typeof Mi));
	console.log('MI_VAL: '+(Mi));
	console.log('WEL_TYPE: '+(typeof window.__el));
	console.log('WEL_VAL: '+(window.__el));
	console.log('QS_SRC: '+(document.querySelector.toString().substring(0,200)));
	
	if(Mi == null || typeof Mi === 'symbol') {
		console.log('RESETTING EL');
		window.__el = {innerHTML:'',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRACE_IB');this.childNodes.push(c)},setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'};
		Mi = window.__el;
	}
	console.log('MI_TAG: '+(Mi?Mi.tagName:'null'));
	console.log('MI_VNODE_BEFORE: '+(Mi?Mi._vnode:'null'));
	
	// Call mount with element directly
	var xl = yl.mount(Mi);
	console.log('XL: '+(typeof xl));
	console.log('MI_VNODE_AFTER: '+(Mi?Mi._vnode:'null'));
	console.log('MI_VUE_APP: '+(Mi&&Mi.__vue_app__?'yes':'no'));
	console.log('MI_CHILDREN: '+(Mi?Mi.childNodes.length:0));
	console.log('MI_HTML: "'+(Mi?Mi.innerHTML:'')+'"');
} catch(e) {
	console.log('MOUNT_ERR: '+(e&&e.message?e.message:''+e));
}
})();`

	modSrc := strings.Replace(bundleSrc, oldEnd, newEnd, 1)
	if modSrc == bundleSrc {
		t.Fatalf("FAILED: pattern not found in bundle")
	}

	_, err = vm.Run(modSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	}

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
