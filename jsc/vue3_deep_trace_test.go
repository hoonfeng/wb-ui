package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3DeepTrace(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	pf := []string{
		`window={process:{env:{NODE_ENV:"production"}}}`,
		// 最小 polyfill
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
		`var __appEl={innerHTML:'x',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){console.log('TRC_APPEND');this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRC_IBEFORE');this.childNodes.push(c)},setAttribute:function(k,v){},getAttribute:function(k){return null},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'}`,
		`document={}`,
		`document.getElementById=function(s){return __appEl}`,
		`document.querySelector=function(s){return __appEl}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),childNodes:[],innerHTML:'',setAttribute:function(k,v){console.log('TRC_ATTR:'+k)},getAttribute:function(k){return null},appendChild:function(c){console.log('TRC_APPEND:'+c.tagName);this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRC_IB:'+c.tagName);this.childNodes.push(c)},style:{},parentNode:null};console.log('TRC_CREATE:'+t+'->'+el.tagName);return el}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){console.log('TRC_TEXT');return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){console.log('TRC_CMT');return{nodeType:8}}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("pf err: %v", err)
		}
	}

	data, _ := os.ReadFile("F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets/app-BwgZYNXz.js")
	src := string(data)

	// Replace: `const vl=_l(ml,[["render",bl]]);console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");...`
	// With: trace steps
	oldEnd := `console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]"),console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"))})();`

	newEnd := `
console.log("A");
var yl=hl(vl);
console.log("B");

// Trace mount step by step
console.log('--- MOUNT_BEGIN ---');
try {
	var result = yl.mount("#app");
	console.log('MOUNT_RESULT: '+(typeof result));
} catch(e) {
	console.log('MOUNT_EXCEPTION: '+(e && e.message ? e.message : ''+e));
}

var container = document.getElementById('app');
console.log('POST_CHILDREN: '+container.childNodes.length);
console.log('POST_VUEAPP: '+(container.__vue_app__?'yes':'no'));
console.log('POST_VNODE: '+(container._vnode?'yes':'no'));
if(container._vnode) {
	console.log('VNODE_TYPE: '+container._vnode.type);
	console.log('VNODE_COMP: '+(container._vnode.component?'yes':'no'));
	if(container._vnode.component) {
		var inst = container._vnode.component;
		console.log('INST_IS_MOUNTED: '+inst.isMounted);
		console.log('INST_SUBTREE: '+(inst.subTree?'yes':'no'));
		if(inst.subTree) {
			console.log('SUBTREE_TYPE: '+inst.subTree.type);
			console.log('SUBTREE_EL: '+(inst.subTree.el?'yes':'no'));
			if(inst.subTree.el) console.log('SUBTREE_EL_TAG: '+inst.subTree.el.tagName);
			console.log('SUBTREE_CHILDREN: '+(typeof inst.subTree.children));
		}
	}
}
console.log('--- MOUNT_END ---');
})();`

	modSrc := strings.Replace(src, oldEnd, newEnd, 1)
	if modSrc == src {
		t.Fatalf("pattern not found!")
	}

	_, err := vm.Run(modSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	}

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
