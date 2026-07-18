package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestVue3UnTrace 注入诊断到 Un 函数，看 render 为什么返回 Comment
func TestVue3UnTrace(t *testing.T) {
	vm := NewInterpreter()
	log := &BufferLogger{}
	vm.SetupGlobal(log)

	// 标准 polyfill
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
		`var __appEl={innerHTML:'x',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){console.log('TRC_AP');this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRC_IB');this.childNodes.push(c)},setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return null},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'}`,
		`document={}`,
		`document.getElementById=function(s){return __appEl}`,
		`document.querySelector=function(s){return __appEl}`,
		`document.createElement=function(t){console.log('TRC_CE:'+t);return{tagName:t.toUpperCase(),childNodes:[],innerHTML:'',setAttribute:function(k,v){},getAttribute:function(k){return null},appendChild:function(c){console.log('TRC_APC:'+c.tagName);this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRC_IB:'+c.tagName);this.childNodes.push(c)},style:{},parentNode:null}}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){},childNodes:[]}`,
		`document.createTextNode=function(t){console.log('TRC_TN');return{nodeType:3,textContent:t}}`,
		`document.createComment=function(t){console.log('TRC_CMT:'+t);return{nodeType:8,textContent:t}}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("pf: %v", err)
		}
	}

	data, _ := os.ReadFile("F:/syproject/gou-ide/cmd/desktop/web-ui-minimal/dist/assets/app-BwgZYNXz.js")
	src := string(data)

	// 替换 Un 的 catch block 注入追踪
	old := "catch(E){We.length=0,Nt(E,e,1),W=qe(st)}"
	new := "catch(E){console.log('UN_ERR:'+(E&&E.message?E.message:''+E));We.length=0,Nt(E,e,1),W=qe(st)}"
	
	modSrc := src
	if strings.Contains(modSrc, old) {
		modSrc = strings.Replace(modSrc, old, new, 1)
		fmt.Fprintf(os.Stderr, "INFO: Injected catch trace\n")
	} else {
		fmt.Fprintf(os.Stderr, "WARN: catch pattern not found!\n")
	}
	
	// 也修改 bundle 末尾
	oldEnd := `console.log("A");var yl=hl(vl);console.log("B");var xl=yl.mount("#app");console.log("D root="+typeof xl);var Us=document.getElementById("app");console.log("E children="+Us.childNodes.length+" html=["+Us.innerHTML.substring(0,50)+"]"),console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"))})();`
	newEnd := `console.log("A");var yl=hl(vl);console.log("B");
console.log('TRACE_MOUNT_CALL');
var xl=yl.mount("#app");
console.log("D root="+typeof xl);
var Us=document.getElementById("app");
console.log("E children="+Us.childNodes.length);
console.log("F __vue_app__="+(Us.__vue_app__?"yes":"no"));
// Check subTree
try {
	var compVnode = Us._vnode;
	console.log('VNODE_COMP: '+(compVnode&&compVnode.component?'yes':'no'));
	if(compVnode && compVnode.component) {
		var inst = compVnode.component;
		console.log('INST_MOUNTED: '+inst.isMounted);
		console.log('INST_SUBTREE: '+(inst.subTree?'yes':'no'));
		if(inst.subTree) {
			console.log('ST_TYP: '+inst.subTree.type);
			console.log('ST_EL: '+(inst.subTree.el?'yes':'no'));
		}
	}
} catch(e) {
	console.log('FINAL_ERR: '+e);
}
})();`

	modSrc = strings.Replace(modSrc, oldEnd, newEnd, 1)
	if modSrc == src {
		t.Fatalf("Failed: end pattern not found!")
	}

	_, err := vm.Run(modSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	}

	out := log.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
