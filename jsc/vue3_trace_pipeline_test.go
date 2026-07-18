package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3TraceRenderPipeline(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// 精确的 DOM polyfill - 追踪所有方法调用
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
		// DOM stub - 每个方法都有 console.log 追踪
		`var __elCache={}`,
		`document={}`,
		`document.getElementById=function(s){return __elCache[s]||null}`,
		`document.createElement=function(t){var el={tagName:t.toUpperCase(),childNodes:[],innerHTML:'',textContent:'',setAttribute:function(k,v){this[k]=v},getAttribute:function(k){return this[k]},appendChild:function(c){console.log('TRACE_APPENDCHILD: tag='+c.tagName+' this.tag='+this.tagName);this.childNodes.push(c)},insertBefore:function(c,r){console.log('TRACE_INSERTBEFORE: c='+c.tagName+' this.tag='+this.tagName);this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null};console.log('TRACE_CREATEELEMENT: '+t+' -> '+el.tagName);return el}`,
		`document.body={appendChild:function(c){console.log('TRACE_BODYAPPEND: '+c.tagName)},insertBefore:function(c,r){this.childNodes.push(c)},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t,nodeValue:t}}`,
		`document.createComment=function(t){return{nodeType:8,textContent:t,nodeValue:t}}`,
		// 初始化 #app 元素
		`document.querySelector=function(s){console.log('TRACE_QUERYSELECTOR: '+s);if(!__elCache[s]){var el=document.createElement('DIV');console.log('TRACE_QSNEW: creating new');__elCache[s]=el}return __elCache[s]}`,
	}
	for _, p := range pf {
		if _, err := vm.Run(p); err != nil {
			t.Fatalf("polyfill: %v", err)
		}
	}

	// 先创建 #app
	vm.Run(`var appEl=document.querySelector('#app');appEl.innerHTML='<p>placeholder</p>';console.log('APP_READY: tag='+appEl.tagName+' children='+appEl.childNodes.length)`)

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

	// Run the bundle (the IIFE)
	_, err = vm.Run(bundleSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle OK\n")
	}

	// Post-mortem diagnostics
	diagCode := `
		console.log('=== POSTMORTEM ===');
		var a = document.querySelector('#app');
		console.log('APP_TAG: '+(a?a.tagName:'null'));
		console.log('APP_CHILDREN: '+(a?a.childNodes.length:0));
		console.log('APP_INNERHTML: "'+(a?a.innerHTML:'')+'"');
		console.log('APP_VUE_APP: '+(a&&a.__vue_app__?'yes':'no'));
		console.log('APP_VNODE: '+(a&&a._vnode?'yes':'no'));
	`
	vm.Run(diagCode)

	out := logger.String()
	fmt.Printf("=== OUTPUT ===\n%s\n=== END ===\n", out)
}
