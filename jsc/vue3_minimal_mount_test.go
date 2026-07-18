package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3MinimalMount 极简测试：直接调用 Vue 3 的 createApp + mount
// 使用 bundle 的 hl 函数，但自定义组件
func TestVue3MinimalMount(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 最简 polyfill
	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
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
		// DOM
		`document={}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),childNodes:[],innerHTML:'',textContent:'',setAttribute:function(k,v){console.log('ATTR: '+k+'='+v)},getAttribute:function(k){return this[k]},appendChild:function(c){console.log('APPEND: '+c.tagName);this.childNodes.push(c)},insertBefore:function(c,r){console.log('INSERTBEFORE: '+c.tagName);this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null};return el}`,
		`document.body={appendChild:function(c){console.log('BODY_APPEND: '+c.tagName)},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){console.log('TEXTNODE: '+t);return{nodeType:3,textContent:t,nodeValue:t}}`,
		`document.createComment=function(t){console.log('COMMENT');return{nodeType:8,textContent:t,nodeValue:t}}`,
		`document.getElementById=function(s){if(!window.__appEl){window.__appEl=document.createElement('div');window.__appEl.innerHTML='test'}return window.__appEl}`,
		`document.querySelector=function(s){return document.getElementById(s)}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill err: %v", err)
		}
	}

	// Read and inject into bundle
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
	
	// Replace bundle end to add trace
	src := string(data)
	oldEnd := `console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]"),console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"))})();`
	
	// Replace substring call that might fail with simpler code
	newEnd := `console.log("A");var yl=hl(vl);console.log("B");
var xl=yl.mount("#app");
console.log("D root="+typeof xl);
var Us=document.getElementById("app");
console.log("E children="+Us.childNodes.length);
console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"));
// Post-mortem: check what was actually created
try {
	var appEl = document.getElementById('app');
	console.log('DIAG_APPEND_COUNT: '+(appEl?appEl.childNodes.length:0));
	console.log('DIAG_EL_KEYS: '+(appEl?Object.keys(appEl).join(','):'null'));
	// Check __vue_app__ and _vnode
	console.log('DIAG_VUE_APP: '+(appEl&&appEl.__vue_app__?'yes':'no'));
	console.log('DIAG_VNODE: '+(appEl&&appEl._vnode?'yes':'no'));
	if(appEl && appEl._vnode) {
		console.log('DIAG_VNODE_TYPE: '+appEl._vnode.type);
		console.log('DIAG_VNODE_EL: '+(appEl._vnode.el?'yes':'no'));
		if(appEl._vnode.el) console.log('DIAG_VNODE_EL_TAG: '+appEl._vnode.el.tagName);
	}
			// Check component instance
	if(appEl && appEl.__vue_app__) {
		var inst = appEl.__vue_app__._instance;
		console.log('DIAG_INSTANCE: '+(inst?'yes':'no'));
		if(inst) {
			console.log('DIAG_IS_MOUNTED: '+inst.isMounted);
			console.log('DIAG_SUBTREE: '+(inst.subTree?'yes':'no'));
			if(inst.subTree) {
				console.log('DIAG_SUBTREE_TYPE: '+inst.subTree.type);
				console.log('DIAG_SUBTREE_EL: '+(inst.subTree.el?'yes':'no'));
				if(inst.subTree.el) console.log('DIAG_SUBTREE_EL_TAG: '+inst.subTree.el.tagName);
			}
		}
	}
} catch(e) {
	console.log('DIAG_ERR: '+e);
}
})();`

	modSrc := strings.Replace(src, oldEnd, newEnd, 1)
	if modSrc == src {
		t.Fatalf("FAILED: pattern not found")
	}

	_, err = vm.Run(modSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	}

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
