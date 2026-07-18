package jsc

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestVue3MountTrace(t *testing.T) {
	vm := NewInterpreter()
	logger := &BufferLogger{}
	vm.SetupGlobal(logger)

	// Polyfills (same as desktop)
	// IMPORTANT: JSC's OpLoadVar silently returns undefined for undeclared variables
	// (no ReferenceError). So window must be explicitly created first.
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
		// DOM stub: elements MUST have insertBefore (Vue's hostInsert calls it)
		`document={}`,
		`document.querySelector=function(s){var el={innerHTML:'',__vue_app__:null,_vnode:null,childNodes:[],appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){this.childNodes.push(c)},setAttribute:function(){},getAttribute:function(){return''},addEventListener:function(){},style:{},parentNode:null,tagName:'DIV'};return el}`,
		`document.createElement=function(t){return{tagName:t.toUpperCase(),innerHTML:'',textContent:'',childNodes:[],setAttribute:function(){},getAttribute:function(){return''},appendChild:function(c){this.childNodes.push(c)},removeChild:function(c){var i=this.childNodes.indexOf(c);if(i>-1)this.childNodes.splice(i,1)},insertBefore:function(c,r){this.childNodes.push(c)},replaceChild:function(n,o){var i=this.childNodes.indexOf(o);if(i>-1)this.childNodes[i]=n},addEventListener:function(){},style:{},parentNode:null}}`,
		`document.body={appendChild:function(c){},insertBefore:function(c,r){this.childNodes.push(c)},childNodes:[]}`,
		`document.createTextNode=function(t){return{nodeType:3,textContent:t,nodeValue:t}}`,
		`document.createComment=function(t){return{nodeType:8,textContent:t,nodeValue:t}}`,
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

	// Run bundle
	_, err = vm.Run(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bundle error: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "Bundle OK\n")
	}

	// Check interpreter state after bundle
	vm.Run(`console.log('WIN: '+(typeof window))`)
	vm.Run(`console.log('DOC: '+(typeof document))`)
	vm.Run(`console.log('S1: '+(window.__S1__||'undef'))`)
	vm.Run(`console.log('S7: '+(window.__S7__||'undef'))`)
	vm.Run(`console.log('S9: '+(window.__S9__||'undef'))`)
	vm.Run(`console.log('BEFORE: '+(window.__BEFORE_MOUNT__||'undef'))`)
	
	// Check mock DOM
	vm.Run(`var a=document.querySelector('#app');console.log('MOCK_CHILDREN: '+a.childNodes.length);console.log('MOCK_HTML: '+a.innerHTML)`)
	
	out := logger.String()
	if out != "" {
		fmt.Fprintf(os.Stderr, "Console:\n%s\n", out)
	}
}
